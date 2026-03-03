package validation

import "context"

type Validator interface {
	Validate(ctx context.Context) []ValidationErr
}
