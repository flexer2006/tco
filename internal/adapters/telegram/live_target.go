package telegram

import (
	"context"
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
		return liveTarget{}, ErrSourceChatEmpty
	}

	if after, ok := strings.CutPrefix(trimmed, "@"); ok {
		username := strings.TrimSpace(after)
		if username == "" {
			return liveTarget{}, ErrUsernameEmpty
		}

		return liveTarget{kind: liveTargetKindUsername, username: username}, nil
	}

	prefix, payload, hasPrefix := strings.Cut(trimmed, ":")
	if hasPrefix {
		switch strings.ToLower(strings.TrimSpace(prefix)) {
		case "username":
			username := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(payload), "@"))
			if username == "" {
				return liveTarget{}, ErrUsernameEmpty
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
		return liveTarget{}, ErrChatIDEmpty
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
		return liveTarget{}, ErrChatIDNotPositive
	}

	return liveTarget{kind: liveTargetKindChannelPeerID, id: channelID}, nil
}

func parseLiveChannelIDFromPeerID(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "-100") {
		return 0, ErrChannelPeerIDPrefix
	}

	channelIDRaw := strings.TrimSpace(strings.TrimPrefix(trimmed, "-100"))
	if channelIDRaw == "" {
		return 0, ErrChannelPeerIDSuffix
	}

	channelID, err := strconv.ParseInt(channelIDRaw, 10, 64)
	if err != nil {
		return 0, err
	}

	if channelID <= 0 {
		return 0, ErrChannelIDNotPositive
	}

	return channelID, nil
}

func parseLiveTargetWithAccessHash(kind liveTargetKind, payload string) (liveTarget, error) {
	idRaw, accessHashRaw, ok := strings.Cut(strings.TrimSpace(payload), ":")
	if !ok {
		return liveTarget{}, fmt.Errorf(
			"%w: %s target must be in form %s:<id>:<access_hash>",
			ErrInvalidAccessHashTargetForm,
			kind,
			kind,
		)
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
		return 0, ErrValueNotPositive
	}

	return parsed, nil
}

func resolveLiveInputPeer(
	ctx context.Context,
	api liveAPI,
	target liveTarget,
) (tg.InputPeerClass, error) {
	switch target.kind {
	case liveTargetKindChat:
		return new(tg.InputPeerChat{ChatID: target.id}), nil
	case liveTargetKindUser:
		return new(tg.InputPeerUser{UserID: target.id, AccessHash: target.accessHash}), nil
	case liveTargetKindChannel:
		return new(tg.InputPeerChannel{ChannelID: target.id, AccessHash: target.accessHash}), nil
	case liveTargetKindChannelPeerID:
		return resolveLiveChannelPeerID(ctx, api, target.id)
	case liveTargetKindUsername:
		return resolveLiveUsername(ctx, api, target.username)
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedLiveTargetKind, target.kind)
	}
}

func resolveLiveChannelPeerID(
	ctx context.Context,
	api liveAPI,
	channelID int64,
) (tg.InputPeerClass, error) {
	dialogs, err := api.MessagesGetDialogs(ctx, new(tg.MessagesGetDialogsRequest{
		OffsetPeer: new(tg.InputPeerEmpty{}),
		Limit:      defaultLiveHistoryPageSize,
	}))
	if err != nil {
		return nil, err
	}

	accessHash, err := findResolvedChannelAccessHash(extractLiveDialogsChats(dialogs), channelID)
	if err != nil {
		return nil, fmt.Errorf("resolve channel access hash for %d: %w", channelID, err)
	}

	return new(tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}), nil
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

func resolveLiveUsername(
	ctx context.Context,
	api liveAPI,
	username string,
) (tg.InputPeerClass, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if trimmed == "" {
		return nil, ErrUsernameEmpty
	}

	resolved, err := api.ContactsResolveUsername(
		ctx,
		new(tg.ContactsResolveUsernameRequest{Username: trimmed}),
	)
	if err != nil {
		return nil, err
	}

	if resolved == nil {
		return nil, ErrResolveUsernameNilResponse
	}

	peer := resolved.GetPeer()
	switch typed := peer.(type) {
	case *tg.PeerUser:
		accessHash, err := findResolvedUserAccessHash(resolved.GetUsers(), typed.GetUserID())
		if err != nil {
			return nil, err
		}

		return new(tg.InputPeerUser{UserID: typed.GetUserID(), AccessHash: accessHash}), nil
	case *tg.PeerChat:
		return new(tg.InputPeerChat{ChatID: typed.GetChatID()}), nil
	case *tg.PeerChannel:
		accessHash, err := findResolvedChannelAccessHash(resolved.GetChats(), typed.GetChannelID())
		if err != nil {
			return nil, err
		}

		return new(tg.InputPeerChannel{ChannelID: typed.GetChannelID(), AccessHash: accessHash}), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedResolvedPeerType, typed)
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
			return 0, fmt.Errorf("%w: %d", ErrResolvedUserNoAccessHash, userID)
		}

		return accessHash, nil
	}

	return 0, fmt.Errorf("%w: %d", ErrResolvedUserNotFound, userID)
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
				return 0, fmt.Errorf("%w: %d", ErrResolvedChannelNoAccessHash, channelID)
			}

			return accessHash, nil
		case *tg.ChannelForbidden:
			if typed.GetID() == channelID {
				return typed.GetAccessHash(), nil
			}
		}
	}

	return 0, fmt.Errorf("%w: %d", ErrResolvedChannelNotFound, channelID)
}
