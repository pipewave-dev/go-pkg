package dynamodb

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/samber/lo"
)

// UpdateADD will update data with ADD expression
func (ddb *dynamodbClient) RecursiveBatchWriteItem(
	ctx context.Context,
	tableName string,
	reqsItems []types.WriteRequest,
	depth int) (unprocessedItems []types.WriteRequest, err error) {

	reqsItemsChunks := lo.Chunk(reqsItems, 25) // DynamoDB allow maximun 25 WriteItem per request

	errorChunks := make([]error, len(reqsItemsChunks))
	unprocessedItemPerChunks := make([][]types.WriteRequest, len(reqsItemsChunks))
	for i := range reqsItemsChunks {
		counter := 0
		unprocessed := reqsItemsChunks[i]
		for len(unprocessed) > 0 {
			if counter > depth {
				unprocessedItemPerChunks[i] = unprocessed
				break
			}
			counter++

			output, errWrite := ddb.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: unprocessed,
				}})
			if errWrite != nil {
				errorChunks[i] = fmt.Errorf("BatchWriteItem fail at chunk [%d]", i)
				unprocessedItemPerChunks[i] = unprocessed
				break
			}
			unprocessed = output.UnprocessedItems[tableName]
		}
	}

	unprocessedItems = lo.Flatten(unprocessedItemPerChunks)
	err = errors.Join(errorChunks...)
	if err != nil {
		return unprocessedItems, err
	}
	return unprocessedItems, nil
}
