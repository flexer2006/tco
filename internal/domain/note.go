package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type note struct {
	createdAt, updatedAt                                  time.Time
	tags                                                  []string
	id, duplicateOf                                       NoteID
	sourceChat, body, hash, embeddingID, clusterID, title string
	sourceMsgID                                           int
}

func newNote(
	id NoteID,
	sourceChat string,
	sourceMsgID int,
	title, body, embeddingID, clusterID string,
	tags []string,
	createdAt, updatedAt time.Time,
	duplicateOf NoteID,
) (note, error) {
	expectedID, err := NewNoteID(sourceChat, sourceMsgID)
	if err != nil {
		return note{}, err
	}

	if id != expectedID {
		return note{}, fmt.Errorf(
			"%w: expected %q for %s/%d, got %q",
			ErrNoteIDMismatch,
			expectedID,
			sourceChat,
			sourceMsgID,
			id,
		)
	}

	if strings.TrimSpace(title) == "" {
		return note{}, ErrTitleEmpty
	}

	if strings.TrimSpace(body) == "" {
		return note{}, ErrBodyEmpty
	}

	if strings.Contains(body, "\r") {
		return note{}, ErrBodyCarriageReturn
	}

	if strings.HasSuffix(body, "\n") {
		return note{}, ErrBodyTrailingNewline
	}

	if strings.TrimSpace(embeddingID) == "" {
		return note{}, ErrEmbeddingIDEmpty
	}

	if strings.TrimSpace(clusterID) == "" {
		return note{}, ErrClusterIDEmpty
	}

	err = validateNoteUTC("created_at", createdAt)
	if err != nil {
		return note{}, err
	}

	err = validateNoteUTC("updated_at", updatedAt)
	if err != nil {
		return note{}, err
	}

	if updatedAt.Before(createdAt) {
		return note{}, ErrUpdatedBeforeCreated
	}

	if duplicateOf != "" && duplicateOf == id {
		return note{}, ErrDuplicateOfEqualsID
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
	const markdownExtraCapacity = 256

	var builder strings.Builder
	builder.Grow(len(n.body) + len(n.title) + len(n.hash) + len(n.tags)*8 + markdownExtraCapacity)
	builder.WriteString("---\n")
	builder.WriteString("id: ")
	builder.WriteString(n.id.String())
	builder.WriteByte('\n')
	builder.WriteString("source_chat: ")
	builder.WriteString(strconv.Quote(n.sourceChat))
	builder.WriteByte('\n')
	builder.WriteString("source_msg_id: ")
	builder.WriteString(strconv.Itoa(n.sourceMsgID))
	builder.WriteByte('\n')
	builder.WriteString("title: ")
	builder.WriteString(strconv.Quote(n.title))
	builder.WriteByte('\n')
	builder.WriteString("body: |-\n")
	builder.WriteString(renderLiteralBlock(n.body))
	builder.WriteByte('\n')
	builder.WriteString("hash: ")
	builder.WriteString(n.hash)
	builder.WriteByte('\n')
	builder.WriteString("embedding_id: ")
	builder.WriteString(n.embeddingID)
	builder.WriteByte('\n')
	builder.WriteString("cluster_id: ")
	builder.WriteString(n.clusterID)
	builder.WriteByte('\n')
	builder.WriteString("tags: ")
	builder.WriteString(renderTags(n.tags))
	builder.WriteByte('\n')
	builder.WriteString("created_at: ")
	builder.WriteString(n.createdAt.Format(time.RFC3339))
	builder.WriteByte('\n')
	builder.WriteString("updated_at: ")
	builder.WriteString(n.updatedAt.Format(time.RFC3339))
	builder.WriteByte('\n')
	builder.WriteString("duplicate_of: ")
	builder.WriteString(strconv.Quote(n.duplicateOf.String()))
	builder.WriteByte('\n')
	builder.WriteString("---\n")
	builder.WriteString(n.body)

	return builder.String()
}

func RenderNoteMarkdown(
	id NoteID,
	sourceChat string,
	sourceMsgID int,
	title, body, embeddingID, clusterID string,
	tags []string,
	createdAt, updatedAt time.Time,
	duplicateOf NoteID,
) (string, error) {
	noteValue, err := newNote(
		id,
		sourceChat,
		sourceMsgID,
		title,
		body,
		embeddingID,
		clusterID,
		tags,
		createdAt,
		updatedAt,
		duplicateOf,
	)
	if err != nil {
		return "", err
	}

	return noteValue.markdown(), nil
}

func validateNoteUTC(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s %w", field, ErrTimeMustNotBeZero)
	}

	if value.Location() != time.UTC {
		return fmt.Errorf("%s %w", field, ErrTimeMustBeUTC)
	}

	return nil
}

func hashRenderedBody(body string) string {
	sum := sha256.Sum256([]byte(body))

	return hex.EncodeToString(sum[:])
}

func renderLiteralBlock(body string) string {
	var builder strings.Builder

	firstLine := true
	for line := range strings.SplitSeq(body, "\n") {
		if !firstLine {
			builder.WriteByte('\n')
		}

		firstLine = false

		if line != "" {
			builder.WriteString("  ")
			builder.WriteString(line)
		}
	}

	return builder.String()
}

func renderTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}

	var builder strings.Builder
	builder.WriteByte('[')

	for i, tag := range tags {
		if i > 0 {
			builder.WriteString(", ")
		}

		builder.WriteString(strconv.Quote(tag))
	}

	builder.WriteByte(']')

	return builder.String()
}
