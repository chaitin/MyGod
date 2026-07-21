package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/appregistry"
	"app/internal/config"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestRandomSecretUsesThirtyTwoLowercaseAlphanumericCharacters(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for range 100 {
		secret, err := randomSecret()
		if err != nil {
			t.Fatalf("generate random secret: %v", err)
		}
		if len(secret) != appSecretLength {
			t.Fatalf("secret length = %d, want %d: %q", len(secret), appSecretLength, secret)
		}
		hasLowercase := false
		hasDigit := false
		for _, character := range secret {
			switch {
			case character >= 'a' && character <= 'z':
				hasLowercase = true
			case character >= '0' && character <= '9':
				hasDigit = true
			default:
				t.Fatalf("secret contains unsupported character %q: %q", character, secret)
			}
		}
		if !hasLowercase || !hasDigit {
			t.Fatalf("secret must contain lowercase letters and digits: %q", secret)
		}
		if _, exists := seen[secret]; exists {
			t.Fatalf("generated duplicate secret: %q", secret)
		}
		seen[secret] = struct{}{}
	}
}

func TestServiceManagesAppsAndConnectionLifecycle(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	appID := "10000000-0000-0000-0000-000000000001"
	secrets := []string{"created-secret", "regenerated-secret"}
	connections := &fakeAppConnections{online: map[string]bool{}}
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "configured-ai-secret"},
		Connections: connections, Now: func() time.Time { return now }, NewID: func() string { return appID },
		GenerateSecret: func() (string, error) {
			value := secrets[0]
			secrets = secrets[1:]
			return value, nil
		},
	})

	created, err := service.Create(context.Background(), CreateCommand{
		Name: "  知识库助手  ", Description: "  回答知识库问题  ", Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	if created.ID != appID || created.Name != "知识库助手" || created.Description != "回答知识库问题" ||
		created.ConnectionSecret != "created-secret" || !created.Enabled || created.ConnectionStatus != ConnectionStatusOffline {
		t.Fatalf("created app = %#v", created)
	}
	if !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Fatalf("created timestamps = %v, %v", created.CreatedAt, created.UpdatedAt)
	}

	apps, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if len(apps) != 2 || apps[0].ID != appregistry.AIAssistantAppID || !apps[0].System || apps[1].ID != appID {
		t.Fatalf("listed apps = %#v", apps)
	}
	if apps[0].ConnectionSecret != "configured-ai-secret" || apps[0].Visibility != VisibilityPublic {
		t.Fatalf("AI assistant app = %#v", apps[0])
	}

	connections.online[appID] = true
	online, err := service.Get(context.Background(), appID)
	if err != nil || online.ConnectionStatus != ConnectionStatusOnline {
		t.Fatalf("online app = %#v, err = %v", online, err)
	}

	now = now.Add(time.Hour)
	disabled, err := service.SetEnabled(context.Background(), SetEnabledCommand{AppID: appID, Enabled: false})
	if err != nil {
		t.Fatalf("disable app: %v", err)
	}
	if disabled.Enabled || disabled.ConnectionStatus != ConnectionStatusDisabled || connections.closeCalls != 1 {
		t.Fatalf("disabled app = %#v, close calls = %d", disabled, connections.closeCalls)
	}
	if _, err := service.SetEnabled(context.Background(), SetEnabledCommand{AppID: appID, Enabled: false}); err != nil || connections.closeCalls != 1 {
		t.Fatalf("idempotent disable err = %v, close calls = %d", err, connections.closeCalls)
	}

	enabled, err := service.SetEnabled(context.Background(), SetEnabledCommand{AppID: appID, Enabled: true})
	if err != nil || !enabled.Enabled || enabled.ConnectionStatus != ConnectionStatusOffline {
		t.Fatalf("enabled app = %#v, err = %v", enabled, err)
	}
	connections.online[appID] = true
	regenerated, err := service.RegenerateSecret(context.Background(), appID)
	if err != nil {
		t.Fatalf("regenerate secret: %v", err)
	}
	if regenerated.ConnectionSecret != "regenerated-secret" || regenerated.ConnectionStatus != ConnectionStatusOffline || connections.closeCalls != 2 {
		t.Fatalf("regenerated app = %#v, close calls = %d", regenerated, connections.closeCalls)
	}

	updatedAI, err := service.Update(context.Background(), UpdateCommand{
		AppID: appregistry.AIAssistantAppID, Name: "茉莉 Pro", Description: "AI Agent", Visibility: VisibilityPublic,
	})
	if err != nil || updatedAI.Name != "茉莉 Pro" || updatedAI.Visibility != VisibilityPublic {
		t.Fatalf("updated AI assistant = %#v, err = %v", updatedAI, err)
	}
	if err := db.Model(&store.App{}).Where("id = ?", appregistry.AIAssistantAppID).Updates(map[string]any{
		"connection_secret": "stale-secret", "visibility": VisibilityCreator,
	}).Error; err != nil {
		t.Fatalf("make AI assistant stale: %v", err)
	}
	connectionAI, err := service.GetForConnection(context.Background(), appregistry.AIAssistantAppID)
	if err != nil || connectionAI.ConnectionSecret != "configured-ai-secret" || connectionAI.Visibility != VisibilityPublic {
		t.Fatalf("connection AI assistant = %#v, err = %v", connectionAI, err)
	}
	if _, err := service.RegenerateSecret(context.Background(), appregistry.AIAssistantAppID); ErrorCodeOf(err) != CodeForbidden || ErrorMessage(err) != "茉莉密钥由配置管理" {
		t.Fatalf("regenerate AI assistant err = %v", err)
	}
	if err := service.Delete(context.Background(), appregistry.AIAssistantAppID); ErrorCodeOf(err) != CodeForbidden || ErrorMessage(err) != "茉莉不能删除" {
		t.Fatalf("delete AI assistant err = %v", err)
	}

	if err := service.Delete(context.Background(), appID); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	if connections.closeCalls != 3 {
		t.Fatalf("close calls after delete = %d", connections.closeCalls)
	}
	if _, err := service.Get(context.Background(), appID); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("get deleted app err = %v", err)
	}
}

func TestServiceListAndGetIncludeAppCreator(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	owner := store.User{
		ID: uuid.NewString(), Email: "owner@example.com", Name: "应用所有者",
		Nickname: "小明", Avatar: "/assets/avatars/builtin/02.webp",
		PasswordHash: "hash", Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	ownerID := owner.ID
	storedApp := store.App{
		ID: uuid.NewString(), Name: "用户应用", CreatorUserID: &ownerID, Enabled: true,
		Visibility: store.AppVisibilityCreator, ConnectionSecret: "user-app-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&storedApp).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "configured-ai-secret"},
	})

	apps, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	var listed App
	for _, value := range apps {
		if value.ID == storedApp.ID {
			listed = value
			break
		}
	}
	if listed.Creator == nil || listed.Creator.ID != owner.ID ||
		listed.Creator.Name != owner.Name || listed.Creator.Nickname != owner.Nickname ||
		listed.Creator.Email != owner.Email || listed.Creator.Avatar != owner.Avatar {
		t.Fatalf("listed creator = %#v", listed.Creator)
	}

	got, err := service.Get(context.Background(), storedApp.ID)
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if got.Creator == nil || got.Creator.ID != owner.ID {
		t.Fatalf("got creator = %#v", got.Creator)
	}
}

func TestServiceValidatesAppManagementInput(t *testing.T) {
	service := NewService(Dependencies{
		DB: openAppTestDB(t), Apps: config.AppsConfig{AIAssistantSecret: "configured-ai-secret"},
		GenerateSecret: func() (string, error) { return "secret", nil },
	})

	for name, cmd := range map[string]CreateCommand{
		"empty name":         {Visibility: VisibilityPublic},
		"long name":          {Name: strings.Repeat("名", 121), Visibility: VisibilityPublic},
		"creator visibility": {Name: "应用", Visibility: VisibilityCreator},
		"unknown visibility": {Name: "应用", Visibility: "private"},
	} {
		if _, err := service.Create(context.Background(), cmd); ErrorCodeOf(err) != CodeInvalidRequest {
			t.Fatalf("%s error = %v, code = %q", name, err, ErrorCodeOf(err))
		}
	}
	if _, err := service.Get(context.Background(), "not-an-id"); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "应用 ID 格式错误" {
		t.Fatalf("invalid app ID err = %v", err)
	}
	if _, err := service.Get(context.Background(), uuid.NewString()); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing app err = %v", err)
	}
}

func TestServicePreservesRestrictedUserManagedAppInAdminRoundTrip(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "admin-roundtrip-owner@example.com", now)
	granted := insertOwnedAppTestUser(t, db, "admin-roundtrip-granted@example.com", now)
	stored := store.App{
		ID: uuid.NewString(), Name: "Restricted App", CreatorUserID: &owner.ID,
		Enabled: true, Visibility: store.AppVisibilityRestricted, ConnectionSecret: "restricted-roundtrip-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := db.Create(&store.AppUserGrant{AppID: stored.ID, UserID: granted.ID, GrantedByUserID: &owner.ID, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	service := NewService(Dependencies{DB: db})
	updated, err := service.Update(context.Background(), UpdateCommand{
		AppID: stored.ID, Name: "Restricted App Updated", Description: "Updated",
		Visibility: VisibilityRestricted,
	})
	if err != nil || updated.Visibility != VisibilityRestricted || updated.Name != "Restricted App Updated" {
		t.Fatalf("updated app = %#v, err = %v", updated, err)
	}
	var grantCount int64
	if err := db.Model(&store.AppUserGrant{}).Where("app_id = ? AND user_id = ?", stored.ID, granted.ID).Count(&grantCount).Error; err != nil || grantCount != 1 {
		t.Fatalf("grant count = %d, err = %v", grantCount, err)
	}
}

func TestConnectionRejectsAppOwnedByDisabledAccount(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "disabled-owner@example.com", now)
	stored := store.App{
		ID: uuid.NewString(), Name: "Stale Enabled App", CreatorUserID: &owner.ID,
		Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "stale-enabled-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := db.Model(&store.User{}).Where("id = ?", owner.ID).Update("status", store.UserStatusDisabled).Error; err != nil {
		t.Fatalf("disable owner: %v", err)
	}
	service := NewService(Dependencies{DB: db})
	connected, err := service.GetForConnection(context.Background(), stored.ID)
	if err != nil || connected.Enabled {
		t.Fatalf("connection app = %#v, err = %v", connected, err)
	}
	if allowed, err := service.CanUserAccess(context.Background(), stored.ID, owner.ID); err != nil || allowed {
		t.Fatalf("disabled owner access = %t, err = %v", allowed, err)
	}
	if _, err := service.SetEnabled(context.Background(), SetEnabledCommand{AppID: stored.ID, Enabled: true}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("enable disabled owner's app error = %v", err)
	}
}

func TestAdminUpdateReconcilesAgainstLockedVisibility(t *testing.T) {
	db := openAppTestDB(t)
	now := time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC)
	owner := insertOwnedAppTestUser(t, db, "admin-lock-owner@example.com", now)
	other := insertOwnedAppTestUser(t, db, "admin-lock-other@example.com", now)
	stored := store.App{
		ID: uuid.NewString(), Name: "Concurrent App", CreatorUserID: &owner.ID,
		Enabled: true, Visibility: store.AppVisibilityPublic, ConnectionSecret: "admin-lock-secret",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	conversation := insertOwnedAppConversation(t, db, stored.ID, other.ID, now)
	insertOwnedAppEvent(t, db, stored.ID, conversation.ID, now)

	service := NewService(Dependencies{DB: db})
	updated, err := service.Update(context.Background(), UpdateCommand{
		AppID: stored.ID, Name: stored.Name, Visibility: VisibilityRestricted,
	})
	if err != nil || updated.Visibility != VisibilityRestricted {
		t.Fatalf("update app = %#v, err = %v", updated, err)
	}
	requireOwnedAppRowCount(t, db, &store.AppConversation{}, 1, "app_id = ? AND user_id = ?", stored.ID, other.ID)
	requireOwnedAppActiveMemberCount(t, db, stored.ID, conversation.ID, 1)
	requireOwnedAppRowCount(t, db, &store.AppEventOutbox{}, 0, "app_id = ?", stored.ID)
}

func TestServiceUploadsAppAvatarThroughFileModule(t *testing.T) {
	db := openAppTestDB(t)
	appID := uuid.NewString()
	storedApp := store.App{
		ID: appID, Name: "知识库助手", Enabled: true, Visibility: VisibilityPublic,
		ConnectionSecret: "secret", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&storedApp).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	uploader := &recordingAppAvatarUploader{}
	service := NewService(Dependencies{
		DB: db, Apps: config.AppsConfig{AIAssistantSecret: "configured-ai-secret"}, Files: uploader,
		NewID: func() string { return "avatar-id" },
	})
	content := appTestWebP(256, 256)

	updated, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AppID: appID, Content: bytes.NewReader(content), Size: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("upload avatar: %v", err)
	}
	wantKey := "avatars/apps/" + appID + "/avatar-id.webp"
	if uploader.objectKey != wantKey || uploader.contentType != avatarContentType || !bytes.Equal(uploader.content, content) {
		t.Fatalf("upload = key %q, type %q, content %#v", uploader.objectKey, uploader.contentType, uploader.content)
	}
	if updated.Avatar != uploader.url {
		t.Fatalf("updated avatar = %q, want %q", updated.Avatar, uploader.url)
	}

	wrongSize := appTestWebP(128, 128)
	if _, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AppID: appID, Content: bytes.NewReader(wrongSize), Size: int64(len(wrongSize)),
	}); ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "头像必须是 256x256 的 WebP 图片" {
		t.Fatalf("wrong dimensions err = %v", err)
	}
	if _, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AppID: appID, Content: bytes.NewReader(nil), Size: MaxAvatarBytes + 1,
	}); ErrorCodeOf(err) != CodeRequestTooLarge {
		t.Fatalf("oversized avatar err = %v", err)
	}
}

type fakeAppConnections struct {
	online     map[string]bool
	closeCalls int
}

func (c *fakeAppConnections) IsOnline(appID string) bool {
	return c.online[appID]
}

func (c *fakeAppConnections) CloseApp(appID string) int {
	c.closeCalls++
	wasOnline := c.online[appID]
	delete(c.online, appID)
	if wasOnline {
		return 1
	}
	return 0
}

type recordingAppAvatarUploader struct {
	objectKey   string
	content     []byte
	contentType string
	url         string
}

func (u *recordingAppAvatarUploader) UploadPublic(_ context.Context, cmd fileapp.UploadPublicCommand) (fileapp.PublicFile, error) {
	u.objectKey = cmd.ObjectKey
	u.content, _ = io.ReadAll(cmd.Content)
	u.contentType = cmd.ContentType
	u.url = "https://assets.example.test/public/" + cmd.ObjectKey
	return fileapp.PublicFile{ObjectKey: cmd.ObjectKey, URL: u.url, ContentType: cmd.ContentType, SizeBytes: cmd.SizeBytes}, nil
}

func openAppTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.Conversation{}, &store.ConversationMember{},
		&store.App{}, &store.AppConversation{}, &store.AppUserGrant{},
		&store.Project{}, &store.ProjectGroup{},
		&store.ConversationTopic{}, &store.ConversationTopicParticipant{},
		&store.AppEventOutbox{}, &store.AppEventAck{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func appTestWebP(width int, height int) []byte {
	content := make([]byte, 30)
	copy(content[0:4], "RIFF")
	binary.LittleEndian.PutUint32(content[4:8], uint32(len(content)-8))
	copy(content[8:12], "WEBP")
	copy(content[12:16], "VP8X")
	binary.LittleEndian.PutUint32(content[16:20], 10)
	w := width - 1
	h := height - 1
	content[24] = byte(w)
	content[25] = byte(w >> 8)
	content[26] = byte(w >> 16)
	content[27] = byte(h)
	content[28] = byte(h >> 8)
	content[29] = byte(h >> 16)
	return content
}
