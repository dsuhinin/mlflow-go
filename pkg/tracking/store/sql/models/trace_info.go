package models

import (
	"database/sql"

	"github.com/mlflow/mlflow-go-backend/pkg/entities"
	"github.com/mlflow/mlflow-go-backend/pkg/utils"
)

const (
	TraceInfoStatusUnspecified = "TRACE_STATUS_UNSPECIFIED"
	TraceInfoStatusOk          = "OK"
	TraceInfoStatusError       = "ERROR"
	TraceInfoStatusInProgress  = "IN_PROGRESS"
)

// TraceInfo mapped from table <trace_info>.
type TraceInfo struct {
	RequestID            string                 `gorm:"column:request_id;primaryKey"`
	ExperimentID         string                 `gorm:"column:experiment_id"`
	ClientRequestID      sql.NullString         `gorm:"column:client_request_id"`
	RequestPreview       sql.NullString         `gorm:"column:request_preview"`
	ResponsePreview      sql.NullString         `gorm:"column:request_preview"`
	TimestampMS          int64                  `gorm:"column:timestamp_ms"`
	ExecutionTimeMS      sql.NullInt64          `gorm:"column:execution_time_ms"`
	Status               string                 `gorm:"column:status"`
	Tags                 []TraceTag             `gorm:"foreignKey:RequestID"`
	TraceRequestMetadata []TraceRequestMetadata `gorm:"foreignKey:RequestID"`
}

func (ti TraceInfo) TableName() string {
	return "trace_info"
}

func (ti TraceInfo) ToEntity() *entities.TraceInfo {
	traceInfo := entities.TraceInfo{
		Tags:                 make([]*entities.TraceTag, 0, len(ti.Tags)),
		Status:               ti.Status,
		RequestID:            ti.RequestID,
		ExperimentID:         ti.ExperimentID,
		TimestampMS:          ti.TimestampMS,
		TraceRequestMetadata: make([]*entities.TraceRequestMetadata, 0, len(ti.TraceRequestMetadata)),
	}

	if ti.ExecutionTimeMS.Valid {
		traceInfo.ExecutionTimeMS = &ti.ExecutionTimeMS.Int64
	}

	for _, tag := range ti.Tags {
		traceInfo.Tags = append(traceInfo.Tags, tag.ToEntity())
	}

	for _, metadata := range ti.TraceRequestMetadata {
		traceInfo.TraceRequestMetadata = append(traceInfo.TraceRequestMetadata, metadata.ToEntity())
	}

	return &traceInfo
}

func (ti TraceInfo) ToTraceInfoV3Entity() *entities.TraceInfoV3 {
	traceInfoV3 := entities.TraceInfoV3{
		Tags:                 make([]*entities.TraceTag, 0, len(ti.Tags)),
		Status:               ti.Status,
		RequestID:            ti.RequestID,
		ExperimentID:         ti.ExperimentID,
		TimestampMS:          ti.TimestampMS,
		TraceRequestMetadata: make([]*entities.TraceRequestMetadata, 0, len(ti.TraceRequestMetadata)),
	}

	if ti.ExecutionTimeMS.Valid {
		traceInfoV3.ExecutionTimeMS = utils.PtrTo(ti.ExecutionTimeMS.Int64)
	}

	if ti.ClientRequestID.Valid {
		traceInfoV3.ClientRequestID = utils.PtrTo(ti.ClientRequestID.String)
	}

	if ti.ResponsePreview.Valid {
		traceInfoV3.ResponsePreview = utils.PtrTo(ti.ResponsePreview.String)
	}

	if ti.RequestPreview.Valid {
		traceInfoV3.RequestPreview = utils.PtrTo(ti.RequestPreview.String)
	}

	for _, tag := range ti.Tags {
		traceInfoV3.Tags = append(traceInfoV3.Tags, tag.ToEntity())
	}

	for _, metadata := range ti.TraceRequestMetadata {
		traceInfoV3.TraceRequestMetadata = append(traceInfoV3.TraceRequestMetadata, metadata.ToEntity())
	}

	return &traceInfoV3
}
