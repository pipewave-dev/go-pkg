package exprbuilder

import (
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Field name
const (
	FieldUserID        = "UserID"
	FieldSessionID     = "SessionID"
	FieldHolderID      = "HolderID"
	FieldLastHeartbeat = "LastHeartbeat"
	FieldTTL           = "TTL"
)

type ddbActiveConnection struct {
	UserID    string // PartitionKey ~ contraint User.ID
	SessionID string // SortKey

	HolderID      string // Pod name holding this connection (env.PodName)
	LastHeartbeat voUnixTime.UnixMilliTime
	TTL           voUnixTime.UnixMilliTime
}

func toDynamoDBItem(e *entities.ActiveConnection) *ddbActiveConnection {
	return &ddbActiveConnection{
		UserID:        e.UserID,
		SessionID:     e.SessionID,
		HolderID:      e.HolderID,
		LastHeartbeat: voUnixTime.UnixMilliTime(e.LastHeartbeat),
		TTL:           voUnixTime.UnixMilliTime(e.TTL),
	}
}

func (e *ddbActiveConnection) toEntity() *entities.ActiveConnection {
	return &entities.ActiveConnection{
		UserID:        e.UserID,
		SessionID:     e.SessionID,
		HolderID:      e.HolderID,
		LastHeartbeat: time.Time(e.LastHeartbeat),
		TTL:           time.Time(e.TTL),
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
