package entities

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mlflow/mlflow-go-backend/pkg/protos"
	"github.com/mlflow/mlflow-go-backend/pkg/utils"
)

type TraceInfo struct {
	RequestID            string
	Status               string
	ExperimentID         string
	TimestampMS          int64
	ExecutionTimeMS      *int64
	Tags                 []*TraceTag
	TraceRequestMetadata []*TraceRequestMetadata
}

func (ti TraceInfo) ToProto() *protos.TraceInfo {
	traceInfo := protos.TraceInfo{
		RequestId:       &ti.RequestID,
		ExperimentId:    &ti.ExperimentID,
		TimestampMs:     &ti.TimestampMS,
		ExecutionTimeMs: ti.ExecutionTimeMS,
		Status:          utils.PtrTo(protos.TraceStatus(protos.TraceStatus_value[ti.Status])),
		RequestMetadata: make([]*protos.TraceRequestMetadata, 0, len(ti.Tags)),
		Tags:            make([]*protos.TraceTag, 0, len(ti.Tags)),
	}

	for _, tag := range ti.Tags {
		traceInfo.Tags = append(traceInfo.Tags, tag.ToProto())
	}

	for _, metadata := range ti.TraceRequestMetadata {
		traceInfo.RequestMetadata = append(traceInfo.RequestMetadata, metadata.ToProto())
	}

	return &traceInfo
}

type TraceInfoV3 struct {
	Status               string
	RequestID            string
	ClientRequestID      *string
	ExperimentID         string
	RequestPreview       *string
	ResponsePreview      *string
	TimestampMS          int64
	ExecutionTimeMS      *int64
	Tags                 []*TraceTag
	TraceRequestMetadata []*TraceRequestMetadata
}

//nolint:cyclop
func (ti TraceInfoV3) ToProto() *protos.TraceInfoV3 {
	traceInfo := protos.TraceInfoV3{
		Tags:    make(map[string]string, len(ti.Tags)),
		TraceId: utils.PtrTo(ti.RequestID),
		TraceLocation: &protos.TraceLocation{
			Type: utils.PtrTo(protos.TraceLocation_MLFLOW_EXPERIMENT),
			Identifier: &protos.TraceLocation_MlflowExperiment{
				MlflowExperiment: &protos.TraceLocation_MlflowExperimentLocation{ExperimentId: &ti.ExperimentID},
			},
		},
		RequestTime:   timestamppb.New(time.UnixMilli(ti.TimestampMS)),
		TraceMetadata: make(map[string]string, len(ti.TraceRequestMetadata)),
	}

	if ti.ExecutionTimeMS != nil {
		traceInfo.ExecutionDuration = durationpb.New(time.Duration(*ti.ExecutionTimeMS) * time.Millisecond)
	}

	if ti.ClientRequestID != nil {
		traceInfo.ClientRequestId = utils.PtrTo(*ti.ClientRequestID)
	}

	if ti.RequestPreview != nil {
		traceInfo.RequestPreview = utils.PtrTo(*ti.RequestPreview)
	}

	if ti.ResponsePreview != nil {
		traceInfo.ResponsePreview = utils.PtrTo(*ti.ResponsePreview)
	}

	for _, tag := range ti.Tags {
		traceInfo.Tags[tag.Key] = tag.Value
	}

	for _, metadata := range ti.TraceRequestMetadata {
		traceInfo.TraceMetadata[metadata.Key] = metadata.Value
	}

	switch ti.Status {
	case protos.TraceInfoV3_OK.String():
		traceInfo.State = utils.PtrTo(protos.TraceInfoV3_OK)
	case protos.TraceInfoV3_ERROR.String():
		traceInfo.State = utils.PtrTo(protos.TraceInfoV3_ERROR)
	case protos.TraceInfoV3_IN_PROGRESS.String():
		traceInfo.State = utils.PtrTo(protos.TraceInfoV3_IN_PROGRESS)
	default:
		traceInfo.State = utils.PtrTo(protos.TraceInfoV3_STATE_UNSPECIFIED)
	}

	return &traceInfo
}
