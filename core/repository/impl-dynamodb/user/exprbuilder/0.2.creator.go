package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
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

	//nolint:exhaustruct
	putItemParams := &dynamodb.PutItemInput{
		TableName: lo.ToPtr(creator.ConfigStore.Env().DynamoDB.Tables.User),
		Item:      userDataAV,
	}

	_, err = ddbClient.PutItem(ctx, putItemParams)
	if err != nil {
		aErr := aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return nil, aErr
	}

	return result, nil
}
