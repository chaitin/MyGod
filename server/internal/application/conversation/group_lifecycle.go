package conversation

import (
	"context"
	"errors"
	"sort"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxProjectDissolveAttempts = 3

func (s *Service) UpdateName(ctx context.Context, cmd UpdateNameCommand) (ConversationMutationResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	name, err := normalizeGroupName(cmd.Name)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	conversation, message, userIDs, err := s.updateName(s.db, actor, conversationID, name)
	if err != nil {
		return ConversationMutationResult{}, mapManagedGroupError(err)
	}
	return s.finishMutation(ctx, actor.ID, conversation, message, userIDs)
}

func (s *Service) UpdateVisibility(ctx context.Context, cmd UpdateVisibilityCommand) (ConversationMutationResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	conversation, message, userIDs, err := s.updateVisibility(s.db, actor, conversationID, cmd.Visibility)
	if err != nil {
		return ConversationMutationResult{}, mapManagedGroupError(err)
	}
	return s.finishMutation(ctx, actor.ID, conversation, message, userIDs)
}

func mapManagedGroupError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return notFound("会话不存在", err)
	case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrNotGroup):
		return forbidden("无权操作群聊", err)
	default:
		return internalError(err)
	}
}

func (s *Service) finishMutation(ctx context.Context, actorID string, conversation store.Conversation, message *store.Message, userIDs []string) (ConversationMutationResult, error) {
	item, err := s.loadItem(s.db, conversation, actorID)
	if err != nil {
		return ConversationMutationResult{}, internalError(err)
	}
	resultMessage := newOptionalMessage(message)
	if resultMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, *resultMessage)
	}
	return ConversationMutationResult{Conversation: item, Message: resultMessage}, nil
}

func (s *Service) updateName(db *gorm.DB, actor store.User, conversationID, name string) (store.Conversation, *store.Message, []string, error) {
	return s.updateGroupField(db, actor, conversationID, name, false)
}

func (s *Service) updateVisibility(db *gorm.DB, actor store.User, conversationID, visibility string) (store.Conversation, *store.Message, []string, error) {
	return s.updateGroupField(db, actor, conversationID, visibility, true)
}

func (s *Service) updateGroupField(db *gorm.DB, actor store.User, conversationID, value string, visibility bool) (store.Conversation, *store.Message, []string, error) {
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
		var current store.ConversationMember
		if err := tx.First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		if visibility {
			if !canSetVisibility(current.Role) {
				return ErrAccessDenied
			}
			if conversation.Visibility == value {
				ids, err := loadActiveUserIDs(tx, conversationID)
				if err != nil {
					return err
				}
				userIDs = ids
				return nil
			}
		} else {
			if !canManage(current.Role) {
				return ErrAccessDenied
			}
			if conversation.Name == value {
				ids, err := loadActiveUserIDs(tx, conversationID)
				if err != nil {
					return err
				}
				userIDs = ids
				return nil
			}
		}
		now := s.now().UTC()
		updates := map[string]any{"updated_at": now}
		if visibility {
			updates["visibility"] = value
		} else {
			updates["name"] = value
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(updates).Error; err != nil {
			return err
		}
		conversation.UpdatedAt = now
		var created store.Message
		var err error
		if visibility {
			conversation.Visibility = value
			created, err = createGroupVisibilityChangedSystemMessage(tx, &conversation, actor, value, now)
		} else {
			conversation.Name = value
			created, err = createGroupNameUpdatedSystemMessage(tx, &conversation, actor, value, now)
		}
		if err != nil {
			return err
		}
		message = &created
		if err := advanceReadSeq(tx, conversationID, actor.ID, created.Seq); err != nil {
			return err
		}
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}
	return conversation, message, userIDs, nil
}

func (s *Service) Join(ctx context.Context, cmd JoinCommand) (ConversationMutationResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return ConversationMutationResult{}, invalidRequest(err.Error(), err)
	}
	actor := actorUser(cmd.Actor)
	conversation, message, userIDs, err := s.join(s.db, actor, conversationID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ConversationMutationResult{}, notFound("会话不存在", err)
		case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrNotGroup):
			return ConversationMutationResult{}, forbidden("无权加入群聊", err)
		case errors.Is(err, ErrMemberCap):
			return ConversationMutationResult{}, invalidRequest("群聊成员不能超过 500 人", err)
		default:
			return ConversationMutationResult{}, internalError(err)
		}
	}
	return s.finishMutation(ctx, actor.ID, conversation, message, userIDs)
}

func (s *Service) join(db *gorm.DB, actor store.User, conversationID string) (store.Conversation, *store.Message, []string, error) {
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
		if conversation.Visibility != store.ConversationVisibilityPublic {
			return ErrAccessDenied
		}
		var members []store.ConversationMember
		if err := tx.Where("conversation_id = ? AND member_type = ?", conversationID, store.ConversationMemberTypeUser).Find(&members).Error; err != nil {
			return err
		}
		activeCount := 0
		var existing *store.ConversationMember
		for index := range members {
			member := &members[index]
			if member.LeftAt == nil {
				activeCount++
			}
			if member.MemberID == actor.ID {
				existing = member
			}
		}
		if existing != nil && existing.LeftAt == nil {
			ids, err := loadActiveUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			userIDs = ids
			return nil
		}
		if activeCount >= MaxGroupMembers {
			return ErrMemberCap
		}
		now := s.now().UTC()
		messageSeq := conversation.LastMessageSeq + 1
		if existing != nil {
			if err := tx.Model(&store.ConversationMember{}).Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, actor.ID).
				Updates(map[string]any{"role": store.ConversationMemberRoleMember, "joined_at": now, "history_visible_from_seq": messageSeq, "left_at": nil, "last_read_seq": messageSeq}).Error; err != nil {
				return err
			}
		} else {
			member := store.ConversationMember{ConversationID: conversationID, MemberType: store.ConversationMemberTypeUser, MemberID: actor.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: messageSeq, LastReadSeq: messageSeq}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		created, err := createGroupMemberJoinedSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		message = &created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return store.Conversation{}, nil, nil, err
	}
	return conversation, message, userIDs, nil
}

func (s *Service) Leave(ctx context.Context, cmd LeaveCommand) (LeaveResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return LeaveResult{}, invalidRequest(err.Error(), err)
	}
	message, userIDs, err := s.leave(s.db, actorUser(cmd.Actor), conversationID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return LeaveResult{}, notFound("会话不存在", err)
		case errors.Is(err, ErrOwnerCannotLeave):
			return LeaveResult{}, forbidden("群主不能退出群聊", err)
		case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrNotGroup):
			return LeaveResult{}, forbidden("无权操作群聊", err)
		default:
			return LeaveResult{}, internalError(err)
		}
	}
	resultMessage := newMessage(message)
	if s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, resultMessage)
	}
	return LeaveResult{ConversationID: conversationID, Message: resultMessage}, nil
}

func (s *Service) leave(db *gorm.DB, actor store.User, conversationID string) (store.Message, []string, error) {
	var message store.Message
	userIDs := []string{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return ErrAccessDenied
		}
		if conversation.Kind != store.ConversationKindGroup {
			return ErrNotGroup
		}
		var current store.ConversationMember
		if err := tx.First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessDenied
			}
			return err
		}
		if current.Role == store.ConversationMemberRoleOwner {
			return ErrOwnerCannotLeave
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationMember{}).Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, actor.ID).Updates(map[string]any{"left_at": now}).Error; err != nil {
			return err
		}
		created, err := createGroupMemberLeftSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		message = created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	return message, userIDs, err
}

func (s *Service) Dissolve(ctx context.Context, cmd DissolveCommand) (DissolveResult, error) {
	conversationID, err := normalizeConversationID(cmd.ConversationID)
	if err != nil {
		return DissolveResult{}, invalidRequest(err.Error(), err)
	}
	userIDs, err := s.dissolve(ctx, actorUser(cmd.Actor), conversationID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return DissolveResult{}, notFound("会话不存在", err)
		case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrNotGroup):
			return DissolveResult{}, forbidden("无权操作群聊", err)
		case errors.Is(err, ErrProjectDissolveConflict):
			return DissolveResult{}, conflict("项目关联发生变化，请重试", err)
		default:
			return DissolveResult{}, internalError(err)
		}
	}
	if s.notifications != nil {
		s.notifications.PublishConversationRemoved(ctx, userIDs, conversationID)
	}
	return DissolveResult{ConversationID: conversationID}, nil
}

func (s *Service) dissolve(ctx context.Context, actor store.User, conversationID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err := preflightDissolution(db, actor.ID, conversationID); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxProjectDissolveAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		userIDs := []string{}
		err := db.Transaction(func(tx *gorm.DB) error {
			projectIDs, err := loadProjectIDs(tx, conversationID)
			if err != nil {
				return err
			}
			projectsByID, err := lockProjectsForDissolution(tx, projectIDs)
			if err != nil {
				return err
			}
			var conversation store.Conversation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
				return err
			}
			currentProjectIDs, err := loadProjectIDs(tx, conversationID)
			if err != nil {
				return err
			}
			if containsAdditionalProjectID(projectIDs, currentProjectIDs) {
				return ErrProjectLockChange
			}
			if conversation.Status != store.ConversationStatusActive {
				return ErrAccessDenied
			}
			if conversation.Kind != store.ConversationKindGroup {
				return ErrNotGroup
			}
			var current store.ConversationMember
			if err := tx.First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, actor.ID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAccessDenied
				}
				return err
			}
			if current.Role != store.ConversationMemberRoleOwner {
				return ErrAccessDenied
			}
			ids, err := loadActiveUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			userIDs = ids
			now := s.now().UTC()
			deleted := tx.Where("conversation_id = ?", conversationID).Delete(&store.ProjectGroup{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected != int64(len(currentProjectIDs)) {
				return ErrProjectMutation
			}
			activeProjectIDs := make([]string, 0, len(currentProjectIDs))
			for _, projectID := range currentProjectIDs {
				project, exists := projectsByID[projectID]
				if exists && !project.DeletedAt.Valid {
					activeProjectIDs = append(activeProjectIDs, projectID)
				}
			}
			if len(activeProjectIDs) > 0 {
				updated := tx.Model(&store.Project{}).Where("id IN ?", activeProjectIDs).Update("updated_at", now)
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != int64(len(activeProjectIDs)) {
					return ErrProjectMutation
				}
			}
			updated := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{"dissolved_at": now, "status": store.ConversationStatusDissolved, "updated_at": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrProjectMutation
			}
			return nil
		})
		if errors.Is(err, ErrProjectLockChange) {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			if attempt == maxProjectDissolveAttempts-1 {
				return nil, ErrProjectDissolveConflict
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		return userIDs, nil
	}
	return nil, ErrProjectDissolveConflict
}

func preflightDissolution(db *gorm.DB, userID, conversationID string) error {
	var conversation store.Conversation
	if err := db.Select("id", "kind", "status").First(&conversation, "id = ?", conversationID).Error; err != nil {
		return err
	}
	if conversation.Status != store.ConversationStatusActive {
		return ErrAccessDenied
	}
	if conversation.Kind != store.ConversationKindGroup {
		return ErrNotGroup
	}
	var current store.ConversationMember
	if err := db.Select("role").First(&current, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccessDenied
		}
		return err
	}
	if current.Role != store.ConversationMemberRoleOwner {
		return ErrAccessDenied
	}
	return nil
}

func loadProjectIDs(tx *gorm.DB, conversationID string) ([]string, error) {
	ids := []string{}
	if err := tx.Model(&store.ProjectGroup{}).Where("conversation_id = ?", conversationID).Pluck("project_id", &ids).Error; err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

func lockProjectsForDissolution(tx *gorm.DB, projectIDs []string) (map[string]store.Project, error) {
	byID := make(map[string]store.Project, len(projectIDs))
	if len(projectIDs) == 0 {
		return byID, nil
	}
	projects := make([]store.Project, 0, len(projectIDs))
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "deleted_at").Where("id IN ?", projectIDs).Order("id ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, project := range projects {
		byID[project.ID] = project
	}
	return byID, nil
}

func containsAdditionalProjectID(locked, current []string) bool {
	set := make(map[string]struct{}, len(locked))
	for _, id := range locked {
		set[id] = struct{}{}
	}
	for _, id := range current {
		if _, ok := set[id]; !ok {
			return true
		}
	}
	return false
}
