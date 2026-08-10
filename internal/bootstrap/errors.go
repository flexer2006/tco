package bootstrap

import "errors"

var (
	ErrUnsupportedRuntimeProfile = errors.New("unsupported RUNTIME_PROFILE")
	ErrUnsupportedEncoderProfile = errors.New(
		"initialize production ONNX encoder: unsupported profile",
	)
	ErrTelegramSourceMode = errors.New(
		"TELEGRAM_SOURCE_MODE must be live for production runtime",
	)
	ErrControlPlaneServerNil  = errors.New("control plane server must not be nil")
	ErrControlPlaneServiceNil = errors.New("control plane service must not be nil")
)
