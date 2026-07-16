package message

import (
	"context"
	"errors"

	"app/internal/store"

	"gorm.io/gorm"
)

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	if cmd.BeforeSeq != nil && cmd.AfterSeq != nil {
		return ListResult{}, InvalidRequestError("before_seq 和 after_seq 不能同时传", nil)
	}
	limit := cmd.Limit
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}
	if limit > MaxHistoryLimit {
		limit = MaxHistoryLimit
	}
	db := s.db.WithContext(ctx)
	member, err := requireReadableConversationMember(db, cmd.AccountID, cmd.ConversationID)
	if err != nil {
		return ListResult{}, mapHistoryAccessError(err)
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	needsReverse := false
	pageQuery := storedMessagePageQuery{
		ConversationID: cmd.ConversationID, VisibleFromSeq: visibleFromSeq, Limit: limit,
	}
	if cmd.BeforeSeq != nil {
		pageQuery.BeforeSeq = cmd.BeforeSeq
		pageQuery.Descending = true
		needsReverse = true
	} else if cmd.AfterSeq != nil {
		pageQuery.AfterSeq = cmd.AfterSeq
	} else {
		pageQuery.Descending = true
		needsReverse = true
	}
	stored, err := s.loadStoredMessagePage(ctx, db, pageQuery)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	if needsReverse {
		reverseStoredMessages(stored)
	}
	hasMoreBefore, hasMoreAfter, err := visibleMessagePageBounds(db, cmd.ConversationID, visibleFromSeq, stored)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	messages, err := newMessagesForUser(db, stored, visibleFromSeq)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	page := Page{HasMoreAfter: hasMoreAfter, HasMoreBefore: hasMoreBefore, Limit: limit}
	if len(stored) > 0 {
		page.OldestSeq = stored[0].Seq
		page.NewestSeq = stored[len(stored)-1].Seq
	}
	return ListResult{Messages: messages, Page: page}, nil
}

func newMessagesForUser(db *gorm.DB, values []store.Message, visibleFromSeq int64) ([]Message, error) {
	result := make([]Message, len(values))
	replyIDs := make([]string, 0, len(values))
	for index, value := range values {
		result[index] = newMessage(value)
		if value.RevokedAt == nil && value.ReplyToMessageID != nil {
			replyIDs = append(replyIDs, *value.ReplyToMessageID)
		}
	}
	if len(replyIDs) == 0 || len(values) == 0 {
		return result, nil
	}
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	conversationID := values[0].ConversationID
	quotedByID := make(map[string]store.Message, len(replyIDs))
	if store.MessagePartitioningEnabled(db) {
		var registries []store.MessageRegistry
		if err := applyOnlineStoredMessageWindow(db).Where(
			"conversation_id = ? AND id IN ? AND deleted_at IS NULL AND seq >= ?",
			conversationID, replyIDs, visibleFromSeq,
		).Find(&registries).Error; err != nil {
			return nil, err
		}
		for _, registry := range registries {
			quotedByID[registry.ID] = store.Message{
				ID: registry.ID, ConversationID: registry.ConversationID, Seq: registry.Seq,
				SenderType: registry.SenderType, SenderID: registry.SenderID,
				Summary: registry.Summary, RevokedAt: registry.RevokedAt,
			}
		}
	} else {
		var quoted []store.Message
		if err := applyOnlineStoredMessageWindow(db).Where(
			"conversation_id = ? AND id IN ? AND deleted_at IS NULL AND seq >= ?",
			conversationID, replyIDs, visibleFromSeq,
		).Find(&quoted).Error; err != nil {
			return nil, err
		}
		for _, message := range quoted {
			quotedByID[message.ID] = message
		}
	}

	userIDs := make([]string, 0)
	appIDs := make([]string, 0)
	for _, quoted := range quotedByID {
		if quoted.SenderID == nil {
			continue
		}
		switch quoted.SenderType {
		case store.MessageSenderTypeUser:
			userIDs = append(userIDs, *quoted.SenderID)
		case store.MessageSenderTypeApp:
			appIDs = append(appIDs, *quoted.SenderID)
		}
	}
	senderNames := make(map[string]string, len(userIDs)+len(appIDs))
	if len(userIDs) > 0 {
		var users []store.User
		if err := db.Select("id", "name").Find(&users, "id IN ?", userIDs).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			senderNames[store.MessageSenderTypeUser+"/"+user.ID] = user.Name
		}
		for _, quoted := range quotedByID {
			if quoted.SenderType != store.MessageSenderTypeUser {
				continue
			}
			senderID := ""
			if quoted.SenderID != nil {
				senderID = *quoted.SenderID
			}
			if _, ok := senderNames[store.MessageSenderTypeUser+"/"+senderID]; !ok {
				return nil, gorm.ErrRecordNotFound
			}
		}
	}
	if len(appIDs) > 0 {
		var apps []store.App
		if err := db.Unscoped().Select("id", "name").Find(&apps, "id IN ?", appIDs).Error; err != nil {
			return nil, err
		}
		for _, app := range apps {
			name := app.Name
			if name == "" {
				name = "应用"
			}
			senderNames[store.MessageSenderTypeApp+"/"+app.ID] = name
		}
	}

	for index, value := range values {
		if value.RevokedAt != nil || value.ReplyToMessageID == nil {
			continue
		}
		quoted, ok := quotedByID[*value.ReplyToMessageID]
		if !ok {
			continue
		}
		senderID := ""
		senderName := ""
		if quoted.SenderID != nil {
			senderID = *quoted.SenderID
			senderName = senderNames[quoted.SenderType+"/"+senderID]
		}
		switch quoted.SenderType {
		case store.MessageSenderTypeApp:
			if senderName == "" {
				senderName = "应用"
			}
		case store.MessageSenderTypeSystem:
			senderName = "系统"
		}
		summary := quoted.Summary
		if quoted.RevokedAt != nil {
			summary = "该消息已被撤回"
		}
		result[index].ReplyTo = &Reply{
			ID: quoted.ID, Sender: Identity{ID: senderID, Name: senderName, Type: quoted.SenderType},
			Seq: quoted.Seq, Summary: summary,
		}
	}
	return result, nil
}

func requireReadableConversationMember(db *gorm.DB, userID, conversationID string) (store.ConversationMember, error) {
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.ConversationMember{}, err
	}
	if conversation.Status != store.ConversationStatusActive {
		return store.ConversationMember{}, errConversationAccessDenied
	}
	var member store.ConversationMember
	if err := db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeUser, userID,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ConversationMember{}, errConversationAccessDenied
		}
		return store.ConversationMember{}, err
	}
	return member, nil
}

func visibleMessagePageBounds(db *gorm.DB, conversationID string, visibleFromSeq int64, messages []store.Message) (bool, bool, error) {
	if len(messages) == 0 {
		return false, false, nil
	}
	oldestSeq := messages[0].Seq
	newestSeq := messages[len(messages)-1].Seq
	hasMoreBefore, err := visibleMessageExists(db, conversationID, visibleFromSeq, "seq < ?", oldestSeq)
	if err != nil {
		return false, false, err
	}
	hasMoreAfter, err := visibleMessageExists(db, conversationID, visibleFromSeq, "seq > ?", newestSeq)
	if err != nil {
		return false, false, err
	}
	return hasMoreBefore, hasMoreAfter, nil
}

func visibleMessageExists(db *gorm.DB, conversationID string, visibleFromSeq int64, condition string, args ...any) (bool, error) {
	model := any(&store.Message{})
	if store.MessagePartitioningEnabled(db) {
		model = &store.MessageRegistry{}
	}
	var value struct{ ID string }
	result := db.Model(model).Select("id").
		Scopes(applyOnlineStoredMessageWindow).
		Where("conversation_id = ? AND deleted_at IS NULL AND seq >= ?", conversationID, visibleFromSeq).
		Where(condition, args...).Limit(1).Find(&value)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func reverseStoredMessages(messages []store.Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func newMessage(value store.Message) Message {
	senderID := ""
	if value.SenderID != nil {
		senderID = *value.SenderID
	}
	clientMessageID := ""
	if value.ClientMessageID != nil {
		clientMessageID = *value.ClientMessageID
	}
	replyToMessageID := ""
	if value.ReplyToMessageID != nil {
		replyToMessageID = *value.ReplyToMessageID
	}
	revokedByUserID := ""
	if value.RevokedByUserID != nil {
		revokedByUserID = *value.RevokedByUserID
	}
	var delegatedBy *Identity
	if value.DelegatedByType != nil && value.DelegatedByID != nil {
		delegatedBy = &Identity{ID: *value.DelegatedByID, Name: value.DelegatedByName, Type: *value.DelegatedByType}
	}
	result := Message{
		ClientMessageID: clientMessageID, ConversationID: value.ConversationID,
		CreatedAt: value.CreatedAt, DelegatedBy: delegatedBy, ID: value.ID,
		ReplyToMessageID: replyToMessageID, RevokedAt: value.RevokedAt,
		RevokedByUserID: revokedByUserID, Sender: Identity{ID: senderID, Type: value.SenderType},
		Seq: value.Seq, Summary: value.Summary,
	}
	if value.RevokedAt == nil {
		result.Body = value.Body
	}
	return result
}

func newMessageForUser(db *gorm.DB, value store.Message, userID string) (Message, error) {
	result := newMessage(value)
	if value.RevokedAt != nil || value.ReplyToMessageID == nil {
		return result, nil
	}
	quoted, ok, err := findVisibleReplyToMessageForUser(db, value.ConversationID, *value.ReplyToMessageID, userID)
	if err != nil || !ok {
		return result, err
	}
	senderID := ""
	if quoted.SenderID != nil {
		senderID = *quoted.SenderID
	}
	senderName, err := messageSenderName(db, quoted.SenderType, senderID)
	if err != nil {
		return Message{}, err
	}
	summary := quoted.Summary
	if quoted.RevokedAt != nil {
		summary = "该消息已被撤回"
	}
	result.ReplyTo = &Reply{
		ID: quoted.ID, Sender: Identity{ID: senderID, Name: senderName, Type: quoted.SenderType},
		Seq: quoted.Seq, Summary: summary,
	}
	return result, nil
}

func findVisibleReplyToMessageForUser(db *gorm.DB, conversationID, messageID, userID string) (store.Message, bool, error) {
	member, err := requireReadableConversationMember(db, userID, conversationID)
	if err != nil {
		return store.Message{}, false, err
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		err = applyOnlineStoredMessageWindow(db).Where(
			"id = ? AND conversation_id = ? AND deleted_at IS NULL AND seq >= ?",
			messageID, conversationID, visibleFromSeq,
		).Limit(1).Take(&registry).Error
		if err == nil {
			value, loadErr := store.LoadMessageByRegistry(messageStorageContext(db), db, registry)
			return value, loadErr == nil, loadErr
		}
	} else {
		var value store.Message
		err = applyOnlineStoredMessageWindow(db).Where(
			"id = ? AND conversation_id = ? AND deleted_at IS NULL AND seq >= ?",
			messageID, conversationID, visibleFromSeq,
		).Limit(1).Take(&value).Error
		if err == nil {
			return value, true, nil
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Message{}, false, nil
	}
	return store.Message{}, false, err
}

func messageSenderName(db *gorm.DB, senderType, senderID string) (string, error) {
	switch senderType {
	case store.MessageSenderTypeUser:
		var user store.User
		if err := db.Select("id", "name").First(&user, "id = ?", senderID).Error; err != nil {
			return "", err
		}
		return user.Name, nil
	case store.MessageSenderTypeApp:
		var app store.App
		err := db.Unscoped().Select("id", "name").First(&app, "id = ?", senderID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "应用", nil
		}
		if err != nil {
			return "", err
		}
		if app.Name == "" {
			return "应用", nil
		}
		return app.Name, nil
	case store.MessageSenderTypeSystem:
		return "系统", nil
	default:
		return "", nil
	}
}

func mapHistoryAccessError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotFoundError("会话不存在", err)
	}
	if errors.Is(err, errConversationAccessDenied) {
		return forbidden("无权访问会话", err)
	}
	return internalError(err)
}
