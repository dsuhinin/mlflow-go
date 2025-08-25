package service

import (
	"context"

	"github.com/mlflow/mlflow-go-backend/pkg/contract"
	"github.com/mlflow/mlflow-go-backend/pkg/entities"
	"github.com/mlflow/mlflow-go-backend/pkg/protos"
)

func (ts TrackingService) LogMetric(
	ctx context.Context,
	input *protos.LogMetric,
) (*protos.LogMetric_Response, *contract.Error) {
	if err := ts.Store.LogMetric(ctx, input.GetRunId(), entities.MetricFromLogMetricProtoInput(input)); err != nil {
		return nil, err
	}

	return &protos.LogMetric_Response{}, nil
}

func (ts TrackingService) LogParam(
	ctx context.Context, input *protos.LogParam,
) (*protos.LogParam_Response, *contract.Error) {
	if err := ts.Store.LogParam(ctx, input.GetRunId(), entities.ParamFromLogMetricProtoInput(input)); err != nil {
		return nil, err
	}

	return &protos.LogParam_Response{}, nil
}

func (ts TrackingService) GetMetricHistory(
	ctx context.Context, input *protos.GetMetricHistory,
) (*protos.GetMetricHistory_Response, *contract.Error) {
	runID := input.GetRunId()
	if input.RunUuid != nil {
		runID = input.GetRunUuid()
	}

	metrics, nextPageToken, err := ts.Store.GetMetricHistory(
		ctx,
		runID,
		input.GetMetricKey(),
		input.GetPageToken(),
		input.MaxResults,
	)
	if err != nil {
		return nil, err
	}

	response := protos.GetMetricHistory_Response{
		Metrics: make([]*protos.Metric, len(metrics)),
	}

	if nextPageToken != "" {
		response.NextPageToken = &nextPageToken
	}

	for i, metric := range metrics {
		response.Metrics[i] = metric.ToProto()
	}

	return &response, nil
}
