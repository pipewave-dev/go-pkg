package repohelper

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestIsConditionalCheckFailedException(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ConditionalCheckFailedException",
			err:      &types.ConditionalCheckFailedException{Message: new(string)},
			expected: true,
		},
		{
			name:     "Wrapped ConditionalCheckFailedException",
			err:      errors.New("wrapped: " + (&types.ConditionalCheckFailedException{Message: new(string)}).Error()),
			expected: false, // errors.As won't unwrap string-wrapped errors
		},
		{
			name:     "Different DynamoDB exception",
			err:      &types.DuplicateItemException{Message: new(string)},
			expected: false,
		},
		{
			name:     "Generic error",
			err:      errors.New("some generic error"),
			expected: false,
		},
		{
			name:     "Nil pointer to ConditionalCheckFailedException",
			err:      (*types.ConditionalCheckFailedException)(nil),
			expected: true, // errors.As with nil pointer still matches in Go
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConditionalCheckFailedException(tt.err)
			if result != tt.expected {
				t.Errorf("IsConditionalCheckFailedException() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsDuplicateItem(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ConditionalCheckFailedException",
			err:      &types.ConditionalCheckFailedException{Message: new(string)},
			expected: true,
		},
		{
			name:     "DuplicateItemException",
			err:      &types.DuplicateItemException{Message: new(string)},
			expected: true,
		},
		{
			name:     "Both ConditionalCheckFailedException and DuplicateItemException (wrapped)",
			err:      &types.ConditionalCheckFailedException{Message: new(string)},
			expected: true,
		},
		{
			name:     "Different DynamoDB exception",
			err:      &types.ResourceNotFoundException{Message: new(string)},
			expected: false,
		},
		{
			name:     "Generic error",
			err:      errors.New("some generic error"),
			expected: false,
		},
		{
			name:     "Nil pointer to ConditionalCheckFailedException",
			err:      (*types.ConditionalCheckFailedException)(nil),
			expected: true, // errors.As with nil pointer still matches in Go
		},
		{
			name:     "Nil pointer to DuplicateItemException",
			err:      (*types.DuplicateItemException)(nil),
			expected: true, // errors.As with nil pointer still matches in Go
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDuplicateItem(tt.err)
			if result != tt.expected {
				t.Errorf("IsDuplicateItem() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestErrorTypeAssertion tests that errors.As works correctly with DynamoDB exception types
func TestErrorTypeAssertion(t *testing.T) {
	// Test that ConditionalCheckFailedException can be detected
	conditionalErr := &types.ConditionalCheckFailedException{Message: new(string)}

	var checkErr *types.ConditionalCheckFailedException
	if !errors.As(conditionalErr, &checkErr) {
		t.Error("Expected errors.As to detect ConditionalCheckFailedException")
	}

	// Test that DuplicateItemException can be detected
	duplicateErr := &types.DuplicateItemException{Message: new(string)}

	var dupErr *types.DuplicateItemException
	if !errors.As(duplicateErr, &dupErr) {
		t.Error("Expected errors.As to detect DuplicateItemException")
	}

	// Test that different types don't cross-match
	var wrongType *types.ResourceNotFoundException
	if errors.As(conditionalErr, &wrongType) {
		t.Error("Expected errors.As to NOT detect ResourceNotFoundException from ConditionalCheckFailedException")
	}
}

// BenchmarkIsConditionalCheckFailedException benchmarks the performance of the error check
func BenchmarkIsConditionalCheckFailedException(b *testing.B) {
	err := &types.ConditionalCheckFailedException{Message: new(string)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsConditionalCheckFailedException(err)
	}
}

// BenchmarkIsDuplicateItem benchmarks the performance of the duplicate item check
func BenchmarkIsDuplicateItem(b *testing.B) {
	err := &types.ConditionalCheckFailedException{Message: new(string)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDuplicateItem(err)
	}
}
