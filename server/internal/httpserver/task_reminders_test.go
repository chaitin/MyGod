package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	messageapp "app/internal/application/message"
	"app/internal/store"
)

func TestHTTPTaskReminderCreateUpdateAndClear(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	owner := insertTestUser(t, db, "task-reminder-owner@example.com", "提醒创建人", store.UserStatusActive, now)
	assignee := insertTestUser(t, db, "task-reminder-assignee@example.com", "提醒负责人", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "提醒项目", UpdatedAt: now})
	grantTaskProjectAccess(t, db, project, owner, assignee, now)
	cookie := loginAsUser(t, server, owner.Email)

	resp, body := postJSON(t, server, "/api/client/projects/"+project.ID+"/tasks", map[string]any{
		"title": "检查发布状态", "assignee_user_id": assignee.ID,
		"reminder": map[string]any{
			"mode": "once", "timezone": "Asia/Singapore", "at": now.Add(time.Hour).Format(time.RFC3339),
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, body = %#v", resp.StatusCode, body)
	}
	task := requireTaskResponse(t, requireSuccess(t, body))
	reminder, ok := task["reminder"].(map[string]any)
	if !ok || reminder["mode"] != "once" || reminder["state"] != "scheduled" || reminder["next_trigger_at"] == nil {
		t.Fatalf("created reminder = %#v", task["reminder"])
	}
	if reminder["timezone"] != "Asia/Shanghai" {
		t.Fatalf("created reminder timezone = %#v", reminder["timezone"])
	}
	taskID := task["id"].(string)
	if count := len(loadTaskNotificationMessages(t, db, assignee.ID)); count != 1 {
		t.Fatalf("creation notification count = %d, want 1", count)
	}

	resp, body = patchJSON(t, server, taskPath(project.ID, taskID), map[string]any{
		"reminder": map[string]any{
			"mode": "recurring", "frequency": "weekly", "timezone": "Asia/Singapore",
			"weekdays": []int{1, 3, 5}, "time": "09:30",
		},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, body = %#v", resp.StatusCode, body)
	}
	updatedReminder := requireTaskResponse(t, requireSuccess(t, body))["reminder"].(map[string]any)
	if updatedReminder["frequency"] != "weekly" || updatedReminder["timezone"] != "Asia/Shanghai" {
		t.Fatalf("updated reminder = %#v", updatedReminder)
	}
	if count := len(loadTaskNotificationMessages(t, db, assignee.ID)); count != 1 {
		t.Fatalf("reminder-only update sent immediate card, count = %d", count)
	}

	resp, body = patchJSON(t, server, taskPath(project.ID, taskID), map[string]any{"reminder": nil}, cookie)
	if resp.StatusCode != http.StatusOK || requireTaskResponse(t, requireSuccess(t, body))["reminder"] != nil {
		t.Fatalf("clear status = %d, body = %#v", resp.StatusCode, body)
	}
}

func TestBuildTaskReminderBody(t *testing.T) {
	body, summary, err := buildTaskReminderBody(t.Context(), messageapp.TaskReminderNotificationCommand{
		ID: "task-1", ProjectID: "project-1", Title: "检查发布状态", Description: "查看监控指标",
		OccurrenceAt: time.Date(2026, 7, 15, 1, 30, 0, 0, time.UTC), Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	var card cardMessageBody
	if err := json.Unmarshal(body, &card); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if card.Title != "任务提醒 - 检查发布状态" || card.Description != "提醒时间：2026 年 7 月 15 日 09:30\n查看监控指标" || card.URL != "/projects/project-1?taskId=task-1" {
		t.Fatalf("card = %#v", card)
	}
	if summary == "" {
		t.Fatal("summary is empty")
	}
}
