package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type note struct {
	id, duplicateOf                                       NoteID
	createdAt, updatedAt                                  time.Time
	tags                                                  []string
	sourceChat, body, hash, embeddingID, clusterID, title string
	sourceMsgID                                           int
}

func newNote(id NoteID, sourceChat string, sourceMsgID int, title, body, embeddingID, clusterID string, tags []string, createdAt, updatedAt time.Time, duplicateOf NoteID) (note, error) {
	expectedID, err := NewNoteID(sourceChat, sourceMsgID)
	if err != nil {
		return note{}, err
	}
	if id != expectedID {
		return note{}, fmt.Errorf("id: expected %q for %s/%d, got %q", expectedID, sourceChat, sourceMsgID, id)
	}
	if strings.TrimSpace(title) == "" {
		return note{}, errors.New("title must not be empty")
	}
	if strings.TrimSpace(body) == "" {
		return note{}, errors.New("body must not be empty")
	}
	if strings.Contains(body, "\r") {
		return note{}, errors.New("body must not contain carriage returns")
	}
	if strings.HasSuffix(body, "\n") {
		return note{}, errors.New("body must not end with a newline")
	}
	if strings.TrimSpace(embeddingID) == "" {
		return note{}, errors.New("embedding id must not be empty")
	}
	if strings.TrimSpace(clusterID) == "" {
		return note{}, errors.New("cluster id must not be empty")
	}
	if err := validateNoteUTC("created_at", createdAt); err != nil {
		return note{}, err
	}
	if err := validateNoteUTC("updated_at", updatedAt); err != nil {
		return note{}, err
	}
	if updatedAt.Before(createdAt) {
		return note{}, errors.New("updated_at must not be before created_at")
	}
	if duplicateOf != "" && duplicateOf == id {
		return note{}, errors.New("duplicate_of must not equal id")
	}
	return note{
		id:          id,
		sourceChat:  sourceChat,
		sourceMsgID: sourceMsgID,
		title:       title,
		body:        body,
		hash:        hashRenderedBody(body),
		embeddingID: embeddingID,
		clusterID:   clusterID,
		tags:        slices.Clone(tags),
		createdAt:   createdAt.UTC(),
		updatedAt:   updatedAt.UTC(),
		duplicateOf: duplicateOf,
	}, nil
}

func (n note) idValue() NoteID           { return n.id }
func (n note) sourceChatValue() string   { return n.sourceChat }
func (n note) sourceMsgIDValue() int     { return n.sourceMsgID }
func (n note) titleValue() string        { return n.title }
func (n note) bodyValue() string         { return n.body }
func (n note) hashValue() string         { return n.hash }
func (n note) embeddingIDValue() string  { return n.embeddingID }
func (n note) clusterIDValue() string    { return n.clusterID }
func (n note) tagsValue() []string       { return slices.Clone(n.tags) }
func (n note) createdAtValue() time.Time { return n.createdAt }
func (n note) updatedAtValue() time.Time { return n.updatedAt }
func (n note) duplicateOfValue() NoteID  { return n.duplicateOf }

func (n note) markdown() string {
	var b strings.Builder
	b.Grow(len(n.body) + len(n.title) + len(n.hash) + len(n.tags)*8 + 256)
	b.WriteString("---\n")
	b.WriteString("id: ")
	b.WriteString(n.id.String())
	b.WriteByte('\n')
	b.WriteString("source_chat: ")
	b.WriteString(n.sourceChat)
	b.WriteByte('\n')
	b.WriteString("source_msg_id: ")
	b.WriteString(strconv.Itoa(n.sourceMsgID))
	b.WriteByte('\n')
	b.WriteString("title: ")
	b.WriteString(n.title)
	b.WriteByte('\n')
	b.WriteString("body: |-\n")
	b.WriteString(renderLiteralBlock(n.body))
	b.WriteByte('\n')
	b.WriteString("hash: ")
	b.WriteString(n.hash)
	b.WriteByte('\n')
	b.WriteString("embedding_id: ")
	b.WriteString(n.embeddingID)
	b.WriteByte('\n')
	b.WriteString("cluster_id: ")
	b.WriteString(n.clusterID)
	b.WriteByte('\n')
	b.WriteString("tags: ")
	b.WriteString(renderTags(n.tags))
	b.WriteByte('\n')
	b.WriteString("created_at: ")
	b.WriteString(n.createdAt.Format(time.RFC3339))
	b.WriteByte('\n')
	b.WriteString("updated_at: ")
	b.WriteString(n.updatedAt.Format(time.RFC3339))
	b.WriteByte('\n')
	b.WriteString("duplicate_of: ")
	b.WriteString(strconv.Quote(n.duplicateOf.String()))
	b.WriteByte('\n')
	b.WriteString("---\n")
	b.WriteString(n.body)
	return b.String()
}

func RenderNoteMarkdown(id NoteID, sourceChat string, sourceMsgID int, title, body, embeddingID, clusterID string, tags []string, createdAt, updatedAt time.Time, duplicateOf NoteID) (string, error) {
	noteValue, err := newNote(id, sourceChat, sourceMsgID, title, body, embeddingID, clusterID, tags, createdAt, updatedAt, duplicateOf)
	if err != nil {
		return "", err
	}
	return noteValue.markdown(), nil
}

func validateNoteUTC(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s must not be zero", field)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", field)
	}
	return nil
}

func hashRenderedBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func renderLiteralBlock(body string) string {
	var b strings.Builder
	firstLine := true
	for line := range strings.SplitSeq(body, "\n") {
		if !firstLine {
			b.WriteByte('\n')
		}
		firstLine = false
		if line != "" {
			b.WriteString("  ")
			b.WriteString(line)
		}
	}
	return b.String()
}

func renderTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, tag := range tags {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(tag))
	}
	b.WriteByte(']')
	return b.String()
}
