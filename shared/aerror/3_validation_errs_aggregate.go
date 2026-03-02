package aerror

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pipewave-dev/go-pkg/pkg/validation"
	"github.com/samber/lo"
)

func ValidationErrsAggregate(
	ctx context.Context,
	validationErrs []validation.ValidationErr,
) ADetailError {
	var validationD ValidationDetail = make(ValidationDetail, len(validationErrs))
	for i, err := range validationErrs {
		validationD[i] = ValidationDetailInside{
			Key:      err.Field,
			ValueStr: fmt.Sprintf("%v", err.Value),
			Message:  err.Message,
		}
	}

	return NewWithDetail(ctx, ErrManyValidation, nil, validationD, map[string]any{
		"Length": strconv.Itoa(len(validationErrs)),
		"DetailMsg": lo.Reduce(
			validationErrs,
			func(msg string, e validation.ValidationErr, idx int) string {
				return msg + fmt.Sprintf("※%d: %s: %s\n", idx, e.Field, e.Error())
			},
			fmt.Sprintf("\n"),
		),
	})
}
