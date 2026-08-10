package pipeline

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/flexer2006/tco/internal/domain"
)

type (
	dedupClusterInput struct {
		RawMessage domain.RawCanonicalMessage
		Embedding  domain.Vector
	}
	dedupClusterResult struct {
		Notes    []dedupClusterNote
		Clusters []dedupCluster
	}
	dedupClusterNote struct {
		Additions                                                     []string
		Embedding                                                     domain.Vector
		Body, NormalizedText, ExactHash, SourceChat, Title, ClusterID string
		NoteID, ExactMasterID, SemanticMasterID, DuplicateOf          domain.NoteID
		SourceMsgID                                                   int
		IsExactMaster, IsSemanticMaster                               bool
	}
	dedupCluster struct {
		Centroid              domain.Vector
		SemanticMasterNoteIDs []domain.NoteID
		ClusterID, Name, Slug string
	}
	candidateState struct {
		embedding                                                     domain.Vector
		additions                                                     []string
		body, normalizedText, exactHash, sourceChat, title, clusterID string
		noteID                                                        domain.NoteID
		semanticMaster, exactMaster                                   *candidateState
		sourceMsgID                                                   int
	}
	clusterState struct {
		centroid              domain.Vector
		centroidRow           []float32
		sum                   []float64
		noteIDs               []domain.NoteID
		clusterID, name, slug string
		memberCount           int
		annSlot               int
	}
)

func runDedupClustering(
	inputs []dedupClusterInput,
	dedupSimilarityThreshold, clusterSimilarityThreshold float64,
) (dedupClusterResult, error) {
	err := validateThreshold(
		"dedup_similarity_threshold",
		dedupSimilarityThreshold,
	)
	if err != nil {
		return dedupClusterResult{}, err
	}

	err = validateThreshold(
		"cluster_similarity_threshold",
		clusterSimilarityThreshold,
	)
	if err != nil {
		return dedupClusterResult{}, err
	}

	if len(inputs) == 0 {
		return dedupClusterResult{Notes: []dedupClusterNote{}, Clusters: []dedupCluster{}}, nil
	}

	candidates, err := buildCandidates(inputs)
	if err != nil {
		return dedupClusterResult{}, err
	}

	exactMasters := resolveExactDedup(candidates)
	semanticMasters := resolveSemanticDedup(exactMasters, dedupSimilarityThreshold)

	propagateSemanticMasters(candidates)
	applyDeterministicAdditions(semanticMasters, exactMasters)
	clusters := assignClusters(semanticMasters, clusterSimilarityThreshold)

	propagateClusterIDs(candidates)

	return dedupClusterResult{
		Notes:    buildNotes(candidates),
		Clusters: buildClusters(clusters),
	}, nil
}

func validateThreshold(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return fmt.Errorf("%s: %w, got %v", name, ErrThresholdOutOfRange, value)
	}

	return nil
}

func buildCandidates(inputs []dedupClusterInput) ([]*candidateState, error) {
	candidates := make([]*candidateState, 0, len(inputs))
	seenIDs := make(map[string]struct{}, len(inputs))
	expectedDimension := 0

	for i, input := range inputs {
		noteID, err := domain.NewNoteID(
			input.RawMessage.SourceChat(),
			input.RawMessage.SourceMsgID(),
		)
		if err != nil {
			return nil, fmt.Errorf("inputs[%d]: %w", i, err)
		}

		if _, exists := seenIDs[noteID.String()]; exists {
			return nil, fmt.Errorf("inputs[%d]: %w %q", i, ErrDuplicateNoteID, noteID)
		}

		seenIDs[noteID.String()] = struct{}{}

		dimension := input.Embedding.Dimension()
		if dimension <= 0 {
			return nil, fmt.Errorf("inputs[%d]: %w", i, ErrEmbeddingEmpty)
		}

		if expectedDimension == 0 {
			expectedDimension = dimension
		} else if dimension != expectedDimension {
			return nil, fmt.Errorf(
				"inputs[%d]: %w: got %d expected %d",
				i,
				ErrEmbeddingDimensionMismatch,
				dimension,
				expectedDimension,
			)
		}

		normalized := domain.NormalizeText(input.RawMessage.Text())
		title := domain.DeriveTitleFromNormalizedText(normalized, input.RawMessage.SourceMsgID())
		candidates = append(candidates, new(candidateState{
			noteID:         noteID,
			sourceChat:     input.RawMessage.SourceChat(),
			sourceMsgID:    input.RawMessage.SourceMsgID(),
			normalizedText: normalized,
			exactHash:      domain.HashNormalizedText(normalized),
			title:          title,
			embedding:      input.Embedding,
		}))
	}

	slices.SortFunc(candidates, compareCandidates)

	return candidates, nil
}

func resolveExactDedup(candidates []*candidateState) []*candidateState {
	bestByHash := make(map[string]*candidateState, len(candidates))
	for _, candidate := range candidates {
		best, exists := bestByHash[candidate.exactHash]
		if !exists || compareCandidates(candidate, best) < 0 {
			bestByHash[candidate.exactHash] = candidate
		}
	}

	exactMasters := make([]*candidateState, 0, len(bestByHash))
	for _, candidate := range candidates {
		candidate.exactMaster = bestByHash[candidate.exactHash]
		if candidate.exactMaster == candidate {
			exactMasters = append(exactMasters, candidate)
		}
	}

	slices.SortFunc(exactMasters, compareCandidates)

	return exactMasters
}

func resolveSemanticDedup(
	exactMasters []*candidateState,
	dedupThreshold float64,
) []*candidateState {
	semanticMasters := make([]*candidateState, 0, len(exactMasters))
	masterRows := make([][]float32, 0, len(exactMasters))

	dim := 0
	if len(exactMasters) > 0 {
		dim = exactMasters[0].embedding.Dimension()
	}

	index := newProjectionIndex(len(exactMasters), dim)

	for _, candidate := range exactMasters {
		bestMaster := (*candidateState)(nil)
		bestSimilarity := similarityUnset
		candidateRow := candidate.embedding.ValuesUnsafe()
		refs := collectANNRefs(index, candidateRow, len(semanticMasters))

		for _, i := range refs {
			similarity := domain.DotFloat32(candidateRow, masterRows[i])
			master := semanticMasters[i]

			if bestMaster == nil || similarity > bestSimilarity ||
				(similarity == bestSimilarity && compareCandidates(master, bestMaster) < 0) {
				bestMaster = master
				bestSimilarity = similarity
			}
		}

		if bestMaster != nil && bestSimilarity >= dedupThreshold {
			candidate.semanticMaster = bestMaster

			continue
		}

		candidate.semanticMaster = candidate
		semanticMasters = append(semanticMasters, candidate)
		masterRows = append(masterRows, candidateRow)
		index.add(candidateRow)
	}

	return semanticMasters
}

func propagateSemanticMasters(candidates []*candidateState) {
	for _, candidate := range candidates {
		if candidate.semanticMaster != nil {
			continue
		}

		candidate.semanticMaster = candidate.exactMaster.semanticMaster
	}
}

func applyDeterministicAdditions(
	semanticMasters []*candidateState,
	exactMasters []*candidateState,
) {
	additionsByMaster := make(map[*candidateState]map[string]sortKey, len(semanticMasters))

	for _, candidate := range exactMasters {
		if candidate.semanticMaster == candidate {
			continue
		}

		master := candidate.semanticMaster
		if candidate.normalizedText == "" {
			continue
		}

		if additionsByMaster[master] == nil {
			additionsByMaster[master] = make(map[string]sortKey)
		}

		currentKey := candidate.sortKey()

		existingKey, exists := additionsByMaster[master][candidate.normalizedText]
		if !exists || compareSortKey(currentKey, existingKey) < 0 {
			additionsByMaster[master][candidate.normalizedText] = currentKey
		}
	}

	for _, master := range semanticMasters {
		entriesMap := additionsByMaster[master]

		entries := make([]additionEntry, 0, len(entriesMap))
		for text, key := range entriesMap {
			entries = append(entries, additionEntry{text: text, key: key})
		}

		slices.SortFunc(entries, func(left, right additionEntry) int {
			return compareSortKey(left.key, right.key)
		})

		master.additions = make([]string, 0, len(entries))
		for _, entry := range entries {
			master.additions = append(master.additions, entry.text)
		}

		master.body = renderMasterBody(master.title, master.normalizedText, master.additions)
	}
}

func assignClusters(semanticMasters []*candidateState, clusterThreshold float64) []*clusterState {
	clusters := make([]*clusterState, 0, len(semanticMasters))

	dim := 0
	if len(semanticMasters) > 0 {
		dim = semanticMasters[0].embedding.Dimension()
	}

	index := newProjectionIndex(len(semanticMasters), dim)

	for _, master := range semanticMasters {
		bestCluster := (*clusterState)(nil)
		bestSimilarity := similarityUnset
		masterRow := master.embedding.ValuesUnsafe()
		refs := collectANNRefs(index, masterRow, len(clusters))

		for _, i := range refs {
			existingCluster := clusters[i]

			similarity := domain.DotFloat32(masterRow, existingCluster.centroidRow)
			if similarity < clusterThreshold {
				continue
			}

			if bestCluster == nil || similarity > bestSimilarity ||
				(similarity == bestSimilarity && cmp.Compare(existingCluster.clusterID, bestCluster.clusterID) < 0) {
				bestCluster = existingCluster
				bestSimilarity = similarity
			}
		}

		if bestCluster == nil {
			newCluster := newClusterState(master)
			newCluster.annSlot = index.add(newCluster.centroidRow)
			clusters = append(clusters, newCluster)
			master.clusterID = newCluster.clusterID

			continue
		}

		bestCluster.add(master)
		master.clusterID = bestCluster.clusterID
		index.update(bestCluster.annSlot, bestCluster.centroidRow)
	}

	return clusters
}

func newClusterState(master *candidateState) *clusterState {
	name := master.title
	chatSlug := domain.Slugify(master.sourceChat)
	clusterID := fmt.Sprintf("cluster-%s-%d", chatSlug, master.sourceMsgID)
	slug := domain.Slugify(name) + "-" + chatSlug + "-" + strconv.Itoa(master.sourceMsgID)
	sum := make([]float64, master.embedding.Dimension())
	master.embedding.AccumulateUnsafe(sum)

	return new(clusterState{
		clusterID:   clusterID,
		name:        name,
		slug:        slug,
		centroid:    master.embedding,
		centroidRow: master.embedding.ValuesUnsafe(),
		sum:         sum,
		noteIDs:     []domain.NoteID{master.noteID},
		memberCount: 1,
		annSlot:     -1,
	})
}

func (state *clusterState) add(candidate *candidateState) {
	candidate.embedding.AccumulateUnsafe(state.sum)
	state.memberCount++
	state.centroid = normalizedMeanVector(state.sum, state.memberCount)
	state.centroidRow = state.centroid.ValuesUnsafe()
	state.noteIDs = append(state.noteIDs, candidate.noteID)
}

func propagateClusterIDs(candidates []*candidateState) {
	for _, candidate := range candidates {
		if candidate.clusterID != "" {
			continue
		}

		candidate.clusterID = candidate.semanticMaster.clusterID
	}
}

func buildNotes(candidates []*candidateState) []dedupClusterNote {
	ordered := slices.Clone(candidates)
	slices.SortFunc(ordered, compareCandidates)

	notes := make([]dedupClusterNote, 0, len(ordered))
	for _, candidate := range ordered {
		isExactMaster := candidate.exactMaster == candidate
		isSemanticMaster := candidate.semanticMaster == candidate

		note := dedupClusterNote{
			NoteID:           candidate.noteID,
			SourceChat:       candidate.sourceChat,
			SourceMsgID:      candidate.sourceMsgID,
			Title:            candidate.title,
			Embedding:        candidate.embedding,
			NormalizedText:   candidate.normalizedText,
			ExactHash:        candidate.exactHash,
			ExactMasterID:    candidate.exactMaster.noteID,
			SemanticMasterID: candidate.semanticMaster.noteID,
			ClusterID:        candidate.clusterID,
			IsExactMaster:    isExactMaster,
			IsSemanticMaster: isSemanticMaster,
			Additions:        []string{},
		}
		if isSemanticMaster {
			note.Body = candidate.body
			note.Additions = cloneStrings(candidate.additions)
		} else {
			note.DuplicateOf = candidate.semanticMaster.noteID
		}

		notes = append(notes, note)
	}

	return notes
}

func buildClusters(clusterStates []*clusterState) []dedupCluster {
	result := make([]dedupCluster, 0, len(clusterStates))
	for _, state := range clusterStates {
		result = append(result, dedupCluster{
			ClusterID:             state.clusterID,
			Name:                  state.name,
			Slug:                  state.slug,
			Centroid:              state.centroid,
			SemanticMasterNoteIDs: slices.Clone(state.noteIDs),
		})
	}

	slices.SortFunc(result, func(left, right dedupCluster) int {
		return cmp.Compare(left.ClusterID, right.ClusterID)
	})

	return result
}

const (
	similarityTileSize = 64
	similarityUnset    = -2.0
)

func normalizedMeanVector(sum []float64, memberCount int) domain.Vector {
	mean := make([]float32, len(sum))

	var sumSquares float64

	for i := range sum {
		mean[i] = float32(sum[i] / float64(memberCount))
		sumSquares += float64(mean[i]) * float64(mean[i])
	}

	norm := math.Sqrt(sumSquares)
	if norm == 0 {
		return domain.Vector{}
	}

	for i := range mean {
		mean[i] = float32(float64(mean[i]) / norm)
	}

	centroid, _ := domain.NewVectorAlreadyNormalized(mean)

	return centroid
}

type sortKey struct {
	sourceChat  string
	sourceMsgID int
}

type additionEntry struct {
	key  sortKey
	text string
}

func compareCandidates(left, right *candidateState) int {
	return cmp.Or(
		cmp.Compare(left.sourceMsgID, right.sourceMsgID),
		cmp.Compare(left.sourceChat, right.sourceChat),
	)
}

func (candidate *candidateState) sortKey() sortKey {
	return sortKey{
		sourceMsgID: candidate.sourceMsgID,
		sourceChat:  candidate.sourceChat,
	}
}

func compareSortKey(left, right sortKey) int {
	return cmp.Or(
		cmp.Compare(left.sourceMsgID, right.sourceMsgID),
		cmp.Compare(left.sourceChat, right.sourceChat),
	)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	return slices.Clone(values)
}
