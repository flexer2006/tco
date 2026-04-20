package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const titleMaxRunes = 72

var newlineNormalizer = strings.NewReplacer("\r\n", "\n", "\r", "\n")

type (
	NoteID              string
	RawCanonicalMessage struct {
		dateUTC            time.Time
		sourceChat, text   string
		editedAtUTC        *time.Time
		forwardedFrom      *string
		replyToSourceMsgID *int
		sourceMsgID        int
		isOutgoing         bool
	}
)

func HashNormalizedText(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum[:])
}

func NewNoteID(sourceChat string, sourceMsgID int) (NoteID, error) {
	if strings.TrimSpace(sourceChat) == "" {
		return "", errors.New("source chat must not be empty")
	}
	if sourceMsgID <= 0 {
		return "", errors.New("source message ID must be positive")
	}
	return NoteID(fmt.Sprintf("%s:%d", sourceChat, sourceMsgID)), nil
}

func (id NoteID) String() string { return string(id) }

func NormalizeText(raw string) string {
	if raw == "" {
		return ""
	}
	text := newlineNormalizer.Replace(raw)
	text = norm.NFC.String(text)
	normalized := make([]string, 0, strings.Count(text, "\n")+1)
	inLeadingBlank := true
	previousBlank := false
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if inLeadingBlank || previousBlank {
				continue
			}
			normalized = append(normalized, "")
			previousBlank = true
			continue
		}
		inLeadingBlank, previousBlank = false, false
		normalized = append(normalized, line)
	}
	for len(normalized) > 0 && normalized[len(normalized)-1] == "" {
		normalized = normalized[:len(normalized)-1]
	}
	return strings.Join(normalized, "\n")
}

func DeriveTitleFromNormalizedText(normalized string, sourceMsgID int) string {
	for line := range strings.SplitSeq(normalized, "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		candidate = stripLeadingMarkers(candidate)
		for {
			next := strings.TrimSpace(stripSurroundingEmphasis(stripTrailingTerminalPunctuation(candidate)))
			if next == candidate {
				break
			}
			candidate = next
		}
		if candidate == "" {
			continue
		}
		return truncateRunes(candidate, titleMaxRunes)
	}
	return fmt.Sprintf("untitled-%d", sourceMsgID)
}

func stripLeadingMarkers(text string) string {
	for {
		trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
		if trimmed == "" {
			return ""
		}
		if after, ok := strings.CutPrefix(trimmed, ">"); ok {
			text = strings.TrimLeftFunc(strings.TrimSpace(after), unicode.IsSpace)
			continue
		}
		if next, ok := cutBulletMarker(trimmed); ok {
			text = next
			continue
		}
		return trimmed
	}
}

//nolint:gocyclo
func cutBulletMarker(text string) (string, bool) {
	if text == "" {
		return "", false
	}
	first := text[0]
	if first == '-' || first == '*' || first == '+' {
		if len(text) == 1 {
			return "", true
		}
		next := text[1]
		if next == ' ' || next == '\t' {
			return strings.TrimLeftFunc(text[2:], unicode.IsSpace), true
		}
		return "", false
	}
	idx := 0
	for idx < len(text) && text[idx] >= '0' && text[idx] <= '9' {
		idx++
	}
	if idx == 0 || idx >= len(text) {
		return "", false
	}
	if text[idx] != '.' && text[idx] != ')' {
		return "", false
	}
	if idx+1 < len(text) {
		next := text[idx+1]
		if next != ' ' && next != '\t' {
			return "", false
		}
	}
	return strings.TrimLeftFunc(text[idx+1:], unicode.IsSpace), true
}

func stripSurroundingEmphasis(text string) string {
	for {
		start, end, ok := surroundingEmphasisBounds(text)
		if !ok {
			return text
		}
		text = text[start:end]
	}
}

func stripTrailingTerminalPunctuation(text string) string {
	return strings.TrimRight(text, ".:")
}

func surroundingEmphasisBounds(text string) (int, int, bool) {
	if text == "" {
		return 0, 0, false
	}
	startRune, startSize := utf8FirstRune(text)
	endRune, endSize := utf8LastRune(text)
	if startSize == 0 || endSize == 0 || startRune != endRune {
		return 0, 0, false
	}
	if !isEmphasisRune(startRune) {
		return 0, 0, false
	}
	if len(text) <= startSize+endSize {
		return 0, 0, false
	}
	return startSize, len(text) - endSize, true
}

func isEmphasisRune(r rune) bool {
	switch r {
	case '*', '_', '~':
		return true
	default:
		return false
	}
}

func utf8FirstRune(text string) (rune, int) {
	for _, r := range text {
		return r, len(string(r))
	}
	return 0, 0
}

func utf8LastRune(text string) (rune, int) {
	var last rune
	var size int
	for _, r := range text {
		last = r
		size = len(string(r))
	}
	if size == 0 {
		return 0, 0
	}
	return last, size
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func NewRawCanonicalMessage(sourceChat string, sourceMsgID int, dateUTC time.Time, text string, forwardedFrom *string, replyToSourceMsgID *int, editedAtUTC *time.Time, isOutgoing bool) (RawCanonicalMessage, error) {
	if strings.TrimSpace(sourceChat) == "" {
		return RawCanonicalMessage{}, errors.New("source chat must not be empty")
	}
	if sourceMsgID <= 0 {
		return RawCanonicalMessage{}, errors.New("source message ID must be positive")
	}
	if err := validateUTCTimestamp("date_utc", dateUTC); err != nil {
		return RawCanonicalMessage{}, err
	}
	if editedAtUTC != nil {
		if err := validateUTCTimestamp("edited_at_utc", *editedAtUTC); err != nil {
			return RawCanonicalMessage{}, err
		}
		if editedAtUTC.Before(dateUTC) {
			return RawCanonicalMessage{}, errors.New("edited_at_utc must be greater than or equal to date_utc")
		}
	}
	if replyToSourceMsgID != nil && *replyToSourceMsgID <= 0 {
		return RawCanonicalMessage{}, fmt.Errorf("reply_to_source_msg_id must be positive, got %d", *replyToSourceMsgID)
	}
	return RawCanonicalMessage{
		sourceChat:         sourceChat,
		sourceMsgID:        sourceMsgID,
		dateUTC:            dateUTC.UTC(),
		text:               text,
		forwardedFrom:      cloneStringPointer(forwardedFrom),
		replyToSourceMsgID: cloneIntPointer(replyToSourceMsgID),
		editedAtUTC:        cloneTimePointer(editedAtUTC),
		isOutgoing:         isOutgoing,
	}, nil
}

func (m RawCanonicalMessage) SourceChat() string { return m.sourceChat }
func (m RawCanonicalMessage) SourceMsgID() int   { return m.sourceMsgID }
func (m RawCanonicalMessage) DateUTC() time.Time { return m.dateUTC }
func (m RawCanonicalMessage) Text() string       { return m.text }
func (m RawCanonicalMessage) EditedAtUTC() *time.Time {
	return cloneTimePointer(m.editedAtUTC)
}

func validateUTCTimestamp(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s must not be zero", field)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", field)
	}
	return nil
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return new(*value)
}
