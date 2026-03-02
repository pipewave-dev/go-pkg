package aerror

import "net/http"

type ErrorCode int

// Origin error (Use origin message, 400)

const (
	ErrOrigin ErrorCode = iota + 1 // Origin error
)

// 400 StatusBadRequest

const (
	start400 ErrorCode = iota + 10 // start400

	ErrInvalidInput // Invalid input.

	// validate errors

	ErrManyValidation           // Your request has {{.Length}} validation errors
	ErrValidateRequired         // Param is required.
	ErrValidateLen              // The length of param must be as specified.
	ErrValidateMin              // The length of param must meet the minimum requirement.
	ErrValidateMax              // The length of param must not exceed the maximum allowed.
	ErrValidateEq               // Param must have a specific value.
	ErrValidateNe               // Param must have a value different from the specified value.
	ErrValidateLt               // Param must be less than the specified value.
	ErrValidateLte              // Param must be less than or equal to the specified value.
	ErrValidateGt               // Param must be greater than the specified value.
	ErrValidateGte              // Param must be greater than or equal to the specified value.
	ErrValidateOneOf            // Param must be one of the predefined values.
	ErrValidateIndividualNumber // Invalid format of individual number.
	ErrValidateImageBase64      // Invalid format of image base64.
	ErrValidatePassword         // Invalid password. Must be at least 8 characters with letters (A-Z, a-z), numbers (0-9), and special characters.
	ErrValidateUsername         // Invalid username. It must be at least 5 characters long, with the first letter being an alphabet character, and the remaining characters can be letters (A-Z, a-z), numbers (0-9), underscore (_) and dot (.).
	ErrValidateEmail            // Invalid email format.
	ErrValidatePhone            // Invalid phone number format.
	ErrValidateUUID             // Invalid UUID format

	ErrInvalidVerifyCode              // Invalid verify code.
	ErrProvideInvalidIdentityType     // Invalid identity type. Must be email or phone number.
	ErrProvideNotExistedIdentity      // Email or phone number does not exist.
	ErrProvideNotYetConfirmedIdentity // Email or phone number has not been confirmed.
	ErrProvideExistedEmail            // Email has already been registered.
	ErrProvideConfirmedEmail          // Email has been confirmed.
	ErrProvideExpiredConfirmCode      // Confirm code has been expired.
	// Oauth2 incorrect params

	LogicErrOauth2ClientIDNotMatch          // Oauth2 authorization code is not found.
	LogicErrOauth2RedirectHostNotAllow      // Oauth2 redirect uri is not allowed.
	LogicErrOauth2AuthorizationCodeNotFound // Oauth2 authorization code is not found.
	LogicErrOauth2AuthorizationCodeExpired  // Oauth2 authorization code is expired.

	LogicErrVerifyConfirmCodFail   // Verify confirm code fail
	NewPasswordSameWithOldPassword // New password cannot be the same as the old password
	ExceedUserInvitationLimit      // Exceed user invitation limit for this user, must waiting before resend invitation.

	SessionHasBeenRevokedBefore // Session has been revoked before.

	ErrEmailAlreadyConfirmed // Email has been confirmed.
	ErrPhoneAlreadyConfirmed // Phone has been confirmed.
	ErrEmailAlreadyExisted   // Email has already been registered.
	ErrPhoneAlreadyExisted   // Phone has already been registered.

	end400 // end400
)

// 401 StatusUnauthorized

const (
	start401 ErrorCode = iota + 2000 // start400

	SessionExceededMaxAge    // Session exceeded max age please login again.
	LogicErrSessionExpired   // Session has been expired.
	LogicErrUserNeedVerified // This user need verifying.
	InvitationTokenInvalid   // Invitation token token is invalid.

	ConfirmationTokenInvalid         // Confirmation token is invalid.
	ResetPasswordTokenInvalid        // Reset password token is invalid.
	RefreshTokenInvalid              // Refresh token is invalid.
	LogicErrMissingAuthHeader        // Missing Authorization header.
	LogicErrAccessTokenVerifyFail    // Can not verify AccessToken signature or token is expired.
	LogicErrSessionHasBeenRevoked    // Session has been revoked.
	LogicErrWrongPassword            // Wrong password.
	LogicErrAlreadyExistedPassword   // Password already exists.
	LogicErrEmailOrPasswordIncorrect // Email or password is incorrect

	ErrAccessTokenFormat         // Access token's format wrong.
	ErrConfirmationTokenFormat   // Confirmation token's format wrong.
	ErrInvitationTokenFormat     // Invitation token's format wrong.
	ErrSignupConfirmTokenInvalid // Signup confirm token is invalid.

	ErrEmailUserAlreadyExists // User already exists with this email.

	end401 // end401
)

// 403 StatusForbidden

const (
	start403 ErrorCode = iota + 4000 // start400

	LogicErrRefreshTokenIsExpired          // Refresh token is expired.
	LogicErrSessionHasBeenRevokedOrExpired // Session  has been revoked or expired.
	LogicErrSessionIsNotExisted            // Session is not existed.
	LogicErrInvalidRefreshToken            // Invalid refresh token.
	LogicErrInvalidToken                   // Invalid token.
	LogicRefreshTokenMustUseOnlyOnce       // Refresh token must be used only once.
	RateLimitExceeded                      // Rate limit exceeded.
	InvalidInputSchema                     // Invalid input schema

	end403 // end403
)

// 404 StatusNotFound

const (
	start404             ErrorCode = iota + 3000 // start400
	UserNotFound                                 // User not found.
	YourAccountNotFound                          // Your account not found.
	RecordNotFound                               // Resource not found.
	ErrDdbDuplicatedItem                         // Duplicated id.

	ErrRecordNotFound // Record not found

	end404 // end404
)

// 500 StatusInternalServerError

const (
	start500 ErrorCode = iota + 6000 // start500

	ErrUnmarshal // Unexpected error.
	ErrMarshal   // Unexpected error.

	ErrUnexpectedInput // Unexpected error.

	ErrUnexpectedConfig // Unexpected config. Check value in config file or flags' value in cli

	ErrUnexpectedTemplate          // Unexpected error.
	ErrUnexpectedSES               // Unexpected error.
	ErrUnexpectedRedis             // Unexpected error.
	ErrUnexpectedDatabase          // Unexpected error.
	ErrUnexpectedDynamoDB          // Unexpected error.
	ErrUnexpectedBussiness         // Unexpected error.
	ErrUnexpectedNetwork           // Unexpected error.
	ErrUnexpectedSyscall           // Unexpected error.
	ErrUnexpectedCodeLogic         // Unexpected error.
	ErrUnexpectedShouldUnreachable // Unexpected error.
	ErrUnexpectedWithMsg           // Unexpected error. {{.Msg}}

	LogicErrDuplicateRecord // Username, email or phone has already been register.

	LogicErrOauth2ClientSecretNotMatch     // Oauth2 client secret is incorrect.
	LogicErrOauth2PkceCodeVerifierNotMatch // Oauth2 PKCE code verifier is not match.

	//

	ErrUnexpectedPubsub // Can not send request to pubsub

	ErrHandlerNotFoundForCommandName // Can not found handler for command name {{.Name}}

	ErrServerIsShuttingDown // Server is shutting down.

	end500 // end500
)

const (
	start500NoTrace ErrorCode = iota + 8000 // start500NoTrace

	ErrPermissionDenied // Permission denied.
	ActionNotAllowed    // Action not allowed.

	end500NoTrace // end500NoTrace
)

// Error .. will return error messageID, need to use i18n package to get error message.
func (e ErrorCode) Error() string {
	return e.String()
}

func (e ErrorCode) Code() string {
	return e.String()
}

func (e ErrorCode) HttpCode() int {
	if e > start400 && e < end400 {
		return http.StatusBadRequest
	}
	if e > start401 && e < end401 {
		return http.StatusUnauthorized
	}
	if e > start403 && e < end403 {
		return http.StatusForbidden
	}
	if e > start404 && e < end404 {
		return http.StatusNotFound
	}
	if e > start500NoTrace && e < end500NoTrace {
		return http.StatusInternalServerError
	}
	if e > start500 && e < end500 {
		return http.StatusInternalServerError
	}
	return http.StatusHTTPVersionNotSupported // unreachable
}
