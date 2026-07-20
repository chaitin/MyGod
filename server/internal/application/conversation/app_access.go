package conversation

import (
	"context"
	"sort"
	"strings"

	"app/internal/store"

	"gorm.io/gorm"
)

func (s *Service) ListForActor(ctx context.Context, cmd AppListCommand) (AppListResult, error) {
	db := s.db.WithContext(ctx)
	actorID := strings.TrimSpace(cmd.ActorID)
	keyword := strings.ToLower(strings.TrimSpace(cmd.Keyword))
	query := db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, actorID).
		Where("conversations.status = ?", store.ConversationStatusActive)
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where(
			`LOWER(conversations.name) LIKE ? OR conversations.name LIKE ? OR EXISTS (
				SELECT 1
				FROM direct_conversations dc
				JOIN users direct_other
					ON direct_other.status = ?
					AND (
						(dc.user_low_id = ? AND direct_other.id = dc.user_high_id)
						OR (dc.user_high_id = ? AND direct_other.id = dc.user_low_id)
					)
				WHERE dc.conversation_id = conversations.id
					AND conversations.kind = ?
					AND (
						LOWER(direct_other.name) LIKE ?
						OR direct_other.name LIKE ?
						OR LOWER(direct_other.nickname) LIKE ?
						OR direct_other.nickname LIKE ?
					)
			)`,
			likeKeyword, likeKeyword, store.UserStatusActive, actorID, actorID, store.ConversationKindDirect,
			likeKeyword, likeKeyword, likeKeyword, likeKeyword,
		)
	}
	var conversations []store.Conversation
	if err := query.Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").Order("conversations.id ASC").Limit(cmd.Limit).Find(&conversations).Error; err != nil {
		return AppListResult{}, err
	}
	topicQuery := db.Model(&store.Conversation{}).
		Joins("JOIN conversation_topic_participants ctp ON ctp.conversation_id = conversations.id").
		Joins("JOIN conversation_topics ct ON ct.conversation_id = conversations.id").
		Joins("JOIN conversations parent_conversations ON parent_conversations.id = ct.parent_conversation_id").
		Joins("JOIN conversation_members parent_cm ON parent_cm.conversation_id = ct.parent_conversation_id").
		Where("ctp.participant_type = ? AND ctp.participant_id = ?", store.ConversationMemberTypeUser, actorID).
		Where("parent_cm.member_type = ? AND parent_cm.member_id = ? AND parent_cm.left_at IS NULL", store.ConversationMemberTypeUser, actorID).
		Where("ct.source_message_seq >= CASE WHEN parent_cm.history_visible_from_seq < 1 THEN 1 ELSE parent_cm.history_visible_from_seq END").
		Where("ct.archived_at IS NULL").
		Where("conversations.status = ? AND parent_conversations.status = ?", store.ConversationStatusActive, store.ConversationStatusActive)
	if keyword != "" {
		topicQuery = topicQuery.Where("LOWER(conversations.name) LIKE ?", "%"+keyword+"%")
	}
	var topics []store.Conversation
	if err := topicQuery.Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").Order("conversations.id ASC").Limit(cmd.Limit).Find(&topics).Error; err != nil {
		return AppListResult{}, err
	}
	conversations = append(conversations, topics...)
	sort.Slice(conversations, func(left, right int) bool {
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
	if len(conversations) > cmd.Limit {
		conversations = conversations[:cmd.Limit]
	}
	ids := conversationIDs(conversations)
	membersByConversation, users, apps, err := s.loadListMembers(db, ids)
	if err != nil {
		return AppListResult{}, err
	}
	counts, err := loadActiveMemberCounts(db, ids)
	if err != nil {
		return AppListResult{}, err
	}
	topicPresentations, err := loadTopicPresentations(db, conversations, actorID)
	if err != nil {
		return AppListResult{}, err
	}
	result := AppListResult{Conversations: make([]AppSummary, 0, len(conversations))}
	for _, conversation := range conversations {
		item := newItem(conversation, actorID, membersByConversation[conversation.ID], users, apps)
		if presentation, ok := topicPresentations[conversation.ID]; ok {
			item = newTopicItem(conversation, actorID, membersByConversation[conversation.ID], users, apps, presentation)
		}
		lastActiveAt := conversation.CreatedAt
		if conversation.LastMessageAt != nil {
			lastActiveAt = *conversation.LastMessageAt
		}
		memberCount := counts[conversation.ID]
		if memberCount == 0 {
			memberCount = item.MemberCount
		}
		result.Conversations = append(result.Conversations, AppSummary{ConversationID: conversation.ID, LastActiveAt: lastActiveAt, MemberCount: memberCount, Name: item.Name, Type: item.Type})
	}
	return result, nil
}

func (s *Service) ListGroupsForActor(ctx context.Context, cmd AppListCommand) (AppGroupListResult, error) {
	db := s.db.WithContext(ctx)
	actorID := strings.TrimSpace(cmd.ActorID)
	keyword := strings.ToLower(strings.TrimSpace(cmd.Keyword))
	query := db.Model(&store.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id").
		Where("cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL", store.ConversationMemberTypeUser, actorID).
		Where("conversations.kind = ? AND conversations.status = ?", store.ConversationKindGroup, store.ConversationStatusActive)
	if keyword != "" {
		query = query.Where("LOWER(conversations.name) LIKE ?", "%"+keyword+"%")
	}
	var conversations []store.Conversation
	if err := query.Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC").Order("conversations.id ASC").Limit(cmd.Limit).Find(&conversations).Error; err != nil {
		return AppGroupListResult{}, err
	}
	ids := conversationIDs(conversations)
	membersByConversation, users, apps, err := s.loadListMembers(db, ids)
	if err != nil {
		return AppGroupListResult{}, err
	}
	result := AppGroupListResult{Groups: make([]Item, 0, len(conversations))}
	for _, conversation := range conversations {
		result.Groups = append(result.Groups, newItem(conversation, actorID, membersByConversation[conversation.ID], users, apps))
	}
	return result, nil
}

func conversationIDs(conversations []store.Conversation) []string {
	ids := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		ids = append(ids, conversation.ID)
	}
	return ids
}

func loadActiveMemberCounts(db *gorm.DB, conversationIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}
	type countRow struct {
		ConversationID string
		Count          int
	}
	var rows []countRow
	if err := db.Model(&store.ConversationMember{}).Select("conversation_id, COUNT(*) AS count").Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).Group("conversation_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ConversationID] = row.Count
	}
	return counts, nil
}
