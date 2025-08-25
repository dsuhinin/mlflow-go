package service

import (
	"context"
	"fmt"

	"github.com/mlflow/mlflow-go-backend/pkg/contract"
	"github.com/mlflow/mlflow-go-backend/pkg/entities"
	"github.com/mlflow/mlflow-go-backend/pkg/protos"
	"github.com/mlflow/mlflow-go-backend/pkg/utils"
)

func (ts TrackingService) SetTraceTag(
	ctx context.Context, input *protos.SetTraceTag,
) (*protos.SetTraceTag_Response, *contract.Error) {
	if err := ts.Store.SetTraceTag(
		ctx, input.GetRequestId(), input.GetKey(), input.GetValue(),
	); err != nil {
		return nil, contract.NewErrorWith(protos.ErrorCode_INTERNAL_ERROR, "failed to create trace_tag", err)
	}

	return &protos.SetTraceTag_Response{}, nil
}

func (ts TrackingService) DeleteTraceTag(
	ctx context.Context, input *protos.DeleteTraceTag,
) (*protos.DeleteTraceTag_Response, *contract.Error) {
	tag, err := ts.Store.GetTraceTag(ctx, input.GetTraceId(), input.GetKey())
	if err != nil {
		return nil, err
	}

	if tag == nil {
		return nil, contract.NewError(
			protos.ErrorCode_RESOURCE_DOES_NOT_EXIST,
			fmt.Sprintf(
				"No trace tag with key '%s' for trace with trace_id '%s'",
				input.GetKey(),
				input.GetTraceId(),
			),
		)
	}

	if err := ts.Store.DeleteTraceTag(ctx, tag); err != nil {
		return nil, err
	}

	return &protos.DeleteTraceTag_Response{}, nil
}

func (ts TrackingService) StartTrace(
	ctx context.Context, input *protos.StartTrace,
) (*protos.StartTrace_Response, *contract.Error) {
	traceInfo, err := ts.Store.SetTrace(
		ctx,
		input.GetExperimentId(),
		input.GetTimestampMs(),
		entities.TraceRequestMetadataFromStartTraceProtoInput(input.GetRequestMetadata()),
		entities.TagsFromStartTraceProtoInput(input.GetTags()),
	)
	if err != nil {
		return nil, contract.NewError(protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error starting trace: %v", err))
	}

	return &protos.StartTrace_Response{
		TraceInfo: traceInfo.ToProto(),
	}, nil
}

func (ts TrackingService) StartTraceV3(
	ctx context.Context,
	input *protos.StartTraceV3,
) (*protos.StartTraceV3_Response, *contract.Error) {
	inputTraceInfo := input.GetTrace().GetTraceInfo()

	traceInfo, err := ts.Store.SetTraceV3(
		ctx, &entities.TraceInfoV3{
			Status:          inputTraceInfo.GetState().String(),
			RequestID:       inputTraceInfo.GetTraceId(),
			TimestampMS:     inputTraceInfo.GetRequestTime().AsTime().UnixMilli(),
			ExperimentID:    inputTraceInfo.GetTraceLocation().GetMlflowExperiment().GetExperimentId(),
			RequestPreview:  utils.PtrTo(inputTraceInfo.GetRequestPreview()),
			ResponsePreview: utils.PtrTo(inputTraceInfo.GetResponsePreview()),
			ClientRequestID: utils.PtrTo(inputTraceInfo.GetClientRequestId()),
			ExecutionTimeMS: utils.PtrTo(inputTraceInfo.GetExecutionDuration().AsDuration().Milliseconds()),
		},
		entities.TraceRequestMetadataFromStartTraceV3ProtoInput(inputTraceInfo.GetTraceMetadata()),
		entities.TagsFromStartTraceV3ProtoInput(inputTraceInfo.GetTags()),
	)
	if err != nil {
		return nil, err
	}

	return &protos.StartTraceV3_Response{
		Trace: &protos.Trace{
			TraceInfo: traceInfo.ToProto(),
		},
	}, nil
}

func (ts TrackingService) EndTrace(
	ctx context.Context, input *protos.EndTrace,
) (*protos.EndTrace_Response, *contract.Error) {
	traceInfo, err := ts.Store.EndTrace(
		ctx,
		input.GetRequestId(),
		input.GetTimestampMs(),
		input.GetStatus().String(),
		entities.TraceRequestMetadataFromStartTraceProtoInput(input.GetRequestMetadata()),
		entities.TagsFromStartTraceProtoInput(input.GetTags()),
	)
	if err != nil {
		return nil, contract.NewError(protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error ending trace: %v", err))
	}

	return &protos.EndTrace_Response{
		TraceInfo: traceInfo.ToProto(),
	}, nil
}

func (ts TrackingService) GetTraceInfo(
	ctx context.Context, input *protos.GetTraceInfo,
) (*protos.GetTraceInfo_Response, *contract.Error) {
	traceInfo, err := ts.Store.GetTraceInfo(ctx, input.GetRequestId())
	if err != nil {
		return nil, err
	}

	return &protos.GetTraceInfo_Response{
		TraceInfo: traceInfo.ToProto(),
	}, nil
}

func (ts TrackingService) GetTraceInfoV3(
	ctx context.Context, input *protos.GetTraceInfoV3,
) (*protos.GetTraceInfoV3_Response, *contract.Error) {
	traceInfo, err := ts.Store.GetTraceV3Info(ctx, input.GetTraceId())
	if err != nil {
		return nil, err
	}

	return &protos.GetTraceInfoV3_Response{
		Trace: &protos.Trace{
			TraceInfo: traceInfo.ToProto(),
		},
	}, nil
}

func (ts TrackingService) DeleteTraces(
	ctx context.Context, input *protos.DeleteTraces,
) (*protos.DeleteTraces_Response, *contract.Error) {
	if input.MaxTimestampMillis == nil && len(input.RequestIds) == 0 {
		return nil, contract.NewError(
			protos.ErrorCode_INVALID_PARAMETER_VALUE,
			"Either `max_timestamp_millis` or `trace_ids` must be specified.",
		)
	}

	if input.MaxTimestampMillis != nil && input.RequestIds != nil {
		return nil, contract.NewError(
			protos.ErrorCode_INVALID_PARAMETER_VALUE,
			"Only one of `max_timestamp_millis` and `trace_ids` can be specified.",
		)
	}

	if input.RequestIds != nil && input.MaxTraces != nil {
		return nil, contract.NewError(
			protos.ErrorCode_INVALID_PARAMETER_VALUE,
			"`max_traces` can't be specified if `trace_ids` is specified.",
		)
	}

	if input.MaxTraces != nil && *input.MaxTraces <= 0 {
		return nil, contract.NewError(
			protos.ErrorCode_INVALID_PARAMETER_VALUE,
			fmt.Sprintf("`max_traces` must be a positive integer, received %d.", *input.MaxTraces),
		)
	}

	result, err := ts.Store.DeleteTraces(
		ctx,
		input.GetExperimentId(),
		input.GetMaxTimestampMillis(),
		input.GetMaxTraces(),
		input.GetRequestIds(),
	)
	if err != nil {
		return nil, err
	}

	return &protos.DeleteTraces_Response{
		TracesDeleted: utils.PtrTo(result),
	}, nil
}
