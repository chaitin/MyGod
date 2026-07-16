package httpserver

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	conversationapp "app/internal/application/conversation"
	"app/internal/appregistry"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// These aliases remain while message history and App WebSocket payloads still
// use the legacy transport DTOs. Conversation persistence lives in the
// application service.
const (
	maxClientConversationListItems   = conversationapp.MaxClientListItems
	maxGroupConversationProjects     = conversationapp.MaxGroupProjects
	messageTypeSystemEvent           = "system_event"
	systemEventMessageRevoked        = "message_revoked"
	builtinAssistantConversationName = appregistry.AIAssistantDefaultName
	builtinAssistantAvatar           = appregistry.AIAssistantDefaultAvatar
)

func builtinAssistantConversationID(userID string) string {
	return conversationapp.BuiltinAssistantConversationID(userID)
}

func ensureBuiltinAssistantConversationTx(db *gorm.DB, conversation *store.Conversation, assistant store.App, userID string, now time.Time) error {
	if err := ensureBuiltinAssistantConversationFields(db, conversation, assistant, userID, now); err != nil {
		return err
	}
	if err := ensureBuiltinAssistantConversationMembers(db, conversation.ID, assistant.ID, userID, now); err != nil {
		return err
	}
	return ensureBuiltinAssistantAppConversation(db, assistant.ID, conversation.ID, userID, now)
}

func ensureBuiltinAssistantConversationFields(db *gorm.DB, conversation *store.Conversation, assistant store.App, userID string, now time.Time) error {
	updates := map[string]any{}
	if conversation.Kind != store.ConversationKindApp {
		updates["kind"] = store.ConversationKindApp
	}
	if conversation.Name != assistant.Name {
		updates["name"] = assistant.Name
	}
	if conversation.Avatar != assistant.Avatar {
		updates["avatar"] = assistant.Avatar
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
	for field, value := range updates {
		switch field {
		case "kind":
			conversation.Kind = value.(string)
		case "name":
			conversation.Name = value.(string)
		case "avatar":
			conversation.Avatar = value.(string)
		case "created_by_user_id":
			conversation.CreatedByUserID = value.(string)
		case "status":
			conversation.Status = value.(string)
		case "posting_policy":
			conversation.PostingPolicy = value.(string)
		case "visibility":
			conversation.Visibility = value.(string)
		case "updated_at":
			conversation.UpdatedAt = value.(time.Time)
		}
	}
	return nil
}

func ensureBuiltinAssistantConversationMembers(db *gorm.DB, conversationID, appID, userID string, now time.Time) error {
	if err := ensureBuiltinAssistantConversationMember(db, store.ConversationMember{
		ConversationID: conversationID, MemberType: store.ConversationMemberTypeUser,
		MemberID: userID, Role: store.ConversationMemberRoleOwner,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}); err != nil {
		return err
	}
	return ensureBuiltinAssistantConversationMember(db, store.ConversationMember{
		ConversationID: conversationID, MemberType: store.ConversationMemberTypeApp,
		MemberID: appID, Role: store.ConversationMemberRoleMember,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	})
}

func ensureBuiltinAssistantConversationMember(db *gorm.DB, member store.ConversationMember) error {
	var existing store.ConversationMember
	err := db.First(
		&existing,
		"conversation_id = ? AND member_type = ? AND member_id = ?",
		member.ConversationID,
		member.MemberType,
		member.MemberID,
	).Error
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
	return db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", member.ConversationID, member.MemberType, member.MemberID).
		Updates(updates).Error
}

func ensureBuiltinAssistantAppConversation(db *gorm.DB, appID, conversationID, userID string, now time.Time) error {
	var existing store.AppConversation
	err := db.First(&existing, "app_id = ? AND user_id = ?", appID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&store.AppConversation{
			AppID: appID, ConversationID: conversationID, UserID: userID, CreatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	if existing.ConversationID == conversationID {
		return nil
	}
	return db.Model(&store.AppConversation{}).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Update("conversation_id", conversationID).Error
}

type conversationProjectResponse struct {
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

type conversationMemberResponse struct {
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Type     string `json:"type"`
}

type groupConversationResponse struct {
	Avatar             string                       `json:"avatar"`
	CreatedAt          time.Time                    `json:"created_at"`
	CreatedByUserID    string                       `json:"created_by_user_id"`
	ID                 string                       `json:"id"`
	LastMessageAt      *time.Time                   `json:"last_message_at"`
	LastMessageID      *string                      `json:"last_message_id"`
	LastMessageSeq     int64                        `json:"last_message_seq"`
	LastMessageSummary string                       `json:"last_message_summary"`
	LastMentionedSeq   int64                        `json:"last_mentioned_seq"`
	LastReadSeq        int64                        `json:"last_read_seq"`
	MemberCount        int                          `json:"member_count"`
	Members            []conversationMemberResponse `json:"members"`
	Name               string                       `json:"name"`
	PostingPolicy      string                       `json:"posting_policy"`
	Status             string                       `json:"status"`
	Type               string                       `json:"type"`
	UnreadCount        int64                        `json:"unread_count"`
	Visibility         string                       `json:"visibility"`
}

type conversationListItemResponse struct {
	Avatar             string                         `json:"avatar"`
	CreatedAt          time.Time                      `json:"created_at"`
	ID                 string                         `json:"id"`
	LastMessageAt      *time.Time                     `json:"last_message_at"`
	LastMessageID      *string                        `json:"last_message_id"`
	LastMessageSeq     int64                          `json:"last_message_seq"`
	LastMessageSummary string                         `json:"last_message_summary"`
	LastMentionedSeq   int64                          `json:"last_mentioned_seq"`
	LastReadSeq        int64                          `json:"last_read_seq"`
	MemberCount        int                            `json:"member_count"`
	Members            []conversationMemberResponse   `json:"members"`
	Name               string                         `json:"name"`
	Projects           *[]conversationProjectResponse `json:"projects,omitempty"`
	Type               string                         `json:"type"`
	UnreadCount        int64                          `json:"unread_count"`
	Visibility         string                         `json:"visibility"`
}

type systemEventUserRef struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
	Type        string `json:"type,omitempty"`
}

type messageRevokedSystemEventBody struct {
	Actor systemEventUserRef `json:"actor"`
	Event string             `json:"event"`
	Type  string             `json:"type"`
}

func normalizeGroupConversationName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", errors.New("群聊名称不能为空")
	}
	if len([]rune(name)) > conversationapp.MaxGroupNameLength {
		return "", errors.New("群聊名称不能超过 120 个字符")
	}
	return name, nil
}

func normalizeGroupMemberIDs(rawIDs []string, creatorID string) ([]string, error) {
	parsedCreatorID, err := uuid.Parse(creatorID)
	if err != nil {
		return nil, errors.New("当前用户 ID 格式错误")
	}
	seen := map[string]struct{}{parsedCreatorID.String(): {}}
	memberIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, errors.New("成员 ID 不能为空")
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, errors.New("成员 ID 格式错误")
		}
		id = parsedID.String()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		memberIDs = append(memberIDs, id)
	}
	return memberIDs, nil
}

func orderDirectConversationUserIDs(first, second string) (string, string) {
	ids := []string{strings.TrimSpace(first), strings.TrimSpace(second)}
	sort.Strings(ids)
	return ids[0], ids[1]
}

func findDirectConversationByUserPair(db *gorm.DB, userLowID, userHighID string) (store.Conversation, error) {
	var direct store.DirectConversation
	if err := db.First(&direct, "user_low_id = ? AND user_high_id = ?", userLowID, userHighID).Error; err != nil {
		return store.Conversation{}, err
	}
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", direct.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	return conversation, nil
}

func findAppConversationByUserAndApp(db *gorm.DB, appID, userID string) (store.Conversation, error) {
	var relation store.AppConversation
	if err := db.First(&relation, "app_id = ? AND user_id = ?", appID, userID).Error; err != nil {
		return store.Conversation{}, err
	}
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", relation.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	return conversation, nil
}

func (s *Server) loadConversationListMembers(conversationIDs []string) (map[string][]store.ConversationMember, map[string]store.User, map[string]store.App, error) {
	byConversation := make(map[string][]store.ConversationMember, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return byConversation, map[string]store.User{}, map[string]store.App{}, nil
	}
	var members []store.ConversationMember
	if err := s.db.Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).Order("conversation_id ASC").Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, nil, nil, err
	}
	for _, member := range members {
		byConversation[member.ConversationID] = append(byConversation[member.ConversationID], member)
	}
	users, apps, err := s.loadConversationMemberIdentities(members)
	if err != nil {
		return nil, nil, nil, err
	}
	return byConversation, users, apps, nil
}

func (s *Server) loadConversationMemberIdentities(members []store.ConversationMember) (map[string]store.User, map[string]store.App, error) {
	usersByID := make(map[string]store.User)
	appsByID := make(map[string]store.App)
	userSet, appSet := make(map[string]struct{}), make(map[string]struct{})
	for _, member := range members {
		switch member.MemberType {
		case store.ConversationMemberTypeUser:
			userSet[member.MemberID] = struct{}{}
		case store.ConversationMemberTypeApp:
			appSet[member.MemberID] = struct{}{}
		}
	}
	userIDs := keys(userSet)
	if len(userIDs) > 0 {
		var users []store.User
		if err := s.db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, nil, err
		}
		for _, user := range users {
			usersByID[user.ID] = user
		}
	}
	appIDs := keys(appSet)
	if len(appIDs) > 0 {
		var apps []store.App
		if err := s.db.Unscoped().Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
			return nil, nil, err
		}
		for _, app := range apps {
			appsByID[app.ID] = app
		}
	}
	return usersByID, appsByID, nil
}

func keys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func newConversationListItemResponse(conversation store.Conversation, currentUserID string, members []store.ConversationMember, users map[string]store.User, apps map[string]store.App) conversationListItemResponse {
	name, avatar := conversation.Name, conversation.Avatar
	lastReadSeq := currentMemberLastReadSeq(currentUserID, members)
	lastMentionedSeq := currentMemberLastMentionedSeq(currentUserID, members)
	if conversation.Kind == store.ConversationKindDirect {
		for _, member := range members {
			if member.MemberID == currentUserID {
				continue
			}
			other, ok := users[member.MemberID]
			if !ok {
				continue
			}
			name, avatar = userDisplayName(other), other.Avatar
			if avatar == "" {
				avatar = store.DefaultUserAvatar
			}
			break
		}
		if strings.TrimSpace(name) == "" {
			name = "私聊"
		}
	} else if strings.TrimSpace(name) == "" {
		name = "群聊"
	}
	return conversationListItemResponse{
		Avatar: avatar, CreatedAt: conversation.CreatedAt, ID: conversation.ID,
		LastMessageAt: conversation.LastMessageAt, LastMessageID: conversation.LastMessageID,
		LastMessageSeq: conversation.LastMessageSeq, LastMessageSummary: conversation.LastMessageSummary,
		LastMentionedSeq: lastMentionedSeq, LastReadSeq: lastReadSeq,
		MemberCount: conversationListMemberCount(conversation.Kind, members),
		Members:     newConversationMemberResponses(members, users, apps), Name: name, Type: conversation.Kind,
		UnreadCount: unreadCount(conversation.LastMessageSeq, lastReadSeq), Visibility: conversation.Visibility,
	}
}

func currentMemberLastReadSeq(currentUserID string, members []store.ConversationMember) int64 {
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser && member.MemberID == currentUserID {
			return member.LastReadSeq
		}
	}
	return 0
}

func currentMemberLastMentionedSeq(currentUserID string, members []store.ConversationMember) int64 {
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser && member.MemberID == currentUserID {
			return member.LastMentionedSeq
		}
	}
	return 0
}

func conversationListMemberCount(kind string, members []store.ConversationMember) int {
	if kind != store.ConversationKindApp {
		return len(members)
	}
	count := 0
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeUser {
			count++
		}
	}
	return count
}

func unreadCount(lastMessageSeq, lastReadSeq int64) int64 {
	if lastReadSeq >= lastMessageSeq {
		return 0
	}
	return lastMessageSeq - lastReadSeq
}

func newConversationMemberResponses(members []store.ConversationMember, users map[string]store.User, apps map[string]store.App) []conversationMemberResponse {
	responses := make([]conversationMemberResponse, 0, len(members))
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeApp {
			if app, ok := apps[member.MemberID]; ok {
				responses = append(responses, conversationMemberResponse{Avatar: app.Avatar, ID: app.ID, Name: app.Name, Role: member.Role, Type: store.ConversationMemberTypeApp})
			}
			continue
		}
		user, ok := users[member.MemberID]
		if !ok {
			continue
		}
		phone := ""
		if user.Phone != nil {
			phone = *user.Phone
		}
		avatar := user.Avatar
		if avatar == "" {
			avatar = store.DefaultUserAvatar
		}
		responses = append(responses, conversationMemberResponse{Avatar: avatar, Email: user.Email, ID: user.ID, Name: user.Name, Nickname: user.Nickname, Phone: phone, Role: member.Role, Type: store.ConversationMemberTypeUser})
	}
	return responses
}

func userDisplayName(user store.User) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}
	return strings.TrimSpace(user.Name)
}

func newMessageRevokedSystemEventBody(actor store.User) (json.RawMessage, string, error) {
	displayName := userDisplayName(actor)
	body, err := json.Marshal(messageRevokedSystemEventBody{
		Actor: systemEventUserRef{DisplayName: displayName, ID: actor.ID},
		Event: systemEventMessageRevoked,
		Type:  messageTypeSystemEvent,
	})
	if err != nil {
		return nil, "", err
	}
	return body, displayName + " 撤回了一条消息", nil
}

func revokedMessageSummary() string { return "该消息已被撤回" }
