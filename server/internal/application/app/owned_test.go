package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceManagesOwnedAppsAndUserAccess(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "owner@example.com", now)
	granted := insertOwnedAppTestUser(t, db, "granted@example.com", now)
	other := insertOwnedAppTestUser(t, db, "other@example.com", now)
	connections := &fakeAppConnections{online: map[string]bool{}}
	secretNumber := 0
	service := NewService(Dependencies{
		DB: db, Connections: connections, Now: func() time.Time { return now },
		GenerateSecret: func() (string, error) {
			secretNumber++
			return fmt.Sprintf("owned-secret-%d", secretNumber), nil
		},
	})

	restricted, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "  财务机器人  ", Description: "  处理财务消息  ",
		Visibility: VisibilityRestricted, UserIDs: []string{owner.ID, granted.ID, granted.ID},
	})
	if err != nil {
		t.Fatalf("create restricted app: %v", err)
	}
	if restricted.Name != "财务机器人" || restricted.Description != "处理财务消息" ||
		restricted.Visibility != VisibilityRestricted || restricted.ConnectionSecret != "owned-secret-1" ||
		restricted.CreatorUserID == nil || *restricted.CreatorUserID != owner.ID ||
		len(restricted.GrantedUserIDs) != 1 || restricted.GrantedUserIDs[0] != granted.ID {
		t.Fatalf("restricted app = %#v", restricted)
	}

	creatorOnly, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "私人应用", Visibility: VisibilityCreator,
	})
	if err != nil {
		t.Fatalf("create creator-only app: %v", err)
	}
	public, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "公共应用", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create public app: %v", err)
	}

	for _, test := range []struct {
		name   string
		appID  string
		userID string
		want   bool
	}{
		{name: "restricted owner", appID: restricted.ID, userID: owner.ID, want: true},
		{name: "restricted grant", appID: restricted.ID, userID: granted.ID, want: true},
		{name: "restricted other", appID: restricted.ID, userID: other.ID, want: false},
		{name: "creator owner", appID: creatorOnly.ID, userID: owner.ID, want: true},
		{name: "creator other", appID: creatorOnly.ID, userID: other.ID, want: false},
		{name: "public other", appID: public.ID, userID: other.ID, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.CanUserAccess(context.Background(), test.appID, test.userID)
			if err != nil || got != test.want {
				t.Fatalf("CanUserAccess() = %t, %v, want %t", got, err, test.want)
			}
		})
	}

	owned, err := service.ListOwned(context.Background(), owner.ID)
	if err != nil || len(owned) != 3 {
		t.Fatalf("owner list = %#v, err = %v", owned, err)
	}
	if values, err := service.ListOwned(context.Background(), other.ID); err != nil || len(values) != 0 {
		t.Fatalf("other list = %#v, err = %v", values, err)
	}
	if _, err := service.GetOwned(context.Background(), OwnedAppCommand{AccountID: other.ID, AppID: restricted.ID}); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("other get error = %v", err)
	}

	newGrants := []string{other.ID}
	updated, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: owner.ID, AppID: restricted.ID, UserIDs: &newGrants,
	})
	if err != nil || len(updated.GrantedUserIDs) != 1 || updated.GrantedUserIDs[0] != other.ID {
		t.Fatalf("updated grants = %#v, err = %v", updated, err)
	}
	if allowed, err := service.CanUserAccess(context.Background(), restricted.ID, granted.ID); err != nil || allowed {
		t.Fatalf("old grant access = %t, %v", allowed, err)
	}
	if allowed, err := service.CanUserAccess(context.Background(), restricted.ID, other.ID); err != nil || !allowed {
		t.Fatalf("new grant access = %t, %v", allowed, err)
	}

	connections.online[restricted.ID] = true
	now = now.Add(time.Hour)
	disabled, err := service.SetOwnedEnabled(context.Background(), SetOwnedEnabledCommand{
		AccountID: owner.ID, AppID: restricted.ID, Enabled: false,
	})
	if err != nil || disabled.Enabled || connections.closeCalls != 2 {
		t.Fatalf("disabled = %#v, close calls = %d, err = %v", disabled, connections.closeCalls, err)
	}
	if allowed, err := service.CanUserAccess(context.Background(), restricted.ID, owner.ID); err != nil || allowed {
		t.Fatalf("disabled owner access = %t, %v", allowed, err)
	}
	if _, err := service.SetOwnedEnabled(context.Background(), SetOwnedEnabledCommand{
		AccountID: owner.ID, AppID: restricted.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("enable owned app: %v", err)
	}
	connections.online[restricted.ID] = true
	rotated, err := service.RegenerateOwnedSecret(context.Background(), OwnedAppCommand{
		AccountID: owner.ID, AppID: restricted.ID,
	})
	if err != nil || rotated.ConnectionSecret != "owned-secret-4" || connections.closeCalls != 3 {
		t.Fatalf("rotated = %#v, close calls = %d, err = %v", rotated, connections.closeCalls, err)
	}
	if err := service.DeleteOwned(context.Background(), OwnedAppCommand{AccountID: owner.ID, AppID: restricted.ID}); err != nil {
		t.Fatalf("delete owned app: %v", err)
	}
	var remainingGrants int64
	if err := db.Model(&store.AppUserGrant{}).Where("app_id = ?", restricted.ID).Count(&remainingGrants).Error; err != nil || remainingGrants != 0 {
		t.Fatalf("remaining grants = %d, err = %v", remainingGrants, err)
	}
	if _, err := service.GetOwned(context.Background(), OwnedAppCommand{AccountID: owner.ID, AppID: restricted.ID}); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("get deleted error = %v", err)
	}
}

func TestServiceRejectsInvalidOwnedAppManagement(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Now().UTC()
	owner := insertOwnedAppTestUser(t, db, "validation-owner@example.com", now)
	service := NewService(Dependencies{DB: db, GenerateSecret: func() (string, error) { return "secret", nil }})

	for name, command := range map[string]CreateOwnedCommand{
		"empty name":         {AccountID: owner.ID, Visibility: VisibilityCreator},
		"long description":   {AccountID: owner.ID, Name: "应用", Description: strings.Repeat("备", MaxDescriptionLength+1)},
		"unknown visibility": {AccountID: owner.ID, Name: "应用", Visibility: "private"},
		"missing grant":      {AccountID: owner.ID, Name: "应用", Visibility: VisibilityRestricted, UserIDs: []string{uuid.NewString()}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.CreateOwned(context.Background(), command); ErrorCodeOf(err) != CodeInvalidRequest {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOwnedAppAuthorizationChangesKeepMembershipsAndRevokeUnauthorizedDirectEvents(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "lifecycle-owner@example.com", now)
	allowed := insertOwnedAppTestUser(t, db, "lifecycle-allowed@example.com", now)
	denied := insertOwnedAppTestUser(t, db, "lifecycle-denied@example.com", now)
	service := NewService(Dependencies{
		DB: db, Now: func() time.Time { return now },
		GenerateSecret: func() (string, error) { return "lifecycle-secret", nil },
	})
	created, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "Lifecycle App", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	allowedConversation := insertOwnedAppConversation(t, db, created.ID, allowed.ID, now)
	deniedConversation := insertOwnedAppConversation(t, db, created.ID, denied.ID, now)
	groupConversation := insertOwnedAppGroup(t, db, created.ID, owner.ID, now)
	for _, conversationID := range []string{allowedConversation.ID, deniedConversation.ID, groupConversation.ID} {
		insertOwnedAppEvent(t, db, created.ID, conversationID, now)
	}
	allowedTopicID := uuid.NewString()
	deniedTopicID := uuid.NewString()
	insertOwnedAppTopicMessageEvent(t, db, created.ID, allowedConversation.ID, allowedTopicID, now)
	insertOwnedAppTopicClosedEvent(t, db, created.ID, allowedConversation.ID, allowedTopicID, now)
	insertOwnedAppTopicMessageEvent(t, db, created.ID, deniedConversation.ID, deniedTopicID, now)
	insertOwnedAppTopicClosedEvent(t, db, created.ID, deniedConversation.ID, deniedTopicID, now)

	grants := []string{allowed.ID}
	visibility := VisibilityRestricted
	updated, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: owner.ID, AppID: created.ID, Visibility: &visibility, UserIDs: &grants,
	})
	if err != nil || updated.Visibility != VisibilityRestricted || len(updated.GrantedUserIDs) != 1 {
		t.Fatalf("update authorization = %#v, err = %v", updated, err)
	}
	requireOwnedAppRowCount(t, db, &store.AppConversation{}, 1, "app_id = ? AND user_id = ?", created.ID, allowed.ID)
	requireOwnedAppRowCount(t, db, &store.AppConversation{}, 1, "app_id = ? AND user_id = ?", created.ID, denied.ID)
	requireOwnedAppActiveMemberCount(t, db, created.ID, allowedConversation.ID, 1)
	requireOwnedAppActiveMemberCount(t, db, created.ID, deniedConversation.ID, 1)
	requireOwnedAppActiveMemberCount(t, db, created.ID, groupConversation.ID, 1)

	var storedDeniedConversation store.Conversation
	if err := db.First(&storedDeniedConversation, "id = ?", deniedConversation.ID).Error; err != nil {
		t.Fatalf("load denied conversation: %v", err)
	}
	if storedDeniedConversation.Status != store.ConversationStatusActive || storedDeniedConversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		t.Fatalf("denied conversation = %#v, want history retained", storedDeniedConversation)
	}
	var remainingEvents []store.AppEventOutbox
	if err := db.Where("app_id = ?", created.ID).Find(&remainingEvents).Error; err != nil {
		t.Fatalf("load remaining events: %v", err)
	}
	if len(remainingEvents) != 4 {
		t.Fatalf("remaining events = %#v", remainingEvents)
	}
	remainingPayloads := ""
	for _, event := range remainingEvents {
		remainingPayloads += string(event.Payload)
	}
	for _, expectedID := range []string{allowedConversation.ID, groupConversation.ID, allowedTopicID} {
		if !strings.Contains(remainingPayloads, expectedID) {
			t.Fatalf("remaining payloads missing %s: %s", expectedID, remainingPayloads)
		}
	}
	for _, deniedID := range []string{deniedConversation.ID, deniedTopicID} {
		if strings.Contains(remainingPayloads, deniedID) {
			t.Fatalf("remaining payloads contain denied ID %s: %s", deniedID, remainingPayloads)
		}
	}

	publicVisibility := VisibilityPublic
	if _, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: owner.ID, AppID: created.ID, Visibility: &publicVisibility,
	}); err != nil {
		t.Fatalf("make app public: %v", err)
	}
	if _, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: owner.ID, AppID: created.ID, Visibility: &visibility,
	}); err != nil {
		t.Fatalf("make app restricted again: %v", err)
	}
	requireOwnedAppRowCount(t, db, &store.AppUserGrant{}, 0, "app_id = ?", created.ID)

	if err := db.Create(&store.AppEventAck{AppID: created.ID, LastAckedCursor: 1, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create ack: %v", err)
	}
	if err := service.DeleteOwned(context.Background(), OwnedAppCommand{AccountID: owner.ID, AppID: created.ID}); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	requireOwnedAppRowCount(t, db, &store.AppConversation{}, 0, "app_id = ?", created.ID)
	requireOwnedAppRowCount(t, db, &store.AppUserGrant{}, 0, "app_id = ?", created.ID)
	requireOwnedAppRowCount(t, db, &store.AppEventOutbox{}, 0, "app_id = ?", created.ID)
	requireOwnedAppRowCount(t, db, &store.AppEventAck{}, 0, "app_id = ?", created.ID)
	var storedAllowedConversation store.Conversation
	if err := db.First(&storedAllowedConversation, "id = ?", allowedConversation.ID).Error; err != nil {
		t.Fatalf("load allowed conversation after delete: %v", err)
	}
	if storedAllowedConversation.Status != store.ConversationStatusActive || storedAllowedConversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		t.Fatalf("allowed conversation = %#v, want history retained", storedAllowedConversation)
	}
}

func TestOwnedAppAuthorizationShrinkKeepsOwnedGroupMembership(t *testing.T) {
	db := openAppTestDB(t)
	if err := db.Exec(`
		CREATE UNIQUE INDEX app_owned_group_one_owner_test
		ON conversation_members (conversation_id)
		WHERE role = 'owner' AND left_at IS NULL
	`).Error; err != nil {
		t.Fatalf("create one-owner index: %v", err)
	}
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	creator := insertOwnedAppTestUser(t, db, "group-transfer-creator@example.com", now)
	firstMember := insertOwnedAppTestUser(t, db, "group-transfer-first@example.com", now)
	admin := insertOwnedAppTestUser(t, db, "group-transfer-admin@example.com", now)
	service := NewService(Dependencies{
		DB: db, Now: func() time.Time { return now },
		GenerateSecret: func() (string, error) { return "group-transfer-secret", nil },
	})
	created, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: creator.ID, Name: "群主应用", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	creatorAppID := created.ID
	group := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "应用群主群",
		CreatedByAppID: &creatorAppID, CreatedByUserID: creator.ID,
		Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen,
		Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: group.ID, MemberType: store.ConversationMemberTypeApp, MemberID: created.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: group.ID, MemberType: store.ConversationMemberTypeUser, MemberID: firstMember.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now.Add(time.Minute), HistoryVisibleFromSeq: 1},
		{ConversationID: group.ID, MemberType: store.ConversationMemberTypeUser, MemberID: admin.ID, Role: store.ConversationMemberRoleAdmin, JoinedAt: now.Add(2 * time.Minute), HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create group members: %v", err)
	}
	visibility := VisibilityCreator
	if _, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: creator.ID, AppID: created.ID, Visibility: &visibility,
	}); err != nil {
		t.Fatalf("shrink visibility: %v", err)
	}
	var existingAdmin store.ConversationMember
	if err := db.First(&existingAdmin,
		"conversation_id = ? AND member_type = ? AND member_id = ?", group.ID, store.ConversationMemberTypeUser, admin.ID,
	).Error; err != nil {
		t.Fatalf("load existing admin: %v", err)
	}
	if existingAdmin.Role != store.ConversationMemberRoleAdmin || existingAdmin.LeftAt != nil {
		t.Fatalf("existing admin = %#v", existingAdmin)
	}
	var appOwner store.ConversationMember
	if err := db.First(&appOwner,
		"conversation_id = ? AND member_type = ? AND member_id = ?", group.ID, store.ConversationMemberTypeApp, created.ID,
	).Error; err != nil {
		t.Fatalf("load app owner: %v", err)
	}
	if appOwner.Role != store.ConversationMemberRoleOwner || appOwner.LeftAt != nil {
		t.Fatalf("app owner = %#v", appOwner)
	}
	var storedGroup store.Conversation
	if err := db.First(&storedGroup, "id = ?", group.ID).Error; err != nil {
		t.Fatalf("load group: %v", err)
	}
	if storedGroup.Status != store.ConversationStatusActive || storedGroup.CreatedByAppID == nil || *storedGroup.CreatedByAppID != created.ID {
		t.Fatalf("stored group = %#v", storedGroup)
	}
}

func TestDeletingApplicationDissolvesOwnedGroupWithoutActiveUser(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 20, 11, 0, 0, 0, time.UTC)
	service := NewService(Dependencies{
		DB: db, Now: func() time.Time { return now }, NewID: uuid.NewString,
		GenerateSecret: func() (string, error) { return "admin-group-owner-secret", nil },
	})
	created, err := service.Create(context.Background(), CreateCommand{
		Name: "后台群主应用", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create admin app: %v", err)
	}
	legacyUser := insertOwnedAppTestUser(t, db, "disabled-group-user@example.com", now)
	if err := db.Model(&store.User{}).Where("id = ?", legacyUser.ID).Update("status", store.UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable legacy user: %v", err)
	}
	creatorAppID := created.ID
	group := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "无人群",
		CreatedByAppID: &creatorAppID, CreatedByUserID: legacyUser.ID,
		Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen,
		Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&store.ConversationMember{
		ConversationID: group.ID, MemberType: store.ConversationMemberTypeApp,
		MemberID: created.ID, Role: store.ConversationMemberRoleOwner,
		JoinedAt: now, HistoryVisibleFromSeq: 1,
	}).Error; err != nil {
		t.Fatalf("create app owner: %v", err)
	}
	project := store.Project{
		ID: uuid.NewString(), Name: "待解绑项目", OwnerUserID: legacyUser.ID, CreatedByUserID: legacyUser.ID,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create linked project: %v", err)
	}
	if err := db.Create(&store.ProjectGroup{
		ProjectID: project.ID, ConversationID: group.ID, LinkedByUserID: legacyUser.ID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("link project group: %v", err)
	}
	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	var storedGroup store.Conversation
	if err := db.First(&storedGroup, "id = ?", group.ID).Error; err != nil {
		t.Fatalf("load dissolved group: %v", err)
	}
	if storedGroup.Status != store.ConversationStatusDissolved || storedGroup.DissolvedAt == nil {
		t.Fatalf("stored group = %#v", storedGroup)
	}
	requireOwnedAppRowCount(t, db, &store.ProjectGroup{}, 0, "conversation_id = ?", group.ID)
	var updatedProject store.Project
	if err := db.First(&updatedProject, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("load updated project: %v", err)
	}
	if !updatedProject.UpdatedAt.Equal(now) {
		t.Fatalf("project updated_at = %v, want %v", updatedProject.UpdatedAt, now)
	}
}

func TestOwnedAppCreationEnforcesAccountQuota(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "quota-owner@example.com", now)
	service := NewService(Dependencies{
		DB:             db,
		GenerateSecret: func() (string, error) { return uuid.NewString(), nil },
	})
	for index := 0; index < MaxOwnedAppsPerAccount; index++ {
		if _, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
			AccountID: owner.ID, Name: fmt.Sprintf("应用 %d", index), Visibility: VisibilityCreator,
		}); err != nil {
			t.Fatalf("create app %d: %v", index, err)
		}
	}
	if _, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "超额应用", Visibility: VisibilityCreator,
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "每个用户最多创建 20 个应用" {
		t.Fatalf("quota error = %v", err)
	}
}

func TestOwnedAppAuthorizationCleanupHandlesLargeOutboxInDatabase(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "large-outbox-owner@example.com", now)
	other := insertOwnedAppTestUser(t, db, "large-outbox-other@example.com", now)
	service := NewService(Dependencies{
		DB: db, GenerateSecret: func() (string, error) { return "large-outbox-secret", nil },
	})
	created, err := service.CreateOwned(context.Background(), CreateOwnedCommand{
		AccountID: owner.ID, Name: "Large Outbox App", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	conversation := insertOwnedAppConversation(t, db, created.ID, other.ID, now)
	payload, err := json.Marshal(map[string]any{"conversation": map[string]any{"id": conversation.ID}})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	events := make([]store.AppEventOutbox, 2500)
	for index := range events {
		events[index] = store.AppEventOutbox{
			AppID: created.ID, Event: "message.created", Payload: payload, CreatedAt: now,
		}
	}
	if err := db.CreateInBatches(&events, 250).Error; err != nil {
		t.Fatalf("create outbox events: %v", err)
	}
	visibility := VisibilityRestricted
	grants := []string{}
	if _, err := service.UpdateOwned(context.Background(), UpdateOwnedCommand{
		AccountID: owner.ID, AppID: created.ID, Visibility: &visibility, UserIDs: &grants,
	}); err != nil {
		t.Fatalf("shrink app authorization: %v", err)
	}
	requireOwnedAppRowCount(t, db, &store.AppEventOutbox{}, 0, "app_id = ?", created.ID)
}

func insertOwnedAppConversation(t *testing.T, db *gorm.DB, appID string, userID string, now time.Time) store.Conversation {
	t.Helper()
	conversation := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindApp, Name: "App Conversation",
		CreatedByUserID: userID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create app conversation: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: userID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: appID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create app conversation members: %v", err)
	}
	if err := db.Create(&store.AppConversation{AppID: appID, UserID: userID, ConversationID: conversation.ID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create app conversation link: %v", err)
	}
	return conversation
}

func insertOwnedAppGroup(t *testing.T, db *gorm.DB, appID string, ownerID string, now time.Time) store.Conversation {
	t.Helper()
	conversation := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "Group",
		CreatedByUserID: ownerID, Status: store.ConversationStatusActive,
		PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	members := []store.ConversationMember{
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: ownerID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
		{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeApp, MemberID: appID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create group members: %v", err)
	}
	return conversation
}

func insertOwnedAppEvent(t *testing.T, db *gorm.DB, appID string, conversationID string, now time.Time) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"conversation": map[string]any{"id": conversationID}})
	if err != nil {
		t.Fatalf("marshal app event: %v", err)
	}
	if err := db.Create(&store.AppEventOutbox{AppID: appID, Event: "message.created", Payload: payload, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create app event: %v", err)
	}
}

func insertOwnedAppTopicMessageEvent(t *testing.T, db *gorm.DB, appID, parentConversationID, topicID string, now time.Time) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"conversation": map[string]any{
			"id":     topicID,
			"parent": map[string]any{"id": parentConversationID},
		},
	})
	if err != nil {
		t.Fatalf("marshal topic message event: %v", err)
	}
	if err := db.Create(&store.AppEventOutbox{AppID: appID, Event: "message.created", Payload: payload, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create topic message event: %v", err)
	}
}

func insertOwnedAppTopicClosedEvent(t *testing.T, db *gorm.DB, appID, parentConversationID, topicID string, now time.Time) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"conversation_id": topicID, "parent_conversation_id": parentConversationID,
	})
	if err != nil {
		t.Fatalf("marshal topic closed event: %v", err)
	}
	if err := db.Create(&store.AppEventOutbox{AppID: appID, Event: "topic.closed", Payload: payload, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create topic closed event: %v", err)
	}
}

func requireOwnedAppActiveMemberCount(t *testing.T, db *gorm.DB, appID string, conversationID string, want int64) {
	t.Helper()
	requireOwnedAppRowCount(t, db, &store.ConversationMember{}, want,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeApp, appID,
	)
}

func requireOwnedAppRowCount(t *testing.T, db *gorm.DB, model any, want int64, query string, args ...any) {
	t.Helper()
	var count int64
	if err := db.Model(model).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != want {
		t.Fatalf("row count = %d, want %d for %s", count, want, query)
	}
}

func insertOwnedAppTestUser(t *testing.T, db *gorm.DB, email string, now time.Time) store.User {
	t.Helper()
	user := store.User{
		ID: uuid.NewString(), Email: email, Name: email, PasswordHash: "hash",
		Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}
