package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/samber/lo"
)

// DefaultRecursiveBatchGetItemDepth is the recommended max retry depth for
// callers of RecursiveBatchGetItem until a real usecase dictates otherwise.
const DefaultRecursiveBatchGetItemDepth = 3

const (
	recursiveBatchGetItemMinBackoff = 100 * time.Millisecond
	recursiveBatchGetItemMaxBackoff = 2 * time.Second
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
		backoff := recursiveBatchGetItemMinBackoff
		for len(unprocessed) > 0 {
			if counter > depth {
				unprocessedKeysAvChunks[i] = unprocessed[tableName].Keys
				break
			}

			if counter > 0 {
				select {
				case <-ctx.Done():
					unprocessedKeysAvChunks[i] = unprocessed[tableName].Keys
					return resultsSlice, lo.Flatten(unprocessedKeysAvChunks), ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
				if backoff > recursiveBatchGetItemMaxBackoff {
					backoff = recursiveBatchGetItemMaxBackoff
				}
			}
			counter++

			output, errRead := ddb.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: unprocessed,
			})
			if errRead != nil {
				return nil, nil, fmt.Errorf("RecursiveBatchGetItem failed: %w", errRead)
			}

			// Always refresh unprocessed before any continue so a chunk that came
			// back empty (e.g. fully throttled) still retries the leftover keys
			// instead of looping on a stale request forever.
			unprocessed = output.UnprocessedKeys

			result, ok := output.Responses[tableName]
			if !ok {
				continue
			}
			resultsSlice = append(resultsSlice, result...)
		}
	}

	unprocessedKeysAV = lo.Flatten(unprocessedKeysAvChunks)

	return resultsSlice, unprocessedKeysAV, nil
}
