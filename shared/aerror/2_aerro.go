package aerror

import (
	"context"

	apperror "github.com/yunerou/aerro/app-error"
)

// This is your own type. Modify it to your own purpose
// alias type
type YourErrorCode = ErrorCode

// This alias type is used to restrict the type of detail
type YourDetailInside = DetailAllowType // Or set `type YourDetailInside = any` to not restrict the type

// ==========================================================================

// Below code only using alias type, you can copy and paste to your code
// COPY FROM HERE =========================

type AError = apperror.AppError[YourErrorCode]

type aError struct {
	AError
}

type ASingleError = *aError

type aDetailError struct {
	apperror.DetailAppError[YourErrorCode]
}
type ADetailError = *aDetailError

type aMultiError struct {
	apperror.MultiAppError[YourErrorCode]
}
type AMultiError = *aMultiError

func (me AMultiError) Errors() []ASingleError {
	appErrs := me.MultiAppError.Errors()
	insider := make([]ASingleError, 0, len(appErrs))
	for _, appErr := range appErrs {
		insider = append(insider, &aError{appErr})
	}
	return insider
}

func New(
	ctx context.Context,
	code YourErrorCode,
	origin error,
	templateData ...map[string]any,
) ASingleError {
	return &aError{
		sglton.New(ctx, code, origin, templateData...),
	}
}

func Append(mErr AMultiError, appErrs ...AError) AMultiError {
	insider := make([]AError, 0, len(appErrs))
	insider = append(insider, appErrs...)

	if mErr == nil {
		mErr = &aMultiError{sglton.Append(nil, insider...)}
	}
	return &aMultiError{sglton.Append(mErr.MultiAppError, insider...)}
}

func NewWithDetail[DeT YourDetailInside](
	ctx context.Context,
	code YourErrorCode,
	origin error,
	detail DeT,
	templateData ...map[string]any,
) ADetailError {
	return &aDetailError{
		sglton.NewWithDetail(ctx, code, origin, detail, templateData...),
	}
}

// TO HERE =========================

// =========================================================================

// You can override any function for your own purpose
// If you want to override json marshal, you can reference this code

/*
func (ad ADetailError) MarshalJSON() ([]byte, error) {
	// only return detail json
	return json.Marshal(ad.Detail())
}
*/

// IsErrRecordNotFound checks if the error is a record not found error
func IsErrRecordNotFound(e AError) bool {
	if e == nil {
		return false
	}
	return e.ErrorCode() == RecordNotFound
}
