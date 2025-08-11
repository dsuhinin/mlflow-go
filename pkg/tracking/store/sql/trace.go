package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mlflow/mlflow-go-backend/pkg/contract"
	"github.com/mlflow/mlflow-go-backend/pkg/entities"
	"github.com/mlflow/mlflow-go-backend/pkg/protos"
	"github.com/mlflow/mlflow-go-backend/pkg/tracking/store/sql/models"
	"github.com/mlflow/mlflow-go-backend/pkg/utils"
)

func (s TrackingSQLStore) SetTrace(
	ctx context.Context,
	experimentID string,
	timestampMS int64,
	metadata []*entities.TraceRequestMetadata,
	tags []*entities.TraceTag,
) (*entities.TraceInfo, error) {
	traceInfo := &models.TraceInfo{
		Tags:                 make([]models.TraceTag, 0, len(tags)),
		Status:               models.TraceInfoStatusInProgress,
		RequestID:            utils.NewUUID(),
		ExperimentID:         experimentID,
		TimestampMS:          timestampMS,
		TraceRequestMetadata: make([]models.TraceRequestMetadata, 0, len(metadata)),
	}

	experiment, err := s.GetExperiment(ctx, experimentID)
	if err != nil {
		return nil, err
	}

	for _, tag := range tags {
		// Very often Python tests mock generation of `request_id` of the flight.
		// It easily works with Python, but it doesn't work with GO,
		// so that's why we need to pass `request_id`
		// from Pythong to Go and override traceInfo.RequestID with value from Python.
		if tag.Key == "mock.generate_request_id.go.testing.tag" {
			traceInfo.RequestID = tag.Value
		} else {
			traceInfo.Tags = append(traceInfo.Tags, models.NewTraceTagFromEntity(traceInfo.RequestID, tag))
		}
	}

	traceArtifactLocationTag, artifactLocationTagErr := GetTraceArtifactLocationTag(experiment, traceInfo.RequestID)
	if artifactLocationTagErr != nil {
		return nil, contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to create trace for experiment_id %q", experimentID),
			artifactLocationTagErr,
		)
	}

	traceInfo.Tags = append(traceInfo.Tags, traceArtifactLocationTag)

	for _, m := range metadata {
		traceInfo.TraceRequestMetadata = append(
			traceInfo.TraceRequestMetadata, models.NewTraceRequestMetadataFromEntity(traceInfo.RequestID, m),
		)
	}

	if err := s.db.WithContext(ctx).Create(&traceInfo).Error; err != nil {
		return nil, contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to create trace for experiment_id %q", experimentID),
			err,
		)
	}

	return traceInfo.ToEntity(), nil
}

//nolint:funlen,cyclop
func (s TrackingSQLStore) SetTraceV3(
	ctx context.Context, traceInfoV3 *entities.TraceInfoV3,
	metadata []*entities.TraceRequestMetadata,
	tags []*entities.TraceTag,
) (*entities.TraceInfoV3, *contract.Error) {
	experiment, err := s.GetExperiment(ctx, traceInfoV3.ExperimentID)
	if err != nil {
		return nil, err
	}

	traceInfo := &models.TraceInfo{
		Tags:         make([]models.TraceTag, 0, len(tags)),
		Status:       traceInfoV3.Status,
		RequestID:    traceInfoV3.RequestID,
		TimestampMS:  traceInfoV3.TimestampMS,
		ExperimentID: traceInfoV3.ExperimentID,
		ExecutionTimeMS: sql.NullInt64{
			Int64: *traceInfoV3.ExecutionTimeMS,
			Valid: traceInfoV3.ExecutionTimeMS != nil && *traceInfoV3.ExecutionTimeMS != 0,
		},
		RequestPreview: sql.NullString{
			String: *traceInfoV3.RequestPreview,
			Valid:  traceInfoV3.RequestPreview != nil && *traceInfoV3.RequestPreview != "",
		},
		ClientRequestID: sql.NullString{
			String: *traceInfoV3.ClientRequestID,
			Valid:  traceInfoV3.ClientRequestID != nil && *traceInfoV3.ClientRequestID != "",
		},
		ResponsePreview: sql.NullString{
			String: *traceInfoV3.ResponsePreview,
			Valid:  traceInfoV3.ResponsePreview != nil && *traceInfoV3.ResponsePreview != "",
		},
		TraceRequestMetadata: make([]models.TraceRequestMetadata, 0, len(metadata)),
	}

	for _, tag := range tags {
		// Very often Python tests mock generation of `request_id` of the flight.
		// It easily works with Python, but it doesn't work with GO,
		// so that's why we need to pass `request_id`
		// from Pythong to Go and override traceInfo.RequestID with value from Python.
		if tag.Key == "mock.generate_request_id.go.testing.tag" {
			traceInfo.RequestID = tag.Value
		} else {
			traceInfo.Tags = append(traceInfo.Tags, models.NewTraceTagFromEntity(traceInfoV3.RequestID, tag))
		}
	}

	traceArtifactLocationTag, artifactLocationTagErr := GetTraceArtifactLocationTag(experiment, traceInfo.RequestID)
	if artifactLocationTagErr != nil {
		return nil, contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to create trace for experiment_id %q", traceInfoV3.ExperimentID),
			artifactLocationTagErr,
		)
	}

	traceInfo.Tags = append(traceInfo.Tags, traceArtifactLocationTag)

	for _, m := range metadata {
		traceInfo.TraceRequestMetadata = append(
			traceInfo.TraceRequestMetadata, models.NewTraceRequestMetadataFromEntity(traceInfo.RequestID, m),
		)
	}

	if err := s.db.WithContext(ctx).Create(&traceInfo).Error; err != nil {
		return nil, contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to create trace for experiment_id %q", experiment.ExperimentID),
			err,
		)
	}

	return traceInfo.ToTraceInfoV3Entity(), nil
}

const (
	BatchSize = 100
)

func (s TrackingSQLStore) SetTraceTag(
	ctx context.Context, requestID, key, value string,
) error {
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}, {Name: "request_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(models.TraceTag{
		Key:       key,
		Value:     value,
		RequestID: requestID,
	}).Error; err != nil {
		return err
	}

	return nil
}

func (s TrackingSQLStore) GetTraceTag(
	ctx context.Context, requestID, key string,
) (*entities.TraceTag, *contract.Error) {
	var tag models.TraceTag
	if err := s.db.WithContext(
		ctx,
	).Where(
		"request_id = ?", requestID,
	).Where(
		"key = ?", key,
	).First(
		&tag,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contract.NewError(
				protos.ErrorCode_RESOURCE_DOES_NOT_EXIST,
				fmt.Sprintf(
					"No trace tag with key '%s' for trace with request_id '%s'",
					key,
					requestID,
				),
			)
		}

		return nil, contract.NewError(protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error getting trace tag: %v", err))
	}

	return tag.ToEntity(), nil
}

func (s TrackingSQLStore) DeleteTraceTag(
	ctx context.Context, tag *entities.TraceTag,
) *contract.Error {
	if err := s.db.WithContext(ctx).Where(
		"request_id = ?", tag.RequestID,
	).Where(
		"key = ?", tag.Key,
	).Delete(
		entities.TraceTag{},
	).Error; err != nil {
		return contract.NewError(protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error deleting trace tag: %v", err))
	}

	return nil
}

func (s TrackingSQLStore) GetTraceInfo(ctx context.Context, reqeustID string) (*entities.TraceInfo, *contract.Error) {
	var traceInfo models.TraceInfo
	if err := s.db.WithContext(
		ctx,
	).Where(
		"request_id = ?", reqeustID,
	).Preload(
		"Tags",
	).Preload(
		"TraceRequestMetadata",
	).First(
		&traceInfo,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contract.NewError(
				protos.ErrorCode_RESOURCE_DOES_NOT_EXIST,
				fmt.Sprintf(
					"Trace with request_id '%s' not found.",
					reqeustID,
				),
			)
		}

		return nil, contract.NewError(
			protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error getting trace info: %v", err),
		)
	}

	return traceInfo.ToEntity(), nil
}

func (s TrackingSQLStore) GetTraceV3Info(
	ctx context.Context, traceID string,
) (*entities.TraceInfoV3, *contract.Error) {
	var traceInfo models.TraceInfo
	if err := s.db.WithContext(
		ctx,
	).Where(
		"request_id = ?", traceID,
	).Preload(
		"Tags",
	).Preload(
		"TraceRequestMetadata",
	).First(
		&traceInfo,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contract.NewError(
				protos.ErrorCode_RESOURCE_DOES_NOT_EXIST,
				fmt.Sprintf(
					"Trace with request_id '%s' not found.",
					traceID,
				),
			)
		}

		return nil, contract.NewError(
			protos.ErrorCode_INTERNAL_ERROR, fmt.Sprintf("error getting trace info: %v", err),
		)
	}

	return traceInfo.ToTraceInfoV3Entity(), nil
}

func (s TrackingSQLStore) EndTrace(
	ctx context.Context,
	reqeustID string,
	timestampMS int64,
	status string,
	metadata []*entities.TraceRequestMetadata,
	tags []*entities.TraceTag,
) (*entities.TraceInfo, error) {
	traceInfo, err := s.GetTraceInfo(ctx, reqeustID)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Model(
			&models.TraceInfo{},
		).Where(
			"request_id = ?", traceInfo.RequestID,
		).UpdateColumns(map[string]interface{}{
			"status":            status,
			"execution_time_ms": timestampMS - traceInfo.TimestampMS,
		}).Error; err != nil {
			return contract.NewErrorWith(
				protos.ErrorCode_INTERNAL_ERROR,
				fmt.Sprintf("failed to update trace with request_id '%s'", reqeustID),
				err,
			)
		}

		if err := s.createTraceTags(transaction, reqeustID, tags); err != nil {
			return err
		}

		if err := s.createTraceMetadata(transaction, reqeustID, metadata); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err //nolint
	}

	traceInfo, err = s.GetTraceInfo(ctx, reqeustID)
	if err != nil {
		return nil, err
	}

	return traceInfo, nil
}

func (s TrackingSQLStore) createTraceTags(transaction *gorm.DB, requestID string, tags []*entities.TraceTag) error {
	traceTags := make([]models.TraceTag, 0, len(tags))
	for _, tag := range tags {
		traceTags = append(traceTags, models.NewTraceTagFromEntity(requestID, tag))
	}

	if err := transaction.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(traceTags, batchSize).Error; err != nil {
		return contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to update trace tags %v", err),
			err,
		)
	}

	return nil
}

func (s TrackingSQLStore) createTraceMetadata(
	transaction *gorm.DB, requestID string, metadata []*entities.TraceRequestMetadata,
) error {
	traceMetadata := make([]models.TraceRequestMetadata, 0, len(metadata))
	for _, m := range metadata {
		traceMetadata = append(traceMetadata, models.NewTraceRequestMetadataFromEntity(requestID, m))
	}

	if err := transaction.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(traceMetadata, batchSize).Error; err != nil {
		return contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to update trace metadata %v", err),
			err,
		)
	}

	return nil
}

func (s TrackingSQLStore) DeleteTraces(
	ctx context.Context,
	experimentID string,
	maxTimestampMillis int64,
	maxTraces int32,
	requestIDs []string,
) (int32, *contract.Error) {
	query := s.db.WithContext(
		ctx,
	).Where(
		"experiment_id = ?", experimentID,
	)

	if maxTimestampMillis != 0 {
		query = query.Where("timestamp_ms <= ?", maxTimestampMillis)
	}

	if len(requestIDs) > 0 {
		query = query.Where("request_id IN (?)", requestIDs)
	}

	if maxTraces != 0 {
		query = query.Where(
			"request_id IN (?)",
			s.db.Select(
				"request_id",
			).Model(
				&models.TraceInfo{},
			).Order(
				"timestamp_ms ASC",
			).Limit(
				int(maxTraces),
			),
		)
	}

	var traces []models.TraceInfo
	if err := query.Debug().Clauses(
		clause.Returning{
			Columns: []clause.Column{
				{Name: "request_id"},
			},
		},
	).Delete(
		&traces,
	).Error; err != nil {
		return 0, contract.NewErrorWith(
			protos.ErrorCode_INTERNAL_ERROR,
			fmt.Sprintf("failed to delete traces %v", err),
			err,
		)
	}

	//nolint:gosec
	return int32(len(traces)), nil
}
