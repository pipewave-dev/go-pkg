package voUnixTime

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/vmihailenco/msgpack/v5"
)

type UnixMilliTime time.Time

// ======================= DynamoDB Marshaler ===========================
const zero = "0"

func (t *UnixMilliTime) fromDynamoNumber(ctx context.Context, rawStr string) aerror.AError {
	if rawStr == zero {
		return nil
	}

	// Convert rawStr to int64
	millisec, err := strconv.ParseInt(rawStr, 10, 64)
	if err != nil {
		return aerror.New(ctx,
			aerror.ErrUnmarshal,
			fmt.Errorf("failed to parse value to int: %w", err))
	}

	// Convert int64 to time.Time
	parsedTime := time.UnixMilli(millisec)

	*t = UnixMilliTime(parsedTime)
	return nil
}

func (t UnixMilliTime) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	return &types.AttributeValueMemberN{
		Value: fmt.Sprintf("%d", time.Time(t).UnixMilli()),
	}, nil
}
func (t *UnixMilliTime) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	avN, ok := av.(*types.AttributeValueMemberN)
	if !ok {
		return &attributevalue.UnmarshalTypeError{
			Value: fmt.Sprintf("%T", av),
			Type:  reflect.TypeOf((*UnixMilliTime)(nil)),
			Err:   fmt.Errorf("expected *types.AttributeValueMemberN, got %T", av),
		}
	}

	result := &UnixMilliTime{}
	err := result.fromDynamoNumber(context.Background(), avN.Value)
	if err != nil {
		return err
	}
	*t = *result
	return nil
}

// ====
func (t UnixMilliTime) MarshalMsgpack() ([]byte, error) {
	return msgpack.Marshal(time.Time(t))
}

func (t *UnixMilliTime) UnmarshalMsgpack(b []byte) error {
	normalTime := time.Time{}
	err := msgpack.Unmarshal(b, &normalTime)
	if err != nil {
		return err
	}
	*t = UnixMilliTime(normalTime)
	return nil
}

// ====
func (t UnixMilliTime) String() string {
	return fmt.Sprintf("%d", time.Time(t).UnixMilli())
}

func (t *UnixMilliTime) FromString(ctx context.Context, s string) aerror.AError {
	return t.fromDynamoNumber(ctx, s)
}
