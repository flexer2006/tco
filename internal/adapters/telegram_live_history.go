package adapters

import (
	"context"
	"errors"
	"slices"

	"github.com/gotd/td/tg"
)

func fetchLiveHistory(ctx context.Context, api liveAPI, peer tg.InputPeerClass, pageSize int) ([]tg.MessageClass, error) {
	if pageSize <= 0 {
		return nil, errors.New("live history page size must be positive")
	}
	var offsetID int
	result := make([]tg.MessageClass, 0, pageSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			Limit:    pageSize,
			OffsetID: offsetID,
		})
		if err != nil {
			return nil, err
		}
		page := extractLiveHistoryMessages(response)
		if len(page) == 0 {
			return result, nil
		}
		result = append(result, page...)
		minID := minLiveMessageID(page)
		if minID <= 0 {
			return result, nil
		}
		nextOffsetID := minID
		if nextOffsetID <= 0 || nextOffsetID == offsetID {
			return result, nil
		}
		offsetID = nextOffsetID
	}
}

func extractLiveHistoryMessages(response tg.MessagesMessagesClass) []tg.MessageClass {
	switch typed := response.(type) {
	case *tg.MessagesMessages:
		return slices.Clone(typed.GetMessages())
	case *tg.MessagesMessagesSlice:
		return slices.Clone(typed.GetMessages())
	case *tg.MessagesChannelMessages:
		return slices.Clone(typed.GetMessages())
	default:
		return nil
	}
}

func minLiveMessageID(messages []tg.MessageClass) (minID int) {
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		msgID := msg.GetID()
		if msgID <= 0 {
			continue
		}
		if minID == 0 || msgID < minID {
			minID = msgID
		}
	}
	return
}
