package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

type UserCreator struct {
	ConfigStore configprovider.ConfigStore
}

type CreateParams struct {
	ID string
}

func (creator *UserCreator) Create(ctx context.Context, ddbClient *dynamodb.Client, params CreateParams) (*entities.User, aerror.AError) {
	now := time.Now()
	result := &entities.User{
		ID:            params.ID,
		LastHeartbeat: now,
		CreatedAt:     now,
	}

	userDataAV, err := toDynamoMap(result)
	if err != nil {
		msg := fmt.Sprintf("*UserCreator unmarshal error: %v", err)
		panic(msg)
	}

	builder := expression.NewBuilder().
		WithCondition(
			expression.Name(FieldID).AttributeNotExists(),
		)
	expr, errB := builder.Build()
	if errB != nil {
		msg := fmt.Sprintf("*UserCreator build expression error: %v", errB)
		panic(msg)
	}
	//nolint:exhaustruct
	putItemParams := &dynamodb.PutItemInput{
		TableName:                 lo.ToPtr(creator.ConfigStore.Env().DynamoDB.Tables.User),
		Item:                      userDataAV,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = ddbClient.PutItem(ctx, putItemParams)
	if err != nil {
		aErr := aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return nil, aErr
	}

	return result, nil
}

type UpsertParams struct {
	ID string
}

func (creator *UserCreator) Upsert(ctx context.Context, ddbClient *dynamodb.Client, params UpsertParams) (*entities.User, aerror.AError) {
	now := time.Now()
	nowMilli := voUnixTime.UnixMilliTime(now)

	key, err := attributevalue.MarshalMap(struct{ ID string }{ID: params.ID})
	if err != nil {
		msg := fmt.Sprintf("*UserCreator.Upsert marshal key error: %v", err)
		panic(msg)
	}

	// CreatedAt is only set on first write: on reconnect/subsequent upserts,
	// IfNotExists keeps the value already stored instead of resetting it to
	// now, matching the Postgres upsert.go behavior.
	update := expression.
		Set(expression.Name(FieldLastHeartbeat), expression.Value(nowMilli)).
		Set(expression.Name(FieldCreatedAt), expression.IfNotExists(expression.Name(FieldCreatedAt), expression.Value(nowMilli)))

	expr, errB := expression.NewBuilder().WithUpdate(update).Build()
	if errB != nil {
		msg := fmt.Sprintf("*UserCreator.Upsert build expression error: %v", errB)
		panic(msg)
	}

	//nolint:exhaustruct
	updateItemParams := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(creator.ConfigStore.Env().DynamoDB.Tables.User),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              types.ReturnValueAllNew,
	}

	output, err := ddbClient.UpdateItem(ctx, updateItemParams)
	if err != nil {
		aErr := aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return nil, aErr
	}

	result, err := fromDynamoMap(output.Attributes)
	if err != nil {
		msg := fmt.Sprintf("*UserCreator.Upsert unmarshal error: %v", err)
		panic(msg)
	}

	return result, nil
}
