package exprbuilder

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
)

// Field name
const (
	FieldUserID         = "UserID"
	FieldInstanceID     = "InstanceID"
	FieldHolderID       = "HolderID"
	FieldConnectionType = "ConnectionType"
	FieldStatus         = "Status"
	FieldConnectedAt    = "ConnectedAt"
	FieldLastHeartbeat  = "LastHeartbeat"
	FieldTTL            = "TTL"
	// FieldTTLSeconds mirrors FieldTTL as epoch seconds (DynamoDB native TTL
	// requires seconds, while FieldTTL is stored in millis for the existing
	// Scan-based cleanup). Enable native TTL on this attribute in the table config.
	FieldTTLSeconds = "TTLSeconds"
)

type ddbActiveConnection struct {
	UserID     string // PartitionKey ~ contraint User.ID
	InstanceID string // SortKey

	HolderID       string // Pod name holding this connection (env.PodName)
	ConnectionType voWs.WsCoreType
	Status         voWs.WsStatus
	ConnectedAt    voUnixTime.UnixMilliTime
	LastHeartbeat  voUnixTime.UnixMilliTime
	TTL            voUnixTime.UnixMilliTime
}

func (e *ddbActiveConnection) toEntity() *entities.ActiveConnection {
	return &entities.ActiveConnection{
		UserID:         e.UserID,
		InstanceID:     e.InstanceID,
		HolderID:       e.HolderID,
		ConnectionType: e.ConnectionType,
		Status:         e.Status,
		ConnectedAt:    time.Time(e.ConnectedAt),
		LastHeartbeat:  time.Time(e.LastHeartbeat),
		TTL:            time.Time(e.TTL),
	}
}

func fromDynamoMap(item map[string]types.AttributeValue) (e *entities.ActiveConnection, err error) {
	ddbItem := &ddbActiveConnection{}
	err = attributevalue.UnmarshalMap(item, ddbItem)
	if err != nil {
		return nil, err
	}

	result := ddbItem.toEntity()
	return result, nil
}
