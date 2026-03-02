package aerror

import (
	"context"
	"log/slog"

	apperror "github.com/yunerou/aerro/app-error"
)

var sglton *apperror.Aerro[YourErrorCode]

func init() {
	sglton = &apperror.Aerro[YourErrorCode]{
		SkipStackFrames: 4,
	}

	// Temporary fallback until SetI18nProvider is called
	sglton.BuildErrorMessage = func(ctx context.Context, code YourErrorCode, origin error, templateData map[string]any) string {
		if code == ErrOrigin {
			return origin.Error()
		}
		// Temporary fallback message
		return code.Msg()
	}

	sglton.HookAfterCreated = func(ctx context.Context, aer apperror.AppError[YourErrorCode]) {
		// Only log 5xx errors (server errors) with ERROR level
		// 4xx errors (client errors like not found) are normal business logic and shouldn't pollute logs
		if aer.ErrorCode() > start500 && aer.ErrorCode() < end500 {
			fields := []slog.Attr{
				slog.String("code", aer.ErrorCode().String()),
			}

			if aer.Origin() != nil {
				fields = append(fields, slog.Any("origin", aer.Origin().Error()))
			}
			if aer.Stacktrace() != nil {
				fields = append(fields, slog.Any("stacktrace", aer.Stacktrace()))
			}

			slog.LogAttrs(ctx, slog.LevelError, "AErrorHook", fields...)
		}
	}

	sglton.StacktraceEnabled = func(code YourErrorCode) bool {
		// Only enable stacktrace for 5xx errors
		if code > start500 && code < end500 {
			return true
		}
		return false
	}
}
