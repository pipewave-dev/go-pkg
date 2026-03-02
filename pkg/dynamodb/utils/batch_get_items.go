package ddbutils

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type BatchGetItemAllOptions struct {
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func BatchGetItemAll(
	ctx context.Context,
	client *dynamodb.Client,
	input *dynamodb.BatchGetItemInput,
	opts ...BatchGetItemAllOptions,
) (result *dynamodb.BatchGetItemOutput, isPartiallyCompleted bool, err error) {
	opt := BatchGetItemAllOptions{
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

	finalOutput := &dynamodb.BatchGetItemOutput{
		Responses:       make(map[string][]map[string]types.AttributeValue),
		UnprocessedKeys: make(map[string]types.KeysAndAttributes),
	}

	currentInput := *input
	currentInput.RequestItems = make(map[string]types.KeysAndAttributes)
	maps.Copy(currentInput.RequestItems, input.RequestItems)

	backoff := opt.MinBackoff

	for i := 0; i <= opt.MaxRetries; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}

		output, err := client.BatchGetItem(ctx, &currentInput)
		if err != nil {
			return nil, false, err
		}

		for tableName, items := range output.Responses {
			finalOutput.Responses[tableName] = append(finalOutput.Responses[tableName], items...)
		}

		if len(output.UnprocessedKeys) == 0 {
			return finalOutput, false, nil
		}

		currentInput.RequestItems = output.UnprocessedKeys

		if i == opt.MaxRetries {
			finalOutput.UnprocessedKeys = output.UnprocessedKeys
			msg := fmt.Errorf("BatchGetItemAll: exceeded max retries (%d) with %d unprocessed tables", opt.MaxRetries, len(output.UnprocessedKeys))
			return finalOutput, true, msg
		}

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

	err = fmt.Errorf("BatchGetItemAll: exceeded max retries (%d) with %d unprocessed tables", opt.MaxRetries, len(finalOutput.UnprocessedKeys))
	return finalOutput, true, err
}
