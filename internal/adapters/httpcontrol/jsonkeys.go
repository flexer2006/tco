package httpcontrol

const (
	jsonKeyStatus   = "status"
	jsonKeyError    = "error"
	jsonKeyReason   = "reason"
	jsonKeyPipeline = "pipeline"

	statusOK           = "ok"
	statusReady        = "ready"
	statusNotReady     = "not_ready"
	statusError        = "error"
	statusRateLimited  = "rate_limited"
	statusForbidden    = "forbidden"
	statusAccepted     = "accepted"
	statusConflict     = "conflict"
	statusUnauthorized = "unauthorized"
	statusInvalidInput = "invalid_input"

	errTooManyAuthAttempts = "too many auth attempts"
)
