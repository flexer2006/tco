package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	runStateIdle      runState = "idle"
	runStateRunning   runState = "running"
	runStateSucceeded runState = "succeeded"
	runStateFailed    runState = "failed"
)

var ErrRunInProgress = errors.New("pipeline run already in progress")

type (
	runState  string
	Readiness struct {
		Reason string `json:"reason,omitzero"`
		Ready  bool   `json:"ready"`
	}
	Status struct {
		State                 runState      `json:"state"`
		LastStartedAt         time.Time     `json:"last_started_at,omitzero"`
		LastFinishedAt        time.Time     `json:"last_finished_at,omitzero"`
		LastError             string        `json:"last_error,omitzero"`
		RunsStarted           int           `json:"runs_started"`
		RunsSucceeded         int           `json:"runs_succeeded"`
		RunsFailed            int           `json:"runs_failed"`
		LastDuration          time.Duration `json:"last_duration,omitzero"`
		Running               bool          `json:"running"`
		LastManifestChanged   bool          `json:"last_manifest_changed"`
		LastProjectionChanged bool          `json:"last_projection_changed"`
	}
	PipelineRunner interface {
		Run(ctx context.Context, sourceChat string) (RunOutcome, error)
	}
	Service struct {
		status                Status
		runner                PipelineRunner
		sourceChat            string
		ctx                   context.Context
		now                   func() time.Time
		cancel, runCancel     context.CancelFunc
		mu                    sync.Mutex
		runCompleted          chan struct{}
		running, shuttingDown bool
	}
)

func NewService(runner PipelineRunner, sourceChat string) (*Service, error) {
	if runner == nil {
		return nil, errors.New("pipeline runner must not be nil")
	}
	sourceChat = strings.TrimSpace(sourceChat)
	if sourceChat == "" {
		return nil, errors.New("source chat must not be empty")
	}
	serviceCtx, cancel := context.WithCancel(context.Background())
	return &Service{
		runner:     runner,
		sourceChat: sourceChat,
		now:        time.Now,
		ctx:        serviceCtx,
		cancel:     cancel,
		status: Status{
			State:   runStateIdle,
			Running: false,
		},
	}, nil
}

func (s *Service) TriggerRun(ctx context.Context) error {
	if s == nil {
		return errors.New("control plane service must not be nil")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return errors.New("control plane service is shutting down")
	}
	if s.running {
		s.mu.Unlock()
		return ErrRunInProgress
	}
	runCtx, runCancel := context.WithCancel(s.ctx)
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

func (s *Service) executeRun(ctx context.Context, runCompleted chan struct{}, startedAt time.Time) {
	defer close(runCompleted)
	outcome := RunOutcome{}
	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("pipeline run panic: %v", recovered)
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
		s.status.LastError = runErr.Error()
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

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return errors.New("control plane service must not be nil")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	s.mu.Lock()
	s.shuttingDown = true
	runCancel := s.runCancel
	runCompleted := s.runCompleted
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
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
