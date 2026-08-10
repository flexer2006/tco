package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/flexer2006/tco/internal/application/pipeline"
)

const (
	runStateIdle      runState = "idle"
	runStateRunning   runState = "running"
	runStateSucceeded runState = "succeeded"
	runStateFailed    runState = "failed"
)

var (
	ErrRunInProgress          = errors.New("pipeline run already in progress")
	ErrPipelineRunnerNil      = errors.New("pipeline runner must not be nil")
	ErrControlPlaneServiceNil = errors.New("control plane service must not be nil")
	ErrServiceShuttingDown    = errors.New("control plane service is shutting down")
	ErrPipelineRunPanic       = errors.New("pipeline run panic")
)

type (
	runState  string
	Readiness struct {
		Reason string `json:"reason,omitzero"`
		Ready  bool   `json:"ready"`
	}
	Status struct {
		LastStartedAt         time.Time     `json:"last_started_at,omitzero"`
		LastFinishedAt        time.Time     `json:"last_finished_at,omitzero"`
		State                 runState      `json:"state"`
		LastError             string        `json:"last_error,omitzero"`
		LastDuration          time.Duration `json:"last_duration,omitzero"`
		RunsStarted           int           `json:"runs_started"`
		RunsSucceeded         int           `json:"runs_succeeded"`
		RunsFailed            int           `json:"runs_failed"`
		Running               bool          `json:"running"`
		LastManifestChanged   bool          `json:"last_manifest_changed"`
		LastProjectionChanged bool          `json:"last_projection_changed"`
	}
	PipelineRunner interface {
		Run(ctx context.Context, sourceChat string) (pipeline.RunOutcome, error)
	}
	Service struct {
		status                Status
		runner                PipelineRunner
		sourceChat            string
		now                   func() time.Time
		runCancel             context.CancelFunc
		mu                    sync.Mutex
		runCompleted          chan struct{}
		running, shuttingDown bool
	}
)

func NewService(runner PipelineRunner, sourceChat string) (*Service, error) {
	if runner == nil {
		return nil, ErrPipelineRunnerNil
	}

	sourceChat = strings.TrimSpace(sourceChat)
	if sourceChat == "" {
		return nil, pipeline.ErrSourceChatEmpty
	}

	return new(Service{
		runner:     runner,
		sourceChat: sourceChat,
		now:        time.Now,
		status: Status{
			State:   runStateIdle,
			Running: false,
		},
	}), nil
}

func (s *Service) TriggerRun(ctx context.Context) error {
	if s == nil {
		return ErrControlPlaneServiceNil
	}

	if ctx == nil {
		return pipeline.ErrContextNil
	}

	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()

		return ErrServiceShuttingDown
	}

	if s.running {
		s.mu.Unlock()

		return ErrRunInProgress
	}

	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	runCompleted := make(chan struct{})
	startedAt := s.now().UTC()
	s.running = true
	s.runCancel = runCancel
	s.runCompleted = runCompleted
	s.status.State = runStateRunning
	s.status.Running = true
	s.status.RunsStarted++
	s.status.LastStartedAt = startedAt
	s.status.LastError = ""

	s.mu.Unlock()
	go s.executeRun(runCtx, runCompleted, startedAt)

	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return ErrControlPlaneServiceNil
	}

	if ctx == nil {
		return pipeline.ErrContextNil
	}

	s.mu.Lock()
	s.shuttingDown = true
	runCancel := s.runCancel
	runCompleted := s.runCompleted
	s.mu.Unlock()

	if runCancel != nil {
		runCancel()
	}

	if runCompleted == nil {
		return nil
	}

	select {
	case <-runCompleted:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for active pipeline run shutdown: %w", ctx.Err())
	}
}

func (s *Service) Status() Status {
	if s == nil {
		return Status{State: runStateFailed, LastError: "control plane service is nil"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	status := s.status
	status.Running = s.running

	return status
}

func (s *Service) Readiness() Readiness {
	if s == nil {
		return Readiness{Ready: false, Reason: "control plane service is nil"}
	}

	if s.runner == nil {
		return Readiness{Ready: false, Reason: "pipeline runner is not configured"}
	}

	if strings.TrimSpace(s.sourceChat) == "" {
		return Readiness{Ready: false, Reason: "source chat is not configured"}
	}

	return Readiness{Ready: true}
}

func (s *Service) executeRun(ctx context.Context, runCompleted chan struct{}, startedAt time.Time) {
	defer close(runCompleted)

	outcome := pipeline.RunOutcome{}

	var runErr error

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("%w: %v", ErrPipelineRunPanic, recovered)
			}
		}()

		outcome, runErr = s.runner.Run(ctx, s.sourceChat)
	}()

	finishedAt := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false
	s.runCancel = nil
	s.runCompleted = nil
	s.status.Running = false
	s.status.LastFinishedAt = finishedAt

	s.status.LastDuration = finishedAt.Sub(startedAt)
	if runErr != nil {
		s.status.State = runStateFailed
		s.status.RunsFailed++
		s.status.LastError = publicPipelineError(runErr)
		s.status.LastManifestChanged = false
		s.status.LastProjectionChanged = false

		return
	}

	s.status.State = runStateSucceeded
	s.status.RunsSucceeded++
	s.status.LastError = ""
	s.status.LastManifestChanged = outcome.ManifestChanged
	s.status.LastProjectionChanged = outcome.ProjectionChanged
}

func publicPipelineError(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, ErrRunInProgress):
		return ErrRunInProgress.Error()
	case errors.Is(err, context.Canceled):
		return "pipeline canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "pipeline timed out"
	default:
		return "pipeline failed"
	}
}
