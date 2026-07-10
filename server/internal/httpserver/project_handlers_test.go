package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAdminUserCreationProvisionsPersonalWorkspace(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	resp, body := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "personal-workspace-admin@example.com",
		"name":  "Admin Created User",
	}, adminCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)

	var user store.User
	if err := db.First(&user, "email = ?", "personal-workspace-admin@example.com").Error; err != nil {
		t.Fatalf("find created user: %v", err)
	}
	requirePersonalWorkspace(t, db, user)
}

func TestFirstTimeThirdPartyUserCreationProvisionsOnePersonalWorkspace(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:    "Personal Workspace SSO",
		Key:     "personal-workspace-sso",
		Enabled: true,
	})
	profile := externalUserProfile{
		ExternalUserID: "personal-workspace-external-user",
		Email:          "personal-workspace-third-party@example.com",
		Name:           "Third Party User",
		Raw:            json.RawMessage(`{"sub":"personal-workspace-external-user"}`),
	}
	subject := &Server{db: db}

	user, err := subject.findOrCreateThirdPartyUser(provider, profile)
	if err != nil {
		t.Fatalf("find or create third-party user: %v", err)
	}
	requirePersonalWorkspace(t, db, user)
	var account store.ThirdPartyAccount
	if err := db.First(
		&account,
		"provider_id = ? AND external_user_id = ?",
		provider.ID,
		profile.ExternalUserID,
	).Error; err != nil {
		t.Fatalf("find third-party account before repeated login: %v", err)
	}
	if account.UserID != user.ID {
		t.Fatalf("third-party account user ID = %q, want %q", account.UserID, user.ID)
	}

	repeatedUser, err := subject.findOrCreateThirdPartyUser(provider, profile)
	if err != nil {
		t.Fatalf("repeat find or create third-party user: %v", err)
	}
	if repeatedUser.ID != user.ID {
		t.Fatalf("repeated user ID = %q, want %q", repeatedUser.ID, user.ID)
	}
	requirePersonalWorkspace(t, db, user)
}

func TestAdminUserCreationRollsBackWhenPersonalWorkspaceInsertFails(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	failPersonalWorkspaceCreates(t, db)

	resp, body := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": "personal-workspace-admin-rollback@example.com",
		"name":  "Rolled Back Admin User",
	}, adminCookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "internal_error")

	requireRowCount(t, db, &store.User{}, 0, "email = ?", "personal-workspace-admin-rollback@example.com")
	requireRowCount(t, db.Unscoped(), &store.Project{}, 0, "1 = 1")
}

func TestFirstTimeThirdPartyUserCreationRollsBackPersonalWorkspaceWhenAccountInsertFails(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:    "Personal Workspace Account Failure SSO",
		Key:     "personal-workspace-account-failure-sso",
		Enabled: true,
	})
	var projectCountBefore int64
	if err := db.Unscoped().Model(&store.Project{}).Count(&projectCountBefore).Error; err != nil {
		t.Fatalf("count projects before third-party account failure: %v", err)
	}

	accountInsertErr := errors.New("forced third-party account insertion failure")
	failCreatesForTable(
		t,
		db,
		"test:fail_third_party_account_create",
		"third_party_accounts",
		accountInsertErr,
	)
	profile := externalUserProfile{
		ExternalUserID: "personal-workspace-account-failure-external-user",
		Email:          "personal-workspace-account-failure@example.com",
		Name:           "Rolled Back Account User",
		Raw:            json.RawMessage(`{"sub":"personal-workspace-account-failure-external-user"}`),
	}

	_, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, profile)
	if !errors.Is(err, accountInsertErr) {
		t.Fatalf("find or create third-party user error = %v, want %v", err, accountInsertErr)
	}

	requireRowCount(t, db, &store.User{}, 0, "email = ?", profile.Email)
	requireRowCount(t, db.Unscoped(), &store.Project{}, projectCountBefore, "1 = 1")
	requireRowCount(
		t,
		db,
		&store.ThirdPartyAccount{},
		0,
		"provider_id = ? AND external_user_id = ?",
		provider.ID,
		profile.ExternalUserID,
	)
}

func TestAdminUserCreationTreatsPersonalWorkspaceUniqueFailureAsInternalError(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	adminCookie := loginAsAdmin(t, server)
	failCreatesForTable(
		t,
		db,
		"test:fail_personal_workspace_unique_create",
		"projects",
		errors.New("UNIQUE constraint failed: projects.owner_user_id"),
	)

	const email = "personal-workspace-unique-failure@example.com"
	resp, body := postJSON(t, server, "/api/admin/users", map[string]any{
		"email": email,
		"name":  "Unique Project Failure User",
	}, adminCookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "internal_error")

	requireRowCount(t, db, &store.User{}, 0, "email = ?", email)
	requireRowCount(t, db.Unscoped(), &store.Project{}, 0, "1 = 1")
}

func TestFirstTimeThirdPartyUserCreationRollsBackWhenPersonalWorkspaceInsertFails(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	provider := insertTestThirdPartyLoginProvider(t, db, store.ThirdPartyLoginProvider{
		Name:    "Personal Workspace Rollback SSO",
		Key:     "personal-workspace-rollback-sso",
		Enabled: true,
	})
	failPersonalWorkspaceCreates(t, db)

	profile := externalUserProfile{
		ExternalUserID: "personal-workspace-rollback-external-user",
		Email:          "personal-workspace-third-party-rollback@example.com",
		Name:           "Rolled Back Third Party User",
		Raw:            json.RawMessage(`{"sub":"personal-workspace-rollback-external-user"}`),
	}
	_, err := (&Server{db: db}).findOrCreateThirdPartyUser(provider, profile)
	if err == nil {
		t.Fatal("find or create third-party user error = nil, want project insertion failure")
	}

	requireRowCount(t, db, &store.User{}, 0, "email = ?", profile.Email)
	requireRowCount(
		t,
		db,
		&store.ThirdPartyAccount{},
		0,
		"provider_id = ? AND external_user_id = ?",
		provider.ID,
		profile.ExternalUserID,
	)
	requireRowCount(t, db.Unscoped(), &store.Project{}, 0, "1 = 1")
}

func requirePersonalWorkspace(t *testing.T, db *gorm.DB, user store.User) store.Project {
	t.Helper()

	var projects []store.Project
	if err := db.Unscoped().Where("owner_user_id = ?", user.ID).Find(&projects).Error; err != nil {
		t.Fatalf("find personal workspace: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("project count for user %q = %d, want 1", user.ID, len(projects))
	}

	project := projects[0]
	if _, err := uuid.Parse(project.ID); err != nil {
		t.Fatalf("project ID = %q, want UUID: %v", project.ID, err)
	}
	if project.Name != "个人工作区" {
		t.Fatalf("project name = %q, want 个人工作区", project.Name)
	}
	if project.Description != "" {
		t.Fatalf("project description = %q, want empty", project.Description)
	}
	if project.Avatar != "" {
		t.Fatalf("project avatar = %q, want empty", project.Avatar)
	}
	if project.OwnerUserID != user.ID {
		t.Fatalf("project owner user ID = %q, want %q", project.OwnerUserID, user.ID)
	}
	if project.CreatedByUserID != user.ID {
		t.Fatalf("project created-by user ID = %q, want %q", project.CreatedByUserID, user.ID)
	}
	if !project.IsPersonal {
		t.Fatal("project is_personal = false, want true")
	}
	if project.CreatedAt.IsZero() {
		t.Fatal("project created_at is zero")
	}
	if !project.CreatedAt.Equal(project.UpdatedAt) {
		t.Fatalf("project timestamps differ: created_at = %v, updated_at = %v", project.CreatedAt, project.UpdatedAt)
	}

	return project
}

func failPersonalWorkspaceCreates(t *testing.T, db *gorm.DB) {
	t.Helper()

	failCreatesForTable(
		t,
		db,
		"test:fail_personal_workspace_create",
		"projects",
		errors.New("forced personal workspace insertion failure"),
	)
}

func failCreatesForTable(t *testing.T, db *gorm.DB, callbackName string, table string, createErr error) {
	t.Helper()

	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			tx.AddError(createErr)
		}
	}); err != nil {
		t.Fatalf("register %s create callback: %v", table, err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove %s create callback: %v", table, err)
		}
	})
}

func requireRowCount(t *testing.T, db *gorm.DB, model any, want int64, query string, args ...any) {
	t.Helper()

	var count int64
	if err := db.Model(model).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count %T rows: %v", model, err)
	}
	if count != want {
		t.Fatalf("%T row count = %d, want %d", model, count, want)
	}
}

func TestProjectListSeparatesPersonalAndPaginatesCollaborativeProjects(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	owner := insertTestUser(t, db, "project-list@example.com", "List Owner", store.UserStatusActive, now)
	owner.Avatar = "/avatars/current-owner.webp"
	if err := db.Model(&owner).Update("avatar", owner.Avatar).Error; err != nil {
		t.Fatalf("update owner avatar: %v", err)
	}
	personal := insertProjectFixture(t, db, projectFixtureInput{
		ID:         "00000000-0000-0000-0000-000000000099",
		Owner:      owner,
		Name:       "Personal",
		Avatar:     "/avatars/stale-personal.webp",
		IsPersonal: true,
		UpdatedAt:  now.Add(-24 * time.Hour),
	})
	newerLow := insertProjectFixture(t, db, projectFixtureInput{
		ID:        "00000000-0000-0000-0000-000000000001",
		Owner:     owner,
		Name:      "Newer Low",
		UpdatedAt: now,
	})
	newerHigh := insertProjectFixture(t, db, projectFixtureInput{
		ID:        "00000000-0000-0000-0000-000000000002",
		Owner:     owner,
		Name:      "Newer High",
		UpdatedAt: now,
	})
	older := insertProjectFixture(t, db, projectFixtureInput{
		ID:        "00000000-0000-0000-0000-000000000003",
		Owner:     owner,
		Name:      "Older",
		UpdatedAt: now.Add(-time.Hour),
	})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := getJSON(t, server, "/api/client/projects?limit=2", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	personalResponse := requireProjectObject(t, data["personal_project"])
	if personalResponse["id"] != personal.ID {
		t.Fatalf("personal_project.id = %v, want %s", personalResponse["id"], personal.ID)
	}
	if personalResponse["avatar"] != owner.Avatar {
		t.Fatalf("personal_project.avatar = %v, want current owner avatar %s", personalResponse["avatar"], owner.Avatar)
	}
	projects := requireObjectList(t, data["projects"])
	if len(projects) != 2 {
		t.Fatalf("projects length = %d, want 2", len(projects))
	}
	if projects[0]["id"] != newerHigh.ID || projects[1]["id"] != newerLow.ID {
		t.Fatalf("project IDs = [%v %v], want [%s %s]", projects[0]["id"], projects[1]["id"], newerHigh.ID, newerLow.ID)
	}
	cursor, ok := data["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor = %#v, want non-empty string", data["next_cursor"])
	}

	resp, body = getJSON(t, server, "/api/client/projects?limit=2&cursor="+cursor, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	secondPage := requireSuccess(t, body)
	secondProjects := requireObjectList(t, secondPage["projects"])
	if len(secondProjects) != 1 || secondProjects[0]["id"] != older.ID {
		t.Fatalf("second page projects = %#v, want only %s", secondProjects, older.ID)
	}
	if secondPage["next_cursor"] != nil {
		t.Fatalf("second page next_cursor = %#v, want null", secondPage["next_cursor"])
	}
	for _, project := range append(projects, secondProjects...) {
		if project["id"] == personal.ID {
			t.Fatal("personal project appeared in collaborative projects")
		}
	}
}

func TestProjectListRejectsInvalidLimitAndCursor(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "project-list-validation@example.com", "List Validation", store.UserStatusActive, time.Now().UTC())
	cookie := loginAsUser(t, server, user.Email)

	for _, path := range []string{
		"/api/client/projects?limit=0",
		"/api/client/projects?limit=101",
		"/api/client/projects?limit=not-a-number",
		"/api/client/projects?cursor=not-a-cursor",
	} {
		t.Run(path, func(t *testing.T) {
			resp, body := getJSON(t, server, path, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
}

func TestProjectCreateSupportsMinimalAndFullRequests(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-create@example.com", "Create Owner", store.UserStatusActive, now)
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Create Group",
		now:             now,
	})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := postJSON(t, server, "/api/client/projects", map[string]any{"name": "  Minimal Project  "}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("minimal status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	minimal := requireProjectObject(t, requireSuccess(t, body))
	if minimal["name"] != "Minimal Project" || minimal["description"] != "" || minimal["avatar"] != "" {
		t.Fatalf("minimal project = %#v", minimal)
	}
	if minimal["is_personal"] != false || minimal["current_user_role"] != store.ProjectRoleOwner {
		t.Fatalf("minimal project immutable fields = %#v", minimal)
	}
	assertProjectZeroCounts(t, minimal)
	minimalStored := requireProjectByID(t, db, minimal["id"].(string))
	if minimalStored.OwnerUserID != owner.ID || minimalStored.CreatedByUserID != owner.ID || minimalStored.IsPersonal {
		t.Fatalf("minimal stored project = %#v", minimalStored)
	}

	resp, body = postJSON(t, server, "/api/client/projects", map[string]any{
		"name":        "Full Project",
		"description": "Full description",
		"avatar":      "/avatars/project.webp",
		"group_ids":   []string{group.ID, group.ID},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("full status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	full := requireProjectObject(t, requireSuccess(t, body))
	if full["description"] != "Full description" || full["avatar"] != "/avatars/project.webp" {
		t.Fatalf("full project = %#v", full)
	}
	if full["group_count"] != float64(1) {
		t.Fatalf("full group_count = %v, want 1", full["group_count"])
	}
	fullStored := requireProjectByID(t, db, full["id"].(string))
	if fullStored.OwnerUserID != owner.ID || fullStored.CreatedByUserID != owner.ID || fullStored.IsPersonal {
		t.Fatalf("full stored project = %#v", fullStored)
	}
	requireRowCount(t, db, &store.ProjectGroup{}, 1, "project_id = ?", fullStored.ID)
}

func TestProjectCreateRollsBackInvalidGroupTargets(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-create-rollback@example.com", "Rollback Owner", store.UserStatusActive, now)
	activeGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Active Group",
		now:             now,
	})
	direct := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{owner.ID},
		name:            "Direct",
		now:             now,
	})
	dissolved := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolved",
		now:             now,
	})
	if err := db.Model(&dissolved).Update("status", store.ConversationStatusDissolved).Error; err != nil {
		t.Fatalf("dissolve group: %v", err)
	}
	cookie := loginAsUser(t, server, owner.Email)

	for name, groupIDs := range map[string][]string{
		"malformed UUID":  {activeGroup.ID, "bad-id"},
		"missing group":   {activeGroup.ID, uuid.NewString()},
		"direct target":   {activeGroup.ID, direct.ID},
		"dissolved group": {activeGroup.ID, dissolved.ID},
	} {
		t.Run(name, func(t *testing.T) {
			var projectCountBefore int64
			if err := db.Model(&store.Project{}).Count(&projectCountBefore).Error; err != nil {
				t.Fatalf("count projects before request: %v", err)
			}
			resp, body := postJSON(t, server, "/api/client/projects", map[string]any{
				"name":      "Must Roll Back",
				"group_ids": groupIDs,
			}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
			requireRowCount(t, db, &store.Project{}, projectCountBefore, "1 = 1")
			requireRowCount(t, db, &store.ProjectGroup{}, 0, "project_id NOT IN (SELECT id FROM projects)")
		})
	}
}

func TestProjectCreateRejectsInvalidNames(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "project-create-validation@example.com", "Create Validation", store.UserStatusActive, time.Now().UTC())
	cookie := loginAsUser(t, server, user.Email)
	for name, value := range map[string]string{
		"blank":    " \t\n ",
		"too long": strings.Repeat("界", 121),
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := postJSON(t, server, "/api/client/projects", map[string]any{"name": value}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
}

func TestProjectCreateRejectsUnknownAndImmutableFields(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "project-create-fields@example.com", "Create Fields", store.UserStatusActive, time.Now().UTC())
	cookie := loginAsUser(t, server, user.Email)
	for name, field := range map[string]string{
		"unknown":  "unexpected",
		"personal": "is_personal",
		"owner":    "owner_user_id",
		"creator":  "created_by_user_id",
	} {
		t.Run(name, func(t *testing.T) {
			var countBefore int64
			if err := db.Model(&store.Project{}).Count(&countBefore).Error; err != nil {
				t.Fatalf("count projects: %v", err)
			}
			resp, body := postJSON(t, server, "/api/client/projects", map[string]any{
				"name": "Rejected Project",
				field:  true,
			}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
			requireRowCount(t, db, &store.Project{}, countBefore, "1 = 1")
		})
	}
}

func TestProjectGetReturnsOwnerAndDynamicCounts(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-get-owner@example.com", "Get Owner", store.UserStatusActive, now)
	owner.Nickname = "Owner Nick"
	owner.Avatar = "/avatars/get-owner.webp"
	if err := db.Model(&owner).Updates(map[string]any{"nickname": owner.Nickname, "avatar": owner.Avatar}).Error; err != nil {
		t.Fatalf("update owner profile: %v", err)
	}
	member := insertTestUser(t, db, "project-get-member@example.com", "Get Member", store.UserStatusActive, now)
	left := insertTestUser(t, db, "project-get-left@example.com", "Left Member", store.UserStatusActive, now)
	leftAt := now
	group := insertTestConversation(t, db, testConversationInput{
		createdByUserID:  owner.ID,
		kind:             store.ConversationKindGroup,
		memberIDs:        []string{owner.ID, member.ID, left.ID},
		memberLeftAtByID: map[string]*time.Time{left.ID: &leftAt},
		name:             "Counted Group",
		now:              now,
	})
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Counted Project", UpdatedAt: now})
	insertProjectGroupFixture(t, db, project.ID, group.ID, owner.ID, now)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusTodo, now, false)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusTodo, now, false)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusInProgress, now, false)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusDone, now, false)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusCanceled, now, false)
	insertTaskFixture(t, db, project.ID, owner.ID, store.TaskStatusDone, now, true)
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	response := requireProjectObject(t, requireSuccess(t, body))
	if response["group_count"] != float64(1) || response["member_count"] != float64(2) {
		t.Fatalf("group/member counts = %v/%v, want 1/2", response["group_count"], response["member_count"])
	}
	ownerResponse := requireProjectObject(t, response["owner"])
	for field, want := range map[string]any{
		"id": owner.ID, "name": owner.Name, "nickname": owner.Nickname, "avatar": owner.Avatar,
	} {
		if ownerResponse[field] != want {
			t.Fatalf("owner.%s = %v, want %v", field, ownerResponse[field], want)
		}
	}
	taskCounts := requireProjectObject(t, response["task_counts"])
	for field, want := range map[string]float64{
		"total": 5, "todo": 2, "in_progress": 1, "done": 1, "canceled": 1,
	} {
		if taskCounts[field] != want {
			t.Fatalf("task_counts.%s = %v, want %v", field, taskCounts[field], want)
		}
	}
}

func TestProjectGetRejectsMalformedAndMissingIDs(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	user := insertTestUser(t, db, "project-get-validation@example.com", "Get Validation", store.UserStatusActive, time.Now().UTC())
	cookie := loginAsUser(t, server, user.Email)

	resp, body := getJSON(t, server, "/api/client/projects/not-a-uuid", cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")

	resp, body = getJSON(t, server, "/api/client/projects/"+uuid.NewString(), cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
}

func TestProjectUpdateChangesOnlyMutableFieldsForOwner(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "project-update@example.com", "Update Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{
		Owner: owner, Name: "Original", Description: "Keep me", Avatar: "/avatars/original.webp", UpdatedAt: now,
	})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := patchJSON(t, server, "/api/client/projects/"+project.ID, map[string]any{
		"name": "  Updated  ",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	updated := requireProjectObject(t, requireSuccess(t, body))
	if updated["name"] != "Updated" || updated["description"] != "Keep me" || updated["avatar"] != "/avatars/original.webp" {
		t.Fatalf("updated response = %#v", updated)
	}
	stored := requireProjectByID(t, db, project.ID)
	if stored.OwnerUserID != owner.ID || stored.CreatedByUserID != owner.ID || stored.IsPersonal {
		t.Fatalf("immutable fields changed: %#v", stored)
	}
	if !stored.UpdatedAt.After(now) {
		t.Fatalf("updated_at = %v, want after %v", stored.UpdatedAt, now)
	}

	resp, body = patchJSON(t, server, "/api/client/projects/"+project.ID, map[string]any{
		"description": "Changed description",
		"avatar":      "",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second update status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	updated = requireProjectObject(t, requireSuccess(t, body))
	if updated["name"] != "Updated" || updated["description"] != "Changed description" || updated["avatar"] != "" {
		t.Fatalf("second updated response = %#v", updated)
	}
}

func TestProjectUpdateRejectsInvalidInput(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	owner := insertTestUser(t, db, "project-update-validation@example.com", "Update Validation", store.UserStatusActive, time.Now().UTC())
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Valid", UpdatedAt: time.Now().UTC()})
	cookie := loginAsUser(t, server, owner.Email)

	for _, testCase := range []struct {
		name  string
		path  string
		value string
	}{
		{name: "malformed ID", path: "/api/client/projects/bad-id", value: "Valid"},
		{name: "blank name", path: "/api/client/projects/" + project.ID, value: "  "},
		{name: "long name", path: "/api/client/projects/" + project.ID, value: strings.Repeat("界", 121)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resp, body := patchJSON(t, server, testCase.path, map[string]any{"name": testCase.value}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
}

func TestProjectUpdateRejectsUnknownAndImmutableFields(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	owner := insertTestUser(t, db, "project-update-fields@example.com", "Update Fields", store.UserStatusActive, time.Now().UTC())
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Unchanged", UpdatedAt: time.Now().UTC()})
	cookie := loginAsUser(t, server, owner.Email)
	for name, field := range map[string]string{
		"unknown":  "unexpected",
		"personal": "is_personal",
		"owner":    "owner_user_id",
		"creator":  "created_by_user_id",
		"groups":   "group_ids",
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := patchJSON(t, server, "/api/client/projects/"+project.ID, map[string]any{
				"name": "Must Not Persist",
				field:  true,
			}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
			if stored := requireProjectByID(t, db, project.ID); stored.Name != "Unchanged" {
				t.Fatalf("project name = %q, want unchanged", stored.Name)
			}
		})
	}
}

func TestProjectDeleteSoftDeletesCollaborativeAndRejectsPersonal(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-delete@example.com", "Delete Owner", store.UserStatusActive, now)
	collaborative := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Delete Me", UpdatedAt: now})
	personal := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Personal", IsPersonal: true, UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/projects/"+collaborative.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	var deleted store.Project
	if err := db.Unscoped().First(&deleted, "id = ?", collaborative.ID).Error; err != nil {
		t.Fatalf("find soft-deleted project: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatal("deleted_at is not set")
	}

	resp, body = requestJSON(t, server, http.MethodDelete, "/api/client/projects/"+personal.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("personal delete status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
	requireProjectByID(t, db, personal.ID)

	resp, body = requestJSON(t, server, http.MethodDelete, "/api/client/projects/not-a-uuid", map[string]any{}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed delete status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestProjectAccessAllowsOnlyOwnerOrActiveHumanGroupMember(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-access-owner@example.com", "Access Owner", store.UserStatusActive, now)
	member := insertTestUser(t, db, "project-access-member@example.com", "Access Member", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "project-access-outsider@example.com", "Access Outsider", store.UserStatusActive, now)
	appOnly := insertTestUser(t, db, "project-access-app@example.com", "App Only", store.UserStatusActive, now)
	leftUser := insertTestUser(t, db, "project-access-left@example.com", "Left User", store.UserStatusActive, now)
	dissolvedUser := insertTestUser(t, db, "project-access-dissolved@example.com", "Dissolved User", store.UserStatusActive, now)
	directUser := insertTestUser(t, db, "project-access-direct@example.com", "Direct User", store.UserStatusActive, now)
	leftAt := now
	activeGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID:  owner.ID,
		kind:             store.ConversationKindGroup,
		memberIDs:        []string{member.ID, leftUser.ID},
		memberLeftAtByID: map[string]*time.Time{leftUser.ID: &leftAt},
		name:             "Active Access Group",
		now:              now,
	})
	if err := db.Create(&store.ConversationMember{
		ConversationID: activeGroup.ID,
		MemberType:     store.ConversationMemberTypeApp,
		MemberID:       appOnly.ID,
		Role:           store.ConversationMemberRoleMember,
		JoinedAt:       now,
	}).Error; err != nil {
		t.Fatalf("create app-only membership: %v", err)
	}
	dissolvedGroup := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{dissolvedUser.ID},
		name:            "Dissolved Access Group",
		now:             now,
	})
	if err := db.Model(&dissolvedGroup).Update("status", store.ConversationStatusDissolved).Error; err != nil {
		t.Fatalf("dissolve access group: %v", err)
	}
	direct := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindDirect,
		memberIDs:       []string{owner.ID, directUser.ID},
		name:            "Direct Access Conversation",
		now:             now,
	})
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Shared Project", UpdatedAt: now})
	insertProjectGroupFixture(t, db, project.ID, activeGroup.ID, owner.ID, now)
	insertProjectGroupFixture(t, db, project.ID, dissolvedGroup.ID, owner.ID, now)
	insertProjectGroupFixture(t, db, project.ID, direct.ID, owner.ID, now)

	memberCookie := loginAsUser(t, server, member.Email)
	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID, memberCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("derived member read status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if role := requireProjectObject(t, requireSuccess(t, body))["current_user_role"]; role != store.ProjectRoleMember {
		t.Fatalf("current_user_role = %v, want member", role)
	}
	for method, request := range map[string]func() (*http.Response, map[string]any){
		"patch": func() (*http.Response, map[string]any) {
			return patchJSON(t, server, "/api/client/projects/"+project.ID, map[string]any{"name": "Denied"}, memberCookie)
		},
		"delete": func() (*http.Response, map[string]any) {
			return requestJSON(t, server, http.MethodDelete, "/api/client/projects/"+project.ID, map[string]any{}, memberCookie)
		},
	} {
		t.Run("derived member "+method, func(t *testing.T) {
			resp, body := request()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "forbidden")
		})
	}

	for name, user := range map[string]store.User{
		"outsider":         outsider,
		"app member":       appOnly,
		"left member":      leftUser,
		"dissolved member": dissolvedUser,
		"direct member":    directUser,
	} {
		t.Run(name, func(t *testing.T) {
			cookie := loginAsUser(t, server, user.Email)
			resp, body := getJSON(t, server, "/api/client/projects/"+project.ID, cookie)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "not_found")
		})
	}
}

func TestProjectGroupBindAllowsOwnerWithoutMembershipAndIsIdempotent(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	oldUpdatedAt := now.Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "project-group-bind-owner@example.com", "Group Bind Owner", store.UserStatusActive, now)
	groupMember := insertTestUser(t, db, "project-group-bind-member@example.com", "Group Member", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Bind Project", UpdatedAt: oldUpdatedAt})
	group := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner,
		Kind:    store.ConversationKindGroup,
		Status:  store.ConversationStatusActive,
		Name:    "Target Group",
		Now:     now,
		Members: []store.ConversationMember{{
			MemberType: store.ConversationMemberTypeUser,
			MemberID:   groupMember.ID,
			Role:       store.ConversationMemberRoleMember,
		}},
	})
	cookie := loginAsUser(t, server, owner.Email)

	path := "/api/client/projects/" + project.ID + "/groups/" + group.ID
	resp, body := putJSON(t, server, path, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bind status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	var link store.ProjectGroup
	if err := db.First(&link, "project_id = ? AND conversation_id = ?", project.ID, group.ID).Error; err != nil {
		t.Fatalf("find project group link: %v", err)
	}
	if link.LinkedByUserID != owner.ID {
		t.Fatalf("linked_by_user_id = %s, want %s", link.LinkedByUserID, owner.ID)
	}
	updatedAfterBind := requireProjectByID(t, db, project.ID).UpdatedAt
	if !updatedAfterBind.After(oldUpdatedAt) {
		t.Fatalf("updated_at after bind = %v, want after %v", updatedAfterBind, oldUpdatedAt)
	}

	resp, body = putJSON(t, server, path, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("idempotent bind status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	requireRowCount(t, db, &store.ProjectGroup{}, 1, "project_id = ? AND conversation_id = ?", project.ID, group.ID)
	updatedAfterIdempotentBind := requireProjectByID(t, db, project.ID).UpdatedAt
	if !updatedAfterIdempotentBind.Equal(updatedAfterBind) {
		t.Fatalf("idempotent bind updated_at = %v, want unchanged %v", updatedAfterIdempotentBind, updatedAfterBind)
	}
}

func TestProjectGroupBindRejectsInvalidTargetsAndUnauthorizedProjects(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-group-reject-owner@example.com", "Group Reject Owner", store.UserStatusActive, now)
	derived := insertTestUser(t, db, "project-group-reject-derived@example.com", "Group Reject Derived", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Protected Project", UpdatedAt: now})
	personal := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Personal", IsPersonal: true, UpdatedAt: now})
	accessGroup := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner,
		Kind:    store.ConversationKindGroup,
		Status:  store.ConversationStatusActive,
		Name:    "Access Group",
		Now:     now,
		Members: []store.ConversationMember{{MemberType: store.ConversationMemberTypeUser, MemberID: derived.ID}},
	})
	insertProjectGroupFixture(t, db, project.ID, accessGroup.ID, owner.ID, now)
	target := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusActive, Name: "Target", Now: now,
	})
	direct := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindDirect, Status: store.ConversationStatusActive, Name: "Direct", Now: now,
	})
	dissolved := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusDissolved, Name: "Dissolved", Now: now,
	})
	ownerCookie := loginAsUser(t, server, owner.Email)
	derivedCookie := loginAsUser(t, server, derived.Email)

	for name, groupID := range map[string]string{
		"malformed UUID": "bad-group-id",
		"missing group":  uuid.NewString(),
		"direct":         direct.ID,
		"dissolved":      dissolved.ID,
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := putJSON(t, server, "/api/client/projects/"+project.ID+"/groups/"+groupID, map[string]any{}, ownerCookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}

	resp, body := putJSON(t, server, "/api/client/projects/bad-project-id/groups/"+target.ID, map[string]any{}, ownerCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed project status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")

	resp, body = putJSON(t, server, "/api/client/projects/"+personal.ID+"/groups/"+target.ID, map[string]any{}, ownerCookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("personal bind status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")

	resp, body = putJSON(t, server, "/api/client/projects/"+project.ID+"/groups/"+target.ID, map[string]any{}, derivedCookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("derived bind status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
}

func TestProjectGroupDeleteIsIdempotentAndUpdatesOnlyOnChange(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	oldUpdatedAt := now.Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "project-group-delete-owner@example.com", "Group Delete Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Unbind Project", UpdatedAt: oldUpdatedAt})
	personal := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Personal", IsPersonal: true, UpdatedAt: now})
	group := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusActive, Name: "Linked Group", Now: now,
	})
	insertProjectGroupFixture(t, db, project.ID, group.ID, owner.ID, oldUpdatedAt)
	cookie := loginAsUser(t, server, owner.Email)
	path := "/api/client/projects/" + project.ID + "/groups/" + group.ID

	resp, body := requestJSON(t, server, http.MethodDelete, path, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unbind status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	requireRowCount(t, db, &store.ProjectGroup{}, 0, "project_id = ? AND conversation_id = ?", project.ID, group.ID)
	updatedAfterDelete := requireProjectByID(t, db, project.ID).UpdatedAt
	if !updatedAfterDelete.After(oldUpdatedAt) {
		t.Fatalf("updated_at after delete = %v, want after %v", updatedAfterDelete, oldUpdatedAt)
	}

	resp, body = requestJSON(t, server, http.MethodDelete, path, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("idempotent unbind status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	updatedAfterIdempotentDelete := requireProjectByID(t, db, project.ID).UpdatedAt
	if !updatedAfterIdempotentDelete.Equal(updatedAfterDelete) {
		t.Fatalf("idempotent unbind updated_at = %v, want unchanged %v", updatedAfterIdempotentDelete, updatedAfterDelete)
	}

	resp, body = requestJSON(t, server, http.MethodDelete, "/api/client/projects/"+personal.ID+"/groups/"+group.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("personal unbind status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
}

func TestProjectGroupListOrdersAndPaginatesActiveGroups(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-group-list-owner@example.com", "Group List Owner", store.UserStatusActive, now)
	derived := insertTestUser(t, db, "project-group-list-derived@example.com", "Group List Derived", store.UserStatusActive, now)
	left := insertTestUser(t, db, "project-group-list-left@example.com", "Group List Left", store.UserStatusActive, now)
	leftAt := now
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Group List Project", UpdatedAt: now})
	memberRows := []store.ConversationMember{
		{MemberType: store.ConversationMemberTypeUser, MemberID: derived.ID},
		{MemberType: store.ConversationMemberTypeApp, MemberID: uuid.NewString()},
		{MemberType: store.ConversationMemberTypeUser, MemberID: left.ID, LeftAt: &leftAt},
	}
	high := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		ID: "00000000-0000-0000-0000-000000000202", Creator: owner, Kind: store.ConversationKindGroup,
		Status: store.ConversationStatusActive, Name: "High", Avatar: "/avatars/high.webp", Now: now, Members: memberRows,
	})
	low := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		ID: "00000000-0000-0000-0000-000000000201", Creator: owner, Kind: store.ConversationKindGroup,
		Status: store.ConversationStatusActive, Name: "Low", Now: now, Members: []store.ConversationMember{{MemberType: store.ConversationMemberTypeUser, MemberID: derived.ID}},
	})
	older := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		ID: "00000000-0000-0000-0000-000000000203", Creator: owner, Kind: store.ConversationKindGroup,
		Status: store.ConversationStatusActive, Name: "Older", Now: now, Members: []store.ConversationMember{{MemberType: store.ConversationMemberTypeUser, MemberID: derived.ID}},
	})
	dissolved := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusDissolved, Name: "Dissolved", Now: now,
	})
	direct := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindDirect, Status: store.ConversationStatusActive, Name: "Direct", Now: now,
	})
	insertProjectGroupFixture(t, db, project.ID, high.ID, owner.ID, now)
	insertProjectGroupFixture(t, db, project.ID, low.ID, owner.ID, now)
	insertProjectGroupFixture(t, db, project.ID, older.ID, owner.ID, now.Add(-time.Hour))
	insertProjectGroupFixture(t, db, project.ID, dissolved.ID, owner.ID, now.Add(time.Hour))
	insertProjectGroupFixture(t, db, project.ID, direct.ID, owner.ID, now.Add(time.Hour))
	cookie := loginAsUser(t, server, derived.Email)

	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID+"/groups?limit=2", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	groups := requireObjectList(t, data["groups"])
	if len(groups) != 2 || groups[0]["id"] != high.ID || groups[1]["id"] != low.ID {
		t.Fatalf("first page groups = %#v, want high then low", groups)
	}
	if groups[0]["name"] != "High" || groups[0]["avatar"] != "/avatars/high.webp" || groups[0]["status"] != store.ConversationStatusActive {
		t.Fatalf("high group summary = %#v", groups[0])
	}
	if groups[0]["member_count"] != float64(2) {
		t.Fatalf("high member_count = %v, want 2", groups[0]["member_count"])
	}
	if _, ok := groups[0]["created_at"].(string); !ok {
		t.Fatalf("relation created_at = %#v, want timestamp", groups[0]["created_at"])
	}
	cursor, ok := data["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor = %#v, want non-empty string", data["next_cursor"])
	}

	resp, body = getJSON(t, server, "/api/client/projects/"+project.ID+"/groups?limit=2&cursor="+cursor, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	secondPage := requireSuccess(t, body)
	secondGroups := requireObjectList(t, secondPage["groups"])
	if len(secondGroups) != 1 || secondGroups[0]["id"] != older.ID {
		t.Fatalf("second page groups = %#v, want older", secondGroups)
	}
	if secondPage["next_cursor"] != nil {
		t.Fatalf("second page next_cursor = %#v, want null", secondPage["next_cursor"])
	}

	for _, path := range []string{
		"/api/client/projects/" + project.ID + "/groups?limit=101",
		"/api/client/projects/" + project.ID + "/groups?cursor=bad-cursor",
		"/api/client/projects/bad-project-id/groups",
	} {
		resp, body = getJSON(t, server, path, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("validation path %s status = %d, want 400, body = %#v", path, resp.StatusCode, body)
		}
		requireError(t, body, "invalid_request")
	}
}

func TestProjectMemberListDeduplicatesSourcesAndIncludesDisabledUsers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-member-owner@example.com", "Charlie Owner", store.UserStatusActive, now)
	crossGroup := insertTestUser(t, db, "project-member-cross@example.com", "Cross User", store.UserStatusActive, now)
	crossGroup.Nickname = "Alpha"
	if err := db.Model(&crossGroup).Update("nickname", crossGroup.Nickname).Error; err != nil {
		t.Fatalf("update cross-group nickname: %v", err)
	}
	disabled := insertTestUser(t, db, "project-member-disabled@example.com", "Bravo Disabled", store.UserStatusDisabled, now)
	other := insertTestUser(t, db, "project-member-other@example.com", "Delta Other", store.UserStatusActive, now)
	left := insertTestUser(t, db, "project-member-left@example.com", "Left Hidden", store.UserStatusActive, now)
	appOnly := insertTestUser(t, db, "project-member-app@example.com", "App Hidden", store.UserStatusActive, now)
	dissolvedOnly := insertTestUser(t, db, "project-member-dissolved@example.com", "Dissolved Hidden", store.UserStatusActive, now)
	directOnly := insertTestUser(t, db, "project-member-direct@example.com", "Direct Hidden", store.UserStatusActive, now)
	leftAt := now
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Member Project", UpdatedAt: now})
	low := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		ID: "00000000-0000-0000-0000-000000000301", Creator: owner, Kind: store.ConversationKindGroup,
		Status: store.ConversationStatusActive, Name: "Member Low", Now: now,
		Members: []store.ConversationMember{
			{MemberType: store.ConversationMemberTypeUser, MemberID: owner.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: crossGroup.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: disabled.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: left.ID, LeftAt: &leftAt},
			{MemberType: store.ConversationMemberTypeApp, MemberID: appOnly.ID},
		},
	})
	high := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		ID: "00000000-0000-0000-0000-000000000302", Creator: owner, Kind: store.ConversationKindGroup,
		Status: store.ConversationStatusActive, Name: "Member High", Now: now,
		Members: []store.ConversationMember{
			{MemberType: store.ConversationMemberTypeUser, MemberID: owner.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: crossGroup.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: other.ID},
		},
	})
	dissolved := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusDissolved, Name: "Member Dissolved", Now: now,
		Members: []store.ConversationMember{{MemberType: store.ConversationMemberTypeUser, MemberID: dissolvedOnly.ID}},
	})
	direct := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindDirect, Status: store.ConversationStatusActive, Name: "Member Direct", Now: now,
		Members: []store.ConversationMember{{MemberType: store.ConversationMemberTypeUser, MemberID: directOnly.ID}},
	})
	for _, groupID := range []string{low.ID, high.ID, dissolved.ID, direct.ID} {
		insertProjectGroupFixture(t, db, project.ID, groupID, owner.ID, now)
	}
	cookie := loginAsUser(t, server, crossGroup.Email)

	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID+"/members", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	members := requireObjectList(t, data["members"])
	if len(members) != 4 {
		t.Fatalf("members length = %d, want 4: %#v", len(members), members)
	}
	wantOrder := []string{crossGroup.ID, disabled.ID, owner.ID, other.ID}
	byID := make(map[string]map[string]any, len(members))
	for index, member := range members {
		if member["id"] != wantOrder[index] {
			t.Fatalf("member %d id = %v, want %s; members = %#v", index, member["id"], wantOrder[index], members)
		}
		byID[member["id"].(string)] = member
	}
	if byID[crossGroup.ID]["display_name"] != "Alpha" || byID[crossGroup.ID]["role"] != store.ProjectRoleMember {
		t.Fatalf("cross-group member = %#v", byID[crossGroup.ID])
	}
	assertStringList(t, byID[crossGroup.ID]["source_group_ids"], []string{low.ID, high.ID})
	if byID[disabled.ID]["status"] != store.UserStatusDisabled {
		t.Fatalf("disabled member status = %v, want disabled", byID[disabled.ID]["status"])
	}
	if byID[owner.ID]["role"] != store.ProjectRoleOwner {
		t.Fatalf("owner role = %v, want owner", byID[owner.ID]["role"])
	}
	assertStringList(t, byID[owner.ID]["source_group_ids"], []string{})
	for _, excludedID := range []string{left.ID, appOnly.ID, dissolvedOnly.ID, directOnly.ID} {
		if _, exists := byID[excludedID]; exists {
			t.Fatalf("excluded member %s appeared: %#v", excludedID, byID[excludedID])
		}
	}
	if data["next_cursor"] != nil {
		t.Fatalf("next_cursor = %#v, want null", data["next_cursor"])
	}
}

func TestProjectMemberListPaginatesDisplayNameTies(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "project-member-page-owner@example.com", "A Owner", store.UserStatusActive, now)
	firstSame := insertTestUser(t, db, "project-member-page-first@example.com", "First", store.UserStatusActive, now)
	secondSame := insertTestUser(t, db, "project-member-page-second@example.com", "Second", store.UserStatusActive, now)
	for _, user := range []*store.User{&firstSame, &secondSame} {
		user.Nickname = "Same"
		if err := db.Model(user).Update("nickname", user.Nickname).Error; err != nil {
			t.Fatalf("update tie nickname: %v", err)
		}
	}
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Member Page Project", UpdatedAt: now})
	group := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner, Kind: store.ConversationKindGroup, Status: store.ConversationStatusActive, Name: "Member Page Group", Now: now,
		Members: []store.ConversationMember{
			{MemberType: store.ConversationMemberTypeUser, MemberID: firstSame.ID},
			{MemberType: store.ConversationMemberTypeUser, MemberID: secondSame.ID},
		},
	})
	insertProjectGroupFixture(t, db, project.ID, group.ID, owner.ID, now)
	cookie := loginAsUser(t, server, owner.Email)
	tieIDs := []string{firstSame.ID, secondSame.ID}
	slices.Sort(tieIDs)

	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID+"/members?limit=2", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireSuccess(t, body)
	members := requireObjectList(t, data["members"])
	if len(members) != 2 || members[0]["id"] != owner.ID || members[1]["id"] != tieIDs[0] {
		t.Fatalf("first page members = %#v, want owner then %s", members, tieIDs[0])
	}
	cursor, ok := data["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor = %#v, want non-empty string", data["next_cursor"])
	}

	resp, body = getJSON(t, server, "/api/client/projects/"+project.ID+"/members?limit=2&cursor="+cursor, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	secondPage := requireSuccess(t, body)
	secondMembers := requireObjectList(t, secondPage["members"])
	if len(secondMembers) != 1 || secondMembers[0]["id"] != tieIDs[1] {
		t.Fatalf("second page members = %#v, want %s", secondMembers, tieIDs[1])
	}
	if secondPage["next_cursor"] != nil {
		t.Fatalf("second page next_cursor = %#v, want null", secondPage["next_cursor"])
	}

	for _, path := range []string{
		"/api/client/projects/" + project.ID + "/members?limit=101",
		"/api/client/projects/" + project.ID + "/members?cursor=bad-cursor",
		"/api/client/projects/bad-project-id/members",
	} {
		resp, body = getJSON(t, server, path, cookie)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("validation path %s status = %d, want 400, body = %#v", path, resp.StatusCode, body)
		}
		requireError(t, body, "invalid_request")
	}
}

type projectFixtureInput struct {
	ID          string
	Owner       store.User
	Name        string
	Description string
	Avatar      string
	IsPersonal  bool
	UpdatedAt   time.Time
}

type projectConversationFixtureInput struct {
	ID      string
	Creator store.User
	Kind    string
	Status  string
	Name    string
	Avatar  string
	Now     time.Time
	Members []store.ConversationMember
}

func insertProjectConversationFixture(t *testing.T, db *gorm.DB, input projectConversationFixtureInput) store.Conversation {
	t.Helper()

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	conversation := store.Conversation{
		ID:              input.ID,
		Kind:            input.Kind,
		Name:            input.Name,
		Avatar:          input.Avatar,
		CreatedByUserID: input.Creator.ID,
		Status:          input.Status,
		PostingPolicy:   store.ConversationPostingPolicyOpen,
		Visibility:      store.ConversationVisibilityPrivate,
		CreatedAt:       input.Now,
		UpdatedAt:       input.Now,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create project conversation fixture: %v", err)
	}
	for _, member := range input.Members {
		member.ConversationID = conversation.ID
		if member.Role == "" {
			member.Role = store.ConversationMemberRoleMember
		}
		if member.JoinedAt.IsZero() {
			member.JoinedAt = input.Now
		}
		if member.HistoryVisibleFromSeq == 0 {
			member.HistoryVisibleFromSeq = 1
		}
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create project conversation member fixture: %v", err)
		}
	}
	return conversation
}

func insertProjectFixture(t *testing.T, db *gorm.DB, input projectFixtureInput) store.Project {
	t.Helper()

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.UpdatedAt.IsZero() {
		input.UpdatedAt = time.Now().UTC()
	}
	project := store.Project{
		ID:              input.ID,
		Name:            input.Name,
		Description:     input.Description,
		Avatar:          input.Avatar,
		OwnerUserID:     input.Owner.ID,
		CreatedByUserID: input.Owner.ID,
		IsPersonal:      input.IsPersonal,
		CreatedAt:       input.UpdatedAt.Add(-time.Minute),
		UpdatedAt:       input.UpdatedAt,
	}
	if err := db.Select("*").Create(&project).Error; err != nil {
		t.Fatalf("create project fixture: %v", err)
	}
	return project
}

func insertProjectGroupFixture(t *testing.T, db *gorm.DB, projectID string, groupID string, linkedByUserID string, createdAt time.Time) store.ProjectGroup {
	t.Helper()

	link := store.ProjectGroup{
		ProjectID:      projectID,
		ConversationID: groupID,
		LinkedByUserID: linkedByUserID,
		CreatedAt:      createdAt,
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create project group fixture: %v", err)
	}
	return link
}

func insertTaskFixture(t *testing.T, db *gorm.DB, projectID string, createdByUserID string, status string, now time.Time, deleted bool) store.Task {
	t.Helper()

	task := store.Task{
		ID:              uuid.NewString(),
		ProjectID:       projectID,
		Title:           status,
		Status:          status,
		Priority:        store.TaskPriorityMedium,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task fixture: %v", err)
	}
	if deleted {
		if err := db.Delete(&task).Error; err != nil {
			t.Fatalf("soft-delete task fixture: %v", err)
		}
	}
	return task
}

func requireProjectByID(t *testing.T, db *gorm.DB, projectID string) store.Project {
	t.Helper()

	var project store.Project
	if err := db.First(&project, "id = ?", projectID).Error; err != nil {
		t.Fatalf("find project %s: %v", projectID, err)
	}
	return project
}

func requireProjectObject(t *testing.T, value any) map[string]any {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", value)
	}
	return object
}

func requireObjectList(t *testing.T, value any) []map[string]any {
	t.Helper()

	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", value)
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requireProjectObject(t, item))
	}
	return result
}

func assertProjectZeroCounts(t *testing.T, project map[string]any) {
	t.Helper()

	if project["group_count"] != float64(0) || project["member_count"] != float64(1) {
		t.Fatalf("group/member counts = %v/%v, want 0/1", project["group_count"], project["member_count"])
	}
	taskCounts := requireProjectObject(t, project["task_counts"])
	for _, field := range []string{"total", "todo", "in_progress", "done", "canceled"} {
		if taskCounts[field] != float64(0) {
			t.Fatalf("task_counts.%s = %v, want 0", field, taskCounts[field])
		}
	}
}

func assertStringList(t *testing.T, value any, want []string) {
	t.Helper()

	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want string array", value)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("array item = %#v, want string", item)
		}
		got = append(got, text)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("string array = %#v, want %#v", got, want)
	}
}
