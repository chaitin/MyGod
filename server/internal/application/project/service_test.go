package project

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	fileapp "app/internal/application/file"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

func TestServiceProjectLifecycleAndListing(t *testing.T) {
	db := openProjectTestDB(t)
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	owner := insertProjectTestUser(t, db, "owner@example.com", now)
	if err := ProvisionPersonalWorkspace(db, owner.ID, now); err != nil {
		t.Fatalf("provision personal workspace: %v", err)
	}
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})

	created, err := service.Create(context.Background(), CreateCommand{
		AccountID: owner.ID,
		Name:      "  Release  ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "Release" || created.CurrentUserRole != RoleOwner || created.MemberCount != 1 {
		t.Fatalf("created project = %#v", created)
	}

	description := "Release coordination"
	updated, err := service.Update(context.Background(), UpdateCommand{
		AccountID:   owner.ID,
		ProjectID:   created.ID,
		Description: &description,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Description != description {
		t.Fatalf("updated description = %q", updated.Description)
	}

	listed, err := service.List(context.Background(), ListCommand{
		AccountID:       owner.ID,
		IncludePersonal: true,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listed.PersonalProject == nil || len(listed.Projects) != 1 || listed.Projects[0].ID != created.ID {
		t.Fatalf("list result = %#v", listed)
	}
}

func TestServiceDeleteRemovesTaskReminders(t *testing.T) {
	db := openProjectTestDB(t)
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	owner := insertProjectTestUser(t, db, "delete-reminders@example.com", now)
	service := NewService(Dependencies{DB: db, Now: func() time.Time { return now }})
	created, err := service.Create(context.Background(), CreateCommand{AccountID: owner.ID, Name: "Release"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	taskValue := store.Task{
		ID: uuid.NewString(), ProjectID: created.ID, Title: "Check release", Status: store.TaskStatusTodo,
		Priority: 2, Labels: pq.StringArray{}, CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&taskValue).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	frequency, timeOfDay := "daily", "14:30"
	reminder := store.TaskReminder{
		ID: uuid.NewString(), TaskID: taskValue.ID, Mode: "recurring", Frequency: &frequency,
		Timezone: "Asia/Shanghai", TimeOfDay: &timeOfDay, Weekdays: pq.Int64Array{},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&reminder).Error; err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	if _, err := service.Delete(context.Background(), ProjectCommand{AccountID: owner.ID, ProjectID: created.ID}); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var count int64
	if err := db.Model(&store.TaskReminder{}).Where("task_id = ?", taskValue.ID).Count(&count).Error; err != nil {
		t.Fatalf("count reminders: %v", err)
	}
	if count != 0 {
		t.Fatalf("reminder count = %d, want 0", count)
	}
}

func TestServiceBindsGroupsAndUploadsAvatarThroughPorts(t *testing.T) {
	db := openProjectTestDB(t)
	now := time.Date(2026, 7, 15, 6, 0, 0, 0, time.UTC)
	owner := insertProjectTestUser(t, db, "owner@example.com", now)
	group := store.Conversation{
		ID:              uuid.NewString(),
		Kind:            store.ConversationKindGroup,
		Name:            "Release group",
		CreatedByUserID: owner.ID,
		Status:          store.ConversationStatusActive,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	storage := &projectAvatarRecorder{}
	service := NewService(Dependencies{DB: db, Files: storage, Now: func() time.Time { return now }})
	created, err := service.Create(context.Background(), CreateCommand{AccountID: owner.ID, Name: "Release"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	mutation, err := service.BindGroup(context.Background(), MutateGroupCommand{AccountID: owner.ID, ProjectID: created.ID, GroupID: group.ID})
	if err != nil || mutation.AlreadyLinked {
		t.Fatalf("bind group = %#v, err = %v", mutation, err)
	}
	repeated, err := service.BindGroup(context.Background(), MutateGroupCommand{AccountID: owner.ID, ProjectID: created.ID, GroupID: group.ID})
	if err != nil || !repeated.AlreadyLinked {
		t.Fatalf("repeat bind = %#v, err = %v", repeated, err)
	}

	content := projectTestWebP(256, 256)
	updated, err := service.UploadAvatar(context.Background(), UploadAvatarCommand{
		AccountID: owner.ID,
		ProjectID: created.ID,
		Size:      int64(len(content)),
		Content:   bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload avatar: %v", err)
	}
	if !strings.HasPrefix(storage.key, "avatars/projects/"+created.ID+"/") || updated.Avatar != storage.url {
		t.Fatalf("storage key = %q, updated = %#v", storage.key, updated)
	}
}

type projectAvatarRecorder struct {
	key string
	url string
}

func (s *projectAvatarRecorder) UploadPublic(_ context.Context, cmd fileapp.UploadPublicCommand) (fileapp.PublicFile, error) {
	s.key = cmd.ObjectKey
	s.url = "https://assets.example.test/public/" + cmd.ObjectKey
	return fileapp.PublicFile{ObjectKey: cmd.ObjectKey, URL: s.url, ContentType: cmd.ContentType, SizeBytes: cmd.SizeBytes}, nil
}

func openProjectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.Project{}, &store.Conversation{}, &store.ProjectGroup{}, &store.ConversationMember{}, &store.Task{}, &store.TaskReminder{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func insertProjectTestUser(t *testing.T, db *gorm.DB, email string, now time.Time) store.User {
	t.Helper()
	user := store.User{ID: uuid.NewString(), Email: email, Name: "Owner", Avatar: store.DefaultUserAvatar, PasswordHash: "hash", Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func projectTestWebP(width, height int) []byte {
	content := make([]byte, 30)
	copy(content[0:4], "RIFF")
	binary.LittleEndian.PutUint32(content[4:8], uint32(len(content)-8))
	copy(content[8:12], "WEBP")
	copy(content[12:16], "VP8X")
	binary.LittleEndian.PutUint32(content[16:20], 10)
	w, h := width-1, height-1
	content[24], content[25], content[26] = byte(w), byte(w>>8), byte(w>>16)
	content[27], content[28], content[29] = byte(h), byte(h>>8), byte(h>>16)
	return content
}
