package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) AddMembers(ctx context.Context, cmd AddMembersCommand) (ConversationMutationResult, error) {
	actor := actorUser(cmd.Actor)
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	members, err := normalizeMemberIDs(cmd.MemberIDs, actor.ID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	apps, err := normalizeAppIDs(cmd.AppIDs)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	if len(members)+len(apps) == 0 {
		return ConversationMutationResult{}, invalidRequest("至少选择一名成员或应用", nil)
	}
	conversation, message, userIDs, err := s.addMembers(s.db, actor, conversationID, members, apps)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ConversationMutationResult{}, notFound("会话不存在", err)
		case errors.Is(err, ErrAccessDenied):
			return ConversationMutationResult{}, forbidden("无权访问会话", err)
		case errors.Is(err, ErrNotGroup):
			return ConversationMutationResult{}, invalidRequest("只能向群聊添加成员", err)
		case errors.Is(err, ErrMemberCap):
			return ConversationMutationResult{}, invalidRequest("群聊成员不能超过 500 人", err)
		case errors.Is(err, ErrGroupAppUnavailable):
			return ConversationMutationResult{}, invalidRequest("所选应用不存在、已停用或你无权访问", err)
		case errors.Is(err, ErrMemberMissing):
			return ConversationMutationResult{}, invalidRequest("成员不存在或已禁用", err)
		default:
			return ConversationMutationResult{}, internalError(err)
		}
	}
	item, err := s.loadItem(s.db, conversation, actor.ID)
	if err != nil {
		return ConversationMutationResult{}, internalError(err)
	}
	resultMessage := newOptionalMessage(message)
	if resultMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, *resultMessage)
	}
	return ConversationMutationResult{Conversation: item, Message: resultMessage}, nil
}

func (s *Service) addMembers(db *gorm.DB, actor store.User, conversationID string, memberIDs, appIDs []string) (store.Conversation, *store.Message, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	userIDs := []string{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return ErrAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return ErrNotGroup
		}
		var currentMember store.ConversationMember
		if err := tx.First(&currentMember, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		var existing []store.ConversationMember
		if err := tx.Where("conversation_id = ?", conversationID).Find(&existing).Error; err != nil {
			return err
		}
		activeCount := 0
		byID := make(map[string]store.ConversationMember, len(existing))
		for _, member := range existing {
			if member.LeftAt == nil {
				activeCount++
			}
			byID[memberKey(member.MemberType, member.MemberID)] = member
		}
		newMemberIDs, reactivatedMemberIDs, addedMemberIDs := classifyMembers(byID, store.ConversationMemberTypeUser, memberIDs)
		newAppIDs, reactivatedAppIDs, addedAppIDs := classifyMembers(byID, store.ConversationMemberTypeApp, appIDs)
		if len(addedMemberIDs)+len(addedAppIDs) == 0 {
			return nil
		}
		if activeCount+len(newMemberIDs)+len(reactivatedMemberIDs)+len(newAppIDs)+len(reactivatedAppIDs) > MaxGroupMembers {
			return ErrMemberCap
		}
		addedUsers, err := loadActiveGroupMembers(tx, addedMemberIDs)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberMissing
			}
			return err
		}
		addedApps, err := loadUserAccessibleGroupApps(tx, addedAppIDs, actor.ID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		systemMessageSeq := conversation.LastMessageSeq + 1
		if len(reactivatedMemberIDs) > 0 {
			if err := reactivateMembers(tx, conversationID, store.ConversationMemberTypeUser, reactivatedMemberIDs, now, systemMessageSeq, conversation.LastMessageSeq); err != nil {
				return err
			}
		}
		if len(reactivatedAppIDs) > 0 {
			if err := reactivateMembers(tx, conversationID, store.ConversationMemberTypeApp, reactivatedAppIDs, now, systemMessageSeq, conversation.LastMessageSeq); err != nil {
				return err
			}
		}
		usersByID := make(map[string]store.User, len(addedUsers))
		for _, user := range addedUsers {
			usersByID[user.ID] = user
		}
		appsByID := make(map[string]store.App, len(addedApps))
		for _, app := range addedApps {
			appsByID[app.ID] = app
		}
		newMembers := make([]store.ConversationMember, 0, len(newMemberIDs)+len(newAppIDs))
		for _, id := range newMemberIDs {
			user := usersByID[id]
			newMembers = append(newMembers, store.ConversationMember{ConversationID: conversationID, MemberType: store.ConversationMemberTypeUser, MemberID: user.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: systemMessageSeq, LastReadSeq: conversation.LastMessageSeq})
		}
		for _, id := range newAppIDs {
			app := appsByID[id]
			newMembers = append(newMembers, store.ConversationMember{ConversationID: conversationID, MemberType: store.ConversationMemberTypeApp, MemberID: app.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: systemMessageSeq, LastReadSeq: conversation.LastMessageSeq})
		}
		if len(newMembers) > 0 {
			if err := tx.Create(&newMembers).Error; err != nil {
				return err
			}
		}
		created, err := createGroupMembersInvitedSystemMessage(tx, &conversation, actor, makeInviteeRefs(addedUsers, addedApps), now)
		if err != nil {
			return err
		}
		message = &created
		if err := advanceReadSeq(tx, conversationID, actor.ID, created.Seq); err != nil {
			return err
		}
		activeUserIDs, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = activeUserIDs
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}
	return conversation, message, userIDs, nil
}

func classifyMembers(existing map[string]store.ConversationMember, memberType string, ids []string) (newIDs, reactivatedIDs, addedIDs []string) {
	newIDs = make([]string, 0, len(ids))
	reactivatedIDs = make([]string, 0, len(ids))
	addedIDs = make([]string, 0, len(ids))
	for _, id := range ids {
		member, ok := existing[memberKey(memberType, id)]
		if ok && member.LeftAt == nil {
			continue
		}
		addedIDs = append(addedIDs, id)
		if ok {
			reactivatedIDs = append(reactivatedIDs, id)
		} else {
			newIDs = append(newIDs, id)
		}
	}
	return
}

func reactivateMembers(tx *gorm.DB, conversationID, memberType string, ids []string, now time.Time, visibleFromSeq, lastReadSeq int64) error {
	return tx.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id IN ?", conversationID, memberType, ids).
		Updates(map[string]any{"role": store.ConversationMemberRoleMember, "joined_at": now, "history_visible_from_seq": visibleFromSeq, "left_at": nil, "last_read_seq": lastReadSeq}).Error
}

func (s *Service) RemoveMember(ctx context.Context, cmd RemoveMemberCommand) (ConversationMutationResult, error) {
	memberType := strings.TrimSpace(cmd.MemberType)
	if memberType != store.ConversationMemberTypeUser && memberType != store.ConversationMemberTypeApp {
		return ConversationMutationResult{}, invalidRequest("成员类型格式错误", nil)
	}
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	memberID, err := normalizeUUID(cmd.MemberID, "成员 ID 格式错误")
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	conversation, message, userIDs, removedUserID, removedTopicIDs, err := s.removeMember(s.db, actor, conversationID, memberType, memberID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ConversationMutationResult{}, notFound("会话不存在或成员不在群聊中", err)
		case errors.Is(err, ErrCannotRemoveSelf):
			return ConversationMutationResult{}, forbidden("不能移出自己", err)
		case errors.Is(err, ErrOwnerCannotRemove):
			return ConversationMutationResult{}, forbidden("群主不能被移出群聊", err)
		case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrNotGroup):
			return ConversationMutationResult{}, forbidden("无权操作群聊", err)
		default:
			return ConversationMutationResult{}, internalError(err)
		}
	}
	item, err := s.loadItem(s.db, conversation, actor.ID)
	if err != nil {
		return ConversationMutationResult{}, internalError(err)
	}
	resultMessage := newOptionalMessage(message)
	if resultMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, *resultMessage)
	}
	if removedUserID != "" && s.notifications != nil {
		s.notifications.PublishConversationRemoved(ctx, []string{removedUserID}, conversation.ID)
		for _, topicID := range removedTopicIDs {
			s.notifications.PublishConversationRemoved(ctx, []string{removedUserID}, topicID)
		}
	}
	return ConversationMutationResult{Conversation: item, Message: resultMessage}, nil
}

func (s *Service) removeMember(db *gorm.DB, actor store.User, conversationID, memberType, memberID string) (store.Conversation, *store.Message, []string, string, []string, error) {
	var conversation store.Conversation
	var message *store.Message
	userIDs := []string{}
	removedUserID := ""
	removedTopicIDs := []string{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return ErrAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return ErrNotGroup
		}
		if memberType == store.ConversationMemberTypeUser && actor.ID == memberID {
			return ErrCannotRemoveSelf
		}
		var current store.ConversationMember
		if err := tx.First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		if !canManage(current.Role) {
			return ErrAccessDenied
		}
		var target store.ConversationMember
		if err := tx.First(&target, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, memberType, memberID).Error; err != nil {
			return err
		}
		if target.Role == store.ConversationMemberRoleOwner {
			return ErrOwnerCannotRemove
		}
		targetRef, err := loadMemberSystemRef(tx, memberType, memberID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationMember{}).Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, memberType, memberID).Updates(map[string]any{"left_at": now}).Error; err != nil {
			return err
		}
		ids, err := removeParentMemberTopicParticipations(tx, conversationID, memberType, memberID)
		if err != nil {
			return err
		}
		removedTopicIDs = ids
		created, err := createGroupMemberRemovedSystemMessage(tx, &conversation, actor, targetRef, now)
		if err != nil {
			return err
		}
		message = &created
		if err := advanceReadSeq(tx, conversationID, actor.ID, created.Seq); err != nil {
			return err
		}
		activeUserIDs, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = activeUserIDs
		if memberType == store.ConversationMemberTypeUser {
			removedUserID = memberID
		}
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, "", nil, err
	}
	return conversation, message, userIDs, removedUserID, removedTopicIDs, nil
}
