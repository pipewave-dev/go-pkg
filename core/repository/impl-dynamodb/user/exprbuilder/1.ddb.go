package exprbuilder

import (
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	FieldID            = "ID"
	FieldLastHeartbeat = "LastHeartbeat"
	FieldCreatedAt     = "CreatedAt"
)

type ddbUser struct {
	ID string

	LastHeartbeat voUnixTime.UnixMilliTime

	CreatedAt voUnixTime.UnixMilliTime `dynamodbav:"create_at"`
}

func toDynamoDBItem(e *entities.User) *ddbUser {
	return &ddbUser{
		ID:            e.ID,
		LastHeartbeat: voUnixTime.UnixMilliTime(e.LastHeartbeat),
		CreatedAt:     voUnixTime.UnixMilliTime(e.CreatedAt),
	}
}

func (e *ddbUser) toEntity() *entities.User {
	return &entities.User{
		ID:            e.ID,
		LastHeartbeat: time.Time(e.LastHeartbeat),
		CreatedAt:     time.Time(e.CreatedAt),
	}
}

func toDynamoMap(e *entities.User) (map[string]types.AttributeValue, error) {
	ddbItem := toDynamoDBItem(e)
	return attributevalue.MarshalMap(ddbItem)
}

func fromDynamoMap(item map[string]types.AttributeValue) (e *entities.User, err error) {
	ddbItem := &ddbUser{}
	err = attributevalue.UnmarshalMap(item, ddbItem)
	if err != nil {
		return nil, err
	}

	result := ddbItem.toEntity()
	return result, nil
}
