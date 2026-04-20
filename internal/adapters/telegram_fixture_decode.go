package adapters

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

type (
	mappedMessage struct {
		DateUTC            time.Time  `json:"date_utc"`
		Kind               string     `json:"kind"`
		Text               string     `json:"text"`
		SourceMsgID        int        `json:"source_msg_id"`
		ReplyToSourceMsgID *int       `json:"reply_to_source_msg_id"`
		EditedAtUTC        *time.Time `json:"edited_at_utc"`
		ForwardedFrom      *string    `json:"forwarded_from"`
		MediaKind          *string    `json:"media_kind"`
		IsOutgoing         bool       `json:"is_outgoing"`
	}
	selectedMessage struct {
		message domain.RawCanonicalMessage
	}
)

var (
	includeAllMessagesOnce  sync.Once
	includeAllMessagesValue bool
)

func mapMessages(sourceChat string, records []mappedMessage) ([]domain.RawCanonicalMessage, error) {
	selectedByID := make(map[int]selectedMessage, len(records))
	for idx, record := range records {
		if !includeMappedMessage(record) {
			continue
		}
		rawMessage, err := domain.NewRawCanonicalMessage(
			sourceChat,
			record.SourceMsgID,
			record.DateUTC,
			record.Text,
			record.ForwardedFrom,
			record.ReplyToSourceMsgID,
			record.EditedAtUTC,
			record.IsOutgoing,
		)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", idx, err)
		}
		current, exists := selectedByID[record.SourceMsgID]
		if !exists || isBetterRecord(rawMessage, current.message) {
			selectedByID[record.SourceMsgID] = selectedMessage{message: rawMessage}
		}
	}

	orderedIDs := make([]int, 0, len(selectedByID))
	for sourceMsgID := range selectedByID {
		orderedIDs = append(orderedIDs, sourceMsgID)
	}
	slices.Sort(orderedIDs)

	result := make([]domain.RawCanonicalMessage, 0, len(orderedIDs))
	for _, sourceMsgID := range orderedIDs {
		result = append(result, selectedByID[sourceMsgID].message)
	}
	return result, nil
}

func includeMappedMessage(record mappedMessage) bool {
	if record.Kind != "message" {
		return false
	}
	if includeAllMessagesEnabled() {
		return true
	}
	if strings.TrimSpace(record.Text) == "" {
		return false
	}
	if record.MediaKind != nil {
		return false
	}
	return true
}

func includeAllMessagesEnabled() bool {
	includeAllMessagesOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("TELEGRAM_INCLUDE_ALL_MESSAGES"))
		if raw == "" {
			includeAllMessagesValue = false
			return
		}
		enabled, err := strconv.ParseBool(raw)
		includeAllMessagesValue = err == nil && enabled
	})
	return includeAllMessagesValue
}

func isBetterRecord(candidate, current domain.RawCanonicalMessage) bool {
	switch compareOptionalTime(candidate.EditedAtUTC(), current.EditedAtUTC()) {
	case 1:
		return true
	case -1:
		return false
	}
	if candidate.DateUTC().After(current.DateUTC()) {
		return true
	}
	if candidate.DateUTC().Before(current.DateUTC()) {
		return false
	}
	return false
}

func compareOptionalTime(left, right *time.Time) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	case left.After(*right):
		return 1
	case left.Before(*right):
		return -1
	default:
		return 0
	}
}
