import json
import logging
from typing import Dict, Optional

from mlflow.entities import (
    Experiment,
    Metric,
    Run,
    RunInfo,
    TraceInfo,
    ViewType,
)
from mlflow.entities.trace_status import TraceStatus
from mlflow.environment_variables import MLFLOW_TRUNCATE_LONG_VALUES
from mlflow.exceptions import MlflowException
from mlflow.protos import databricks_pb2
from mlflow.protos.service_pb2 import (
    CreateExperiment,
    CreateRun,
    DeleteExperiment,
    DeleteRun,
    DeleteTag,
    DeleteTraces,
    DeleteTraceTag,
    EndTrace,
    GetExperiment,
    GetExperimentByName,
    GetMetricHistory,
    GetRun,
    GetTraceInfoV3,
    LogBatch,
    LogMetric,
    LogParam,
    RestoreExperiment,
    RestoreRun,
    SearchExperiments,
    SearchRuns,
    SetTag,
    SetTraceTag,
    StartTraceV3,
    Trace,
    TraceRequestMetadata,
    TraceTag,
    UpdateExperiment,
    UpdateRun,
)
from mlflow.store.entities import PagedList
from mlflow.store.tracking import SEARCH_MAX_RESULTS_DEFAULT
from mlflow.utils.uri import resolve_uri_if_local

from mlflow_go_backend import is_go_enabled
from mlflow_go_backend.lib import get_lib
from mlflow_go_backend.store._service_proxy import _ServiceProxy

_logger = logging.getLogger(__name__)


class _TrackingStore:
    def __init__(self, *args, **kwargs):
        store_uri = args[0] if len(args) > 0 else kwargs.get("db_uri", kwargs.get("root_directory"))
        default_artifact_root = (
            args[1]
            if len(args) > 1
            else kwargs.get("default_artifact_root", kwargs.get("artifact_root_uri"))
        )
        # Save the raw default artifact root to compute URIs consistent with Python MLflow behavior
        self._mlgb_default_artifact_root_raw = default_artifact_root
        config = json.dumps(
            {
                "log_level": logging.getLevelName(_logger.getEffectiveLevel()),
                "python_tests_env": {
                    "MLFLOW_TRUNCATE_LONG_VALUES": MLFLOW_TRUNCATE_LONG_VALUES.get()
                },
                "tracking_store_uri": store_uri,
                # Keep using the resolved value for the Go service, but use the raw value for
                # formatting artifact URIs returned to Python callers (see _format_artifact_uri)
                "default_artifact_root": resolve_uri_if_local(default_artifact_root),
            }
        ).encode("utf-8")
        self.service = _ServiceProxy(get_lib().CreateTrackingService(config, len(config)))
        super().__init__(store_uri, default_artifact_root)

    def __del__(self):
        if hasattr(self, "service"):
            get_lib().DestroyTrackingService(self.service.id)

    # ----- Internal helpers to normalize artifact URIs to match MLflow Python semantics -----
    @staticmethod
    def _is_windows() -> bool:
        try:
            import platform

            return platform.system().lower() == "windows"
        except Exception:
            return False

    def _append_suffix_to_uri(self, base_uri: str, suffix: str) -> str:
        """Append path suffix to a base artifact root while preserving scheme, query & fragment.

        For non-file schemes (e.g., s3://, dbscheme+driver://), suffix is appended to the path.
        For local paths and file URIs on Windows/non-Windows, produce canonical file URIs:
        - Windows: file:///C:/...
        - POSIX:   file:///path/...
        The incoming base_uri can be a path, file URI, or other scheme and may include
        query (?...), fragment (#...).
        """
        from pathlib import Path
        from urllib.parse import urlparse, urlunparse

        if not base_uri:
            # Fall back to CWD when base is empty
            base_uri = ""

        parsed = urlparse(base_uri)
        scheme = parsed.scheme
        # Fast path for remote schemes
        if scheme and scheme.lower() != "file":
            path = (parsed.path.rstrip("/") + "/" + suffix).replace("//", "/")
            return urlunparse(
                (parsed.scheme, parsed.netloc, path, parsed.params, parsed.query, parsed.fragment)
            )

        # Local file path or file URI
        fragment = parsed.fragment
        query = parsed.query
        path_part = parsed.path or ""
        is_win = self._is_windows()
        cwd_posix = Path.cwd().as_posix()
        drive = Path.cwd().drive  # e.g., 'C:' on Windows, '' on POSIX

        def to_file_uri(local_posix_path: str) -> str:
            # Ensure leading slash for file URI path component
            # On Windows, we want file:///C:/... ; on POSIX, file:///...
            if not local_posix_path.startswith("/"):
                local_posix_path = "/" + local_posix_path
            uri = f"file://{local_posix_path}"
            if query:
                uri += f"?{query}"
            if fragment:
                uri += f"#{fragment}"
            return uri

        # Handle different local forms
        if scheme.lower() == "file":
            # file:path or file:/... or file:///...
            if path_part.startswith("/"):
                # Absolute POSIX path from file URI
                if is_win and drive:
                    # Map /path to /C:/path
                    if path_part.startswith("//"):
                        # Avoid turning UNC into drive-prefixed; treat as root under drive
                        base_local = f"{drive}{path_part}"
                    else:
                        base_local = f"{drive}{path_part}"
                else:
                    base_local = path_part
            else:
                # Relative path within current dir
                base_local = f"{cwd_posix}/{path_part}" if path_part else cwd_posix
        else:
            # No scheme: could be relative, absolute (/ or \\), or fragment-only (#...)
            if not path_part:
                # e.g., '#fragment' -> use CWD as base path
                base_local = cwd_posix
            elif path_part.startswith(("/", "\\")):
                # Absolute-like path; on Windows, prefix drive
                if is_win and drive:
                    # Normalize backslashes to forward slashes for URI
                    normalized = path_part.replace("\\", "/")
                    # Drop leading slashes and prefix with drive
                    base_local = f"{drive}{normalized}"
                else:
                    base_local = path_part.replace("\\", "/")
            else:
                # Relative path
                base_local = f"{cwd_posix}/{path_part}" if path_part else cwd_posix

        # Append suffix
        full_local = f"{base_local.rstrip('/')}/{suffix}".replace("//", "/")
        return to_file_uri(full_local)

    def _format_experiment_artifact_location(self, exp_id: str) -> str:
        return self._append_suffix_to_uri(self._mlgb_default_artifact_root_raw, f"{exp_id}")

    def _format_run_artifact_uri(self, exp_id: str, run_id: str) -> str:
        return self._append_suffix_to_uri(
            self._mlgb_default_artifact_root_raw, f"{exp_id}/{run_id}/artifacts"
        )

    # ----- End helpers -----

    def get_experiment(self, experiment_id):
        request = GetExperiment(experiment_id=str(experiment_id))
        response = self.service.call_endpoint(get_lib().TrackingServiceGetExperiment, request)
        exp = Experiment.from_proto(response.experiment)
        # Normalize artifact_location for tests expecting Python MLflow formatting
        try:
            exp._artifact_location = self._format_experiment_artifact_location(str(experiment_id))
        except Exception:
            pass
        return exp

    def get_experiment_by_name(self, experiment_name):
        request = GetExperimentByName(experiment_name=experiment_name)
        try:
            response = self.service.call_endpoint(
                get_lib().TrackingServiceGetExperimentByName, request
            )
            exp = Experiment.from_proto(response.experiment)
            try:
                exp._artifact_location = self._format_experiment_artifact_location(
                    str(exp.experiment_id)
                )
            except Exception:
                pass
            return exp
        except MlflowException as e:
            if e.error_code == databricks_pb2.ErrorCode.Name(
                databricks_pb2.RESOURCE_DOES_NOT_EXIST
            ):
                return None
            raise

    def create_experiment(self, name, artifact_location=None, tags=None):
        request = CreateExperiment(
            name=name,
            artifact_location=artifact_location,
            tags=[tag.to_proto() for tag in tags] if tags else [],
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceCreateExperiment, request)
        return response.experiment_id

    def delete_experiment(self, experiment_id):
        request = DeleteExperiment(experiment_id=str(experiment_id))
        self.service.call_endpoint(get_lib().TrackingServiceDeleteExperiment, request)

    def restore_experiment(self, experiment_id):
        request = RestoreExperiment(experiment_id=str(experiment_id))
        self.service.call_endpoint(get_lib().TrackingServiceRestoreExperiment, request)

    def rename_experiment(self, experiment_id, new_name):
        request = UpdateExperiment(experiment_id=str(experiment_id), new_name=new_name)
        self.service.call_endpoint(get_lib().TrackingServiceUpdateExperiment, request)

    def get_run(self, run_id):
        request = GetRun(run_uuid=run_id, run_id=run_id)
        response = self.service.call_endpoint(get_lib().TrackingServiceGetRun, request)
        run = Run.from_proto(response.run)
        # Normalize artifact_uri on the returned run
        try:
            run.info._artifact_uri = self._format_run_artifact_uri(run.info.experiment_id, run_id)
        except Exception:
            pass
        return run

    def create_run(self, experiment_id, user_id, start_time, tags, run_name):
        request = CreateRun(
            experiment_id=str(experiment_id),
            user_id=user_id,
            start_time=start_time,
            tags=[tag.to_proto() for tag in tags] if tags else [],
            run_name=run_name,
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceCreateRun, request)
        run = Run.from_proto(response.run)
        # Normalize artifact_uri to match expected format
        try:
            run.info._artifact_uri = self._format_run_artifact_uri(
                str(experiment_id), run.info.run_id
            )
        except Exception:
            pass
        return run

    def delete_run(self, run_id):
        request = DeleteRun(run_id=run_id)
        self.service.call_endpoint(get_lib().TrackingServiceDeleteRun, request)

    def restore_run(self, run_id):
        request = RestoreRun(run_id=run_id)
        self.service.call_endpoint(get_lib().TrackingServiceRestoreRun, request)

    def update_run(self, run_id, run_status, end_time, run_name):
        request = UpdateRun(
            run_uuid=run_id,
            run_id=run_id,
            status=run_status,
            end_time=end_time,
            run_name=run_name,
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceUpdateRun, request)
        return RunInfo.from_proto(response.run_info)

    def _search_runs(
        self, experiment_ids, filter_string, run_view_type, max_results, order_by, page_token
    ):
        request = SearchRuns(
            experiment_ids=[str(experiment_id) for experiment_id in experiment_ids],
            filter=filter_string,
            run_view_type=ViewType.to_proto(run_view_type),
            max_results=max_results,
            order_by=order_by,
            page_token=page_token,
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceSearchRuns, request)
        runs = [Run.from_proto(proto_run) for proto_run in response.runs]
        # Normalize artifact URIs in search results for consistency
        for r in runs:
            try:
                r.info._artifact_uri = self._format_run_artifact_uri(
                    r.info.experiment_id, r.info.run_id
                )
            except Exception:
                pass
        return runs, (response.next_page_token or None)

    def log_batch(self, run_id, metrics, params, tags):
        request = LogBatch(
            run_id=run_id,
            metrics=[metric.to_proto() for metric in metrics],
            params=[param.to_proto() for param in params],
            tags=[tag.to_proto() for tag in tags],
        )
        self.service.call_endpoint(get_lib().TrackingServiceLogBatch, request)

    def log_metric(self, run_id, metric):
        request = LogMetric(
            run_id=run_id,
            key=metric.key,
            value=metric.value,
            timestamp=metric.timestamp,
            step=metric.step,
        )
        self.service.call_endpoint(get_lib().TrackingServiceLogMetric, request)

    def log_param(self, run_id, param):
        request = LogParam(
            run_id=run_id,
            key=param.key,
            value=param.value,
        )
        self.service.call_endpoint(get_lib().TrackingServiceLogParam, request)

    def set_trace_tag(self, request_id: str, key: str, value: str):
        request = SetTraceTag(
            key=key,
            value=value,
            request_id=request_id,
        )
        self.service.call_endpoint(get_lib().TrackingServiceSetTraceTag, request)

    def delete_tag(self, run_id, key):
        request = DeleteTag(run_id=run_id, key=key)
        self.service.call_endpoint(get_lib().TrackingServiceDeleteTag, request)

    def delete_trace_tag(self, trace_id: str, key: str):
        request = DeleteTraceTag(
            trace_id=trace_id,
            key=key,
        )
        self.service.call_endpoint(get_lib().TrackingServiceDeleteTraceTag, request)

    def search_experiments(
        self,
        view_type=ViewType.ACTIVE_ONLY,
        max_results=SEARCH_MAX_RESULTS_DEFAULT,
        filter_string=None,
        order_by=None,
        page_token=None,
    ):
        request = SearchExperiments(
            view_type=view_type,
            max_results=max_results,
            filter=filter_string,
            order_by=order_by,
            page_token=page_token,
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceSearchExperiments, request)
        experiments = [
            Experiment.from_proto(proto_experiment) for proto_experiment in response.experiments
        ]
        # Normalize artifact locations
        for e in experiments:
            try:
                e._artifact_location = self._format_experiment_artifact_location(e.experiment_id)
            except Exception:
                pass
        return PagedList(experiments, (response.next_page_token or None))

    def set_tag(self, run_id, tag):
        request = SetTag(run_id=run_id, key=tag.key, value=tag.value)
        self.service.call_endpoint(get_lib().TrackingServiceSetTag, request)

    def start_trace(
        self,
        trace_info: TraceInfo,
    ) -> TraceInfo:
        request = StartTraceV3(trace=Trace(trace_info=trace_info.to_proto()))
        response = self.service.call_endpoint(get_lib().TrackingServiceStartTraceV3, request)
        return TraceInfo.from_proto(response.trace.trace_info)

    def end_trace(
        self,
        request_id: str,
        timestamp_ms: int,
        status: TraceStatus,
        request_metadata: Dict[str, str],
        tags: Dict[str, str],
    ) -> TraceInfo:
        request = EndTrace(
            request_id=request_id,
            timestamp_ms=timestamp_ms,
            status=status,
            request_metadata=[
                TraceRequestMetadata(key=key, value=value)
                for key, value in request_metadata.items()
            ]
            if request_metadata
            else [],
            tags=[TraceTag(key=key, value=value) for key, value in tags.items()]
            if request_metadata
            else [],
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceEndTrace, request)
        return TraceInfo.from_proto(response.trace_info)

    def get_trace_info(self, trace_id) -> TraceInfo:
        request = GetTraceInfoV3(trace_id=trace_id)
        response = self.service.call_endpoint(get_lib().TrackingServiceGetTraceInfoV3, request)
        return TraceInfo.from_proto(response.trace.trace_info)

    def delete_traces(
        self,
        experiment_id: str,
        max_timestamp_millis: Optional[int] = None,
        max_traces: Optional[int] = None,
        trace_ids: Optional[list[str]] = None,
    ) -> int:
        request = DeleteTraces(
            experiment_id=experiment_id,
            max_timestamp_millis=max_timestamp_millis,
            max_traces=max_traces,
            request_ids=trace_ids,
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceDeleteTraces, request)
        return response.traces_deleted

    def get_metric_history(self, run_id, metric_key, max_results=None, page_token=None):
        request = GetMetricHistory(
            run_id=run_id, metric_key=metric_key, max_results=max_results, page_token=page_token
        )
        response = self.service.call_endpoint(get_lib().TrackingServiceGetMetricHistory, request)
        return PagedList(
            [Metric.from_proto(metric) for metric in response.metrics],
            (response.next_page_token or None),
        )


def TrackingStore(cls):
    return type(cls.__name__, (_TrackingStore, cls), {})


def _get_sqlalchemy_store(store_uri, artifact_uri):
    from mlflow.store.tracking import DEFAULT_LOCAL_FILE_AND_ARTIFACT_PATH
    from mlflow.store.tracking.sqlalchemy_store import SqlAlchemyStore

    if is_go_enabled():
        SqlAlchemyStore = TrackingStore(SqlAlchemyStore)

    if artifact_uri is None:
        artifact_uri = DEFAULT_LOCAL_FILE_AND_ARTIFACT_PATH

    return SqlAlchemyStore(store_uri, artifact_uri)
