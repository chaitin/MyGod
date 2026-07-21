package app

import (
	"errors"
	"sort"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errApplicationOwnedGroupProjectLockChange = errors.New("application-owned group project lock set changed")

func revokeUnauthorizedDirectAppEvents(
	tx *gorm.DB,
	appID string,
	ownerID string,
	visibility string,
	grantedUserIDs []string,
) error {
	if visibility == store.AppVisibilityPublic {
		return nil
	}

	allowedUserIDs := []string{ownerID}
	if visibility == store.AppVisibilityRestricted {
		allowedUserIDs = append(allowedUserIDs, grantedUserIDs...)
	}
	return deleteUnauthorizedDirectAppEvents(tx, appID, allowedUserIDs)
}

func deleteStoredApp(tx *gorm.DB, stored *store.App, now time.Time) error {
	locked, err := lockAppForUpdate(tx, stored.ID)
	if err != nil {
		return err
	}
	*stored = locked
	if err := transferOrDissolveApplicationOwnedGroups(tx, stored.ID, now); err != nil {
		return err
	}
	if err := tx.Model(&store.ConversationMember{}).
		Where("member_type = ? AND member_id = ? AND left_at IS NULL", store.ConversationMemberTypeApp, stored.ID).
		Update("left_at", now).Error; err != nil {
		return err
	}
	if err := tx.Where("app_id = ?", stored.ID).Delete(&store.AppConversation{}).Error; err != nil {
		return err
	}
	if err := tx.Where("app_id = ?", stored.ID).Delete(&store.AppUserGrant{}).Error; err != nil {
		return err
	}
	if err := tx.Where("app_id = ?", stored.ID).Delete(&store.AppEventOutbox{}).Error; err != nil {
		return err
	}
	if err := tx.Where("app_id = ?", stored.ID).Delete(&store.AppEventAck{}).Error; err != nil {
		return err
	}
	return tx.Delete(stored).Error
}

func transferOrDissolveApplicationOwnedGroups(tx *gorm.DB, appID string, now time.Time) error {
	var owned []store.ConversationMember
	if err := tx.Model(&store.ConversationMember{}).
		Joins("JOIN conversations c ON c.id = conversation_members.conversation_id").
		Where("conversation_members.member_type = ? AND conversation_members.member_id = ?", store.ConversationMemberTypeApp, appID).
		Where("conversation_members.role = ? AND conversation_members.left_at IS NULL", store.ConversationMemberRoleOwner).
		Where("c.kind = ? AND c.status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Order("conversation_members.conversation_id ASC").Find(&owned).Error; err != nil {
		return err
	}
	conversationIDs := make([]string, 0, len(owned))
	for _, owner := range owned {
		conversationIDs = append(conversationIDs, owner.ConversationID)
	}
	lockedProjects, err := lockApplicationOwnedGroupProjects(tx, conversationIDs)
	if err != nil {
		return err
	}
	for _, owner := range owned {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", owner.ConversationID).Error; err != nil {
			return err
		}
		projectIDs, err := loadApplicationOwnedGroupProjectIDs(tx, conversation.ID)
		if err != nil {
			return err
		}
		for _, projectID := range projectIDs {
			if _, ok := lockedProjects[projectID]; !ok {
				return errApplicationOwnedGroupProjectLockChange
			}
		}
		var candidate store.ConversationMember
		result := tx.Model(&store.ConversationMember{}).
			Select("conversation_members.*").
			Joins("JOIN users u ON u.id = conversation_members.member_id").
			Where("conversation_members.conversation_id = ?", conversation.ID).
			Where("conversation_members.member_type = ? AND conversation_members.left_at IS NULL", store.ConversationMemberTypeUser).
			Where("u.status = ?", store.UserStatusActive).
			Order("CASE conversation_members.role WHEN 'admin' THEN 0 ELSE 1 END").
			Order("conversation_members.joined_at ASC").Order("conversation_members.member_id ASC").
			Limit(1).Find(&candidate)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			if err := tx.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeApp, appID).
				Updates(map[string]any{"role": store.ConversationMemberRoleMember, "left_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&store.ConversationMember{}).
				Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversation.ID, store.ConversationMemberTypeUser, candidate.MemberID).
				Update("role", store.ConversationMemberRoleOwner).Error; err != nil {
				return err
			}
			continue
		}
		if err := dissolveApplicationOwnedGroup(tx, conversation.ID, projectIDs, lockedProjects, now); err != nil {
			return err
		}
	}
	return nil
}

func lockApplicationOwnedGroupProjects(tx *gorm.DB, conversationIDs []string) (map[string]store.Project, error) {
	locked := make(map[string]store.Project)
	if len(conversationIDs) == 0 {
		return locked, nil
	}
	var projectIDs []string
	if err := tx.Model(&store.ProjectGroup{}).
		Where("conversation_id IN ?", conversationIDs).Distinct().Pluck("project_id", &projectIDs).Error; err != nil {
		return nil, err
	}
	if len(projectIDs) == 0 {
		return locked, nil
	}
	sort.Strings(projectIDs)
	var projects []store.Project
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "deleted_at").Where("id IN ?", projectIDs).Order("id ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, project := range projects {
		locked[project.ID] = project
	}
	return locked, nil
}

func loadApplicationOwnedGroupProjectIDs(tx *gorm.DB, conversationID string) ([]string, error) {
	var projectIDs []string
	if err := tx.Model(&store.ProjectGroup{}).Where("conversation_id = ?", conversationID).
		Order("project_id ASC").Pluck("project_id", &projectIDs).Error; err != nil {
		return nil, err
	}
	return projectIDs, nil
}

func dissolveApplicationOwnedGroup(
	tx *gorm.DB,
	conversationID string,
	projectIDs []string,
	lockedProjects map[string]store.Project,
	now time.Time,
) error {
	deleted := tx.Where("conversation_id = ?", conversationID).Delete(&store.ProjectGroup{})
	if deleted.Error != nil {
		return deleted.Error
	}
	if deleted.RowsAffected != int64(len(projectIDs)) {
		return errApplicationOwnedGroupProjectLockChange
	}
	activeProjectIDs := make([]string, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		project, ok := lockedProjects[projectID]
		if ok && !project.DeletedAt.Valid {
			activeProjectIDs = append(activeProjectIDs, projectID)
		}
	}
	if len(activeProjectIDs) > 0 {
		updated := tx.Model(&store.Project{}).Where("id IN ?", activeProjectIDs).Update("updated_at", now)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != int64(len(activeProjectIDs)) {
			return errApplicationOwnedGroupProjectLockChange
		}
	}
	updated := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).
		Updates(map[string]any{
			"dissolved_at": now, "status": store.ConversationStatusDissolved,
			"updated_at": now,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return errApplicationOwnedGroupProjectLockChange
	}
	return tx.Model(&store.Conversation{}).
		Where("id IN (?)", tx.Model(&store.ConversationTopic{}).
			Select("conversation_id").Where("parent_conversation_id = ?", conversationID)).
		Updates(map[string]any{
			"dissolved_at": now, "status": store.ConversationStatusDissolved,
			"posting_policy": store.ConversationPostingPolicyMuted, "updated_at": now,
		}).Error
}

func deleteUnauthorizedDirectAppEvents(tx *gorm.DB, appID string, allowedUserIDs []string) error {
	return tx.Exec(`
		DELETE FROM app_event_outbox
		WHERE app_id = ?
		  AND EXISTS (
				SELECT 1
				FROM app_conversations ac
				WHERE ac.app_id = ?
				  AND ac.user_id NOT IN ?
				  AND CAST(ac.conversation_id AS TEXT) IN (
					app_event_outbox.payload -> 'conversation' ->> 'id',
					app_event_outbox.payload -> 'conversation' -> 'parent' ->> 'id',
					app_event_outbox.payload ->> 'parent_conversation_id'
				  )
			)`,
		appID,
		appID,
		allowedUserIDs,
	).Error
}
