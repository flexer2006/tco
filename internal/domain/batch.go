package domain

import "fmt"

const (
	Incremental ModeRun   = "incremental"
	FullRebuild ModeRun   = "full_rebuild"
	Streaming   ModeBatch = "streaming"
	PostScan    ModeBatch = "post_scan"
)

type (
	ModeRun   string
	ModeBatch string
	Policy    struct {
		runMode   ModeRun
		batchMode ModeBatch
		batchSize int
	}
)

func NewPolicy(runMode ModeRun, batchMode ModeBatch, batchSize int) (Policy, error) {
	if !isValidRunMode(runMode) {
		return Policy{}, fmt.Errorf("%w %q", ErrInvalidRunMode, runMode)
	}

	if !isValidBatchMode(batchMode) {
		return Policy{}, fmt.Errorf("%w %q", ErrInvalidBatchMode, batchMode)
	}

	if batchSize <= 0 {
		return Policy{}, fmt.Errorf("%w, got %d", ErrBatchSizeNotPositive, batchSize)
	}

	return Policy{
		runMode:   runMode,
		batchMode: batchMode,
		batchSize: batchSize,
	}, nil
}

func (p Policy) RunMode() ModeRun     { return p.runMode }
func (p Policy) BatchMode() ModeBatch { return p.batchMode }
func (p Policy) BatchSize() int       { return p.batchSize }

func isValidRunMode(mode ModeRun) bool {
	switch mode {
	case Incremental, FullRebuild:
		return true
	default:
		return false
	}
}

func isValidBatchMode(mode ModeBatch) bool {
	switch mode {
	case Streaming, PostScan:
		return true
	default:
		return false
	}
}
