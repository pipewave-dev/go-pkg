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
	FieldSessionID      = "SessionID"
	FieldHolderID       = "HolderID"
	FieldConnectionType = "ConnectionType"
	FieldStatus         = "Status"
	FieldConnectedAt    = "ConnectedAt"
	FieldLastHeartbeat  = "LastHeartbeat"
	FieldTTL            = "TTL"
)

type ddbActiveConnection struct {
	UserID    string // PartitionKey ~ contraint User.ID
	SessionID string // SortKey

	HolderID       string // Pod name holding this connection (env.PodName)
	ConnectionType voWs.WsCoreType
	Status         voWs.WsStatus
	ConnectedAt    voUnixTime.UnixMilliTime
	LastHeartbeat  voUnixTime.UnixMilliTime
	TTL            voUnixTime.UnixMilliTime
}

func toDynamoDBItem(e *entities.ActiveConnection) *ddbActiveConnection {
	return &ddbActiveConnection{
		UserID:         e.UserID,
		SessionID:      e.SessionID,
		HolderID:       e.HolderID,
		ConnectionType: e.ConnectionType,
		Status:         e.Status,
		ConnectedAt:    voUnixTime.UnixMilliTime(e.ConnectedAt),
		LastHeartbeat:  voUnixTime.UnixMilliTime(e.LastHeartbeat),
		TTL:            voUnixTime.UnixMilliTime(e.TTL),
	}
}

func (e *ddbActiveConnection) toEntity() *entities.ActiveConnection {
	return &entities.ActiveConnection{
		UserID:         e.UserID,
		SessionID:      e.SessionID,
		HolderID:       e.HolderID,
		ConnectionType: e.ConnectionType,
		Status:         e.Status,
		ConnectedAt:    time.Time(e.ConnectedAt),
		LastHeartbeat:  time.Time(e.LastHeartbeat),
		TTL:            time.Time(e.TTL),
	}
}

func toDynamoMap(e *entities.ActiveConnection) (map[string]types.AttributeValue, error) {
	ddbItem := toDynamoDBItem(e)
	return attributevalue.MarshalMap(ddbItem)
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
