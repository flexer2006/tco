package telegram

import (
	"context"
	"slices"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	defaultHistoryMaxMessages = 5000
	maxFloodWaitRetries       = 20
)

func fetchLiveHistory(
	ctx context.Context,
	api liveAPI,
	peer tg.InputPeerClass,
	pageSize, maxMessages, minExclusiveID int,
) ([]tg.MessageClass, error) {
	if pageSize <= 0 {
		return nil, ErrLiveHistoryPageSizeNotPositive
	}

	if maxMessages <= 0 {
		maxMessages = defaultHistoryMaxMessages
	}

	var offsetID int

	result := make([]tg.MessageClass, 0, min(pageSize, maxMessages))

	for {
		err := ctx.Err()
		if err != nil {
			return nil, err
		}

		if len(result) >= maxMessages {
			return result[:maxMessages], nil
		}

		limit := pageSize
		if remaining := maxMessages - len(result); remaining < limit {
			limit = remaining
		}

		response, err := messagesGetHistoryWithFloodWait(ctx, api, new(tg.MessagesGetHistoryRequest{
			Peer:     peer,
			Limit:    limit,
			OffsetID: offsetID,
		}))
		if err != nil {
			return nil, err
		}

		page := extractLiveHistoryMessages(response)
		if len(page) == 0 {
			return result, nil
		}

		stop := false

		for _, msg := range page {
			if msg == nil {
				continue
			}

			msgID := msg.GetID()
			if msgID <= 0 {
				continue
			}

			if minExclusiveID > 0 && msgID <= minExclusiveID {
				stop = true

				continue
			}

			result = append(result, msg)
			if len(result) >= maxMessages {
				return result[:maxMessages], nil
			}
		}

		if stop {
			return result, nil
		}

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

func messagesGetHistoryWithFloodWait(
	ctx context.Context,
	api liveAPI,
	request *tg.MessagesGetHistoryRequest,
) (tg.MessagesMessagesClass, error) {
	var lastErr error

	for range maxFloodWaitRetries {
		err := ctx.Err()
		if err != nil {
			return nil, err
		}

		response, err := api.MessagesGetHistory(ctx, request)
		if err == nil {
			return response, nil
		}

		lastErr = err

		wait, ok := tgerr.AsFloodWait(err)
		if !ok {
			return nil, err
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()

			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
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

func minLiveMessageID(messages []tg.MessageClass) int {
	minID := 0

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

	return minID
}
