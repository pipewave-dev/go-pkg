package exprbuilder

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

type UserQuerier struct {
	ConfigStore configprovider.ConfigStore
}

type ByIDParams struct {
	ID string
}

func (querier *UserQuerier) ByID(ctx context.Context, ddbClient *dynamodb.Client, params ByIDParams) (result *entities.User, aErr aerror.AError) {
	// if params.ID == "" {
	// 	aErr = aerror.New(ctx, aerror.ErrUnexpectedInput, nil)
	// 	return nil, aErr
	// }

	// Query using PartitionKey (ID as partition key)
	keyEx := expression.Key(FieldID).Equal(expression.Value(params.ID))
	builder := expression.NewBuilder().
		WithKeyCondition(keyEx)
	expr, errB := builder.Build()
	if errB != nil {
		msg := fmt.Sprintf("QueryByID builder failed: %v", errB)
		panic(msg)
	}

	// Execute query
	//nolint:exhaustruct
	queryParams := &dynamodb.QueryInput{
		TableName:                 lo.ToPtr(querier.ConfigStore.Env().DynamoDB.Tables.User),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	output, errC := ddbClient.Query(ctx, queryParams)
	if errC != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, errC)
		return nil, aErr
	}

	// Unmarshal result
	if output.Count == 0 {
		aErr = aerror.New(ctx, aerror.RecordNotFound, nil)
		return nil, aErr
	}
	if output.Count != 1 {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, fmt.Errorf("too many result in query, expected 1 but got %d", output.Count))
		return nil, aErr
	}

	result, errM := fromDynamoMap(output.Items[0])
	if errM != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, errM)
		return nil, aErr
	}

	return result, nil
}
