package entitycard

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	projectapp "app/internal/application/project"
	"app/internal/store"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestServiceResolvesAllEntityCardTemplates(t *testing.T) {
	db := openEntityCardTestDB(t)
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	owner := createEntityCardUser(t, db, "owner@example.com", "项目负责人", "老板", now)
	assignee := createEntityCardUser(t, db, "assignee@example.com", "张三", "", now)
	app := store.App{
		ID: uuid.NewString(), Name: "设计助手", Description: "**智能** 设计应用", Enabled: true,
		Visibility: store.AppVisibilityPublic, ConnectionSecret: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	group := store.Conversation{
		ID: uuid.NewString(), Kind: store.ConversationKindGroup, Name: "设计群", CreatedByUserID: owner.ID,
		Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen,
		Visibility: store.ConversationVisibilityPublic, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	for index, user := range []store.User{owner, assignee} {
		role := store.ConversationMemberRoleMember
		if index == 0 {
			role = store.ConversationMemberRoleOwner
		}
		if err := db.Create(&store.ConversationMember{
			ConversationID: group.ID, MemberType: store.ConversationMemberTypeUser,
			MemberID: user.ID, Role: role, JoinedAt: now, HistoryVisibleFromSeq: 1,
		}).Error; err != nil {
			t.Fatalf("create group member: %v", err)
		}
	}
	projectID := uuid.NewString()
	if err := db.Create(&store.Project{
		ID: projectID, Name: "官网项目", Description: "**官网** 改版项目",
		OwnerUserID: owner.ID, CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	dueDate := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	task := store.Task{
		ID: uuid.NewString(), ProjectID: projectID, Title: "完成首页改版", Status: store.TaskStatusInProgress,
		Priority: store.TaskPriorityMedium, AssigneeUserID: &assignee.ID, DueDate: &dueDate,
		CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	projects := &entityCardProjectReader{projects: map[string]projectapp.Project{
		projectID: {ID: projectID, Name: "官网项目", Description: "**官网** 改版项目"},
	}}
	service := NewService(Dependencies{
		DB: db, Projects: projects,
	})

	for _, testCase := range []struct {
		entityID        string
		entityType      string
		wantDescription string
		wantTitle       string
		wantURL         string
	}{
		{owner.ID, TypeUser, "姓名: 项目负责人\n昵称: 老板\n邮箱: owner@example.com", "联系人 - 老板", "/contacts/user/" + owner.ID},
		{app.ID, TypeApp, "智能 设计应用", "应用 - 设计助手", "/contacts/app/" + app.ID},
		{group.ID, TypeGroup, "2 位成员", "群聊 - 设计群", "/contacts/group/" + group.ID},
		{projectID, TypeProject, "官网 改版项目", "项目 - 官网项目", "/projects/" + projectID},
		{task.ID, TypeTask, "状态: 进行中\n负责人: 张三\n截止日期: 2026-07-20", "任务 - 完成首页改版", "/projects/" + projectID + "?taskId=" + task.ID},
	} {
		t.Run(testCase.entityType, func(t *testing.T) {
			card, err := service.Resolve(context.Background(), ResolveCommand{
				AccountID: owner.ID, EntityID: testCase.entityID, EntityType: testCase.entityType,
			})
			if err != nil {
				t.Fatalf("resolve card: %v", err)
			}
			if card.Title != testCase.wantTitle || card.Description != testCase.wantDescription || card.URL != testCase.wantURL {
				t.Fatalf("card = %#v", card)
			}
		})
	}
	if projects.lastCommand.AccountID != owner.ID || projects.lastCommand.ProjectID != projectID {
		t.Fatalf("project command = %#v", projects.lastCommand)
	}
}

func TestServicePreservesValidationOrderAndHidesInaccessibleEntities(t *testing.T) {
	db := openEntityCardTestDB(t)
	now := time.Now().UTC()
	owner := createEntityCardUser(t, db, "owner@example.com", "Owner", "", now)
	outsider := createEntityCardUser(t, db, "outsider@example.com", "Outsider", "", now)
	projectID := uuid.NewString()
	if err := db.Create(&store.Project{
		ID: projectID, Name: "Private", OwnerUserID: owner.ID, CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := store.Task{
		ID: uuid.NewString(), ProjectID: projectID, Title: "Private task", Status: store.TaskStatusTodo,
		Priority: store.TaskPriorityMedium, CreatedByUserID: owner.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	projects := &entityCardProjectReader{err: &projectapp.Error{Code: projectapp.CodeNotFound, Message: "项目不存在"}}
	service := NewService(Dependencies{DB: db, Projects: projects})

	_, err := service.Resolve(context.Background(), ResolveCommand{AccountID: "bad", EntityType: "unknown", EntityID: "bad"})
	if ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "授权用户 ID 格式错误" {
		t.Fatalf("account validation error = %v, code = %q", err, ErrorCodeOf(err))
	}
	_, err = service.Resolve(context.Background(), ResolveCommand{AccountID: outsider.ID, EntityType: "unknown", EntityID: "bad"})
	if ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "不支持的对象卡片类型" {
		t.Fatalf("type validation error = %v, code = %q", err, ErrorCodeOf(err))
	}
	_, err = service.Resolve(context.Background(), ResolveCommand{AccountID: outsider.ID, EntityType: TypeTask, EntityID: task.ID})
	if ErrorCodeOf(err) != CodeNotFound || ErrorMessage(err) != "对象不存在或无权访问" {
		t.Fatalf("inaccessible task error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func TestTitleAndDetailsKeepLegacyBoundsAndOmissions(t *testing.T) {
	if got := Details(
		Detail{Label: "姓名", Value: "张三"}, Detail{Label: "昵称", Value: ""}, Detail{Label: "邮箱", Value: "zhangsan@example.com"},
	); got != "姓名: 张三\n邮箱: zhangsan@example.com" {
		t.Fatalf("details = %q", got)
	}
	got := Title("任务", strings.Repeat("任务", 200))
	if len([]rune(got)) != maxTitleLength || !strings.HasSuffix(got, "…") {
		t.Fatalf("title length = %d, title = %q", len([]rune(got)), got)
	}
}

type entityCardProjectReader struct {
	projects    map[string]projectapp.Project
	err         error
	lastCommand projectapp.ProjectCommand
}

func (r *entityCardProjectReader) Get(_ context.Context, cmd projectapp.ProjectCommand) (projectapp.Project, error) {
	r.lastCommand = cmd
	if r.err != nil {
		return projectapp.Project{}, r.err
	}
	project, ok := r.projects[cmd.ProjectID]
	if !ok {
		return projectapp.Project{}, &projectapp.Error{Code: projectapp.CodeNotFound, Message: "项目不存在"}
	}
	return project, nil
}

func createEntityCardUser(t *testing.T, db *gorm.DB, email, name, nickname string, now time.Time) store.User {
	t.Helper()
	user := store.User{
		ID: uuid.NewString(), Email: email, Name: name, Nickname: nickname,
		PasswordHash: "test", Status: store.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func openEntityCardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:entity-card-%p?mode=memory&cache=shared", t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&store.User{}, &store.App{}, &store.Conversation{}, &store.ConversationMember{},
		&store.Project{}, &store.ProjectGroup{}, &store.Task{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}
