package message

import (
	"context"
	"errors"
	"sort"
	"strings"

	"app/internal/store"

	"gorm.io/gorm"
)

func (s *Service) AuthorizeAppConversation(ctx context.Context, cmd AppConversationAccessCommand) (AppConversationAccess, error) {
	db := s.db.WithContext(ctx)
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", cmd.ConversationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AppConversationAccess{}, NotFoundError("会话不存在", err)
		}
		return AppConversationAccess{}, internalError(err)
	}
	if conversation.Status != store.ConversationStatusActive {
		return AppConversationAccess{}, forbidden("无权访问会话", errConversationAccessDenied)
	}
	var member store.ConversationMember
	err := db.First(
		&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		cmd.ConversationID, store.ConversationMemberTypeApp, cmd.AppID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AppConversationAccess{}, forbidden("无权访问会话", errConversationAccessDenied)
	}
	if err != nil {
		return AppConversationAccess{}, internalError(err)
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	return AppConversationAccess{HistoryVisibleFromSeq: visibleFromSeq}, nil
}

func (s *Service) AuthorizeRunAsTrigger(ctx context.Context, cmd RunAsTriggerCommand) error {
	db := s.db.WithContext(ctx)
	var conversationID string
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		err := applyOnlineStoredMessageWindow(db).Select("conversation_id").First(
			&registry,
			"id = ? AND sender_type = ? AND sender_id = ? AND deleted_at IS NULL",
			cmd.TriggerMessageID, cmd.ActorType, cmd.ActorID,
		).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden("触发消息无效", err)
		}
		if err != nil {
			return internalError(err)
		}
		conversationID = registry.ConversationID
	} else {
		var trigger store.Message
		err := applyOnlineStoredMessageWindow(db).Select("conversation_id").First(
			&trigger,
			"id = ? AND sender_type = ? AND sender_id = ? AND deleted_at IS NULL",
			cmd.TriggerMessageID, cmd.ActorType, cmd.ActorID,
		).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden("触发消息无效", err)
		}
		if err != nil {
			return internalError(err)
		}
		conversationID = trigger.ConversationID
	}
	if cmd.AuthorizationConversationID != "" && conversationID != cmd.AuthorizationConversationID {
		return forbidden("触发消息无效", errConversationAccessDenied)
	}
	_, err := s.AuthorizeAppConversation(ctx, AppConversationAccessCommand{
		AppID: cmd.AppID, ConversationID: conversationID,
	})
	return err
}

func (s *Service) ListForApp(ctx context.Context, cmd ListForAppCommand) (ListForAppResult, error) {
	visibleFromSeq := cmd.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	beforeOrEqualSeq := cmd.BeforeOrEqualSeq
	messages, err := s.loadStoredMessagePage(ctx, s.db, storedMessagePageQuery{
		ConversationID: cmd.ConversationID, VisibleFromSeq: visibleFromSeq,
		BeforeOrEqualSeq: &beforeOrEqualSeq, Limit: cmd.Limit, Descending: true,
	})
	if err != nil {
		return ListForAppResult{}, internalError(err)
	}
	reverseStoredMessages(messages)
	payloads, err := s.newAppHistoryMessages(ctx, messages)
	if err != nil {
		return ListForAppResult{}, internalError(err)
	}
	return ListForAppResult{Messages: payloads}, nil
}

func (s *Service) ReadForUser(ctx context.Context, cmd ReadForUserCommand) (ReadForUserResult, error) {
	db := s.db.WithContext(ctx)
	conversation, err := findAppHistoryConversationForUser(db, cmd)
	if err != nil {
		return ReadForUserResult{}, mapAppHistoryError(err)
	}
	member, err := requireReadableConversationMember(db, cmd.AccountID, conversation.ID)
	if err != nil {
		return ReadForUserResult{}, mapAppHistoryError(err)
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	pageQuery := storedMessagePageQuery{
		ConversationID: conversation.ID, VisibleFromSeq: visibleFromSeq,
		Limit: cmd.Limit, Descending: true,
	}
	if cmd.BeforeSeq > 0 {
		pageQuery.BeforeSeq = &cmd.BeforeSeq
	}
	messages, err := s.loadStoredMessagePage(ctx, db, pageQuery)
	if err != nil {
		return ReadForUserResult{}, internalError(err)
	}
	reverseStoredMessages(messages)
	payloads, err := s.newAppHistoryMessages(ctx, messages)
	if err != nil {
		return ReadForUserResult{}, internalError(err)
	}
	summary, err := newAppConversationSummary(db, conversation, cmd.AccountID)
	if err != nil {
		return ReadForUserResult{}, internalError(err)
	}
	return ReadForUserResult{Conversation: summary, Messages: payloads}, nil
}

func findAppHistoryConversationForUser(db *gorm.DB, cmd ReadForUserCommand) (store.Conversation, error) {
	switch {
	case cmd.ConversationID != "":
		var conversation store.Conversation
		if err := db.First(&conversation, "id = ?", cmd.ConversationID).Error; err != nil {
			return store.Conversation{}, err
		}
		return conversation, nil
	case cmd.UserID != "":
		ids := []string{strings.TrimSpace(cmd.AccountID), strings.TrimSpace(cmd.UserID)}
		sort.Strings(ids)
		var direct store.DirectConversation
		if err := db.First(&direct, "user_low_id = ? AND user_high_id = ?", ids[0], ids[1]).Error; err != nil {
			return store.Conversation{}, err
		}
		var conversation store.Conversation
		if err := db.First(&conversation, "id = ?", direct.ConversationID).Error; err != nil {
			return store.Conversation{}, err
		}
		return conversation, nil
	case cmd.AppID != "":
		var relation store.AppConversation
		if err := db.First(&relation, "app_id = ? AND user_id = ?", cmd.AppID, cmd.AccountID).Error; err != nil {
			return store.Conversation{}, err
		}
		var conversation store.Conversation
		if err := db.First(&conversation, "id = ?", relation.ConversationID).Error; err != nil {
			return store.Conversation{}, err
		}
		return conversation, nil
	default:
		return store.Conversation{}, errors.New("conversation selector is required")
	}
}

func (s *Service) newAppHistoryMessages(ctx context.Context, messages []store.Message) ([]AppHistoryMessage, error) {
	userIDs := make([]string, 0)
	appIDs := make([]string, 0)
	for _, message := range messages {
		if message.SenderID == nil {
			continue
		}
		switch message.SenderType {
		case store.MessageSenderTypeUser:
			userIDs = append(userIDs, *message.SenderID)
		case store.MessageSenderTypeApp:
			appIDs = append(appIDs, *message.SenderID)
		}
	}
	db := s.db.WithContext(ctx)
	usersByID := map[string]store.User{}
	if len(userIDs) > 0 {
		var users []store.User
		if err := db.Find(&users, "id IN ?", userIDs).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			usersByID[user.ID] = user
		}
	}
	appsByID := map[string]store.App{}
	if len(appIDs) > 0 {
		var apps []store.App
		if err := db.Find(&apps, "id IN ?", appIDs).Error; err != nil {
			return nil, err
		}
		for _, app := range apps {
			appsByID[app.ID] = app
		}
	}
	result := make([]AppHistoryMessage, 0, len(messages))
	for _, message := range messages {
		summary := message.Summary
		body := message.Body
		if message.RevokedAt != nil {
			summary = "该消息已被撤回"
			body = nil
		}
		result = append(result, AppHistoryMessage{
			Body: body, CreatedAt: message.CreatedAt, ID: message.ID,
			Sender: appHistorySender(message, usersByID, appsByID), Seq: message.Seq, Summary: summary,
		})
	}
	return result, nil
}

func appHistorySender(message store.Message, users map[string]store.User, apps map[string]store.App) Identity {
	sender := Identity{Type: message.SenderType}
	if message.SenderID == nil {
		return sender
	}
	sender.ID = *message.SenderID
	switch message.SenderType {
	case store.MessageSenderTypeUser:
		if user, ok := users[*message.SenderID]; ok {
			sender.Email = user.Email
			sender.Name = user.Name
			sender.Nickname = user.Nickname
		}
	case store.MessageSenderTypeApp:
		if app, ok := apps[*message.SenderID]; ok {
			sender.Name = app.Name
		}
	}
	return sender
}

func newAppConversationSummary(db *gorm.DB, conversation store.Conversation, currentUserID string) (AppConversationSummary, error) {
	var members []store.ConversationMember
	if err := db.Where("conversation_id = ? AND left_at IS NULL", conversation.ID).
		Order("conversation_id ASC").Order("joined_at ASC").Find(&members).Error; err != nil {
		return AppConversationSummary{}, err
	}
	name := conversation.Name
	if conversation.Kind == store.ConversationKindDirect {
		for _, member := range members {
			if member.MemberID == currentUserID || member.MemberType != store.ConversationMemberTypeUser {
				continue
			}
			var user store.User
			if err := db.First(&user, "id = ?", member.MemberID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return AppConversationSummary{}, err
			}
			name = strings.TrimSpace(user.Nickname)
			if name == "" {
				name = strings.TrimSpace(user.Name)
			}
			break
		}
		if strings.TrimSpace(name) == "" {
			name = "私聊"
		}
	} else if strings.TrimSpace(name) == "" {
		name = "群聊"
	}
	lastActiveAt := conversation.CreatedAt
	if conversation.LastMessageAt != nil {
		lastActiveAt = *conversation.LastMessageAt
	}
	return AppConversationSummary{
		ConversationID: conversation.ID, LastActiveAt: lastActiveAt,
		MemberCount: len(members), Name: name, Type: conversation.Kind,
	}, nil
}

func mapAppHistoryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotFoundError("会话不存在", err)
	}
	if errors.Is(err, errConversationAccessDenied) {
		return forbidden("无权访问会话", err)
	}
	return internalError(err)
}
