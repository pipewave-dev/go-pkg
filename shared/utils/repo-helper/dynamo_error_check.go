package repohelper

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Helper function to check if error is ConditionalCheckFailedException
func IsConditionalCheckFailedException(err error) bool {
	if err == nil {
		return false
	}
	var conditionalCheckErr *types.ConditionalCheckFailedException
	return errors.As(err, &conditionalCheckErr)
}

func IsDuplicateItem(err error) bool {
	if err == nil {
		return false
	}
	var (
		errConditionalCheck *types.ConditionalCheckFailedException
		errDuplicateItem    *types.DuplicateItemException
	)

	// Handle duplicate item errors
	if errors.As(err, &errConditionalCheck) || errors.As(err, &errDuplicateItem) {
		return true
	}

	return false
}
