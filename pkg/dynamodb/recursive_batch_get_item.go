package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/samber/lo"
)

func (ddb *dynamodbClient) RecursiveBatchGetItem(
	ctx context.Context,
	tableName string,
	keysAV []map[string]types.AttributeValue,
	depth int,
) (item []map[string]types.AttributeValue, unprocessedKeysAV []map[string]types.AttributeValue, err error) {
	keysAvChunks := lo.Chunk(keysAV, 100) // Maximum items per API

	resultsSlice := make([]map[string]types.AttributeValue, 0)
	unprocessedKeysAvChunks := make([][]map[string]types.AttributeValue, len(keysAvChunks))
	for i, keysAvChunk := range keysAvChunks {
		unprocessed := map[string]types.KeysAndAttributes{
			tableName: {
				Keys: keysAvChunk,
			},
		}
		counter := 0
		for len(unprocessed) > 0 {
			if counter > depth {
				unprocessedKeysAvChunks[i] = unprocessed[tableName].Keys
				break
			}
			output, errRead := ddb.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: unprocessed,
			})
			if errRead != nil {
				return nil, nil, fmt.Errorf("RecursiveBatchGetItem failed: %w", errRead)
			}
			result, ok := output.Responses[tableName]
			if !ok {
				continue
			}
			resultsSlice = append(resultsSlice, result...)

			unprocessed = output.UnprocessedKeys
		}
	}

	unprocessedKeysAV = lo.Flatten(unprocessedKeysAvChunks)

	return resultsSlice, unprocessedKeysAV, nil
}
