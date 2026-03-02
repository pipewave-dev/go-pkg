package dynamodb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/samber/lo"
)

// UpdateADD will update data with ADD expression
func (ddb *dynamodbClient) ExecuteTransactWriteItems(
	ctx context.Context,
	items ...*types.TransactWriteItem,
) (err error) {
	items = lo.Filter(items, func(item *types.TransactWriteItem, _ int) bool {
		return item != nil
	})

	itemsV := lo.Map(items, func(item *types.TransactWriteItem, index int) types.TransactWriteItem {
		return *item
	})

	_, err = ddb.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: itemsV,
	})
	if err != nil {
		slog.ErrorContext(ctx, "(*dynamodbClient).ExecuteTransactWriteItems #1 Execute transaction fail",
			slog.Any("err", err))

		return fmt.Errorf("ExecuteTransactWriteItems failed: %w", err)
	}

	return nil
}
