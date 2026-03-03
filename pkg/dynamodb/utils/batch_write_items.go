package ddbutils

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type BatchWriteItemOptions struct {
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func BatchWriteItemAll(
	ctx context.Context,
	client *dynamodb.Client,
	input *dynamodb.BatchWriteItemInput,
	opts ...BatchWriteItemOptions,
) (result *dynamodb.BatchWriteItemOutput, isPartiallyCompleted bool, err error) {
	opt := BatchWriteItemOptions{
		MaxRetries: 5,
		MinBackoff: 100 * time.Millisecond,
		MaxBackoff: 2 * time.Second,
	}
	if len(opts) > 0 {
		if opts[0].MaxRetries > 0 {
			opt.MaxRetries = opts[0].MaxRetries
		}
		if opts[0].MinBackoff > 0 {
			opt.MinBackoff = opts[0].MinBackoff
		}
		if opts[0].MaxBackoff > 0 {
			opt.MaxBackoff = opts[0].MaxBackoff
		}
	}

	finalOutput := &dynamodb.BatchWriteItemOutput{
		UnprocessedItems:      make(map[string][]types.WriteRequest),
		ItemCollectionMetrics: make(map[string][]types.ItemCollectionMetrics),
		ConsumedCapacity:      []types.ConsumedCapacity{},
	}

	currentInput := *input
	currentInput.RequestItems = make(map[string][]types.WriteRequest)
	maps.Copy(currentInput.RequestItems, input.RequestItems)

	backoff := opt.MinBackoff

	for i := 0; i <= opt.MaxRetries; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}

		output, err := client.BatchWriteItem(ctx, &currentInput)
		if err != nil {
			return nil, false, err
		}

		// Accumulate ItemCollectionMetrics
		for tableName, metrics := range output.ItemCollectionMetrics {
			finalOutput.ItemCollectionMetrics[tableName] = append(finalOutput.ItemCollectionMetrics[tableName], metrics...)
		}

		// Accumulate ConsumedCapacity
		finalOutput.ConsumedCapacity = append(finalOutput.ConsumedCapacity, output.ConsumedCapacity...)

		// Check if all items processed
		if len(output.UnprocessedItems) == 0 {
			return finalOutput, false, nil
		}

		// Retry unprocessed items
		currentInput.RequestItems = output.UnprocessedItems

		// If max retries reached, return with error
		if i == opt.MaxRetries {
			finalOutput.UnprocessedItems = output.UnprocessedItems
			msg := fmt.Errorf("BatchWriteItemAll: exceeded max retries (%d) with %d unprocessed tables", opt.MaxRetries, len(output.UnprocessedItems))
			return finalOutput, true, msg
		}

		// Exponential backoff
		select {
		case <-ctx.Done():
			return finalOutput, true, ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > opt.MaxBackoff {
			backoff = opt.MaxBackoff
		}
	}

	err = fmt.Errorf("BatchWriteItemAll: exceeded max retries (%d) with %d unprocessed tables", opt.MaxRetries, len(finalOutput.UnprocessedItems))
	return finalOutput, true, err
}
