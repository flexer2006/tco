package adapters

import (
	"fmt"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
	"time"

	"github.com/gotd/td/tg"
)

func mapLiveMessages(sourceChat string, records []tg.MessageClass) ([]domain.RawCanonicalMessage, error) {
	mappedRecords := make([]mappedMessage, 0, len(records))
	for _, record := range records {
		mapped, include := mapLiveMessage(record)
		if include {
			mappedRecords = append(mappedRecords, mapped)
		}
	}
	return mapMessages(sourceChat, mappedRecords)
}

func mapLiveMessage(record tg.MessageClass) (mappedMessage, bool) {
	typed, ok := record.(*tg.Message)
	if !ok {
		return mappedMessage{}, false
	}
	dateUTC := time.Unix(int64(typed.GetDate()), 0).UTC()
	editedAtUTC := liveEditedAtUTC(typed)
	replyToSourceMsgID := liveReplyToSourceMsgID(typed)
	forwardedFrom := liveForwardedFrom(typed)
	mediaKind := liveMediaKind(typed)
	return mappedMessage{
		SourceMsgID:        typed.GetID(),
		DateUTC:            dateUTC,
		Kind:               "message",
		Text:               typed.GetMessage(),
		ReplyToSourceMsgID: replyToSourceMsgID,
		EditedAtUTC:        editedAtUTC,
		ForwardedFrom:      forwardedFrom,
		MediaKind:          mediaKind,
		IsOutgoing:         typed.GetOut(),
	}, true
}

func liveEditedAtUTC(record *tg.Message) *time.Time {
	if record == nil {
		return nil
	}
	editDate, ok := record.GetEditDate()
	if !ok || editDate <= 0 {
		return nil
	}
	editedAtUTC := time.Unix(int64(editDate), 0).UTC()
	return new(editedAtUTC)
}

func liveReplyToSourceMsgID(record *tg.Message) *int {
	if record == nil {
		return nil
	}
	replyTo, ok := record.GetReplyTo()
	if !ok {
		return nil
	}
	replyHeader, ok := replyTo.(*tg.MessageReplyHeader)
	if !ok {
		return nil
	}
	replyID, ok := replyHeader.GetReplyToMsgID()
	if !ok || replyID <= 0 {
		return nil
	}
	return new(replyID)
}

func liveForwardedFrom(record *tg.Message) *string {
	if record == nil {
		return nil
	}
	fwdHeader, ok := record.GetFwdFrom()
	if !ok {
		return nil
	}
	if name, ok := fwdHeader.GetFromName(); ok {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			return new(trimmed)
		}
	}
	if author, ok := fwdHeader.GetPostAuthor(); ok {
		trimmed := strings.TrimSpace(author)
		if trimmed != "" {
			return new(trimmed)
		}
	}
	fromID, ok := fwdHeader.GetFromID()
	if !ok {
		return nil
	}
	labeled := labelLivePeer(fromID)
	if labeled == "" {
		return nil
	}
	return new(labeled)
}

func liveMediaKind(record *tg.Message) *string {
	if record == nil {
		return nil
	}
	media, ok := record.GetMedia()
	if !ok {
		return nil
	}
	if _, empty := media.(*tg.MessageMediaEmpty); empty {
		return nil
	}
	typeName := media.TypeName()
	return new(typeName)
}

func labelLivePeer(peer tg.PeerClass) string {
	switch typed := peer.(type) {
	case *tg.PeerUser:
		return fmt.Sprintf("user:%d", typed.GetUserID())
	case *tg.PeerChat:
		return fmt.Sprintf("chat:%d", typed.GetChatID())
	case *tg.PeerChannel:
		return fmt.Sprintf("channel:%d", typed.GetChannelID())
	default:
		return ""
	}
}
