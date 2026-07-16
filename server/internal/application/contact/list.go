package contact

import (
	"context"
	"sort"

	"app/internal/appregistry"
	"app/internal/store"

	"gorm.io/gorm"
)

func (s *Service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	db := s.db.WithContext(ctx)
	if _, err := appregistry.EnsureAIAssistantApp(db, s.apps); err != nil {
		return ListResult{}, internalError(err)
	}
	keyword := normalizeKeyword(cmd.Keyword)
	users, err := s.listUsers(db, keyword)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	identity := Identity{ID: cmd.AccountID, Type: IdentityTypeUser}
	apps, err := s.listApps(db, identity, keyword)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	groups, err := s.listGroups(db, identity, keyword)
	if err != nil {
		return ListResult{}, internalError(err)
	}
	return ListResult{Apps: apps, Groups: groups, Users: users}, nil
}

func (s *Service) ListUsers(ctx context.Context, cmd ListUsersCommand) (ListUsersResult, error) {
	users, err := s.listUsers(s.db.WithContext(ctx), normalizeKeyword(cmd.Keyword))
	if err != nil {
		return ListUsersResult{}, internalError(err)
	}
	return ListUsersResult{Users: users}, nil
}

func (s *Service) ListAppsForIdentity(ctx context.Context, cmd ListForIdentityCommand) (ListAppsResult, error) {
	apps, err := s.listApps(s.db.WithContext(ctx), cmd.Identity, normalizeKeyword(cmd.Keyword))
	if err != nil {
		return ListAppsResult{}, internalError(err)
	}
	return ListAppsResult{Apps: apps}, nil
}

func (s *Service) ListGroupsForIdentity(ctx context.Context, cmd ListForIdentityCommand) (ListGroupsResult, error) {
	groups, err := s.listGroups(s.db.WithContext(ctx), cmd.Identity, normalizeKeyword(cmd.Keyword))
	if err != nil {
		return ListGroupsResult{}, internalError(err)
	}
	return ListGroupsResult{Groups: groups}, nil
}

func (s *Service) listUsers(db *gorm.DB, keyword string) ([]User, error) {
	query := db.Model(&store.User{}).Where("status = ?", store.UserStatusActive)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(nickname) LIKE ? OR phone LIKE ?", like, like, like, like)
	}
	var values []store.User
	if err := query.Order("name ASC").Order("email ASC").Order("id ASC").Find(&values).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	online := map[string]bool{}
	if s.userPresence != nil {
		online = s.userPresence.OnlineStatus(ids)
	}
	result := make([]User, 0, len(values))
	for _, value := range values {
		result = append(result, newUser(value, online[value.ID]))
	}
	return result, nil
}

func (s *Service) listApps(db *gorm.DB, identity Identity, keyword string) ([]App, error) {
	query := db.Model(&store.App{}).Where("enabled = ?", true)
	if identity.Type == IdentityTypeApp {
		query = query.Where("visibility = ? OR id = ?", store.AppVisibilityPublic, identity.ID)
	} else {
		query = query.Where(
			"visibility = ? OR (visibility = ? AND creator_user_id = ?)",
			store.AppVisibilityPublic,
			store.AppVisibilityCreator,
			identity.ID,
		)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}
	var values []store.App
	if err := query.Order("LOWER(name) ASC").Order("id ASC").Find(&values).Error; err != nil {
		return nil, err
	}
	result := make([]App, 0, len(values))
	for _, value := range values {
		online := s.appPresence != nil && s.appPresence.IsOnline(value.ID)
		result = append(result, App{
			Avatar: value.Avatar, Description: value.Description, ID: value.ID,
			Name: value.Name, Online: online, Type: ContactTypeApp,
		})
	}
	return result, nil
}

func (s *Service) listGroups(db *gorm.DB, identity Identity, keyword string) ([]Group, error) {
	memberType := store.ConversationMemberTypeUser
	if identity.Type == IdentityTypeApp {
		memberType = store.ConversationMemberTypeApp
	}
	const memberExistsSQL = "EXISTS (SELECT 1 FROM conversation_members cm WHERE cm.conversation_id = conversations.id AND cm.member_type = ? AND cm.member_id = ? AND cm.left_at IS NULL)"
	query := db.Model(&store.Conversation{}).
		Where("kind = ? AND status = ?", store.ConversationKindGroup, store.ConversationStatusActive).
		Where("(visibility = ? OR "+memberExistsSQL+")", store.ConversationVisibilityPublic, memberType, identity.ID)
	if keyword != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+keyword+"%")
	}
	var values []store.Conversation
	if err := query.
		Order(gorm.Expr("CASE WHEN "+memberExistsSQL+" THEN 0 ELSE 1 END", memberType, identity.ID)).
		Order("LOWER(name) ASC").Order("id ASC").Find(&values).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.ID)
	}
	memberCounts, joined, err := loadGroupMembership(db, identity, ids)
	if err != nil {
		return nil, err
	}
	joinedIDs := make([]string, 0, len(joined))
	for _, value := range values {
		if joined[value.ID] {
			joinedIDs = append(joinedIDs, value.ID)
		}
	}
	avatarMembers, err := loadGroupAvatarMembers(db, joinedIDs)
	if err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(values))
	for _, value := range values {
		result = append(result, Group{
			Avatar: value.Avatar, AvatarMembers: avatarMembers[value.ID], ID: value.ID,
			Joined: joined[value.ID], MemberCount: memberCounts[value.ID], Name: value.Name,
			Type: ContactTypeGroup, Visibility: value.Visibility,
		})
	}
	return result, nil
}

func loadGroupMembership(db *gorm.DB, identity Identity, groupIDs []string) (map[string]int, map[string]bool, error) {
	memberCounts := make(map[string]int, len(groupIDs))
	joined := make(map[string]bool, len(groupIDs))
	if len(groupIDs) == 0 {
		return memberCounts, joined, nil
	}
	type countRow struct {
		ConversationID string
		Count          int
	}
	var counts []countRow
	if err := db.Model(&store.ConversationMember{}).
		Select("conversation_id, COUNT(*) AS count").
		Where("conversation_id IN ? AND member_type = ? AND left_at IS NULL", groupIDs, store.ConversationMemberTypeUser).
		Group("conversation_id").Scan(&counts).Error; err != nil {
		return nil, nil, err
	}
	for _, count := range counts {
		memberCounts[count.ConversationID] = count.Count
	}
	memberType := store.ConversationMemberTypeUser
	if identity.Type == IdentityTypeApp {
		memberType = store.ConversationMemberTypeApp
	}
	var members []store.ConversationMember
	if err := db.Where(
		"conversation_id IN ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		groupIDs, memberType, identity.ID,
	).Find(&members).Error; err != nil {
		return nil, nil, err
	}
	for _, member := range members {
		joined[member.ConversationID] = true
	}
	return memberCounts, joined, nil
}

func loadGroupAvatarMembers(db *gorm.DB, groupIDs []string) (map[string][]GroupAvatarMember, error) {
	result := make(map[string][]GroupAvatarMember, len(groupIDs))
	if len(groupIDs) == 0 {
		return result, nil
	}
	ranked := db.Model(&store.ConversationMember{}).
		Select(`conversation_members.*, ROW_NUMBER() OVER (
			PARTITION BY conversation_id
			ORDER BY CASE role WHEN ? THEN 0 WHEN ? THEN 1 ELSE 2 END,
			joined_at ASC, member_type ASC, member_id ASC
		) AS avatar_rank`, store.ConversationMemberRoleOwner, store.ConversationMemberRoleAdmin).
		Where("conversation_id IN ? AND left_at IS NULL", groupIDs)
	var members []store.ConversationMember
	if err := db.Table("(?) AS ranked_members", ranked).
		Where("avatar_rank <= 4").Order("conversation_id ASC").Order("avatar_rank ASC").
		Scan(&members).Error; err != nil {
		return nil, err
	}
	users, apps, err := loadMemberIdentities(db, members)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeApp {
			if app, ok := apps[member.MemberID]; ok {
				result[member.ConversationID] = append(result[member.ConversationID], GroupAvatarMember{Avatar: app.Avatar, Name: app.Name, Role: member.Role})
			}
			continue
		}
		if user, ok := users[member.MemberID]; ok {
			avatar := user.Avatar
			if avatar == "" {
				avatar = store.DefaultUserAvatar
			}
			result[member.ConversationID] = append(result[member.ConversationID], GroupAvatarMember{Avatar: avatar, Name: user.Name, Nickname: user.Nickname, Role: member.Role})
		}
	}
	for groupID, values := range result {
		sort.SliceStable(values, func(left, right int) bool {
			return avatarMemberRoleRank(values[left].Role) < avatarMemberRoleRank(values[right].Role)
		})
		if len(values) > 4 {
			values = values[:4]
		}
		result[groupID] = values
	}
	return result, nil
}

func loadMemberIdentities(db *gorm.DB, members []store.ConversationMember) (map[string]store.User, map[string]store.App, error) {
	userSet, appSet := map[string]struct{}{}, map[string]struct{}{}
	for _, member := range members {
		if member.MemberType == store.ConversationMemberTypeApp {
			appSet[member.MemberID] = struct{}{}
		} else if member.MemberType == store.ConversationMemberTypeUser {
			userSet[member.MemberID] = struct{}{}
		}
	}
	users := make(map[string]store.User, len(userSet))
	if ids := setKeys(userSet); len(ids) > 0 {
		var values []store.User
		if err := db.Where("id IN ?", ids).Find(&values).Error; err != nil {
			return nil, nil, err
		}
		for _, value := range values {
			users[value.ID] = value
		}
	}
	apps := make(map[string]store.App, len(appSet))
	if ids := setKeys(appSet); len(ids) > 0 {
		var values []store.App
		if err := db.Unscoped().Where("id IN ?", ids).Find(&values).Error; err != nil {
			return nil, nil, err
		}
		for _, value := range values {
			apps[value.ID] = value
		}
	}
	return users, apps, nil
}

func setKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func avatarMemberRoleRank(role string) int {
	switch role {
	case store.ConversationMemberRoleOwner:
		return 0
	case store.ConversationMemberRoleAdmin:
		return 1
	default:
		return 2
	}
}

func newUser(value store.User, online bool) User {
	phone := ""
	if value.Phone != nil {
		phone = *value.Phone
	}
	avatar := value.Avatar
	if avatar == "" {
		avatar = store.DefaultUserAvatar
	}
	return User{
		Avatar: avatar, Email: value.Email, ID: value.ID, LastOnlineAt: value.LastOnlineAt,
		Name: value.Name, Nickname: value.Nickname, Online: online, Phone: phone, Type: ContactTypeUser,
	}
}
