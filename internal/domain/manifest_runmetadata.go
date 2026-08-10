package domain

import (
	"fmt"
	"strings"
	"time"
)

type (
	Thresholds struct {
		dedupSimilarity, clusterSimilarity float64
	}
	RunMetadata struct {
		timestamps Timestamps
		counts     Counts
		runMode    ModeRun
		batchMode  ModeBatch
		thresholds Thresholds
		runID      string
		batchSize  int
	}
)

func NewThresholds(dedupSimilarity, clusterSimilarity float64) (Thresholds, error) {
	err := validateSimilarity("dedup_similarity", dedupSimilarity)
	if err != nil {
		return Thresholds{}, err
	}

	err = validateSimilarity("cluster_similarity", clusterSimilarity)
	if err != nil {
		return Thresholds{}, err
	}

	return Thresholds{dedupSimilarity: dedupSimilarity, clusterSimilarity: clusterSimilarity}, nil
}

func (t Thresholds) DedupSimilarity() float64   { return t.dedupSimilarity }
func (t Thresholds) ClusterSimilarity() float64 { return t.clusterSimilarity }
func (t Thresholds) Clone() Thresholds          { return t }

type Counts struct {
	notes, canonicalNotes, duplicateNotes, clusters int
}

func NewCounts(notes, canonicalNotes, duplicateNotes, clusters int) (Counts, error) {
	if notes < 0 {
		return Counts{}, fmt.Errorf("%w, got %d", ErrNotesNegative, notes)
	}

	if canonicalNotes < 0 {
		return Counts{}, fmt.Errorf("%w, got %d", ErrCanonicalNotesNegative, canonicalNotes)
	}

	if duplicateNotes < 0 {
		return Counts{}, fmt.Errorf("%w, got %d", ErrDuplicateNotesNegative, duplicateNotes)
	}

	if clusters < 0 {
		return Counts{}, fmt.Errorf("%w, got %d", ErrClustersNegative, clusters)
	}

	if canonicalNotes+duplicateNotes != notes {
		return Counts{}, fmt.Errorf(
			"%w (%d + %d != %d)",
			ErrNotesCountMismatch,
			canonicalNotes,
			duplicateNotes,
			notes,
		)
	}

	return Counts{
		notes:          notes,
		canonicalNotes: canonicalNotes,
		duplicateNotes: duplicateNotes,
		clusters:       clusters,
	}, nil
}

func (c Counts) Notes() int          { return c.notes }
func (c Counts) CanonicalNotes() int { return c.canonicalNotes }
func (c Counts) DuplicateNotes() int { return c.duplicateNotes }
func (c Counts) Clusters() int       { return c.clusters }
func (c Counts) Clone() Counts       { return c }

type Timestamps struct {
	startedAtUTC, finishedAtUTC time.Time
}

func NewTimestamps(startedAtUTC, finishedAtUTC time.Time) (Timestamps, error) {
	err := validateUTC("started_at_utc", startedAtUTC)
	if err != nil {
		return Timestamps{}, err
	}

	err = validateUTC("finished_at_utc", finishedAtUTC)
	if err != nil {
		return Timestamps{}, err
	}

	if finishedAtUTC.Before(startedAtUTC) {
		return Timestamps{}, ErrFinishedBeforeStarted
	}

	return Timestamps{startedAtUTC: startedAtUTC.UTC(), finishedAtUTC: finishedAtUTC.UTC()}, nil
}

func (t Timestamps) StartedAtUTC() time.Time  { return t.startedAtUTC }
func (t Timestamps) FinishedAtUTC() time.Time { return t.finishedAtUTC }
func (t Timestamps) Clone() Timestamps        { return t }

func NewRunMetadata(
	runID string,
	runMode ModeRun,
	batchMode ModeBatch,
	batchSize int,
	thresholds Thresholds,
	counts Counts,
	timestamps Timestamps,
) (RunMetadata, error) {
	if strings.TrimSpace(runID) == "" {
		return RunMetadata{}, ErrRunIDEmpty
	}

	_, err := NewPolicy(runMode, batchMode, batchSize)
	if err != nil {
		return RunMetadata{}, err
	}

	return RunMetadata{
		runID:      runID,
		runMode:    runMode,
		batchMode:  batchMode,
		batchSize:  batchSize,
		thresholds: thresholds.Clone(),
		counts:     counts.Clone(),
		timestamps: timestamps.Clone(),
	}, nil
}

func (m RunMetadata) RunID() string          { return m.runID }
func (m RunMetadata) RunMode() ModeRun       { return m.runMode }
func (m RunMetadata) BatchMode() ModeBatch   { return m.batchMode }
func (m RunMetadata) BatchSize() int         { return m.batchSize }
func (m RunMetadata) Thresholds() Thresholds { return m.thresholds }
func (m RunMetadata) Counts() Counts         { return m.counts }
func (m RunMetadata) Timestamps() Timestamps { return m.timestamps }
func (m RunMetadata) Clone() RunMetadata {
	return RunMetadata{
		runID:      m.runID,
		runMode:    m.runMode,
		batchMode:  m.batchMode,
		batchSize:  m.batchSize,
		thresholds: m.thresholds.Clone(),
		counts:     m.counts.Clone(),
		timestamps: m.timestamps.Clone(),
	}
}
