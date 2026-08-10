package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

const (
	defaultDedupThreshold   = 0.95
	defaultClusterThreshold = 0.80
)

type (
	Orchestrator struct {
		Policy                           domain.Policy
		Source                           ports.TelegramSource
		Encoder                          ports.EmbeddingEncoder
		ManifestStore                    ports.ManifestStore
		VaultProjector                   ports.VaultProjector
		ProjectionDirty                  ports.ProjectionDirtyMarker
		DedupThreshold, ClusterThreshold float64
		HistoryMaxMessages               int
	}
	RunOutcome struct {
		Manifest                           domain.Manifest
		ProjectionStats                    ports.ProjectionStats
		ManifestChanged, ProjectionChanged bool
	}
	OrchestratorOption func(*Orchestrator)
)

func WithThresholds(dedupThreshold, clusterThreshold float64) OrchestratorOption {
	return func(orchestrator *Orchestrator) {
		orchestrator.DedupThreshold = dedupThreshold
		orchestrator.ClusterThreshold = clusterThreshold
	}
}

func WithProjectionDirtyMarker(marker ports.ProjectionDirtyMarker) OrchestratorOption {
	return func(orchestrator *Orchestrator) {
		orchestrator.ProjectionDirty = marker
	}
}

func WithHistoryMaxMessages(maxMessages int) OrchestratorOption {
	return func(orchestrator *Orchestrator) {
		orchestrator.HistoryMaxMessages = maxMessages
	}
}

func NewOrchestrator(
	source ports.TelegramSource,
	encoder ports.EmbeddingEncoder,
	manifestStore ports.ManifestStore,
	vaultProjector ports.VaultProjector,
	policy domain.Policy,
	options ...OrchestratorOption,
) (*Orchestrator, error) {
	if source == nil {
		return nil, ErrSourceNil
	}

	if encoder == nil {
		return nil, ErrEncoderNil
	}

	if manifestStore == nil {
		return nil, ErrManifestStoreNil
	}

	if vaultProjector == nil {
		return nil, ErrVaultProjectorNil
	}

	_, err := domain.NewPolicy(
		policy.RunMode(),
		policy.BatchMode(),
		policy.BatchSize(),
	)
	if err != nil {
		return nil, err
	}

	orchestrator := new(Orchestrator{
		Source:           source,
		Encoder:          encoder,
		ManifestStore:    manifestStore,
		VaultProjector:   vaultProjector,
		Policy:           policy,
		DedupThreshold:   defaultDedupThreshold,
		ClusterThreshold: defaultClusterThreshold,
	})

	for _, option := range options {
		if option == nil {
			continue
		}

		option(orchestrator)
	}

	return orchestrator, nil
}

func (o *Orchestrator) Run(ctx context.Context, sourceChat string) (RunOutcome, error) {
	if o == nil {
		return RunOutcome{}, ErrOrchestratorNil
	}

	if ctx == nil {
		return RunOutcome{}, ErrContextNil
	}

	if strings.TrimSpace(sourceChat) == "" {
		return RunOutcome{}, ErrSourceChatEmpty
	}

	err := ctx.Err()
	if err != nil {
		return RunOutcome{}, err
	}

	previousManifest, err := o.loadPreviousManifest()
	if err != nil {
		return RunOutcome{}, err
	}

	meta := o.Encoder.Metadata()

	err = ensureManifestMetadataCompatibility(
		previousManifest,
		meta,
		o.Policy.RunMode(),
	)
	if err != nil {
		return RunOutcome{}, err
	}

	fetched, err := o.fetchMessages(ctx, sourceChat, previousManifest)
	if err != nil {
		return RunOutcome{}, err
	}

	inputs, messages, err := o.buildClusterInputs(ctx, previousManifest, fetched)
	if err != nil {
		return RunOutcome{}, err
	}

	if len(inputs) == 0 {
		return o.runWithNoNewInputs(previousManifest)
	}

	dedupResult, err := runDedupClustering(inputs, o.DedupThreshold, o.ClusterThreshold)
	if err != nil {
		return RunOutcome{}, err
	}

	manifestValue, err := o.buildManifest(sourceChat, messages, dedupResult, meta)
	if err != nil {
		return RunOutcome{}, err
	}

	return o.persistAndProject(previousManifest, manifestValue)
}

func (o *Orchestrator) loadPreviousManifest() (domain.Manifest, error) {
	previousManifest, loadErr := o.ManifestStore.Load()
	if loadErr != nil {
		if !errors.Is(loadErr, os.ErrNotExist) {
			return domain.Manifest{}, fmt.Errorf("load existing Manifest: %w", loadErr)
		}

		return domain.Manifest{}, nil
	}

	return previousManifest, nil
}

func (o *Orchestrator) fetchMessages(
	ctx context.Context,
	sourceChat string,
	previousManifest domain.Manifest,
) ([]domain.RawCanonicalMessage, error) {
	fetchReq := ports.MessageFetchRequest{
		SourceChat:            sourceChat,
		MaxMessages:           o.HistoryMaxMessages,
		MinExclusiveMessageID: 0,
	}
	if o.Policy.RunMode() == domain.Incremental && previousManifest.SchemaVersion() != 0 {
		fetchReq.MinExclusiveMessageID = maxNoteSourceMsgID(previousManifest)
	}

	return o.Source.FetchMessages(ctx, fetchReq)
}

func (o *Orchestrator) persistAndProject(
	previousManifest, manifestValue domain.Manifest,
) (RunOutcome, error) {
	changed := manifestFingerprint(previousManifest) != manifestFingerprint(manifestValue)

	dirty, dirtyErr := o.projectionDirty()
	if dirtyErr != nil {
		return RunOutcome{}, dirtyErr
	}

	manifestChanged := false

	if changed {
		markErr := o.markProjectionDirty()
		if markErr != nil {
			return RunOutcome{}, markErr
		}

		dirty = true

		saved, saveErr := o.ManifestStore.Save(manifestValue)
		if saveErr != nil {
			return RunOutcome{}, saveErr
		}

		manifestChanged = saved
	}

	projectionStats := ports.ProjectionStats{}
	projectionChanged := false

	if manifestChanged || dirty {
		stats, err := o.projectAndClearDirty(manifestValue)
		if err != nil {
			return RunOutcome{}, err
		}

		projectionStats = stats
		projectionChanged = true
	}

	return RunOutcome{
		Manifest:          manifestValue,
		ManifestChanged:   manifestChanged,
		ProjectionChanged: projectionChanged,
		ProjectionStats:   projectionStats,
	}, nil
}

func (o *Orchestrator) runWithNoNewInputs(previousManifest domain.Manifest) (RunOutcome, error) {
	dirty, dirtyErr := o.projectionDirty()
	if dirtyErr != nil {
		return RunOutcome{}, dirtyErr
	}

	if previousManifest.SchemaVersion() == 0 {
		return RunOutcome{}, nil
	}

	if !dirty {
		return RunOutcome{Manifest: previousManifest}, nil
	}

	stats, projectErr := o.projectAndClearDirty(previousManifest)
	if projectErr != nil {
		return RunOutcome{}, projectErr
	}

	return RunOutcome{
		Manifest:          previousManifest,
		ProjectionChanged: true,
		ProjectionStats:   stats,
	}, nil
}

func (o *Orchestrator) buildClusterInputs(
	ctx context.Context,
	previous domain.Manifest,
	fetched []domain.RawCanonicalMessage,
) ([]dedupClusterInput, []domain.RawCanonicalMessage, error) {
	if o.Policy.RunMode() != domain.Incremental || previous.SchemaVersion() == 0 {
		inputs, err := o.encode(ctx, fetched)

		return inputs, fetched, err
	}

	fetchedByID := make(map[string]domain.RawCanonicalMessage, len(fetched))
	for _, message := range fetched {
		noteID, err := domain.NewNoteID(message.SourceChat(), message.SourceMsgID())
		if err != nil {
			return nil, nil, err
		}

		fetchedByID[noteID.String()] = message
	}

	existing := previous.Notes()

	existingIDs := make(map[string]struct{}, len(existing))
	for _, note := range existing {
		existingIDs[note.ID().String()] = struct{}{}
	}

	newMessages := make([]domain.RawCanonicalMessage, 0, len(fetched))
	for _, message := range fetched {
		noteID, err := domain.NewNoteID(message.SourceChat(), message.SourceMsgID())
		if err != nil {
			return nil, nil, err
		}

		if _, ok := existingIDs[noteID.String()]; ok {
			continue
		}

		newMessages = append(newMessages, message)
	}

	if len(newMessages) == 0 {
		return nil, nil, nil
	}

	newInputs, err := o.encode(ctx, newMessages)
	if err != nil {
		return nil, nil, err
	}

	previousInputs, previousMessages, err := inputsFromPreviousManifest(previous, fetchedByID)
	if err != nil {
		return nil, nil, err
	}

	previousInputs = append(previousInputs, newInputs...)
	previousMessages = append(previousMessages, newMessages...)

	return previousInputs, previousMessages, nil
}

func inputsFromPreviousManifest(
	manifest domain.Manifest,
	fetchedByID map[string]domain.RawCanonicalMessage,
) ([]dedupClusterInput, []domain.RawCanonicalMessage, error) {
	notes := manifest.Notes()
	inputs := make([]dedupClusterInput, 0, len(notes))

	messages := make([]domain.RawCanonicalMessage, 0, len(notes))
	for _, note := range notes {
		raw, ok := fetchedByID[note.ID().String()]
		if !ok {
			text := strings.TrimSpace(note.Body())
			if text == "" {
				text = strings.TrimSpace(note.Title())
			}

			if text == "" {
				text = note.ID().String()
			}

			var err error

			raw, err = domain.NewRawCanonicalMessage(
				note.SourceChat(),
				note.SourceMsgID(),
				note.CreatedAt(),
				text,
				nil,
				nil,
				nil,
				false,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("rebuild message for %s: %w", note.ID(), err)
			}
		}

		messages = append(messages, raw)
		inputs = append(inputs, dedupClusterInput{RawMessage: raw, Embedding: note.Embedding()})
	}

	return inputs, messages, nil
}

func maxNoteSourceMsgID(manifest domain.Manifest) int {
	maxID := 0
	for _, note := range manifest.Notes() {
		if note.SourceMsgID() > maxID {
			maxID = note.SourceMsgID()
		}
	}

	return maxID
}

func manifestFingerprint(manifest domain.Manifest) string {
	if manifest.SchemaVersion() == 0 {
		return ""
	}

	hasher := sha256.New()
	_, _ = hasher.Write([]byte(strconv.Itoa(manifest.SchemaVersion())))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(manifest.ModelID()))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(manifest.ModelHash()))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(manifest.ModelProfile()))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(strconv.Itoa(manifest.VectorDimension())))

	_, _ = hasher.Write([]byte{0})
	for _, note := range manifest.Notes() {
		_, _ = hasher.Write([]byte(note.ID().String()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(note.Hash()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(note.ClusterID()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(note.Title()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(note.Body()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(note.DuplicateOf().String()))
		_, _ = hasher.Write([]byte{0})
	}

	for _, cluster := range manifest.Clusters() {
		_, _ = hasher.Write([]byte(cluster.ID()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(cluster.Slug()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(cluster.Name()))

		_, _ = hasher.Write([]byte{0})
		for _, noteID := range cluster.NoteIDs() {
			_, _ = hasher.Write([]byte(noteID.String()))
			_, _ = hasher.Write([]byte{0})
		}
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

func (o *Orchestrator) projectionDirty() (bool, error) {
	if o.ProjectionDirty == nil {
		return false, nil
	}

	return o.ProjectionDirty.IsDirty()
}

func (o *Orchestrator) markProjectionDirty() error {
	if o.ProjectionDirty == nil {
		return nil
	}

	return o.ProjectionDirty.MarkDirty()
}

func (o *Orchestrator) projectAndClearDirty(
	manifestValue domain.Manifest,
) (ports.ProjectionStats, error) {
	stats, err := o.VaultProjector.Project(manifestValue)
	if err != nil {
		_ = o.markProjectionDirty()

		return ports.ProjectionStats{}, err
	}

	if o.ProjectionDirty != nil {
		clearErr := o.ProjectionDirty.ClearDirty()
		if clearErr != nil {
			return ports.ProjectionStats{}, clearErr
		}
	}

	return stats, nil
}
