package conversation

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	db := s.db
	accountID := strings.TrimSpace(cmd.AccountID)
	assistantID := builtinAssistantConversationID(accountID)
	assistant, hasAssistant, err := s.ensureBuiltinAssistantConversation(db, accountID)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	limit := MaxClientListItems
	if hasAssistant {
		limit--
	}
	var pins []store.ConversationPin
	if err := db.Where("user_id = ?", accountID).Find(&pins).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	pinnedConversationIDs := make(map[string]struct{}, len(pins))
	for _, pin := range pins {
		pinnedConversationIDs[pin.ConversationID] = struct{}{}
	}
	var conversations []store.Conversation
	if err := db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Joins("LEFT JOIN conversation_pins cp ON cp.conversation_id = conversations.id AND cp.user_id = ?", accountID).
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, accountID).
		Where("conversations.id <> ?", assistantID).
		Where("conversations.kind <> ?", store.ConversationKindTopic).
		Where("conversations.status = ?", store.ConversationStatusActive).
		Order("CASE WHEN cp.user_id IS NULL THEN 1 ELSE 0 END ASC").
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").Limit(limit).Find(&conversations).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	var topicConversations []store.Conversation
	if err := db.Model(&store.Conversation{}).
		Joins("JOIN conversation_topic_participants ctp ON ctp.conversation_id = conversations.id").
		Joins("JOIN conversation_topics ct ON ct.conversation_id = conversations.id").
		Joins("JOIN conversations parent_conversations ON parent_conversations.id = ct.parent_conversation_id").
		Joins("JOIN conversation_members parent_cm ON parent_cm.conversation_id = ct.parent_conversation_id").
		Joins("LEFT JOIN conversation_pins cp ON cp.conversation_id = conversations.id AND cp.user_id = ?", accountID).
		Where("ctp.participant_type = ? AND ctp.participant_id = ?", store.ConversationMemberTypeUser, accountID).
		Where("parent_cm.member_type = ? AND parent_cm.member_id = ? AND parent_cm.left_at IS NULL", store.ConversationMemberTypeUser, accountID).
		Where("ct.source_message_seq >= CASE WHEN parent_cm.history_visible_from_seq < 1 THEN 1 ELSE parent_cm.history_visible_from_seq END").
		Where("ct.archived_at IS NULL").
		Where("conversations.status = ? AND parent_conversations.status = ?", store.ConversationStatusActive, store.ConversationStatusActive).
		Order("CASE WHEN cp.user_id IS NULL THEN 1 ELSE 0 END ASC").
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").
		Order("conversations.id ASC").Limit(limit).Find(&topicConversations).Error; err != nil {
		return ListResult{}, internalError(err)
	}
	conversations = append(conversations, topicConversations...)
	sort.Slice(conversations, func(left, right int) bool {
		_, leftPinned := pinnedConversationIDs[conversations[left].ID]
		_, rightPinned := pinnedConversationIDs[conversations[right].ID]
		if leftPinned != rightPinned {
			return leftPinned
		}
		leftAt := conversations[left].CreatedAt
		if conversations[left].LastMessageAt != nil {
			leftAt = *conversations[left].LastMessageAt
		}
		rightAt := conversations[right].CreatedAt
		if conversations[right].LastMessageAt != nil {
			rightAt = *conversations[right].LastMessageAt
		}
		if !leftAt.Equal(rightAt) {
			return leftAt.After(rightAt)
		}
		return conversations[left].ID < conversations[right].ID
	})
	if len(conversations) > limit {
		conversations = conversations[:limit]
	}
	ids := make([]string, 0, len(conversations)+1)
	if hasAssistant {
		ids = append(ids, assistant.ID)
	}
	for _, conversation := range conversations {
		ids = append(ids, conversation.ID)
	}
	membersByConversation, users, apps, err := s.loadListMembers(db, ids)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	accessibleAppIDs, err := loadUserAccessibleAppIDSet(db, accountID, apps)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	topicPresentations, err := loadTopicPresentations(db, conversations, accountID)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	projects := make(map[string][]Project, len(ids))
	if s.projects != nil {
		projectConversationSet := make(map[string]struct{}, len(ids))
		projectConversationByItem := make(map[string]string, len(ids))
		for _, conversationID := range ids {
			projectConversationID := conversationID
			if presentation, ok := topicPresentations[conversationID]; ok {
				projectConversationID = presentation.topic.ParentConversationID
			}
			projectConversationByItem[conversationID] = projectConversationID
			projectConversationSet[projectConversationID] = struct{}{}
		}
		values, err := s.projects.ListForConversations(ctx, sortedKeys(projectConversationSet))
		if err != nil {
			return ListResult{}, internalError(err)
		}
		for conversationID, projectConversationID := range projectConversationByItem {
			items := values[projectConversationID]
			projects[conversationID] = make([]Project, 0, len(items))
			for _, item := range items {
				projects[conversationID] = append(projects[conversationID], Project{Avatar: item.Avatar, Description: item.Description, ID: item.ID, Name: item.Name})
			}
		}
	}
	result := ListResult{Conversations: make([]Item, 0, len(conversations)+1)}
	appendItem := func(conversation store.Conversation) {
		item := newItem(conversation, accountID, membersByConversation[conversation.ID], users, apps)
		var parent *store.Conversation
		if presentation, ok := topicPresentations[conversation.ID]; ok {
			item = newTopicItem(conversation, accountID, membersByConversation[conversation.ID], users, apps, presentation)
			parent = &presentation.parent
		}
		item.CanSend = canUserSendConversation(conversation, parent, membersByConversation[conversation.ID], accessibleAppIDs)
		conversationProjects := projects[conversation.ID]
		if conversationProjects == nil {
			conversationProjects = []Project{}
		}
		_, item.Pinned = pinnedConversationIDs[conversation.ID]
		if conversation.ID == assistantID {
			item.Pinned = true
		}
		item.Projects = &conversationProjects
		result.Conversations = append(result.Conversations, item)
	}
	if hasAssistant {
		appendItem(assistant)
	}
	for _, conversation := range conversations {
		appendItem(conversation)
	}
	return result, nil
}

func (s *Service) ensureBuiltinAssistantConversation(db *gorm.DB, userID string) (store.Conversation, bool, error) {
	assistant, err := appregistry.EnsureAIAssistantApp(db, s.apps)
	if err != nil {
		return store.Conversation{}, false, err
	}
	if !assistant.Enabled {
		return store.Conversation{}, false, nil
	}
	id, now := builtinAssistantConversationID(userID), s.now().UTC()
	conversation := store.Conversation{}
	err = db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", id).Error
		if err == nil {
			if err := ensureBuiltinAssistantConversationFields(tx, &conversation, assistant, userID, now); err != nil {
				return err
			}
			if err := ensureBuiltinAssistantConversationMembers(tx, conversation.ID, assistant.ID, userID, now); err != nil {
				return err
			}
			return ensureBuiltinAssistantAppConversation(tx, assistant.ID, conversation.ID, userID, now)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		conversation = store.Conversation{ID: id, Kind: store.ConversationKindApp, Name: assistant.Name, Avatar: assistant.Avatar, CreatedByUserID: userID, Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		if err := ensureBuiltinAssistantConversationMembers(tx, conversation.ID, assistant.ID, userID, now); err != nil {
			return err
		}
		return ensureBuiltinAssistantAppConversation(tx, assistant.ID, conversation.ID, userID, now)
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if findErr := db.First(&conversation, "id = ?", id).Error; findErr == nil {
				return conversation, true, nil
			}
		}
		return store.Conversation{}, false, err
	}
	return conversation, true, nil
}

func ensureBuiltinAssistantConversationFields(db *gorm.DB, conversation *store.Conversation, app store.App, userID string, now time.Time) error {
	updates := map[string]any{}
	if conversation.Kind != store.ConversationKindApp {
		updates["kind"] = store.ConversationKindApp
	}
	if conversation.Name != app.Name {
		updates["name"] = app.Name
	}
	if conversation.Avatar != app.Avatar {
		updates["avatar"] = app.Avatar
	}
	if conversation.CreatedByUserID != userID {
		updates["created_by_user_id"] = userID
	}
	if conversation.Status != store.ConversationStatusActive {
		updates["status"] = store.ConversationStatusActive
	}
	if conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		updates["posting_policy"] = store.ConversationPostingPolicyOpen
	}
	if conversation.Visibility == "" {
		updates["visibility"] = store.ConversationVisibilityPrivate
	}
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = now
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(updates).Error; err != nil {
		return err
	}
	return db.First(conversation, "id = ?", conversation.ID).Error
}

func ensureBuiltinAssistantConversationMembers(db *gorm.DB, conversationID, appID, userID string, now time.Time) error {
	if err := ensureBuiltinAssistantConversationMember(db, store.ConversationMember{ConversationID: conversationID, MemberType: store.ConversationMemberTypeUser, MemberID: userID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1}); err != nil {
		return err
	}
	return ensureBuiltinAssistantConversationMember(db, store.ConversationMember{ConversationID: conversationID, MemberType: store.ConversationMemberTypeApp, MemberID: appID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1})
}

func ensureBuiltinAssistantConversationMember(db *gorm.DB, member store.ConversationMember) error {
	var existing store.ConversationMember
	err := db.First(&existing, "conversation_id = ? AND member_type = ? AND member_id = ?", member.ConversationID, member.MemberType, member.MemberID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&member).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{}
	if existing.Role != member.Role {
		updates["role"] = member.Role
	}
	if existing.HistoryVisibleFromSeq < 1 {
		updates["history_visible_from_seq"] = int64(1)
	}
	if existing.LeftAt != nil {
		updates["left_at"] = nil
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Model(&store.ConversationMember{}).Where("conversation_id = ? AND member_type = ? AND member_id = ?", member.ConversationID, member.MemberType, member.MemberID).Updates(updates).Error
}

func ensureBuiltinAssistantAppConversation(db *gorm.DB, appID, conversationID, userID string, now time.Time) error {
	var existing store.AppConversation
	err := db.First(&existing, "app_id = ? AND user_id = ?", appID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&store.AppConversation{AppID: appID, ConversationID: conversationID, UserID: userID, CreatedAt: now}).Error
	}
	if err != nil {
		return err
	}
	if existing.ConversationID == conversationID {
		return nil
	}
	return db.Model(&store.AppConversation{}).Where("app_id = ? AND user_id = ?", appID, userID).Update("conversation_id", conversationID).Error
}

func BuiltinAssistantConversationID(userID string) string {
	namespace := uuid.NewSHA1(uuid.NameSpaceURL, []byte("mygod:builtin-assistant-conversation"))
	return uuid.NewSHA1(namespace, []byte(strings.ToLower(strings.TrimSpace(userID)))).String()
}

func EnsureBuiltinAssistantConversationTx(
	db *gorm.DB,
	assistant store.App,
	recipient store.User,
	now time.Time,
) (store.Conversation, error) {
	conversationID := BuiltinAssistantConversationID(recipient.ID)
	candidate := store.Conversation{
		ID: conversationID, Kind: store.ConversationKindApp, Name: assistant.Name, Avatar: assistant.Avatar,
		CreatedByUserID: recipient.ID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return store.Conversation{}, err
	}
	var conversation store.Conversation
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantConversationFields(db, &conversation, assistant, recipient.ID, now); err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantConversationMembers(db, conversation.ID, assistant.ID, recipient.ID, now); err != nil {
		return store.Conversation{}, err
	}
	if err := ensureBuiltinAssistantAppConversation(db, assistant.ID, conversation.ID, recipient.ID, now); err != nil {
		return store.Conversation{}, err
	}
	return conversation, nil
}

func builtinAssistantConversationID(userID string) string {
	return BuiltinAssistantConversationID(userID)
}
