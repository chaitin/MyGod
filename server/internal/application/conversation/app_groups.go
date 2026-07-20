package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	appapp "app/internal/application/app"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplicationGroupIdentity struct {
	Avatar   string
	Email    string
	ID       string
	Name     string
	Nickname string
	Type     string
}

type ApplicationGroupDetail struct {
	Avatar         string
	CreatedAt      time.Time
	CreatedBy      ApplicationGroupIdentity
	CurrentAppRole string
	ID             string
	MemberCount    int64
	Name           string
	Owner          ApplicationGroupIdentity
	PostingPolicy  string
	Status         string
	UpdatedAt      time.Time
	Visibility     string
}

type ApplicationGroupMember struct {
	ApplicationGroupIdentity
	JoinedAt time.Time
	Role     string
}

type ApplicationGroupMembersResult struct {
	Members  []ApplicationGroupMember
	Page     int
	PageSize int
	Total    int64
}

type ApplicationGroupMutationResult struct {
	Conversation ApplicationGroupDetail
	Message      *Message
}

type CreateGroupAsApplicationCommand struct {
	AppID     string
	AppIDs    []string
	MemberIDs []string
	Name      string
}

type GetGroupForApplicationCommand struct {
	AppID          string
	ConversationID string
}

type ListGroupMembersForApplicationCommand struct {
	AppID          string
	ConversationID string
	Page           int
	PageSize       int
}

type AddGroupMembersAsApplicationCommand struct {
	AppID          string
	AppIDs         []string
	ConversationID string
	MemberIDs      []string
}

type RemoveGroupMemberAsApplicationCommand struct {
	AppID          string
	ConversationID string
	MemberID       string
	MemberType     string
}

type SetGroupMemberRoleAsApplicationCommand struct {
	AppID          string
	ConversationID string
	MemberID       string
	MemberType     string
	Role           string
}

type UpdateGroupNameAsApplicationCommand struct {
	AppID          string
	ConversationID string
	Name           string
}

type DissolveGroupAsApplicationCommand struct {
	AppID          string
	ConversationID string
}

func (s *Service) CreateGroupAsApplication(ctx context.Context, cmd CreateGroupAsApplicationCommand) (ApplicationGroupMutationResult, error) {
	appID, err := normalizeUUID(cmd.AppID, "应用 ID 格式错误")
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	name, err := normalizeGroupName(cmd.Name)
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	memberIDs, err := normalizeApplicationGroupIDs(cmd.MemberIDs, "成员 ID")
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	if len(memberIDs) == 0 {
		return ApplicationGroupMutationResult{}, invalidRequest("至少选择一名成员", nil)
	}
	appIDs, err := normalizeApplicationGroupIDs(cmd.AppIDs, "应用 ID")
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	appIDs = removeApplicationGroupID(appIDs, appID)
	if len(memberIDs)+len(appIDs)+1 > MaxGroupMembers {
		return ApplicationGroupMutationResult{}, invalidRequest("群聊成员不能超过 500 人", ErrMemberCap)
	}

	var actor store.App
	var conversation store.Conversation
	var storedMessage *store.Message
	userIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedApp, err := appapp.LockUsableApp(tx, appID)
		if err != nil {
			return err
		}
		actor = lockedApp
		users, err := loadApplicationVisibleUsers(tx, actor, memberIDs)
		if err != nil {
			return err
		}
		apps, err := loadVisibleGroupApps(tx, appIDs)
		if err != nil {
			return err
		}
		if len(apps) != len(appIDs) {
			return ErrGroupAppUnavailable
		}
		now := s.now().UTC()
		legacyCreatorUserID := users[0].ID
		if actor.CreatorUserID != nil {
			legacyCreatorUserID = *actor.CreatorUserID
		}
		creatorAppID := actor.ID
		conversation = store.Conversation{
			ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: name,
			CreatedByAppID: &creatorAppID, CreatedByUserID: legacyCreatorUserID,
			Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen,
			Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		members := make([]store.ConversationMember, 0, len(users)+len(apps)+1)
		members = append(members, store.ConversationMember{
			ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp,
			MemberID: actor.ID, Role: store.ConversationMemberRoleOwner,
			JoinedAt: now, HistoryVisibleFromSeq: 1,
		})
		for _, user := range users {
			members = append(members, store.ConversationMember{
				ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser,
				MemberID: user.ID, Role: store.ConversationMemberRoleMember,
				JoinedAt: now, HistoryVisibleFromSeq: 1,
			})
			userIDs = append(userIDs, user.ID)
		}
		for _, app := range apps {
			members = append(members, store.ConversationMember{
				ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp,
				MemberID: app.ID, Role: store.ConversationMemberRoleMember,
				JoinedAt: now, HistoryVisibleFromSeq: 1,
			})
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}
		created, err := createGroupMembersInvitedByApplicationSystemMessage(
			tx, &conversation, actor, makeInviteeRefs(users, apps), now,
		)
		if err != nil {
			return err
		}
		storedMessage = &created
		return nil
	})
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupMutationError(err)
	}
	if storedMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, newMessage(*storedMessage))
	}
	return s.loadApplicationGroupMutationResult(ctx, appID, conversation.ID, storedMessage)
}

func (s *Service) GetGroupForApplication(ctx context.Context, cmd GetGroupForApplicationCommand) (ApplicationGroupDetail, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupDetail{}, err
	}
	detail, err := loadApplicationGroupDetail(s.db.WithContext(ctx), appID, conversationID)
	if err != nil {
		return ApplicationGroupDetail{}, mapApplicationGroupReadError(err)
	}
	return detail, nil
}

func (s *Service) ListGroupMembersForApplication(ctx context.Context, cmd ListGroupMembersForApplicationCommand) (ApplicationGroupMembersResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupMembersResult{}, err
	}
	page := cmd.Page
	if page < 1 {
		page = 1
	}
	pageSize := cmd.PageSize
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > MaxGroupMembers {
		pageSize = MaxGroupMembers
	}
	db := s.db.WithContext(ctx)
	if _, _, err := loadApplicationGroupMembership(db, appID, conversationID, false); err != nil {
		return ApplicationGroupMembersResult{}, mapApplicationGroupReadError(err)
	}
	query := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND left_at IS NULL", conversationID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ApplicationGroupMembersResult{}, internalError(err)
	}
	var storedMembers []store.ConversationMember
	if err := query.
		Order(applicationGroupRoleOrderSQL()).Order("joined_at ASC").Order("member_type ASC").Order("member_id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&storedMembers).Error; err != nil {
		return ApplicationGroupMembersResult{}, internalError(err)
	}
	users, apps, err := loadMemberIdentities(db, storedMembers)
	if err != nil {
		return ApplicationGroupMembersResult{}, internalError(err)
	}
	members := make([]ApplicationGroupMember, 0, len(storedMembers))
	for _, member := range storedMembers {
		identity, ok := applicationGroupMemberIdentity(member, users, apps)
		if !ok {
			continue
		}
		members = append(members, ApplicationGroupMember{
			ApplicationGroupIdentity: identity, JoinedAt: member.JoinedAt, Role: member.Role,
		})
	}
	return ApplicationGroupMembersResult{Members: members, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) AddGroupMembersAsApplication(ctx context.Context, cmd AddGroupMembersAsApplicationCommand) (ApplicationGroupMutationResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	memberIDs, err := normalizeApplicationGroupIDs(cmd.MemberIDs, "成员 ID")
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	appIDs, err := normalizeApplicationGroupIDs(cmd.AppIDs, "应用 ID")
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	appIDs = removeApplicationGroupID(appIDs, appID)
	if len(memberIDs)+len(appIDs) == 0 {
		return ApplicationGroupMutationResult{}, invalidRequest("至少选择一名成员或应用", nil)
	}

	var storedMessage *store.Message
	userIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := appapp.LockUsableApp(tx, appID)
		if err != nil {
			return err
		}
		conversation, current, err := loadApplicationGroupMembership(tx, appID, conversationID, true)
		if err != nil {
			return err
		}
		if !canManage(current.Role) {
			return ErrAccessDenied
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
			ids, err := loadActiveUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			userIDs = ids
			return nil
		}
		if activeCount+len(newMemberIDs)+len(reactivatedMemberIDs)+len(newAppIDs)+len(reactivatedAppIDs) > MaxGroupMembers {
			return ErrMemberCap
		}
		users, err := loadApplicationVisibleUsers(tx, actor, addedMemberIDs)
		if err != nil {
			return err
		}
		apps, err := loadVisibleGroupApps(tx, addedAppIDs)
		if err != nil {
			return err
		}
		if len(apps) != len(addedAppIDs) {
			return ErrGroupAppUnavailable
		}
		now := s.now().UTC()
		visibleFromSeq := conversation.LastMessageSeq + 1
		if len(reactivatedMemberIDs) > 0 {
			if err := reactivateMembers(tx, conversationID, store.ConversationMemberTypeUser, reactivatedMemberIDs, now, visibleFromSeq, conversation.LastMessageSeq); err != nil {
				return err
			}
		}
		if len(reactivatedAppIDs) > 0 {
			if err := reactivateMembers(tx, conversationID, store.ConversationMemberTypeApp, reactivatedAppIDs, now, visibleFromSeq, conversation.LastMessageSeq); err != nil {
				return err
			}
		}
		usersByID := make(map[string]store.User, len(users))
		for _, user := range users {
			usersByID[user.ID] = user
		}
		appsByID := make(map[string]store.App, len(apps))
		for _, app := range apps {
			appsByID[app.ID] = app
		}
		newMembers := make([]store.ConversationMember, 0, len(newMemberIDs)+len(newAppIDs))
		for _, id := range newMemberIDs {
			newMembers = append(newMembers, store.ConversationMember{
				ConversationID: conversationID, MemberType: store.ConversationMemberTypeUser,
				MemberID: id, Role: store.ConversationMemberRoleMember, JoinedAt: now,
				HistoryVisibleFromSeq: visibleFromSeq, LastReadSeq: conversation.LastMessageSeq,
			})
		}
		for _, id := range newAppIDs {
			newMembers = append(newMembers, store.ConversationMember{
				ConversationID: conversationID, MemberType: store.ConversationMemberTypeApp,
				MemberID: id, Role: store.ConversationMemberRoleMember, JoinedAt: now,
				HistoryVisibleFromSeq: visibleFromSeq, LastReadSeq: conversation.LastMessageSeq,
			})
		}
		if len(newMembers) > 0 {
			if err := tx.Create(&newMembers).Error; err != nil {
				return err
			}
		}
		orderedUsers := make([]store.User, 0, len(addedMemberIDs))
		for _, id := range addedMemberIDs {
			orderedUsers = append(orderedUsers, usersByID[id])
		}
		orderedApps := make([]store.App, 0, len(addedAppIDs))
		for _, id := range addedAppIDs {
			orderedApps = append(orderedApps, appsByID[id])
		}
		created, err := createGroupMembersInvitedByApplicationSystemMessage(
			tx, &conversation, actor, makeInviteeRefs(orderedUsers, orderedApps), now,
		)
		if err != nil {
			return err
		}
		storedMessage = &created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupMutationError(err)
	}
	if storedMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, newMessage(*storedMessage))
	}
	return s.loadApplicationGroupMutationResult(ctx, appID, conversationID, storedMessage)
}

func (s *Service) RemoveGroupMemberAsApplication(ctx context.Context, cmd RemoveGroupMemberAsApplicationCommand) (ApplicationGroupMutationResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	memberType, memberID, err := normalizeApplicationGroupMember(cmd.MemberType, cmd.MemberID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	if memberType == store.ConversationMemberTypeApp && memberID == appID {
		return ApplicationGroupMutationResult{}, forbidden("不能移出自己", ErrCannotRemoveSelf)
	}

	var storedMessage *store.Message
	userIDs := []string{}
	removedUserID := ""
	removedTopicIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := appapp.LockUsableApp(tx, appID)
		if err != nil {
			return err
		}
		conversation, current, err := loadApplicationGroupMembership(tx, appID, conversationID, true)
		if err != nil {
			return err
		}
		if !canManage(current.Role) {
			return ErrAccessDenied
		}
		var target store.ConversationMember
		if err := tx.First(&target,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID, memberType, memberID,
		).Error; err != nil {
			return err
		}
		if target.Role == store.ConversationMemberRoleOwner {
			return ErrOwnerCannotRemove
		}
		if current.Role == store.ConversationMemberRoleAdmin && target.Role == store.ConversationMemberRoleAdmin {
			return ErrAccessDenied
		}
		if memberType == store.ConversationMemberTypeUser {
			var remainingActiveUsers int64
			if err := tx.Model(&store.ConversationMember{}).
				Joins("JOIN users u ON u.id = conversation_members.member_id").
				Where("conversation_members.conversation_id = ?", conversationID).
				Where("conversation_members.member_type = ? AND conversation_members.left_at IS NULL", store.ConversationMemberTypeUser).
				Where("conversation_members.member_id <> ?", memberID).
				Where("u.status = ?", store.UserStatusActive).
				Count(&remainingActiveUsers).Error; err != nil {
				return err
			}
			if remainingActiveUsers == 0 {
				return newError(CodeConflict, "群聊至少需要保留一名有效用户", nil)
			}
		}
		targetRef, err := loadMemberSystemRef(tx, memberType, memberID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, memberType, memberID).
			Update("left_at", now).Error; err != nil {
			return err
		}
		removedTopicIDs, err = removeParentMemberTopicParticipations(tx, conversationID, memberType, memberID)
		if err != nil {
			return err
		}
		created, err := createGroupMemberRemovedByApplicationSystemMessage(tx, &conversation, actor, targetRef, now)
		if err != nil {
			return err
		}
		storedMessage = &created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		if memberType == store.ConversationMemberTypeUser {
			removedUserID = memberID
		}
		return nil
	})
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupMutationError(err)
	}
	if storedMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, newMessage(*storedMessage))
	}
	if removedUserID != "" && s.notifications != nil {
		s.notifications.PublishConversationRemoved(ctx, []string{removedUserID}, conversationID)
		for _, topicID := range removedTopicIDs {
			s.notifications.PublishConversationRemoved(ctx, []string{removedUserID}, topicID)
		}
	}
	return s.loadApplicationGroupMutationResult(ctx, appID, conversationID, storedMessage)
}

func (s *Service) SetGroupMemberRoleAsApplication(ctx context.Context, cmd SetGroupMemberRoleAsApplicationCommand) (ApplicationGroupMutationResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	memberType, memberID, err := normalizeApplicationGroupMember(cmd.MemberType, cmd.MemberID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	role := strings.TrimSpace(cmd.Role)
	if role != store.ConversationMemberRoleAdmin && role != store.ConversationMemberRoleMember {
		return ApplicationGroupMutationResult{}, invalidRequest("成员角色只支持 admin 或 member", nil)
	}
	if memberType == store.ConversationMemberTypeApp && memberID == appID {
		return ApplicationGroupMutationResult{}, forbidden("不能修改自己的群主角色", ErrAccessDenied)
	}

	var storedMessage *store.Message
	userIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := appapp.LockUsableApp(tx, appID)
		if err != nil {
			return err
		}
		conversation, current, err := loadApplicationGroupMembership(tx, appID, conversationID, true)
		if err != nil {
			return err
		}
		if current.Role != store.ConversationMemberRoleOwner {
			return ErrAccessDenied
		}
		var target store.ConversationMember
		if err := tx.First(&target,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID, memberType, memberID,
		).Error; err != nil {
			return err
		}
		if target.Role == store.ConversationMemberRoleOwner {
			return ErrAccessDenied
		}
		if target.Role == role {
			ids, err := loadActiveUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			userIDs = ids
			return nil
		}
		targetRef, err := loadMemberSystemRef(tx, memberType, memberID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if err := tx.Model(&store.ConversationMember{}).
			Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, memberType, memberID).
			Updates(map[string]any{"role": role}).Error; err != nil {
			return err
		}
		created, err := createGroupMemberRoleUpdatedByApplicationSystemMessage(
			tx, &conversation, actor, targetRef, role, now,
		)
		if err != nil {
			return err
		}
		storedMessage = &created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupMutationError(err)
	}
	if storedMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, newMessage(*storedMessage))
	}
	return s.loadApplicationGroupMutationResult(ctx, appID, conversationID, storedMessage)
}

func (s *Service) UpdateGroupNameAsApplication(ctx context.Context, cmd UpdateGroupNameAsApplicationCommand) (ApplicationGroupMutationResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return ApplicationGroupMutationResult{}, err
	}
	name, err := normalizeGroupName(cmd.Name)
	if err != nil {
		return ApplicationGroupMutationResult{}, invalidRequest(err.Error(), err)
	}
	var storedMessage *store.Message
	userIDs := []string{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := appapp.LockUsableApp(tx, appID)
		if err != nil {
			return err
		}
		conversation, current, err := loadApplicationGroupMembership(tx, appID, conversationID, true)
		if err != nil {
			return err
		}
		if !canManage(current.Role) {
			return ErrAccessDenied
		}
		if conversation.Name == name {
			ids, err := loadActiveUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			userIDs = ids
			return nil
		}
		now := s.now().UTC()
		if err := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).
			Updates(map[string]any{"name": name, "updated_at": now}).Error; err != nil {
			return err
		}
		conversation.Name = name
		conversation.UpdatedAt = now
		created, err := createGroupNameUpdatedByApplicationSystemMessage(tx, &conversation, actor, name, now)
		if err != nil {
			return err
		}
		storedMessage = &created
		ids, err := loadActiveUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		userIDs = ids
		return nil
	})
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupMutationError(err)
	}
	if storedMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, newMessage(*storedMessage))
	}
	return s.loadApplicationGroupMutationResult(ctx, appID, conversationID, storedMessage)
}

func (s *Service) DissolveGroupAsApplication(ctx context.Context, cmd DissolveGroupAsApplicationCommand) (DissolveResult, error) {
	appID, conversationID, err := normalizeApplicationGroupSelector(cmd.AppID, cmd.ConversationID)
	if err != nil {
		return DissolveResult{}, err
	}
	userIDs, topicIDs, err := s.dissolveAsMember(
		ctx,
		store.ConversationMemberTypeApp,
		appID,
		conversationID,
		func(tx *gorm.DB) error {
			_, err := appapp.LockUsableApp(tx, appID)
			return err
		},
	)
	if err != nil {
		return DissolveResult{}, mapApplicationGroupMutationError(err)
	}
	if s.notifications != nil {
		s.notifications.PublishConversationRemoved(ctx, userIDs, conversationID)
		for _, topicID := range topicIDs {
			s.notifications.PublishConversationRemoved(ctx, userIDs, topicID)
		}
	}
	return DissolveResult{ConversationID: conversationID}, nil
}

func (s *Service) loadApplicationGroupMutationResult(ctx context.Context, appID, conversationID string, message *store.Message) (ApplicationGroupMutationResult, error) {
	detail, err := loadApplicationGroupDetail(s.db.WithContext(ctx), appID, conversationID)
	if err != nil {
		return ApplicationGroupMutationResult{}, mapApplicationGroupReadError(err)
	}
	return ApplicationGroupMutationResult{Conversation: detail, Message: newOptionalMessage(message)}, nil
}

func normalizeApplicationGroupSelector(rawAppID, rawConversationID string) (string, string, error) {
	appID, err := normalizeUUID(rawAppID, "应用 ID 格式错误")
	if err != nil {
		return "", "", invalidRequest(err.Error(), err)
	}
	conversationID, err := normalizeConversationID(rawConversationID)
	if err != nil {
		return "", "", invalidRequest(err.Error(), err)
	}
	return appID, conversationID, nil
}

func normalizeApplicationGroupMember(rawType, rawID string) (string, string, error) {
	memberType := strings.TrimSpace(rawType)
	if memberType != store.ConversationMemberTypeUser && memberType != store.ConversationMemberTypeApp {
		return "", "", invalidRequest("成员类型只支持 user 或 app", nil)
	}
	memberID, err := normalizeUUID(rawID, "成员 ID 格式错误")
	if err != nil {
		return "", "", invalidRequest(err.Error(), err)
	}
	return memberType, memberID, nil
}

func normalizeApplicationGroupIDs(rawIDs []string, label string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawIDs))
	result := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := normalizeUUID(rawID, label+"格式错误")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func removeApplicationGroupID(ids []string, excluded string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != excluded {
			result = append(result, id)
		}
	}
	return result
}

func loadApplicationVisibleUsers(db *gorm.DB, actor store.App, userIDs []string) ([]store.User, error) {
	users, err := loadActiveGroupMembers(db, userIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemberMissing
		}
		return nil, err
	}
	if actor.Visibility == store.AppVisibilityPublic {
		return users, nil
	}
	allowed := make(map[string]struct{}, len(userIDs))
	if actor.CreatorUserID != nil {
		allowed[*actor.CreatorUserID] = struct{}{}
	}
	if actor.Visibility == store.AppVisibilityRestricted && len(userIDs) > 0 {
		var grantedIDs []string
		if err := db.Model(&store.AppUserGrant{}).
			Where("app_id = ? AND user_id IN ?", actor.ID, userIDs).
			Pluck("user_id", &grantedIDs).Error; err != nil {
			return nil, err
		}
		for _, id := range grantedIDs {
			allowed[id] = struct{}{}
		}
	}
	for _, id := range userIDs {
		if _, ok := allowed[id]; !ok {
			return nil, ErrAccessDenied
		}
	}
	return users, nil
}

func loadApplicationGroupMembership(db *gorm.DB, appID, conversationID string, requireActive bool) (store.Conversation, store.ConversationMember, error) {
	var conversation store.Conversation
	query := db
	if requireActive {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.Conversation{}, store.ConversationMember{}, err
	}
	if conversation.Kind != store.ConversationKindGroup {
		return store.Conversation{}, store.ConversationMember{}, ErrNotGroup
	}
	if requireActive && conversation.Status != store.ConversationStatusActive {
		return store.Conversation{}, store.ConversationMember{}, ErrAccessDenied
	}
	var member store.ConversationMember
	if err := db.First(&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeApp, appID,
	).Error; err != nil {
		return store.Conversation{}, store.ConversationMember{}, err
	}
	return conversation, member, nil
}

func loadApplicationGroupDetail(db *gorm.DB, appID, conversationID string) (ApplicationGroupDetail, error) {
	conversation, current, err := loadApplicationGroupMembership(db, appID, conversationID, false)
	if err != nil {
		return ApplicationGroupDetail{}, err
	}
	var owner store.ConversationMember
	if err := db.First(&owner,
		"conversation_id = ? AND role = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberRoleOwner,
	).Error; err != nil {
		return ApplicationGroupDetail{}, err
	}
	ownerIdentity, err := loadApplicationGroupIdentity(db, owner.MemberType, owner.MemberID)
	if err != nil {
		return ApplicationGroupDetail{}, err
	}
	createdByType := store.ConversationMemberTypeUser
	createdByID := conversation.CreatedByUserID
	if conversation.CreatedByAppID != nil {
		createdByType = store.ConversationMemberTypeApp
		createdByID = *conversation.CreatedByAppID
	}
	createdBy, err := loadApplicationGroupIdentity(db, createdByType, createdByID)
	if err != nil {
		return ApplicationGroupDetail{}, err
	}
	var memberCount int64
	if err := db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND left_at IS NULL", conversationID).
		Count(&memberCount).Error; err != nil {
		return ApplicationGroupDetail{}, err
	}
	return ApplicationGroupDetail{
		Avatar: conversation.Avatar, CreatedAt: conversation.CreatedAt, CreatedBy: createdBy,
		CurrentAppRole: current.Role, ID: conversation.ID, MemberCount: memberCount,
		Name: conversation.Name, Owner: ownerIdentity, PostingPolicy: conversation.PostingPolicy,
		Status: conversation.Status, UpdatedAt: conversation.UpdatedAt, Visibility: conversation.Visibility,
	}, nil
}

func loadApplicationGroupIdentity(db *gorm.DB, identityType, identityID string) (ApplicationGroupIdentity, error) {
	switch identityType {
	case store.ConversationMemberTypeUser:
		var user store.User
		if err := db.First(&user, "id = ?", identityID).Error; err != nil {
			return ApplicationGroupIdentity{}, err
		}
		return ApplicationGroupIdentity{
			Avatar: user.Avatar, Email: user.Email, ID: user.ID, Name: user.Name,
			Nickname: user.Nickname, Type: store.ConversationMemberTypeUser,
		}, nil
	case store.ConversationMemberTypeApp:
		var app store.App
		if err := db.Unscoped().First(&app, "id = ?", identityID).Error; err != nil {
			return ApplicationGroupIdentity{}, err
		}
		return ApplicationGroupIdentity{
			Avatar: app.Avatar, ID: app.ID, Name: app.Name, Type: store.ConversationMemberTypeApp,
		}, nil
	default:
		return ApplicationGroupIdentity{}, gorm.ErrRecordNotFound
	}
}

func applicationGroupMemberIdentity(member store.ConversationMember, users map[string]store.User, apps map[string]store.App) (ApplicationGroupIdentity, bool) {
	if member.MemberType == store.ConversationMemberTypeUser {
		user, ok := users[member.MemberID]
		if !ok {
			return ApplicationGroupIdentity{}, false
		}
		return ApplicationGroupIdentity{
			Avatar: user.Avatar, Email: user.Email, ID: user.ID, Name: user.Name,
			Nickname: user.Nickname, Type: store.ConversationMemberTypeUser,
		}, true
	}
	app, ok := apps[member.MemberID]
	if !ok {
		return ApplicationGroupIdentity{}, false
	}
	return ApplicationGroupIdentity{
		Avatar: app.Avatar, ID: app.ID, Name: app.Name, Type: store.ConversationMemberTypeApp,
	}, true
}

func applicationGroupRoleOrderSQL() string {
	return "CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END"
}

func mapApplicationGroupReadError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, ErrAccessDenied):
		return notFound("群聊不存在", err)
	case errors.Is(err, ErrNotGroup):
		return invalidRequest("会话不是群聊", err)
	default:
		return internalError(err)
	}
}

func mapApplicationGroupMutationError(err error) error {
	var conversationErr *Error
	if errors.As(err, &conversationErr) {
		return err
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return notFound("群聊或成员不存在", err)
	case errors.Is(err, ErrAccessDenied), errors.Is(err, ErrOwnerCannotRemove), errors.Is(err, ErrCannotRemoveSelf):
		return forbidden("无权操作群聊", err)
	case errors.Is(err, ErrNotGroup):
		return invalidRequest("会话不是群聊", err)
	case errors.Is(err, ErrMemberCap):
		return invalidRequest("群聊成员不能超过 500 人", err)
	case errors.Is(err, ErrGroupAppUnavailable):
		return invalidRequest("只有已启用且所有人可见的应用才能加入群聊", err)
	case errors.Is(err, ErrMemberMissing):
		return invalidRequest("成员不存在或已禁用", err)
	case errors.Is(err, ErrProjectDissolveConflict):
		return conflict("项目关联发生变化，请重试", err)
	default:
		return internalError(err)
	}
}

type groupMemberRoleUpdatedSystemEventBody struct {
	Actor  systemEventUserRef `json:"actor"`
	Event  string             `json:"event"`
	Role   string             `json:"role"`
	Target systemEventUserRef `json:"target"`
	Type   string             `json:"type"`
}

func applicationSystemRef(app store.App) systemEventUserRef {
	return systemEventUserRef{DisplayName: app.Name, ID: app.ID, Type: store.ConversationMemberTypeApp}
}

func createGroupMembersInvitedByApplicationSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.App, invitees []systemEventUserRef, now time.Time) (store.Message, error) {
	names := make([]string, 0, len(invitees))
	for _, invitee := range invitees {
		names = append(names, invitee.DisplayName)
	}
	body, err := json.Marshal(groupMembersInvitedSystemEventBody{
		Event: systemEventGroupMembersInvited, Invitees: invitees,
		Inviter: applicationSystemRef(actor), Type: messageTypeSystemEvent,
	})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(
		db, conversation, body,
		actor.Name+" 邀请 "+strings.Join(names, groupMembersInvitedSummarySeparator)+" 加入群聊", now,
	)
}

func createGroupMemberRemovedByApplicationSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.App, target systemEventUserRef, now time.Time) (store.Message, error) {
	body, err := json.Marshal(groupMemberRemovedSystemEventBody{
		Actor: applicationSystemRef(actor), Event: systemEventGroupMemberRemoved,
		Target: target, Type: messageTypeSystemEvent,
	})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, actor.Name+" 已将 "+target.DisplayName+" 移出群聊", now)
}

func createGroupMemberRoleUpdatedByApplicationSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.App, target systemEventUserRef, role string, now time.Time) (store.Message, error) {
	body, err := json.Marshal(groupMemberRoleUpdatedSystemEventBody{
		Actor: applicationSystemRef(actor), Event: "group_member_role_updated",
		Role: role, Target: target, Type: messageTypeSystemEvent,
	})
	if err != nil {
		return store.Message{}, err
	}
	action := "取消了 " + target.DisplayName + " 的管理员"
	if role == store.ConversationMemberRoleAdmin {
		action = "将 " + target.DisplayName + " 设为管理员"
	}
	return createSystemMessage(db, conversation, body, actor.Name+" "+action, now)
}

func createGroupNameUpdatedByApplicationSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.App, name string, now time.Time) (store.Message, error) {
	body, err := json.Marshal(groupNameUpdatedSystemEventBody{
		Actor: applicationSystemRef(actor), Event: systemEventGroupNameUpdated,
		Name: name, Type: messageTypeSystemEvent,
	})
	if err != nil {
		return store.Message{}, err
	}
	return createSystemMessage(db, conversation, body, actor.Name+" 修改群聊名称为 "+name, now)
}
