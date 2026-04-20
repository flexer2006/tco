package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type (
	NoteRecord struct {
		id                                                    NoteID
		createdAt, updatedAt                                  time.Time
		tags                                                  []string
		embedding                                             Vector
		sourceChat, title, body, hash, embeddingID, clusterID string
		duplicateOf                                           NoteID
		sourceMsgID                                           int
	}
	ClusterRecord struct {
		centroid             Vector
		noteIDs              []NoteID
		createdAt, updatedAt time.Time
		id, name, slug       string
	}
)

func NewNoteRecord(id NoteID, sourceChat string, sourceMsgID int, title, body, embeddingID string, embeddingValue Vector, clusterID string, tags []string, createdAt, updatedAt time.Time, duplicateOf NoteID) (NoteRecord, error) {
	validated, err := newNote(id, sourceChat, sourceMsgID, title, body, embeddingID, clusterID, tags, createdAt, updatedAt, duplicateOf)
	if err != nil {
		return NoteRecord{}, err
	}
	if embeddingValue.Dimension() == 0 {
		return NoteRecord{}, errors.New("embedding must not be empty")
	}
	tagCopy := validated.tagsValue()
	if len(tagCopy) == 0 {
		tagCopy = []string{}
	} else {
		slices.Sort(tagCopy)
	}
	return NoteRecord{
		id:          validated.idValue(),
		sourceChat:  validated.sourceChatValue(),
		title:       validated.titleValue(),
		sourceMsgID: validated.sourceMsgIDValue(),
		body:        validated.bodyValue(),
		hash:        validated.hashValue(),
		embeddingID: validated.embeddingIDValue(),
		clusterID:   validated.clusterIDValue(),
		tags:        tagCopy,
		createdAt:   validated.createdAtValue(),
		updatedAt:   validated.updatedAtValue(),
		embedding:   embeddingValue,
		duplicateOf: validated.duplicateOfValue(),
	}, nil
}

func (r NoteRecord) ID() NoteID           { return r.id }
func (r NoteRecord) SourceChat() string   { return r.sourceChat }
func (r NoteRecord) SourceMsgID() int     { return r.sourceMsgID }
func (r NoteRecord) Title() string        { return r.title }
func (r NoteRecord) Body() string         { return r.body }
func (r NoteRecord) Hash() string         { return r.hash }
func (r NoteRecord) EmbeddingID() string  { return r.embeddingID }
func (r NoteRecord) Embedding() Vector    { return r.embedding }
func (r NoteRecord) ClusterID() string    { return r.clusterID }
func (r NoteRecord) Tags() []string       { return slices.Clone(r.tags) }
func (r NoteRecord) CreatedAt() time.Time { return r.createdAt }
func (r NoteRecord) UpdatedAt() time.Time { return r.updatedAt }
func (r NoteRecord) DuplicateOf() NoteID  { return r.duplicateOf }
func (r NoteRecord) Clone() NoteRecord {
	clone := r
	clone.tags = slices.Clone(r.tags)
	if len(clone.tags) == 0 {
		clone.tags = []string{}
	}
	return clone
}

func NewClusterRecord(id, name, slug string, centroid Vector, noteIDs []NoteID, createdAt, updatedAt time.Time) (ClusterRecord, error) {
	noteIDCopy := slices.Clone(noteIDs)
	slices.SortFunc(noteIDCopy, compareNoteIDs)
	for i := 1; i < len(noteIDCopy); i++ {
		if noteIDCopy[i-1] == noteIDCopy[i] {
			return ClusterRecord{}, fmt.Errorf("note_ids[%d]: duplicate note id %q", i, noteIDCopy[i])
		}
	}
	validated, err := newCluster(id, name, slug, centroid, noteIDCopy, createdAt, updatedAt)
	if err != nil {
		return ClusterRecord{}, err
	}
	return ClusterRecord{
		id:        validated.idValue(),
		name:      validated.nameValue(),
		slug:      validated.slugValue(),
		centroid:  validated.centroidValue(),
		noteIDs:   noteIDCopy,
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

func (r ClusterRecord) ID() string           { return r.id }
func (r ClusterRecord) Name() string         { return r.name }
func (r ClusterRecord) Slug() string         { return r.slug }
func (r ClusterRecord) Centroid() Vector     { return r.centroid }
func (r ClusterRecord) NoteIDs() []NoteID    { return slices.Clone(r.noteIDs) }
func (r ClusterRecord) CreatedAt() time.Time { return r.createdAt }
func (r ClusterRecord) UpdatedAt() time.Time { return r.updatedAt }
func (r ClusterRecord) Clone() ClusterRecord {
	clone := r
	clone.noteIDs = slices.Clone(r.noteIDs)
	return clone
}

func compareNoteIDs(a, b NoteID) int {
	aID, aOK := noteSortKey(a)
	bID, bOK := noteSortKey(b)
	if aOK && bOK {
		switch {
		case aID < bID:
			return -1
		case aID > bID:
			return 1
		}
	}
	return strings.Compare(a.String(), b.String())
}

func noteSortKey(noteID NoteID) (int, bool) {
	text := noteID.String()
	idx := strings.LastIndex(text, ":")
	if idx < 0 || idx == len(text)-1 {
		return 0, false
	}
	value, err := parsePositiveInt(text[idx+1:])
	if err != nil {
		return 0, false
	}
	return value, true
}

func parsePositiveInt(text string) (int, error) {
	value := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid integer %q", text)
		}
		value = value*10 + int(r-'0')
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid integer %q", text)
	}
	return value, nil
}
