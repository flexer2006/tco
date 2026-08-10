package application

import (
	"github.com/flexer2006/tco/internal/application/control"
	"github.com/flexer2006/tco/internal/application/onboarding"
	"github.com/flexer2006/tco/internal/application/pipeline"
	"github.com/flexer2006/tco/internal/ports"
)

type (
	ManifestStore         = ports.ManifestStore
	VaultProjector        = ports.VaultProjector
	ProjectionDirtyMarker = ports.ProjectionDirtyMarker
	TelegramSource        = ports.TelegramSource
	EmbeddingEncoder      = ports.EmbeddingEncoder
	ProjectionStats       = ports.ProjectionStats
	EmbeddingMetadata     = ports.EmbeddingMetadata
	MessageFetchRequest   = ports.MessageFetchRequest

	Orchestrator       = pipeline.Orchestrator
	RunOutcome         = pipeline.RunOutcome
	OrchestratorOption = pipeline.OrchestratorOption

	Onboarding        = onboarding.Onboarding
	AuthBackend       = onboarding.AuthBackend
	ServiceOption     = onboarding.ServiceOption
	ErrorKind         = onboarding.ErrorKind
	OperationError    = onboarding.OperationError
	InvalidInputError = onboarding.InvalidInputError

	Service        = control.Service
	Readiness      = control.Readiness
	Status         = control.Status
	PipelineRunner = control.PipelineRunner
)

const (
	ErrorKindAuth          = onboarding.ErrorKindAuth
	ErrorKindNetwork       = onboarding.ErrorKindNetwork
	ErrorKindRateLimit     = onboarding.ErrorKindRateLimit
	ErrorKindInvalidTarget = onboarding.ErrorKindInvalidTarget
	ErrorKindTimeout       = onboarding.ErrorKindTimeout
	ErrorKindInternal      = onboarding.ErrorKindInternal
)
