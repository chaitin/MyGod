package conversation

import (
	"context"
	"errors"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) CreateGroup(ctx context.Context, cmd CreateGroupCommand) (CreateGroupResult, error) {
	if len(cmd.ProjectIDs) > MaxGroupProjects {
		return CreateGroupResult{}, invalidRequest("群聊最多关联 100 个项目", nil)
	}
	actor := actorUser(cmd.Actor)
	name, err := normalizeGroupName(cmd.Name)
	if err != nil {
		return CreateGroupResult{}, invalidRequest(err.Error(), err)
	}
	members, err := normalizeMemberIDs(cmd.MemberIDs, actor.ID)
	if err != nil {
		return CreateGroupResult{}, invalidRequest(err.Error(), err)
	}
	apps, err := normalizeAppIDs(cmd.AppIDs)
	if err != nil {
		return CreateGroupResult{}, invalidRequest(err.Error(), err)
	}
	projects, err := normalizeProjectIDs(cmd.ProjectIDs)
	if err != nil {
		return CreateGroupResult{}, invalidRequest("项目 ID 格式错误", err)
	}
	if len(members)+len(apps)+1 > MaxGroupMembers {
		return CreateGroupResult{}, invalidRequest("群聊成员不能超过 500 人", ErrMemberCap)
	}
	conversation, message, candidates, userIDs, err := s.createGroup(ctx, actor, name, members, apps, projects)
	if err != nil {
		switch {
		case errors.Is(err, ErrMemberMissing):
			return CreateGroupResult{}, invalidRequest("成员或应用不存在或不可用", err)
		case errors.Is(err, ErrMemberCap):
			return CreateGroupResult{}, invalidRequest("群聊成员不能超过 500 人", err)
		case errors.Is(err, ErrProjectInvalid):
			return CreateGroupResult{}, invalidRequest("项目 ID 格式错误", err)
		case errors.Is(err, ErrProjectPersonal):
			return CreateGroupResult{}, invalidRequest("个人项目不能关联群聊", err)
		case errors.Is(err, ErrProjectUnowned), errors.Is(err, ErrProjectMissing):
			return CreateGroupResult{}, notFound("项目不存在", err)
		default:
			return CreateGroupResult{}, internalError(err)
		}
	}
	resultMessage := newOptionalMessage(message)
	if resultMessage != nil && s.notifications != nil {
		s.notifications.PublishConversationMessage(ctx, userIDs, *resultMessage)
	}
	return CreateGroupResult{Conversation: newGroup(conversation, candidates, actor.ID), Message: resultMessage}, nil
}

func (s *Service) createGroup(ctx context.Context, actor store.User, name string, memberIDs, appIDs, projectIDs []string) (store.Conversation, *store.Message, []memberCandidate, []string, error) {
	if len(memberIDs)+len(appIDs)+1 > MaxGroupMembers {
		return store.Conversation{}, nil, nil, nil, ErrMemberCap
	}
	if err := ctx.Err(); err != nil {
		return store.Conversation{}, nil, nil, nil, err
	}
	db := s.db.WithContext(ctx)
	members, err := loadActiveGroupMembers(db, memberIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, nil, nil, nil, ErrMemberMissing
		}
		return store.Conversation{}, nil, nil, nil, err
	}
	apps, err := loadVisibleGroupApps(db, actor.ID, appIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Conversation{}, nil, nil, nil, ErrMemberMissing
		}
		return store.Conversation{}, nil, nil, nil, err
	}
	now := s.now().UTC()
	conversation := store.Conversation{ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: name, CreatedByUserID: actor.ID, Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now}
	candidates := make([]memberCandidate, 0, len(members)+len(apps)+1)
	candidates = append(candidates, memberCandidate{memberType: store.ConversationMemberTypeUser, role: store.ConversationMemberRoleOwner, user: actor})
	for _, member := range members {
		candidates = append(candidates, memberCandidate{memberType: store.ConversationMemberTypeUser, role: store.ConversationMemberRoleMember, user: member})
	}
	for _, app := range apps {
		candidates = append(candidates, memberCandidate{app: app, memberType: store.ConversationMemberTypeApp, role: store.ConversationMemberRoleMember})
	}
	var message *store.Message
	userIDs := make([]string, 0, len(candidates))
	if err := db.Transaction(func(tx *gorm.DB) error {
		projects, err := lockOwnedProjects(tx, projectIDs, actor.ID)
		if err != nil {
			return err
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		systemMessageSeq := conversation.LastMessageSeq
		if len(members)+len(apps) > 0 {
			systemMessageSeq++
		}
		conversationMembers := make([]store.ConversationMember, 0, len(candidates))
		for _, candidate := range candidates {
			memberID := candidate.user.ID
			if candidate.memberType == store.ConversationMemberTypeApp {
				memberID = candidate.app.ID
			}
			lastReadSeq := int64(0)
			if candidate.memberType == store.ConversationMemberTypeUser && memberID == actor.ID {
				lastReadSeq = systemMessageSeq
			}
			conversationMembers = append(conversationMembers, store.ConversationMember{ConversationID: conversation.ID, MemberType: candidate.memberType, MemberID: memberID, Role: candidate.role, JoinedAt: now, HistoryVisibleFromSeq: 1, LastReadSeq: lastReadSeq})
			if candidate.memberType == store.ConversationMemberTypeUser {
				userIDs = append(userIDs, memberID)
			}
		}
		if err := tx.Create(&conversationMembers).Error; err != nil {
			return err
		}
		if len(members)+len(apps) > 0 {
			created, err := createGroupMembersInvitedSystemMessage(tx, &conversation, actor, makeInviteeRefs(members, apps), now)
			if err != nil {
				return err
			}
			message = &created
		}
		if len(projects) > 0 {
			links := make([]store.ProjectGroup, 0, len(projects))
			for _, project := range projects {
				links = append(links, store.ProjectGroup{ProjectID: project.ID, ConversationID: conversation.ID, LinkedByUserID: actor.ID, CreatedAt: now})
			}
			if err := tx.Create(&links).Error; err != nil {
				return err
			}
			ids := make([]string, 0, len(projects))
			for _, project := range projects {
				ids = append(ids, project.ID)
			}
			result := tx.Model(&store.Project{}).Where("id IN ?", ids).Update("updated_at", now)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != int64(len(ids)) {
				return ErrProjectMutation
			}
		}
		return nil
	}); err != nil {
		return store.Conversation{}, nil, nil, nil, err
	}
	return conversation, message, candidates, userIDs, nil
}

func lockOwnedProjects(tx *gorm.DB, projectIDs []string, userID string) ([]store.Project, error) {
	if len(projectIDs) == 0 {
		return []store.Project{}, nil
	}
	projects := make([]store.Project, 0, len(projectIDs))
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "owner_user_id", "is_personal", "deleted_at").Where("id IN ?", projectIDs).Order("id ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	if len(projects) != len(projectIDs) {
		return nil, ErrProjectMissing
	}
	for _, project := range projects {
		if project.DeletedAt.Valid {
			return nil, ErrProjectMissing
		}
		if project.OwnerUserID != userID {
			return nil, ErrProjectUnowned
		}
		if project.IsPersonal {
			return nil, ErrProjectPersonal
		}
	}
	return projects, nil
}
