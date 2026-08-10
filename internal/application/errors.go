package application

import (
	"github.com/flexer2006/tco/internal/application/control"
	"github.com/flexer2006/tco/internal/application/onboarding"
)

var (
	ErrInvalidInput      = onboarding.ErrInvalidInput
	ErrInvalidTransition = onboarding.ErrInvalidTransition
	ErrRunInProgress     = control.ErrRunInProgress
)
