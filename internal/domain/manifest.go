package domain

import (
	"cmp"
	"slices"
	"time"
)

const (
	SchemaVersion            = 1
	DefaultNormalizationRule = NormalizationRuleL2Unit
)

type Manifest struct {
	runMetadata                                         RunMetadata
	lastRunUTC                                          time.Time
	notes                                               []NoteRecord
	clusters                                            []ClusterRecord
	modelID, modelHash, modelProfile, normalizationRule string
	schemaVersion, vectorDimension                      int
}

func NewManifestWithModelProfile(schemaVersion int, modelID, modelHash, modelProfile string, vectorDimension int, normalizationRule string, lastRunUTC time.Time, notes []NoteRecord, clusters []ClusterRecord, runMetadata RunMetadata) (Manifest, error) {
	canonicalNotes := canonicalizeNoteRecords(notes)
	canonicalClusters := canonicalizeClusterRecords(clusters)
	manifestValue := Manifest{
		schemaVersion:     schemaVersion,
		modelID:           modelID,
		modelHash:         modelHash,
		modelProfile:      modelProfile,
		normalizationRule: normalizationRule,
		vectorDimension:   vectorDimension,
		lastRunUTC:        lastRunUTC.UTC(),
		notes:             canonicalNotes,
		clusters:          canonicalClusters,
		runMetadata:       runMetadata.Clone(),
	}
	if err := Validate(manifestValue); err != nil {
		return Manifest{}, err
	}
	return manifestValue, nil
}

func (m Manifest) SchemaVersion() int   { return m.schemaVersion }
func (m Manifest) ModelID() string      { return m.modelID }
func (m Manifest) ModelHash() string    { return m.modelHash }
func (m Manifest) ModelProfile() string { return m.modelProfile }
func (m Manifest) NormalizationRule() string {
	return m.normalizationRule
}
func (m Manifest) VectorDimension() int {
	return m.vectorDimension
}
func (m Manifest) LastRunUTC() time.Time { return m.lastRunUTC }
func (m Manifest) Notes() []NoteRecord {
	if len(m.notes) == 0 {
		return []NoteRecord{}
	}
	return slices.Clone(m.notes)
}
func (m Manifest) Clusters() []ClusterRecord {
	if len(m.clusters) == 0 {
		return []ClusterRecord{}
	}
	return slices.Clone(m.clusters)
}
func (m Manifest) RunMetadata() RunMetadata { return m.runMetadata }

func (m Manifest) Clone() Manifest {
	return Manifest{
		schemaVersion:     m.schemaVersion,
		modelID:           m.modelID,
		modelHash:         m.modelHash,
		modelProfile:      m.modelProfile,
		normalizationRule: m.normalizationRule,
		vectorDimension:   m.vectorDimension,
		lastRunUTC:        m.lastRunUTC,
		notes:             cloneNoteRecords(m.notes),
		clusters:          cloneClusterRecords(m.clusters),
		runMetadata:       m.runMetadata.Clone(),
	}
}

func canonicalizeNoteRecords(records []NoteRecord) []NoteRecord {
	if len(records) == 0 {
		return []NoteRecord{}
	}
	cloned := cloneNoteRecords(records)
	slices.SortFunc(cloned, func(a, b NoteRecord) int {
		return cmp.Compare(a.ID().String(), b.ID().String())
	})
	return cloned
}

func canonicalizeClusterRecords(records []ClusterRecord) []ClusterRecord {
	if len(records) == 0 {
		return []ClusterRecord{}
	}
	cloned := cloneClusterRecords(records)
	slices.SortFunc(cloned, func(a, b ClusterRecord) int {
		return cmp.Compare(a.ID(), b.ID())
	})
	return cloned
}

func cloneNoteRecords(records []NoteRecord) []NoteRecord {
	if len(records) == 0 {
		return []NoteRecord{}
	}
	cloned := make([]NoteRecord, len(records))
	for i, record := range records {
		cloned[i] = record.Clone()
	}
	return cloned
}

func cloneClusterRecords(records []ClusterRecord) []ClusterRecord {
	if len(records) == 0 {
		return []ClusterRecord{}
	}
	cloned := make([]ClusterRecord, len(records))
	for i, record := range records {
		cloned[i] = record.Clone()
	}
	return cloned
}
