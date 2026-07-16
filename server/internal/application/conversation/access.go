package conversation

import (
	"errors"
	"sort"
	"strings"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type memberCandidate struct {
	app        store.App
	memberType string
	role       string
	user       store.User
}

func normalizeUUID(rawID, message string) (string, error) {
	id := strings.TrimSpace(rawID)
	parsed, err := uuid.Parse(id)
	if id == "" || err != nil {
		return "", errors.New(message)
	}
	return parsed.String(), nil
}

func normalizeConversationID(rawID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New("会话 ID 不能为空")
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("会话 ID 格式错误")
	}
	return parsed.String(), nil
}

func normalizeDirectUserID(rawID, currentUserID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New("用户 ID 不能为空")
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("用户 ID 格式错误")
	}
	current, err := uuid.Parse(currentUserID)
	if err != nil {
		return "", errors.New("当前用户 ID 格式错误")
	}
	if parsed == current {
		return "", errors.New("不能和自己创建私聊")
	}
	return parsed.String(), nil
}

func normalizeGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("群聊名称不能为空")
	}
	if len([]rune(name)) > MaxGroupNameLength {
		return "", errors.New("群聊名称不能超过 120 个字符")
	}
	return name, nil
}

func normalizeMemberIDs(rawIDs []string, creatorID string) ([]string, error) {
	parsedCreatorID, err := uuid.Parse(creatorID)
	if err != nil {
		return nil, errors.New("当前用户 ID 格式错误")
	}
	seen := map[string]struct{}{parsedCreatorID.String(): {}}
	ids := make([]string, 0, len(rawIDs))
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
		ids = append(ids, id)
	}
	return ids, nil
}

func normalizeAppIDs(rawIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawIDs))
	ids := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, errors.New("应用 ID 不能为空")
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, errors.New("应用 ID 格式错误")
		}
		id = parsedID.String()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func normalizeProjectIDs(rawIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawIDs))
	ids := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := normalizeUUID(rawID, "项目 ID 格式错误")
		if err != nil {
			return nil, ErrProjectInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func loadActiveGroupMembers(db *gorm.DB, memberIDs []string) ([]store.User, error) {
	var users []store.User
	if err := db.Where("id IN ? AND status = ?", memberIDs, store.UserStatusActive).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) != len(memberIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	byID := make(map[string]store.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}
	ordered := make([]store.User, 0, len(memberIDs))
	for _, id := range memberIDs {
		user, ok := byID[id]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		ordered = append(ordered, user)
	}
	return ordered, nil
}

func loadVisibleGroupApps(db *gorm.DB, currentUserID string, appIDs []string) ([]store.App, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	var apps []store.App
	if err := db.Where("id IN ? AND enabled = ?", appIDs, true).
		Where("visibility = ? OR (visibility = ? AND creator_user_id = ?)", store.AppVisibilityPublic, store.AppVisibilityCreator, currentUserID).
		Find(&apps).Error; err != nil {
		return nil, err
	}
	if len(apps) != len(appIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	byID := make(map[string]store.App, len(apps))
	for _, app := range apps {
		byID[app.ID] = app
	}
	ordered := make([]store.App, 0, len(appIDs))
	for _, id := range appIDs {
		app, ok := byID[id]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		ordered = append(ordered, app)
	}
	return ordered, nil
}

func (s *Service) loadListMembers(db *gorm.DB, conversationIDs []string) (map[string][]store.ConversationMember, map[string]store.User, map[string]store.App, error) {
	byConversation := make(map[string][]store.ConversationMember, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return byConversation, map[string]store.User{}, map[string]store.App{}, nil
	}
	var members []store.ConversationMember
	if err := db.Where("conversation_id IN ? AND left_at IS NULL", conversationIDs).
		Order("conversation_id ASC").Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, nil, nil, err
	}
	for _, member := range members {
		byConversation[member.ConversationID] = append(byConversation[member.ConversationID], member)
	}
	users, apps, err := loadMemberIdentities(db, members)
	if err != nil {
		return nil, nil, nil, err
	}
	return byConversation, users, apps, nil
}

func loadMemberIdentities(db *gorm.DB, members []store.ConversationMember) (map[string]store.User, map[string]store.App, error) {
	usersByID := make(map[string]store.User)
	appsByID := make(map[string]store.App)
	userSet := make(map[string]struct{})
	appSet := make(map[string]struct{})
	for _, member := range members {
		switch member.MemberType {
		case store.ConversationMemberTypeUser:
			userSet[member.MemberID] = struct{}{}
		case store.ConversationMemberTypeApp:
			appSet[member.MemberID] = struct{}{}
		}
	}
	userIDs := sortedKeys(userSet)
	if len(userIDs) > 0 {
		var users []store.User
		if err := db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, nil, err
		}
		for _, user := range users {
			usersByID[user.ID] = user
		}
	}
	appIDs := sortedKeys(appSet)
	if len(appIDs) > 0 {
		var apps []store.App
		if err := db.Unscoped().Where("id IN ?", appIDs).Find(&apps).Error; err != nil {
			return nil, nil, err
		}
		for _, app := range apps {
			appsByID[app.ID] = app
		}
	}
	return usersByID, appsByID, nil
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) loadItem(db *gorm.DB, conversation store.Conversation, currentUserID string) (Item, error) {
	membersByConversation, users, apps, err := s.loadListMembers(db, []string{conversation.ID})
	if err != nil {
		return Item{}, err
	}
	return newItem(conversation, currentUserID, membersByConversation[conversation.ID], users, apps), nil
}

func newItem(conversation store.Conversation, currentUserID string, members []store.ConversationMember, users map[string]store.User, apps map[string]store.App) Item {
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
	return Item{
		Avatar: avatar, CreatedAt: conversation.CreatedAt, ID: conversation.ID,
		LastMessageAt: conversation.LastMessageAt, LastMessageID: conversation.LastMessageID,
		LastMessageSeq: conversation.LastMessageSeq, LastMessageSummary: conversation.LastMessageSummary,
		LastMentionedSeq: lastMentionedSeq, LastReadSeq: lastReadSeq,
		MemberCount: listMemberCount(conversation.Kind, members), Members: newMembers(members, users, apps),
		Name: name, Type: conversation.Kind, UnreadCount: unreadCount(conversation.LastMessageSeq, lastReadSeq),
		Visibility: conversation.Visibility,
	}
}

func newGroup(conversation store.Conversation, candidates []memberCandidate, currentUserID string) Group {
	members := make([]Member, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.memberType == store.ConversationMemberTypeApp {
			members = append(members, Member{Avatar: candidate.app.Avatar, ID: candidate.app.ID, Name: candidate.app.Name, Role: candidate.role, Type: store.ConversationMemberTypeApp})
			continue
		}
		members = append(members, newUserMember(candidate.user, candidate.role))
	}
	lastReadSeq := int64(0)
	if currentUserID == conversation.CreatedByUserID {
		lastReadSeq = conversation.LastMessageSeq
	}
	return Group{
		Avatar: conversation.Avatar, CreatedAt: conversation.CreatedAt, CreatedByUserID: conversation.CreatedByUserID,
		ID: conversation.ID, LastMessageAt: conversation.LastMessageAt, LastMessageID: conversation.LastMessageID,
		LastMessageSeq: conversation.LastMessageSeq, LastMessageSummary: conversation.LastMessageSummary,
		LastReadSeq: lastReadSeq, MemberCount: len(members), Members: members, Name: conversation.Name,
		PostingPolicy: conversation.PostingPolicy, Status: conversation.Status, Type: conversation.Kind,
		UnreadCount: unreadCount(conversation.LastMessageSeq, lastReadSeq), Visibility: conversation.Visibility,
	}
}

func newMembers(members []store.ConversationMember, users map[string]store.User, apps map[string]store.App) []Member {
	result := make([]Member, 0, len(members))
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeApp {
			app, ok := apps[member.MemberID]
			if ok {
				result = append(result, Member{Avatar: app.Avatar, ID: app.ID, Name: app.Name, Role: member.Role, Type: store.ConversationMemberTypeApp})
			}
			continue
		}
		user, ok := users[member.MemberID]
		if ok {
			result = append(result, newUserMember(user, member.Role))
		}
	}
	return result
}

func newUserMember(user store.User, role string) Member {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	avatar := user.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}
	return Member{Avatar: avatar, Email: user.Email, ID: user.ID, Name: user.Name, Nickname: user.Nickname, Phone: phone, Role: role, Type: store.ConversationMemberTypeUser}
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

func listMemberCount(kind string, members []store.ConversationMember) int {
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

func userDisplayName(user store.User) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}
	return strings.TrimSpace(user.Name)
}

func memberKey(memberType, memberID string) string { return memberType + "/" + memberID }

func loadActiveUserIDs(db *gorm.DB, conversationID string) ([]string, error) {
	var members []store.ConversationMember
	if err := db.Where("conversation_id = ? AND member_type = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser).
		Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.MemberID)
	}
	return ids, nil
}

func advanceReadSeq(db *gorm.DB, conversationID, userID string, seq int64) error {
	return db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, userID).
		Update("last_read_seq", gorm.Expr("CASE WHEN last_read_seq > ? THEN last_read_seq ELSE ? END", seq, seq)).Error
}

func canManage(role string) bool {
	return role == store.ConversationMemberRoleOwner || role == store.ConversationMemberRoleAdmin
}

func canSetVisibility(role string) bool { return role == store.ConversationMemberRoleOwner }

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
