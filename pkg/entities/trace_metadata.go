//nolint:dupl
package entities

import "github.com/mlflow/mlflow-go-backend/pkg/protos"

type TraceRequestMetadata struct {
	Key       string
	Value     string
	RequestID string
}

func (trm TraceRequestMetadata) ToProto() *protos.TraceRequestMetadata {
	return &protos.TraceRequestMetadata{
		Key:   &trm.Key,
		Value: &trm.Value,
	}
}

func TraceRequestMetadataFromStartTraceProtoInput(
	protoMetadata []*protos.TraceRequestMetadata,
) []*TraceRequestMetadata {
	entityMetadata := make([]*TraceRequestMetadata, 0, len(protoMetadata))
	for _, m := range protoMetadata {
		entityMetadata = append(entityMetadata, &TraceRequestMetadata{
			Key:   m.GetKey(),
			Value: m.GetValue(),
		})
	}

	return entityMetadata
}

func TraceRequestMetadataFromStartTraceV3ProtoInput(protoMetadata map[string]string) []*TraceRequestMetadata {
	entityMetadata := make([]*TraceRequestMetadata, 0, len(protoMetadata))
	for k, v := range protoMetadata {
		entityMetadata = append(entityMetadata, &TraceRequestMetadata{
			Key:   k,
			Value: v,
		})
	}

	return entityMetadata
}
