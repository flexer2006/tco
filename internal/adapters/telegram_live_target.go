package adapters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

const (
	liveTargetKindChat          liveTargetKind = "chat"
	liveTargetKindUser          liveTargetKind = "user"
	liveTargetKindChannel       liveTargetKind = "channel"
	liveTargetKindUsername      liveTargetKind = "username"
	liveTargetKindChannelPeerID liveTargetKind = "channel_peer_id"
)

type (
	liveTarget struct {
		username   string
		kind       liveTargetKind
		id         int64
		accessHash int64
	}
	liveTargetKind string
)

func parseLiveTarget(sourceChat string) (liveTarget, error) {
	trimmed := strings.TrimSpace(sourceChat)
	if trimmed == "" {
		return liveTarget{}, errors.New("source chat must not be empty")
	}
	if after, ok := strings.CutPrefix(trimmed, "@"); ok {
		username := strings.TrimSpace(after)
		if username == "" {
			return liveTarget{}, errors.New("username must not be empty")
		}
		return liveTarget{kind: liveTargetKindUsername, username: username}, nil
	}
	prefix, payload, hasPrefix := strings.Cut(trimmed, ":")
	if hasPrefix {
		switch strings.ToLower(strings.TrimSpace(prefix)) {
		case "username":
			username := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(payload), "@"))
			if username == "" {
				return liveTarget{}, errors.New("username must not be empty")
			}
			return liveTarget{kind: liveTargetKindUsername, username: username}, nil
		case "chat":
			chatTarget, err := parseLiveChatTarget(payload)
			if err != nil {
				return liveTarget{}, fmt.Errorf("parse chat id: %w", err)
			}
			return chatTarget, nil
		case "user":
			return parseLiveTargetWithAccessHash(liveTargetKindUser, payload)
		case "channel":
			return parseLiveTargetWithAccessHash(liveTargetKindChannel, payload)
		}
	}
	chatTarget, err := parseLiveChatTarget(trimmed)
	if err == nil {
		return chatTarget, nil
	}
	_, parseNumericErr := strconv.ParseInt(trimmed, 10, 64)
	if parseNumericErr == nil {
		return liveTarget{}, err
	}
	return liveTarget{kind: liveTargetKindUsername, username: trimmed}, nil
}

func parseLiveChatTarget(raw string) (liveTarget, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return liveTarget{}, errors.New("chat id must not be empty")
	}
	chatID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return liveTarget{}, err
	}
	if chatID > 0 {
		return liveTarget{kind: liveTargetKindChat, id: chatID}, nil
	}
	channelID, err := parseLiveChannelIDFromPeerID(trimmed)
	if err != nil {
		return liveTarget{}, errors.New("chat id must be positive")
	}
	return liveTarget{kind: liveTargetKindChannelPeerID, id: channelID}, nil
}

func parseLiveChannelIDFromPeerID(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "-100") {
		return 0, errors.New("channel peer id must start with -100")
	}
	channelIDRaw := strings.TrimSpace(strings.TrimPrefix(trimmed, "-100"))
	if channelIDRaw == "" {
		return 0, errors.New("channel peer id must contain channel id suffix")
	}
	channelID, err := strconv.ParseInt(channelIDRaw, 10, 64)
	if err != nil {
		return 0, err
	}
	if channelID <= 0 {
		return 0, errors.New("channel id must be positive")
	}
	return channelID, nil
}

func parseLiveTargetWithAccessHash(kind liveTargetKind, payload string) (liveTarget, error) {
	idRaw, accessHashRaw, ok := strings.Cut(strings.TrimSpace(payload), ":")
	if !ok {
		return liveTarget{}, fmt.Errorf("%s target must be in form %s:<id>:<access_hash>", kind, kind)
	}
	id, err := parsePositiveInt64(idRaw)
	if err != nil {
		return liveTarget{}, fmt.Errorf("parse %s id: %w", kind, err)
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(accessHashRaw), 10, 64)
	if err != nil {
		return liveTarget{}, fmt.Errorf("parse %s access hash: %w", kind, err)
	}
	return liveTarget{kind: kind, id: id, accessHash: accessHash}, nil
}

func parsePositiveInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, errors.New("value must be positive")
	}
	return parsed, nil
}

func resolveLiveInputPeer(ctx context.Context, api liveAPI, target liveTarget) (tg.InputPeerClass, error) {
	switch target.kind {
	case liveTargetKindChat:
		return &tg.InputPeerChat{ChatID: target.id}, nil
	case liveTargetKindUser:
		return &tg.InputPeerUser{UserID: target.id, AccessHash: target.accessHash}, nil
	case liveTargetKindChannel:
		return &tg.InputPeerChannel{ChannelID: target.id, AccessHash: target.accessHash}, nil
	case liveTargetKindChannelPeerID:
		return resolveLiveChannelPeerID(ctx, api, target.id)
	case liveTargetKindUsername:
		return resolveLiveUsername(ctx, api, target.username)
	default:
		return nil, fmt.Errorf("unsupported live target kind %q", target.kind)
	}
}

func resolveLiveChannelPeerID(ctx context.Context, api liveAPI, channelID int64) (tg.InputPeerClass, error) {
	dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      defaultLiveHistoryPageSize,
	})
	if err != nil {
		return nil, err
	}
	accessHash, err := findResolvedChannelAccessHash(extractLiveDialogsChats(dialogs), channelID)
	if err != nil {
		return nil, fmt.Errorf("resolve channel access hash for %d: %w", channelID, err)
	}
	return &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}, nil
}

func extractLiveDialogsChats(response tg.MessagesDialogsClass) []tg.ChatClass {
	switch typed := response.(type) {
	case *tg.MessagesDialogs:
		return slices.Clone(typed.GetChats())
	case *tg.MessagesDialogsSlice:
		return slices.Clone(typed.GetChats())
	default:
		return nil
	}
}

func resolveLiveUsername(ctx context.Context, api liveAPI, username string) (tg.InputPeerClass, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if trimmed == "" {
		return nil, errors.New("username must not be empty")
	}
	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: trimmed})
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, errors.New("resolve username returned nil response")
	}
	peer := resolved.GetPeer()
	switch typed := peer.(type) {
	case *tg.PeerUser:
		accessHash, err := findResolvedUserAccessHash(resolved.GetUsers(), typed.GetUserID())
		if err != nil {
			return nil, err
		}
		return &tg.InputPeerUser{UserID: typed.GetUserID(), AccessHash: accessHash}, nil
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: typed.GetChatID()}, nil
	case *tg.PeerChannel:
		accessHash, err := findResolvedChannelAccessHash(resolved.GetChats(), typed.GetChannelID())
		if err != nil {
			return nil, err
		}
		return &tg.InputPeerChannel{ChannelID: typed.GetChannelID(), AccessHash: accessHash}, nil
	default:
		return nil, fmt.Errorf("unsupported resolved peer type %T", typed)
	}
}

func findResolvedUserAccessHash(users []tg.UserClass, userID int64) (int64, error) {
	for _, user := range users {
		typed, ok := user.(*tg.User)
		if !ok || typed.GetID() != userID {
			continue
		}
		accessHash, hasAccessHash := typed.GetAccessHash()
		if !hasAccessHash {
			return 0, fmt.Errorf("resolved user %d has no access hash", userID)
		}
		return accessHash, nil
	}
	return 0, fmt.Errorf("resolved user %d not found", userID)
}

func findResolvedChannelAccessHash(chats []tg.ChatClass, channelID int64) (int64, error) {
	for _, chat := range chats {
		switch typed := chat.(type) {
		case *tg.Channel:
			if typed.GetID() != channelID {
				continue
			}
			accessHash, hasAccessHash := typed.GetAccessHash()
			if !hasAccessHash {
				return 0, fmt.Errorf("resolved channel %d has no access hash", channelID)
			}
			return accessHash, nil
		case *tg.ChannelForbidden:
			if typed.GetID() == channelID {
				return typed.GetAccessHash(), nil
			}
		}
	}
	return 0, fmt.Errorf("resolved channel %d not found", channelID)
}
