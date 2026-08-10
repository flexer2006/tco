package config

import "errors"

var (
	ErrControlPlaneTokenRequired = errors.New(
		"CONTROL_PLANE_TOKEN: required when ALLOW_INSECURE_BIND=1",
	)
	ErrControlPlaneTokenTooShort = errors.New(
		"CONTROL_PLANE_TOKEN: must be at least 16 characters",
	)
	ErrControlPlaneTLSRequired = errors.New(
		"CONTROL_PLANE_TLS_CERT_FILE and CONTROL_PLANE_TLS_KEY_FILE: required when ALLOW_INSECURE_BIND=1",
	)
	ErrControlPlaneTLSPairIncomplete = errors.New(
		"CONTROL_PLANE_TLS_CERT_FILE and CONTROL_PLANE_TLS_KEY_FILE: both must be set together",
	)
	ErrControlPlaneTLSCertMissing = errors.New("CONTROL_PLANE_TLS_CERT_FILE: file does not exist")
	ErrControlPlaneTLSKeyMissing  = errors.New("CONTROL_PLANE_TLS_KEY_FILE: file does not exist")
	ErrRequiredEnvNotSet          = errors.New("required environment variable is not set")
	ErrInvalidEnumValue           = errors.New("invalid value")
	ErrMustBeGreaterThanZero      = errors.New("must be greater than 0")
	ErrMustBeUnitInterval         = errors.New("must be greater than 0 and at most 1")
	ErrPortOutOfRange             = errors.New("must be in range")
	ErrInvalidRuntimeProfile      = errors.New("RUNTIME_PROFILE: invalid value")
	ErrInvalidTelegramSourceMode  = errors.New("TELEGRAM_SOURCE_MODE: invalid value")
	ErrModelProfileEmpty          = errors.New("model_profile: must not be empty")
	ErrUnsupportedModelProfile    = errors.New("model_profile: unsupported value")
	ErrHostPortFormat             = errors.New("must be in host:port format")
	ErrHostEmpty                  = errors.New("host must not be empty")
	ErrInvalidPort                = errors.New("invalid port")
	ErrMustNotBeEmpty             = errors.New("must not be empty")
	ErrNonLoopbackBind            = errors.New(
		"refusing non-loopback bind without ALLOW_INSECURE_BIND=1",
	)
)
