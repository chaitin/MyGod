package httpserver

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestTaskCreateTitleOnlyUsesDefaults(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-create-defaults@example.com", "Task Owner", store.UserStatusActive, now)
	owner.Nickname = "Owner Nickname"
	if err := db.Model(&owner).Update("nickname", owner.Nickname).Error; err != nil {
		t.Fatalf("set owner nickname: %v", err)
	}
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Task Project", UpdatedAt: now.Add(-time.Hour)})
	projectBefore := requireProjectByID(t, db, project.ID)
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
		"title": "  First task  ",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	task := requireTaskResponse(t, requireSuccess(t, body))
	if task["project_id"] != project.ID || task["title"] != "First task" || task["description"] != "" {
		t.Fatalf("task identity/text = %#v", task)
	}
	if task["status"] != store.TaskStatusTodo || task["priority"] != float64(store.TaskPriorityMedium) {
		t.Fatalf("task status/priority = %v/%v", task["status"], task["priority"])
	}
	for _, field := range []string{"assignee", "start_date", "due_date", "completed_at", "canceled_at"} {
		if task[field] != nil {
			t.Fatalf("%s = %#v, want null", field, task[field])
		}
	}
	assertTaskStringList(t, task["labels"], []string{})
	creator := requireTaskObject(t, task["creator"])
	assertTaskUserSummary(t, creator, owner)
	if _, err := time.Parse(time.RFC3339Nano, task["created_at"].(string)); err != nil {
		t.Fatalf("created_at = %#v, want RFC3339 timestamp: %v", task["created_at"], err)
	}

	stored := requireTaskByID(t, db, task["id"].(string), false)
	if stored.Title != "First task" || stored.Description != "" || stored.Status != store.TaskStatusTodo || stored.Priority != store.TaskPriorityMedium {
		t.Fatalf("stored defaults = %#v", stored)
	}
	if stored.AssigneeUserID != nil || stored.StartDate != nil || stored.DueDate != nil || stored.CompletedAt != nil || stored.CanceledAt != nil {
		t.Fatalf("stored nullable defaults = %#v", stored)
	}
	if stored.Labels == nil || len(stored.Labels) != 0 {
		t.Fatalf("stored labels = %#v, want non-nil empty", stored.Labels)
	}
	projectAfter := requireProjectByID(t, db, project.ID)
	if !projectAfter.UpdatedAt.After(projectBefore.UpdatedAt) {
		t.Fatalf("project updated_at = %v, want after %v", projectAfter.UpdatedAt, projectBefore.UpdatedAt)
	}
}

func TestTaskCreateAcceptsFullRequestAndValidAssignee(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-create-full-owner@example.com", "Full Owner", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-create-full-assignee@example.com", "Full Assignee", store.UserStatusActive, now)
	assignee.Nickname = "Assigned"
	if err := db.Model(&assignee).Update("nickname", assignee.Nickname).Error; err != nil {
		t.Fatalf("set assignee nickname: %v", err)
	}
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Full Task Project", UpdatedAt: now.Add(-time.Hour)})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
		"title":            "  Full task  ",
		"description":      "**Markdown** details",
		"status":           store.TaskStatusInProgress,
		"priority":         store.TaskPriorityHigh,
		"assignee_user_id": assignee.ID,
		"start_date":       "2026-07-11",
		"due_date":         "2026-07-15",
		"labels":           []string{" Backend ", "urgent", "BACKEND"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	task := requireTaskResponse(t, requireSuccess(t, body))
	if task["title"] != "Full task" || task["description"] != "**Markdown** details" || task["status"] != store.TaskStatusInProgress {
		t.Fatalf("full task text/status = %#v", task)
	}
	if task["priority"] != float64(store.TaskPriorityHigh) || task["start_date"] != "2026-07-11" || task["due_date"] != "2026-07-15" {
		t.Fatalf("full task priority/dates = %#v", task)
	}
	assertTaskStringList(t, task["labels"], []string{"Backend", "urgent"})
	assertTaskUserSummary(t, requireTaskObject(t, task["assignee"]), assignee)
	if task["completed_at"] != nil || task["canceled_at"] != nil {
		t.Fatalf("in-progress terminal timestamps = %v/%v, want null/null", task["completed_at"], task["canceled_at"])
	}

	stored := requireTaskByID(t, db, task["id"].(string), false)
	if stored.AssigneeUserID == nil || *stored.AssigneeUserID != assignee.ID {
		t.Fatalf("stored assignee = %#v, want %s", stored.AssigneeUserID, assignee.ID)
	}
	if stored.StartDate == nil || stored.StartDate.Format("2006-01-02") != "2026-07-11" || stored.DueDate == nil || stored.DueDate.Format("2006-01-02") != "2026-07-15" {
		t.Fatalf("stored dates = %v/%v", stored.StartDate, stored.DueDate)
	}
}

func TestTaskCreateSetsTerminalTimestamps(t *testing.T) {
	for _, testCase := range []struct {
		status          string
		wantCompletedAt bool
		wantCanceledAt  bool
	}{
		{status: store.TaskStatusDone, wantCompletedAt: true},
		{status: store.TaskStatusCanceled, wantCanceledAt: true},
	} {
		t.Run(testCase.status, func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()

			now := time.Now().UTC()
			owner := insertTestUser(t, db, "task-create-terminal-"+testCase.status+"@example.com", "Terminal Owner", store.UserStatusActive, now)
			project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Terminal Project", UpdatedAt: now})
			cookie := loginAsUser(t, server, owner.Email)
			before := time.Now().UTC()

			resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
				"title":  "Terminal task",
				"status": testCase.status,
			}, cookie)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
			}
			task := requireTaskResponse(t, requireSuccess(t, body))
			assertTaskTimestampPresence(t, task, "completed_at", testCase.wantCompletedAt, before)
			assertTaskTimestampPresence(t, task, "canceled_at", testCase.wantCanceledAt, before)
		})
	}
}

func TestTaskCreateAllowsDerivedMemberAndHidesInaccessibleProject(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-create-access-owner@example.com", "Access Owner", store.UserStatusActive, now)
	member := insertTestUser(t, db, "task-create-access-member@example.com", "Access Member", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "task-create-access-outsider@example.com", "Access Outsider", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Access Project", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, member, now)

	memberCookie := loginAsUser(t, server, member.Email)
	resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{"title": "Member-created"}, memberCookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("derived member status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	task := requireTaskResponse(t, requireSuccess(t, body))
	if task["created_by_user_id"] != nil {
		t.Fatalf("response exposed created_by_user_id: %#v", task)
	}
	if requireTaskObject(t, task["creator"])["id"] != member.ID {
		t.Fatalf("creator = %#v, want member %s", task["creator"], member.ID)
	}

	outsiderCookie := loginAsUser(t, server, outsider.Email)
	resp, body = postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{"title": "Hidden"}, outsiderCookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
}

func TestTaskCreateRejectsInvalidFieldsAndStrictJSONViolations(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-create-invalid@example.com", "Validation Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Validation Project", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	path := "/api/client/projects/" + project.ID + "/tasks"

	labels21 := make([]string, 21)
	for index := range labels21 {
		labels21[index] = "label-" + string(rune('a'+index))
	}
	testCases := []struct {
		name string
		raw  string
	}{
		{name: "missing title", raw: `{}`},
		{name: "blank title", raw: `{"title":"   "}`},
		{name: "long rune title", raw: `{"title":"` + strings.Repeat("界", 241) + `"}`},
		{name: "null title", raw: `{"title":null}`},
		{name: "null description", raw: `{"title":"Valid","description":null}`},
		{name: "invalid status", raw: `{"title":"Valid","status":"blocked"}`},
		{name: "null status", raw: `{"title":"Valid","status":null}`},
		{name: "low priority", raw: `{"title":"Valid","priority":0}`},
		{name: "high priority", raw: `{"title":"Valid","priority":4}`},
		{name: "non-integer priority", raw: `{"title":"Valid","priority":2.5}`},
		{name: "null priority", raw: `{"title":"Valid","priority":null}`},
		{name: "invalid start date", raw: `{"title":"Valid","start_date":"2026-02-30"}`},
		{name: "date with timestamp", raw: `{"title":"Valid","due_date":"2026-07-11T00:00:00Z"}`},
		{name: "reversed dates", raw: `{"title":"Valid","start_date":"2026-07-12","due_date":"2026-07-11"}`},
		{name: "null assignee", raw: `{"title":"Valid","assignee_user_id":null}`},
		{name: "null start date", raw: `{"title":"Valid","start_date":null}`},
		{name: "null due date", raw: `{"title":"Valid","due_date":null}`},
		{name: "null labels", raw: `{"title":"Valid","labels":null}`},
		{name: "blank label", raw: `{"title":"Valid","labels":["ok","  "]}`},
		{name: "long label", raw: `{"title":"Valid","labels":["` + strings.Repeat("界", 33) + `"]}`},
		{name: "too many labels", raw: mustTaskJSON(t, map[string]any{"title": "Valid", "labels": labels21})},
		{name: "unknown field", raw: `{"title":"Valid","wat":true}`},
		{name: "immutable project id", raw: `{"title":"Valid","project_id":"` + project.ID + `"}`},
		{name: "immutable creator id", raw: `{"title":"Valid","created_by_user_id":"` + owner.ID + `"}`},
		{name: "top-level null", raw: `null`},
		{name: "top-level array", raw: `[]`},
		{name: "trailing payload", raw: `{"title":"Valid"} {}`},
		{name: "wrong title type", raw: `{"title":123}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resp, body := requestRawTaskJSON(t, server, http.MethodPost, path, testCase.raw, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
	requireRowCount(t, db, &store.Task{}, 0, "project_id = ?", project.ID)
}

func TestTaskCreateValidatesAssigneeStatusAndAccess(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-create-assignee-owner@example.com", "Assignee Owner", store.UserStatusActive, now)
	disabled := insertTestUser(t, db, "task-create-assignee-disabled@example.com", "Disabled Assignee", store.UserStatusDisabled, now)
	outsider := insertTestUser(t, db, "task-create-assignee-outsider@example.com", "Outsider Assignee", store.UserStatusActive, now)
	member := insertTestUser(t, db, "task-create-assignee-member@example.com", "Member Assignee", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Assignee Project", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, member, now)
	cookie := loginAsUser(t, server, owner.Email)
	path := "/api/client/projects/" + project.ID + "/tasks"

	for name, assigneeID := range map[string]string{
		"malformed UUID": "bad-id",
		"missing user":   uuid.NewString(),
		"disabled user":  disabled.ID,
		"without access": outsider.ID,
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := postJSON(t, server, path, map[string]any{
				"title":            "Invalid assignee",
				"assignee_user_id": assigneeID,
			}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}

	resp, body := postJSON(t, server, path, map[string]any{
		"title":            "Valid assignee",
		"assignee_user_id": member.ID,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("valid assignee status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	assertTaskUserSummary(t, requireTaskObject(t, requireTaskResponse(t, requireSuccess(t, body))["assignee"]), member)
}

func TestTaskGetReturnsDetailAndScopesTaskToProject(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-get-owner@example.com", "Get Owner", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-get-assignee@example.com", "Get Assignee", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "task-get-outsider@example.com", "Get Outsider", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Get Project", UpdatedAt: now})
	otherProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Other Project", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	start := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	due := start.AddDate(0, 0, 2)
	task := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Assignee: &assignee, Title: "Get me", Description: "Details",
		Status: store.TaskStatusInProgress, Priority: store.TaskPriorityHigh, StartDate: &start, DueDate: &due,
		Labels: []string{"alpha", "beta"}, UpdatedAt: now,
	})
	deleted := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Deleted", UpdatedAt: now})
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("soft-delete detail fixture: %v", err)
	}
	ownerCookie := loginAsUser(t, server, owner.Email)

	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID+"/tasks/"+task.ID, ownerCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireTaskResponse(t, requireSuccess(t, body))
	if data["id"] != task.ID || data["project_id"] != project.ID || data["title"] != task.Title {
		t.Fatalf("detail = %#v", data)
	}
	assertTaskStringList(t, data["labels"], []string{"alpha", "beta"})
	assertTaskUserSummary(t, requireTaskObject(t, data["assignee"]), assignee)
	assertTaskUserSummary(t, requireTaskObject(t, data["creator"]), owner)

	for name, path := range map[string]string{
		"task project mismatch": "/api/client/projects/" + otherProject.ID + "/tasks/" + task.ID,
		"soft deleted task":     "/api/client/projects/" + project.ID + "/tasks/" + deleted.ID,
		"missing task":          "/api/client/projects/" + project.ID + "/tasks/" + uuid.NewString(),
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := getJSON(t, server, path, ownerCookie)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "not_found")
		})
	}

	outsiderCookie := loginAsUser(t, server, outsider.Email)
	resp, body = getJSON(t, server, "/api/client/projects/"+project.ID+"/tasks/"+task.ID, outsiderCookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("inaccessible detail status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
}

func TestTaskUpdateClearsNullableFieldsAndLabels(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-update-clear@example.com", "Clear Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Clear Project", UpdatedAt: now.Add(-time.Hour)})
	start := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	due := start.AddDate(0, 0, 3)
	task := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Assignee: &owner, Title: "Clear fields", Status: store.TaskStatusTodo,
		StartDate: &start, DueDate: &due, Labels: []string{"one", "two"}, UpdatedAt: now.Add(-time.Hour),
	})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := patchJSON(t, server, taskPath(project.ID, task.ID), map[string]any{
		"assignee_user_id": nil,
		"start_date":       nil,
		"due_date":         nil,
		"labels":           []string{},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireTaskResponse(t, requireSuccess(t, body))
	for _, field := range []string{"assignee", "start_date", "due_date"} {
		if data[field] != nil {
			t.Fatalf("%s = %#v, want null", field, data[field])
		}
	}
	assertTaskStringList(t, data["labels"], []string{})
	stored := requireTaskByID(t, db, task.ID, false)
	if stored.AssigneeUserID != nil || stored.StartDate != nil || stored.DueDate != nil || len(stored.Labels) != 0 || stored.Labels == nil {
		t.Fatalf("stored cleared fields = %#v", stored)
	}
	projectAfter := requireProjectByID(t, db, project.ID)
	if !projectAfter.UpdatedAt.After(project.UpdatedAt) {
		t.Fatalf("project updated_at = %v, want after %v", projectAfter.UpdatedAt, project.UpdatedAt)
	}
}

func TestTaskUpdateRejectsStrictJSONAndImmutableFields(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-update-invalid@example.com", "Update Validation Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Update Validation", UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Original", Labels: []string{"keep"}, UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	path := taskPath(project.ID, task.ID)

	for _, testCase := range []struct {
		name string
		raw  string
	}{
		{name: "null title", raw: `{"title":null}`},
		{name: "null description", raw: `{"description":null}`},
		{name: "null status", raw: `{"status":null}`},
		{name: "null priority", raw: `{"priority":null}`},
		{name: "null labels", raw: `{"labels":null}`},
		{name: "immutable project", raw: `{"project_id":"` + project.ID + `"}`},
		{name: "immutable creator", raw: `{"created_by_user_id":"` + owner.ID + `"}`},
		{name: "unknown field", raw: `{"unexpected":true}`},
		{name: "top-level null", raw: `null`},
		{name: "top-level array", raw: `[]`},
		{name: "trailing payload", raw: `{} {}`},
		{name: "wrong type", raw: `{"priority":"high"}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resp, body := requestRawTaskJSON(t, server, http.MethodPatch, path, testCase.raw, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
	stored := requireTaskByID(t, db, task.ID, false)
	if stored.Title != "Original" || len(stored.Labels) != 1 || stored.Labels[0] != "keep" {
		t.Fatalf("invalid PATCH mutated task: %#v", stored)
	}
}

func TestTaskStatusTransitionsAndPreservesSameStatusTimestamp(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-status@example.com", "Status Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Status Project", UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Transitions", Status: store.TaskStatusTodo, UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	path := taskPath(project.ID, task.ID)

	done := patchTaskStatus(t, server, path, cookie, store.TaskStatusDone)
	completedAt, ok := done["completed_at"].(string)
	if !ok || done["canceled_at"] != nil {
		t.Fatalf("done timestamps = %v/%v", done["completed_at"], done["canceled_at"])
	}
	sameDone := patchTaskStatus(t, server, path, cookie, store.TaskStatusDone)
	if sameDone["completed_at"] != completedAt || sameDone["canceled_at"] != nil {
		t.Fatalf("same-status timestamps = %v/%v, want %s/null", sameDone["completed_at"], sameDone["canceled_at"], completedAt)
	}

	canceled := patchTaskStatus(t, server, path, cookie, store.TaskStatusCanceled)
	if canceled["completed_at"] != nil || canceled["canceled_at"] == nil {
		t.Fatalf("canceled timestamps = %v/%v", canceled["completed_at"], canceled["canceled_at"])
	}
	inProgress := patchTaskStatus(t, server, path, cookie, store.TaskStatusInProgress)
	if inProgress["completed_at"] != nil || inProgress["canceled_at"] != nil {
		t.Fatalf("in-progress timestamps = %v/%v", inProgress["completed_at"], inProgress["canceled_at"])
	}
	todo := patchTaskStatus(t, server, path, cookie, store.TaskStatusTodo)
	if todo["completed_at"] != nil || todo["canceled_at"] != nil {
		t.Fatalf("todo timestamps = %v/%v", todo["completed_at"], todo["canceled_at"])
	}
}

func TestTaskUpdateValidatesDatesAgainstExistingAndPatchedValues(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-update-dates@example.com", "Dates Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Dates Project", UpdatedAt: now})
	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Dates", StartDate: &start, DueDate: &due, UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	path := taskPath(project.ID, task.ID)

	for name, body := range map[string]map[string]any{
		"patched start after existing due":  {"start_date": "2026-07-21"},
		"patched due before existing start": {"due_date": "2026-07-09"},
		"invalid patched date":              {"start_date": "2026-02-29"},
	} {
		t.Run(name, func(t *testing.T) {
			resp, response := patchJSON(t, server, path, body, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, response)
			}
			requireError(t, response, "invalid_request")
		})
	}

	resp, body := patchJSON(t, server, path, map[string]any{"start_date": nil, "due_date": "2026-07-09"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear/patch dates status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	data := requireTaskResponse(t, requireSuccess(t, body))
	if data["start_date"] != nil || data["due_date"] != "2026-07-09" {
		t.Fatalf("clear/patch dates = %v/%v", data["start_date"], data["due_date"])
	}
}

func TestTaskUpdateRevalidatesOnlyNewAssignees(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-update-assignee-owner@example.com", "Assignment Owner", store.UserStatusActive, now)
	assigned := insertTestUser(t, db, "task-update-assignee-current@example.com", "Current Assignee", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "task-update-assignee-outsider@example.com", "New Outsider", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Assignment Project", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assigned, now)
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Assignee: &assigned, Title: "Assigned", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	path := taskPath(project.ID, task.ID)

	leftAt := time.Now().UTC()
	if err := db.Model(&store.ConversationMember{}).
		Where("member_type = ? AND member_id = ?", store.ConversationMemberTypeUser, assigned.ID).
		Update("left_at", leftAt).Error; err != nil {
		t.Fatalf("remove current assignee access: %v", err)
	}
	resp, body := patchJSON(t, server, path, map[string]any{"title": "Still assigned"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("omitted assignee status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	assertTaskUserSummary(t, requireTaskObject(t, requireTaskResponse(t, requireSuccess(t, body))["assignee"]), assigned)

	resp, body = patchJSON(t, server, path, map[string]any{"assignee_user_id": assigned.ID}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unchanged assignee status = %d, want 200, body = %#v", resp.StatusCode, body)
	}

	resp, body = patchJSON(t, server, path, map[string]any{"assignee_user_id": outsider.ID}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("new inaccessible assignee status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")

	resp, body = patchJSON(t, server, path, map[string]any{"assignee_user_id": owner.ID}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new valid assignee status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	assertTaskUserSummary(t, requireTaskObject(t, requireTaskResponse(t, requireSuccess(t, body))["assignee"]), owner)
}

func TestTaskUpdateAndDeleteAllowDerivedMembers(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-member-mutation-owner@example.com", "Member Mutation Owner", store.UserStatusActive, now)
	member := insertTestUser(t, db, "task-member-mutation-member@example.com", "Member Mutator", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Member Mutation", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, member, now)
	updateTask := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Update by member", UpdatedAt: now})
	deleteTask := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Delete by member", UpdatedAt: now})
	cookie := loginAsUser(t, server, member.Email)

	resp, body := patchJSON(t, server, taskPath(project.ID, updateTask.ID), map[string]any{"title": "Member updated"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member update status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if requireTaskResponse(t, requireSuccess(t, body))["title"] != "Member updated" {
		t.Fatalf("member update body = %#v", body)
	}
	resp, body = requestJSON(t, server, http.MethodDelete, taskPath(project.ID, deleteTask.ID), map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member delete status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if requireSuccess(t, body)["task_id"] != deleteTask.ID {
		t.Fatalf("delete body = %#v", body)
	}
}

func TestTaskDeleteSoftDeletesUpdatesActivityAndScopesToProject(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "task-delete@example.com", "Delete Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Delete Project", UpdatedAt: now})
	otherProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Delete Other", UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Delete me", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := requestJSON(t, server, http.MethodDelete, taskPath(otherProject.ID, task.ID), map[string]any{}, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("mismatch delete status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
	requireTaskByID(t, db, task.ID, false)

	resp, body = requestJSON(t, server, http.MethodDelete, taskPath(project.ID, task.ID), map[string]any{}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	if requireSuccess(t, body)["task_id"] != task.ID {
		t.Fatalf("delete response = %#v", body)
	}
	deleted := requireTaskByID(t, db, task.ID, true)
	if !deleted.DeletedAt.Valid {
		t.Fatalf("deleted_at = %#v, want valid", deleted.DeletedAt)
	}
	projectAfter := requireProjectByID(t, db, project.ID)
	if !projectAfter.UpdatedAt.After(project.UpdatedAt) {
		t.Fatalf("project updated_at = %v, want after %v", projectAfter.UpdatedAt, project.UpdatedAt)
	}
	resp, body = getJSON(t, server, taskPath(project.ID, task.ID), cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted detail status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
}

func TestTaskUpdateNoopDoesNotTouchTaskOrProjectActivity(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	owner := insertTestUser(t, db, "task-update-noop@example.com", "Noop Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Noop Project", UpdatedAt: now})
	task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Noop", Labels: []string{}, UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)

	for name, patch := range map[string]map[string]any{
		"empty":       {},
		"same values": {"title": task.Title, "status": task.Status, "priority": task.Priority, "labels": []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := patchJSON(t, server, taskPath(project.ID, task.ID), patch, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
			}
		})
	}
	stored := requireTaskByID(t, db, task.ID, false)
	projectAfter := requireProjectByID(t, db, project.ID)
	if !stored.UpdatedAt.Equal(task.UpdatedAt) || !projectAfter.UpdatedAt.Equal(project.UpdatedAt) {
		t.Fatalf("noop timestamps task/project = %v/%v, want %v/%v", stored.UpdatedAt, projectAfter.UpdatedAt, task.UpdatedAt, project.UpdatedAt)
	}
}

func TestTaskUpdateMutationsLockProjectAndTaskRows(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-mutation-locks@example.com", "Task Locks Owner", store.UserStatusActive, now)
	createProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Create Lock", UpdatedAt: now})
	updateProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Update Lock", UpdatedAt: now})
	deleteProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Delete Lock", UpdatedAt: now})
	updateTask := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: updateProject.ID, Creator: owner, Title: "Update Lock", UpdatedAt: now})
	deleteTask := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: deleteProject.ID, Creator: owner, Title: "Delete Lock", UpdatedAt: now})
	targets := map[string]struct{}{createProject.ID: {}, updateProject.ID: {}, deleteProject.ID: {}, updateTask.ID: {}, deleteTask.ID: {}}
	recorder := registerTaskQueryLockRecorder(t, db, "test:task_mutation_locks", targets)
	cookie := loginAsUser(t, server, owner.Email)

	requests := []struct {
		method string
		path   string
		body   map[string]any
		status int
	}{
		{method: http.MethodPost, path: "/api/client/projects/" + createProject.ID + "/tasks", body: map[string]any{"title": "Create Locked"}, status: http.StatusCreated},
		{method: http.MethodPatch, path: taskPath(updateProject.ID, updateTask.ID), body: map[string]any{"title": "Updated"}, status: http.StatusOK},
		{method: http.MethodDelete, path: taskPath(deleteProject.ID, deleteTask.ID), body: map[string]any{}, status: http.StatusOK},
	}
	for _, request := range requests {
		resp, body := requestJSON(t, server, request.method, request.path, request.body, cookie)
		if resp.StatusCode != request.status {
			t.Fatalf("%s %s status = %d, want %d, body = %#v", request.method, request.path, resp.StatusCode, request.status, body)
		}
	}

	records := recorder.snapshot()
	for _, targetID := range []string{createProject.ID, updateProject.ID, deleteProject.ID, updateTask.ID, deleteTask.ID} {
		found := false
		for _, record := range records {
			if record.ID == targetID && record.Locked && record.InTransaction {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("target %s had no FOR UPDATE query inside transaction; records = %#v", targetID, records)
		}
	}
}

func TestTaskUpdateAndDeleteLifecycleMissesReturnNotFoundAndRollback(t *testing.T) {
	for _, operation := range []struct {
		name   string
		method string
		body   map[string]any
		zero   func(*testing.T, *gorm.DB, string)
	}{
		{name: "update task row", method: http.MethodPatch, body: map[string]any{"title": "Changed"}, zero: func(t *testing.T, db *gorm.DB, name string) {
			registerTaskMutationZeroRowsCallback(t, db, name, false)
		}},
		{name: "delete task row", method: http.MethodDelete, body: map[string]any{}, zero: func(t *testing.T, db *gorm.DB, name string) {
			registerTaskMutationZeroRowsCallback(t, db, name, true)
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()
			now := time.Now().UTC().Add(-time.Hour)
			owner := insertTestUser(t, db, "task-lifecycle-"+strings.ReplaceAll(operation.name, " ", "-")+"@example.com", "Lifecycle Owner", store.UserStatusActive, now)
			project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Lifecycle Project", UpdatedAt: now})
			task := insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Original", UpdatedAt: now})
			cookie := loginAsUser(t, server, owner.Email)
			operation.zero(t, db, "test:task_lifecycle_"+strings.ReplaceAll(operation.name, " ", "_"))

			resp, body := requestJSON(t, server, operation.method, taskPath(project.ID, task.ID), operation.body, cookie)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "not_found")
			stored := requireTaskByID(t, db, task.ID, false)
			if stored.Title != "Original" || stored.DeletedAt.Valid {
				t.Fatalf("task lifecycle mutation was not rolled back: %#v", stored)
			}
			projectAfter := requireProjectByID(t, db, project.ID)
			if !projectAfter.UpdatedAt.Equal(project.UpdatedAt) {
				t.Fatalf("project activity changed on task lifecycle miss: %v != %v", projectAfter.UpdatedAt, project.UpdatedAt)
			}
		})
	}

	for _, operation := range []struct {
		name       string
		method     string
		body       map[string]any
		createTask bool
	}{
		{name: "create project activity", method: http.MethodPost, body: map[string]any{"title": "Rolled back create"}},
		{name: "update project activity", method: http.MethodPatch, body: map[string]any{"title": "Rolled back update"}, createTask: true},
		{name: "delete project activity", method: http.MethodDelete, body: map[string]any{}, createTask: true},
	} {
		t.Run(operation.name, func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()
			now := time.Now().UTC().Add(-time.Hour)
			owner := insertTestUser(t, db, "task-project-lifecycle-"+strings.ReplaceAll(operation.name, " ", "-")+"@example.com", "Project Lifecycle Owner", store.UserStatusActive, now)
			project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Project Activity Miss", UpdatedAt: now})
			path := "/api/client/projects/" + project.ID + "/tasks"
			var task store.Task
			if operation.createTask {
				task = insertTaskTestFixture(t, db, taskFixtureInput{ProjectID: project.ID, Creator: owner, Title: "Original", UpdatedAt: now})
				path = taskPath(project.ID, task.ID)
			}
			cookie := loginAsUser(t, server, owner.Email)
			registerTaskProjectActivityZeroRowsCallback(t, db, "test:"+strings.ReplaceAll(operation.name, " ", "_"))

			resp, body := requestJSON(t, server, operation.method, path, operation.body, cookie)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "not_found")
			if operation.createTask {
				stored := requireTaskByID(t, db, task.ID, false)
				if stored.Title != "Original" || stored.DeletedAt.Valid {
					t.Fatalf("task mutation was not rolled back: %#v", stored)
				}
			} else {
				requireRowCount(t, db.Unscoped(), &store.Task{}, 0, "project_id = ?", project.ID)
			}
		})
	}
}

func TestTaskListFiltersEveryFieldAndCombinesCriteria(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-list-filters-owner@example.com", "List Owner", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-list-filters-assignee@example.com", "List Assignee", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "List Filters", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	date := func(day int) *time.Time {
		value := time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC)
		return &value
	}
	matching := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Assignee: &assignee, Title: "Release API", Description: "Markdown launch plan",
		Status: store.TaskStatusInProgress, Priority: store.TaskPriorityHigh, StartDate: date(10), DueDate: date(20),
		Labels: []string{"Backend", "urgent"}, UpdatedAt: now.Add(4 * time.Minute),
	})
	todo := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Title: "Release notes", Description: "Writer review",
		Status: store.TaskStatusTodo, Priority: store.TaskPriorityLow, StartDate: date(5), DueDate: date(15),
		Labels: []string{"BackendAPI"}, UpdatedAt: now.Add(3 * time.Minute),
	})
	done := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Title: "Documentation", Description: "MARKDOWN reference",
		Status: store.TaskStatusDone, Priority: store.TaskPriorityMedium, StartDate: date(15), DueDate: date(25),
		Labels: []string{"Frontend"}, UpdatedAt: now.Add(2 * time.Minute),
	})
	canceled := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Title: "Old operation", Description: "No match",
		Status: store.TaskStatusCanceled, Priority: store.TaskPriorityMedium, StartDate: date(12), DueDate: date(22),
		Labels: []string{"ops"}, UpdatedAt: now.Add(time.Minute),
	})
	controlLabel := insertTaskTestFixture(t, db, taskFixtureInput{
		ProjectID: project.ID, Creator: owner, Title: "Control label", Status: store.TaskStatusCanceled,
		Priority: store.TaskPriorityMedium, Labels: []string{"Line\nBreak"}, UpdatedAt: now,
	})
	cookie := loginAsUser(t, server, owner.Email)
	base := "/api/client/projects/" + project.ID + "/tasks?"

	for _, testCase := range []struct {
		name  string
		query string
		want  []string
	}{
		{name: "keyword title case insensitive", query: "keyword=" + url.QueryEscape("rElEaSe"), want: []string{matching.ID, todo.ID}},
		{name: "keyword description case insensitive", query: "keyword=" + url.QueryEscape("markdown"), want: []string{matching.ID, done.ID}},
		{name: "multiple statuses", query: "status=todo,done", want: []string{todo.ID, done.ID}},
		{name: "multiple priorities", query: "priority=1,3", want: []string{matching.ID, todo.ID}},
		{name: "assignee", query: "assignee_user_id=" + assignee.ID, want: []string{matching.ID}},
		{name: "exact label", query: "label=" + url.QueryEscape(" Backend "), want: []string{matching.ID}},
		{name: "label substring is not equal", query: "label=Back", want: []string{}},
		{name: "label with control character", query: "label=" + url.QueryEscape("Line\nBreak"), want: []string{controlLabel.ID}},
		{name: "start from", query: "start_date_from=2026-07-10", want: []string{matching.ID, done.ID, canceled.ID}},
		{name: "start to", query: "start_date_to=2026-07-10", want: []string{matching.ID, todo.ID}},
		{name: "due from", query: "due_date_from=2026-07-20", want: []string{matching.ID, done.ID, canceled.ID}},
		{name: "due to", query: "due_date_to=2026-07-20", want: []string{matching.ID, todo.ID}},
		{
			name: "combined",
			query: "keyword=launch&status=in_progress,done&priority=3&assignee_user_id=" + assignee.ID +
				"&label=Backend&start_date_from=2026-07-10&start_date_to=2026-07-10" +
				"&due_date_from=2026-07-20&due_date_to=2026-07-20",
			want: []string{matching.ID},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertTaskListIDs(t, server, base+testCase.query, cookie, testCase.want)
		})
	}
}

func TestTaskListRejectsInvalidFilters(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-list-invalid@example.com", "List Validation Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "List Validation", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	base := "/api/client/projects/" + project.ID + "/tasks?"
	badJSONCursor := base64.RawURLEncoding.EncodeToString([]byte(`{"updated_at":"bad","id":"bad"}`))
	unknownCursor := base64.RawURLEncoding.EncodeToString([]byte(`{"updated_at":"2026-07-11T00:00:00Z","id":"` + uuid.NewString() + `","extra":true}`))

	for name, query := range map[string]string{
		"empty status":          "status=",
		"status empty element":  "status=todo,,done",
		"invalid status":        "status=blocked",
		"empty priority":        "priority=",
		"priority empty item":   "priority=1,,3",
		"invalid priority low":  "priority=0",
		"invalid priority high": "priority=4",
		"invalid priority text": "priority=high",
		"bad assignee UUID":     "assignee_user_id=bad-id",
		"blank label":           "label=" + url.QueryEscape("   "),
		"long label":            "label=" + url.QueryEscape(strings.Repeat("界", 33)),
		"bad start from":        "start_date_from=2026-02-30",
		"bad start to":          "start_date_to=2026-07-11T00:00:00Z",
		"reversed start bounds": "start_date_from=2026-07-12&start_date_to=2026-07-11",
		"bad due from":          "due_date_from=not-a-date",
		"bad due to":            "due_date_to=2026-02-29",
		"reversed due bounds":   "due_date_from=2026-07-12&due_date_to=2026-07-11",
		"zero limit":            "limit=0",
		"too large limit":       "limit=101",
		"noninteger limit":      "limit=many",
		"bad base64 cursor":     "cursor=not-a-cursor",
		"bad cursor fields":     "cursor=" + url.QueryEscape(badJSONCursor),
		"unknown cursor field":  "cursor=" + url.QueryEscape(unknownCursor),
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := getJSON(t, server, base+query, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
}

func TestTaskListOrdersAndPaginatesTiedTimestamps(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Date(2026, 7, 11, 1, 2, 3, 456789000, time.UTC)
	owner := insertTestUser(t, db, "task-list-page@example.com", "Page Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Page Project", UpdatedAt: now})
	newest := insertTaskTestFixture(t, db, taskFixtureInput{
		ID: "00000000-0000-0000-0000-000000000004", ProjectID: project.ID, Creator: owner, Title: "Newest", UpdatedAt: now.Add(time.Hour),
	})
	tieHigh := insertTaskTestFixture(t, db, taskFixtureInput{
		ID: "00000000-0000-0000-0000-000000000003", ProjectID: project.ID, Creator: owner, Title: "Tie high", UpdatedAt: now,
	})
	tieLow := insertTaskTestFixture(t, db, taskFixtureInput{
		ID: "00000000-0000-0000-0000-000000000002", ProjectID: project.ID, Creator: owner, Title: "Tie low", UpdatedAt: now,
	})
	older := insertTaskTestFixture(t, db, taskFixtureInput{
		ID: "00000000-0000-0000-0000-000000000001", ProjectID: project.ID, Creator: owner, Title: "Older", UpdatedAt: now.Add(-time.Hour),
	})
	deleted := insertTaskTestFixture(t, db, taskFixtureInput{
		ID: "00000000-0000-0000-0000-000000000005", ProjectID: project.ID, Creator: owner, Title: "Deleted newest", UpdatedAt: now.Add(2 * time.Hour),
	})
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("soft-delete page fixture: %v", err)
	}
	cookie := loginAsUser(t, server, owner.Email)
	base := "/api/client/projects/" + project.ID + "/tasks"

	resp, body := getJSON(t, server, base+"?limit=2", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	first, cursor := requireTaskListResponse(t, requireSuccess(t, body))
	assertTaskResponseIDs(t, first, []string{newest.ID, tieHigh.ID})
	if cursor == nil {
		t.Fatal("first page next_cursor = null, want value")
	}
	rawCursor, err := base64.RawURLEncoding.DecodeString(*cursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	var cursorFields map[string]string
	if err := json.Unmarshal(rawCursor, &cursorFields); err != nil {
		t.Fatalf("unmarshal next cursor: %v", err)
	}
	if cursorFields["id"] != tieHigh.ID || cursorFields["updated_at"] != tieHigh.UpdatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("cursor = %#v, want tie-high position", cursorFields)
	}

	resp, body = getJSON(t, server, base+"?limit=2&cursor="+url.QueryEscape(*cursor), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second page status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	second, next := requireTaskListResponse(t, requireSuccess(t, body))
	assertTaskResponseIDs(t, second, []string{tieLow.ID, older.ID})
	if next != nil {
		t.Fatalf("second page next_cursor = %q, want null", *next)
	}
}

func TestTaskListUsesDefaultLimitFifty(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-list-default-limit@example.com", "Default Limit Owner", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Default Limit", UpdatedAt: now})
	for index := 0; index < 51; index++ {
		insertTaskTestFixture(t, db, taskFixtureInput{
			ProjectID: project.ID, Creator: owner, Title: "Task " + string(rune('A'+index)), UpdatedAt: now.Add(time.Duration(index) * time.Second),
		})
	}
	cookie := loginAsUser(t, server, owner.Email)
	resp, body := getJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	tasks, cursor := requireTaskListResponse(t, requireSuccess(t, body))
	if len(tasks) != 50 || cursor == nil {
		t.Fatalf("default page length/cursor = %d/%v, want 50/non-null", len(tasks), cursor)
	}
}

func TestTaskListReturnsEmptyArrayAndHidesInaccessibleProject(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "task-list-empty-owner@example.com", "Empty Owner", store.UserStatusActive, now)
	outsider := insertTestUser(t, db, "task-list-empty-outsider@example.com", "Empty Outsider", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Empty Project", UpdatedAt: now})
	ownerCookie := loginAsUser(t, server, owner.Email)
	path := "/api/client/projects/" + project.ID + "/tasks"

	resp, body := getJSON(t, server, path+"?keyword=nothing", ownerCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	tasks, cursor := requireTaskListResponse(t, requireSuccess(t, body))
	if len(tasks) != 0 || tasks == nil || cursor != nil {
		t.Fatalf("empty list = %#v, cursor = %#v", tasks, cursor)
	}

	outsiderCookie := loginAsUser(t, server, outsider.Email)
	resp, body = getJSON(t, server, path, outsiderCookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("inaccessible status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
}

func grantTaskProjectAccess(t *testing.T, db *gorm.DB, project store.Project, owner store.User, member store.User, now time.Time) {
	t.Helper()

	group := insertProjectConversationFixture(t, db, projectConversationFixtureInput{
		Creator: owner,
		Kind:    store.ConversationKindGroup,
		Status:  store.ConversationStatusActive,
		Name:    "Task Access Group",
		Now:     now,
		Members: []store.ConversationMember{{
			MemberType: store.ConversationMemberTypeUser,
			MemberID:   member.ID,
		}},
	})
	insertProjectGroupFixture(t, db, project.ID, group.ID, owner.ID, now)
}

func requestRawTaskJSON(t *testing.T, server *httptest.Server, method string, path string, raw string, cookie *http.Cookie) (*http.Response, map[string]any) {
	t.Helper()

	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(raw))
	if err != nil {
		t.Fatalf("create raw task request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("send raw task request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode raw task response: %v", err)
	}
	return resp, body
}

func mustTaskJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal task JSON: %v", err)
	}
	return string(raw)
}

func requireTaskResponse(t *testing.T, value map[string]any) map[string]any {
	t.Helper()

	for _, field := range []string{
		"id", "project_id", "title", "description", "status", "priority", "assignee", "creator",
		"start_date", "due_date", "labels", "completed_at", "canceled_at", "created_at", "updated_at",
	} {
		if _, exists := value[field]; !exists {
			t.Fatalf("task response missing %s: %#v", field, value)
		}
	}
	return value
}

func requireTaskObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want object", value)
	}
	return object
}

func assertTaskUserSummary(t *testing.T, summary map[string]any, user store.User) {
	t.Helper()
	if summary["id"] != user.ID || summary["name"] != user.Name || summary["nickname"] != user.Nickname || summary["avatar"] != user.Avatar {
		t.Fatalf("user summary = %#v, want id/name/nickname/avatar from %#v", summary, user)
	}
	if len(summary) != 4 {
		t.Fatalf("user summary fields = %#v, want exactly id/name/nickname/avatar", summary)
	}
}

func assertTaskStringList(t *testing.T, value any, want []string) {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", value)
	}
	if len(items) != len(want) {
		t.Fatalf("array = %#v, want %#v", items, want)
	}
	for index, item := range items {
		if item != want[index] {
			t.Fatalf("array = %#v, want %#v", items, want)
		}
	}
}

func assertTaskTimestampPresence(t *testing.T, task map[string]any, field string, want bool, lowerBound time.Time) {
	t.Helper()
	value := task[field]
	if !want {
		if value != nil {
			t.Fatalf("%s = %#v, want null", field, value)
		}
		return
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("%s = %#v, want timestamp string", field, value)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		t.Fatalf("%s = %q, want RFC3339 timestamp: %v", field, text, err)
	}
	if parsed.Before(lowerBound) {
		t.Fatalf("%s = %v, want at or after %v", field, parsed, lowerBound)
	}
}

func requireTaskByID(t *testing.T, db *gorm.DB, taskID string, unscoped bool) store.Task {
	t.Helper()
	query := db
	if unscoped {
		query = query.Unscoped()
	}
	var task store.Task
	if err := query.First(&task, "id = ?", taskID).Error; err != nil {
		t.Fatalf("find task %s: %v", taskID, err)
	}
	return task
}

type taskFixtureInput struct {
	ID          string
	ProjectID   string
	Creator     store.User
	Assignee    *store.User
	Title       string
	Description string
	Status      string
	Priority    int16
	StartDate   *time.Time
	DueDate     *time.Time
	Labels      []string
	CompletedAt *time.Time
	CanceledAt  *time.Time
	UpdatedAt   time.Time
}

func insertTaskTestFixture(t *testing.T, db *gorm.DB, input taskFixtureInput) store.Task {
	t.Helper()
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Title == "" {
		input.Title = "Task fixture"
	}
	if input.Status == "" {
		input.Status = store.TaskStatusTodo
	}
	if input.Priority == 0 {
		input.Priority = store.TaskPriorityMedium
	}
	if input.UpdatedAt.IsZero() {
		input.UpdatedAt = time.Now().UTC()
	}
	labels := make(pq.StringArray, len(input.Labels))
	copy(labels, input.Labels)
	task := store.Task{
		ID:              input.ID,
		ProjectID:       input.ProjectID,
		Title:           input.Title,
		Description:     input.Description,
		Status:          input.Status,
		Priority:        input.Priority,
		StartDate:       input.StartDate,
		DueDate:         input.DueDate,
		Labels:          labels,
		CreatedByUserID: input.Creator.ID,
		CompletedAt:     input.CompletedAt,
		CanceledAt:      input.CanceledAt,
		CreatedAt:       input.UpdatedAt.Add(-time.Minute),
		UpdatedAt:       input.UpdatedAt,
	}
	if input.Assignee != nil {
		task.AssigneeUserID = &input.Assignee.ID
	}
	if err := db.Select("*").Create(&task).Error; err != nil {
		t.Fatalf("create task fixture: %v", err)
	}
	return task
}

func taskPath(projectID string, taskID string) string {
	return "/api/client/projects/" + projectID + "/tasks/" + taskID
}

func patchTaskStatus(t *testing.T, server *httptest.Server, path string, cookie *http.Cookie, status string) map[string]any {
	t.Helper()
	resp, body := patchJSON(t, server, path, map[string]any{"status": status}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status %s = %d, want 200, body = %#v", status, resp.StatusCode, body)
	}
	return requireTaskResponse(t, requireSuccess(t, body))
}

type taskQueryLockRecord struct {
	Table         string
	ID            string
	Locked        bool
	InTransaction bool
}

type taskQueryLockRecorder struct {
	mu      sync.Mutex
	records []taskQueryLockRecord
}

func (recorder *taskQueryLockRecorder) snapshot() []taskQueryLockRecord {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]taskQueryLockRecord(nil), recorder.records...)
}

func registerTaskQueryLockRecorder(t *testing.T, db *gorm.DB, callbackName string, targetIDs map[string]struct{}) *taskQueryLockRecorder {
	t.Helper()
	recorder := &taskQueryLockRecorder{}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || (tx.Statement.Schema.Table != "projects" && tx.Statement.Schema.Table != "tasks") {
			return
		}
		id := ""
		for _, variable := range tx.Statement.Vars {
			candidate, ok := variable.(string)
			if !ok {
				continue
			}
			if _, exists := targetIDs[candidate]; exists {
				id = candidate
				break
			}
		}
		if id == "" {
			return
		}
		locking, hasLock := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		_, inTransaction := tx.Statement.ConnPool.(*sql.Tx)
		recorder.mu.Lock()
		recorder.records = append(recorder.records, taskQueryLockRecord{
			Table: tx.Statement.Schema.Table, ID: id,
			Locked: hasLock && locking.Strength == "UPDATE", InTransaction: inTransaction,
		})
		recorder.mu.Unlock()
	}); err != nil {
		t.Fatalf("register task query lock recorder: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove task query lock recorder: %v", err)
		}
	})
	return recorder
}

func registerTaskMutationZeroRowsCallback(t *testing.T, db *gorm.DB, callbackName string, deleteOperation bool) {
	t.Helper()
	callback := func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "tasks" {
			return
		}
		tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1 = 0"}}})
	}
	if deleteOperation {
		if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, callback); err != nil {
			t.Fatalf("register task delete zero-rows callback: %v", err)
		}
		t.Cleanup(func() {
			if err := db.Callback().Delete().Remove(callbackName); err != nil {
				t.Errorf("remove task delete zero-rows callback: %v", err)
			}
		})
		return
	}
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, callback); err != nil {
		t.Fatalf("register task update zero-rows callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Update().Remove(callbackName); err != nil {
			t.Errorf("remove task update zero-rows callback: %v", err)
		}
	})
}

func registerTaskProjectActivityZeroRowsCallback(t *testing.T, db *gorm.DB, callbackName string) {
	t.Helper()
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "projects" {
			return
		}
		tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1 = 0"}}})
	}); err != nil {
		t.Fatalf("register task project activity zero-rows callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Update().Remove(callbackName); err != nil {
			t.Errorf("remove task project activity zero-rows callback: %v", err)
		}
	})
}

func assertTaskListIDs(t *testing.T, server *httptest.Server, path string, cookie *http.Cookie, want []string) {
	t.Helper()
	resp, body := getJSON(t, server, path, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	tasks, _ := requireTaskListResponse(t, requireSuccess(t, body))
	assertTaskResponseIDs(t, tasks, want)
}

func requireTaskListResponse(t *testing.T, data map[string]any) ([]map[string]any, *string) {
	t.Helper()
	rawTasks, ok := data["tasks"].([]any)
	if !ok {
		t.Fatalf("tasks = %#v, want array", data["tasks"])
	}
	tasks := make([]map[string]any, 0, len(rawTasks))
	for _, rawTask := range rawTasks {
		tasks = append(tasks, requireTaskResponse(t, requireTaskObject(t, rawTask)))
	}
	value, exists := data["next_cursor"]
	if !exists {
		t.Fatalf("list response missing next_cursor: %#v", data)
	}
	if value == nil {
		return tasks, nil
	}
	cursor, ok := value.(string)
	if !ok {
		t.Fatalf("next_cursor = %#v, want string or null", value)
	}
	return tasks, &cursor
}

func assertTaskResponseIDs(t *testing.T, tasks []map[string]any, want []string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("task count = %d, want %d; tasks = %#v", len(tasks), len(want), tasks)
	}
	for index, task := range tasks {
		if task["id"] != want[index] {
			t.Fatalf("task IDs at %d = %v, want %s; tasks = %#v", index, task["id"], want[index], tasks)
		}
	}
}
