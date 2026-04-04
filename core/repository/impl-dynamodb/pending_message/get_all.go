package pendingMessageRepo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

const fnGetAll = "pendingMessageRepo.GetAll"

func (r *pendingMessageRepo) GetAll(ctx context.Context, userID, instanceID string) (msgs [][]byte, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetAll)
	defer op.Finish(aErr)

	keyEx := expression.Key("SessionKey").Equal(expression.Value(sessionKey(userID, instanceID)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		panic(fmt.Sprintf("pendingMessageRepo.GetAll build expression error: %v", err))
	}

	//nolint:exhaustruct
	input := &dynamodb.QueryInput{
		TableName:                 lo.ToPtr(r.c.Env().DynamoDB.Tables.PendingMessage),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          lo.ToPtr(true), // ascending by SendAt
	}

	paginator := dynamodb.NewQueryPaginator(r.ddb.Client(), input)

	for paginator.HasMorePages() {
		output, err2 := paginator.NextPage(ctx)
		if err2 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
			return nil, aErr
		}

		for _, item := range output.Items {
			var row ddbPendingMessage
			if err3 := attributevalue.UnmarshalMap(item, &row); err3 != nil {
				aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err3)
				return nil, aErr
			}
			msgs = append(msgs, row.Message)
		}
	}

	return msgs, nil
}
