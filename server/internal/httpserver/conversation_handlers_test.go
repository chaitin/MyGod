package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	conversationapp "app/internal/application/conversation"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestNewConversationListItemResponseUsesDirectFallbackCopy(t *testing.T) {
	currentUserID := "user-1"

	response := newConversationListItemResponse(
		store.Conversation{
			ID:   "conversation-1",
			Kind: store.ConversationKindDirect,
		},
		currentUserID,
		[]store.ConversationMember{
			{
				ConversationID: "conversation-1",
				MemberType:     store.ConversationMemberTypeUser,
				MemberID:       currentUserID,
				Role:           store.ConversationMemberRoleOwner,
			},
		},
		map[string]store.User{},
		nil,
	)

	if response.Name != "私聊" {
		t.Fatalf("response.Name = %q, want %q", response.Name, "私聊")
	}
}

func TestCreateGroupConversationLinksOwnedProjects(t *testing.T) {
	for _, projectCount := range []int{1, 2} {
		t.Run(string(rune('0'+projectCount))+" projects", func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()

			oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
			owner := insertTestUser(t, db, "group-project-owner@example.com", "Group Project Owner", store.UserStatusActive, oldUpdatedAt)
			member := insertTestUser(t, db, "group-project-member@example.com", "Group Project Member", store.UserStatusActive, oldUpdatedAt)
			projects := make([]store.Project, 0, projectCount)
			projectIDs := make([]string, 0, projectCount)
			for index := range projectCount {
				project := insertProjectFixture(t, db, projectFixtureInput{
					Owner:     owner,
					Name:      "Linked Project " + string(rune('A'+index)),
					UpdatedAt: oldUpdatedAt,
				})
				projects = append(projects, project)
				projectIDs = append(projectIDs, project.ID)
			}

			resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
				"name":        "Project Group",
				"member_ids":  []string{member.ID},
				"project_ids": projectIDs,
			}, loginAsUser(t, server, owner.Email))
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
			}
			conversationID := requireSuccess(t, body)["conversation"].(map[string]any)["id"].(string)

			var links []store.ProjectGroup
			if err := db.Where("conversation_id = ?", conversationID).Order("project_id ASC").Find(&links).Error; err != nil {
				t.Fatalf("find project group links: %v", err)
			}
			if len(links) != len(projects) {
				t.Fatalf("project group link count = %d, want %d", len(links), len(projects))
			}
			linksByProjectID := make(map[string]store.ProjectGroup, len(links))
			for _, link := range links {
				linksByProjectID[link.ProjectID] = link
				if link.LinkedByUserID != owner.ID {
					t.Fatalf("link %s linked_by_user_id = %s, want %s", link.ProjectID, link.LinkedByUserID, owner.ID)
				}
			}
			for _, project := range projects {
				link, ok := linksByProjectID[project.ID]
				if !ok {
					t.Fatalf("missing link for project %s", project.ID)
				}
				storedProject := requireProjectByID(t, db, project.ID)
				if !storedProject.UpdatedAt.Equal(link.CreatedAt) {
					t.Fatalf("project %s updated_at = %v, want relation time %v", project.ID, storedProject.UpdatedAt, link.CreatedAt)
				}
				if !storedProject.UpdatedAt.After(oldUpdatedAt) {
					t.Fatalf("project %s updated_at = %v, want after %v", project.ID, storedProject.UpdatedAt, oldUpdatedAt)
				}
			}
			requireRowCount(t, db, &store.ConversationMember{}, 2, "conversation_id = ?", conversationID)
			requireRowCount(t, db, &store.Message{}, 1, "conversation_id = ?", conversationID)
		})
	}
}

func TestCreateGroupConversationProjectIDsDeduplicateCanonicalUUIDs(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "group-project-dedupe@example.com", "Group Project Dedupe", store.UserStatusActive, now)
	project := insertProjectFixture(t, db, projectFixtureInput{
		ID:        "00000000-0000-0000-0000-0000000000ab",
		Owner:     owner,
		Name:      "Dedupe Project",
		UpdatedAt: now,
	})

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name": "Dedupe Group",
		"project_ids": []string{
			project.ID,
			"  " + strings.ToUpper(project.ID) + "  ",
			project.ID,
		},
	}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}
	conversationID := requireSuccess(t, body)["conversation"].(map[string]any)["id"].(string)
	requireRowCount(t, db, &store.ProjectGroup{}, 1, "project_id = ? AND conversation_id = ?", project.ID, conversationID)
}

func TestCreateGroupConversationRejectsRawProjectIDsOverLimit(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "group-project-limit@example.com", "Group Project Limit", store.UserStatusActive, now)
	projectID := uuid.NewString()
	projectIDs := make([]string, 101)
	for index := range projectIDs {
		projectIDs[index] = projectID
	}

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Too Many Raw Projects",
		"project_ids": projectIDs,
	}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
	requireNoGroupCreationWrites(t, db)
}

func TestCreateGroupConversationProjectQueryCountIsConstant(t *testing.T) {
	for _, projectCount := range []int{1, 5} {
		t.Run(string(rune('0'+projectCount))+" projects", func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()

			now := time.Now().UTC().Add(-time.Hour)
			owner := insertTestUser(t, db, "group-project-query-count@example.com", "Group Project Query Count", store.UserStatusActive, now)
			projectIDs := make([]string, 0, projectCount)
			for index := range projectCount {
				project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Counted Project " + string(rune('A'+index)), UpdatedAt: now})
				projectIDs = append(projectIDs, project.ID)
			}
			cookie := loginAsUser(t, server, owner.Email)
			counter := registerGroupProjectStatementCounter(t, db, "test:count_group_create_project_statements")

			resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
				"name":        "Constant Project Work",
				"project_ids": projectIDs,
			}, cookie)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
			}
			if got := counter.queries.Load(); got != 1 {
				t.Fatalf("project SELECT count = %d, want 1", got)
			}
			if got := counter.updates.Load(); got != 1 {
				t.Fatalf("project UPDATE count = %d, want 1", got)
			}
		})
	}
}

func TestCreateGroupConversationProjectIDsAllowOmittedAndEmpty(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "group-project-optional@example.com", "Group Project Optional", store.UserStatusActive, now)
	cookie := loginAsUser(t, server, owner.Email)

	for name, requestBody := range map[string]map[string]any{
		"omitted": {"name": "No Projects Omitted"},
		"empty":   {"name": "No Projects Empty", "project_ids": []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := postJSON(t, server, "/api/client/conversations/groups", requestBody, cookie)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
			}
			conversationID := requireSuccess(t, body)["conversation"].(map[string]any)["id"].(string)
			requireRowCount(t, db, &store.ProjectGroup{}, 0, "conversation_id = ?", conversationID)
		})
	}
}

func TestCreateGroupConversationProjectIDsRejectNullNonArrayAndInvalidUUID(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "group-project-format@example.com", "Group Project Format", store.UserStatusActive, now)
	cookie := loginAsUser(t, server, owner.Email)

	for name, projectIDs := range map[string]any{
		"null":         nil,
		"non-array":    uuid.NewString(),
		"invalid UUID": []string{"not-a-project-id"},
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
				"name":        "Invalid Project IDs",
				"project_ids": projectIDs,
			}, cookie)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "invalid_request")
		})
	}
	requireNoGroupCreationWrites(t, db)
}

func TestCreateGroupConversationRejectsPersonalProject(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "group-personal-project@example.com", "Group Personal Project", store.UserStatusActive, now)
	personal := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Personal", IsPersonal: true, UpdatedAt: now})

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Personal Project Group",
		"project_ids": []string{personal.ID},
	}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "invalid_request")
	requireNoGroupCreationWrites(t, db)
	requireProjectUpdatedAt(t, db, personal.ID, now)
}

func TestCreateGroupConversationHidesUnownedMissingAndSoftDeletedProjects(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "group-hidden-project-owner@example.com", "Group Hidden Owner", store.UserStatusActive, now)
	other := insertTestUser(t, db, "group-hidden-project-other@example.com", "Group Hidden Other", store.UserStatusActive, now)
	unowned := insertProjectFixture(t, db, projectFixtureInput{Owner: other, Name: "Unowned", UpdatedAt: now})
	softDeleted := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Deleted", UpdatedAt: now})
	if err := db.Delete(&softDeleted).Error; err != nil {
		t.Fatalf("soft-delete project fixture: %v", err)
	}
	cookie := loginAsUser(t, server, owner.Email)

	for name, projectID := range map[string]string{
		"unowned":      unowned.ID,
		"missing":      uuid.NewString(),
		"soft-deleted": softDeleted.ID,
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
				"name":        "Hidden Project Group",
				"project_ids": []string{projectID},
			}, cookie)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "not_found")
		})
	}
	requireNoGroupCreationWrites(t, db)
	requireProjectUpdatedAt(t, db, unowned.ID, now)
	requireProjectUpdatedAt(t, db, softDeleted.ID, now)
}

func TestCreateGroupConversationInvalidProjectRollsBackAllWrites(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "group-invalid-project-owner@example.com", "Group Invalid Project Owner", store.UserStatusActive, oldUpdatedAt)
	member := insertTestUser(t, db, "group-invalid-project-member@example.com", "Group Invalid Project Member", store.UserStatusActive, oldUpdatedAt)
	valid := insertProjectFixture(t, db, projectFixtureInput{
		ID:        "00000000-0000-0000-0000-000000000001",
		Owner:     owner,
		Name:      "Valid Project",
		UpdatedAt: oldUpdatedAt,
	})
	missingID := "00000000-0000-0000-0000-000000000002"

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Rolled Back Group",
		"member_ids":  []string{member.ID},
		"project_ids": []string{valid.ID, missingID},
	}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "not_found")
	requireNoGroupCreationWrites(t, db)
	requireProjectUpdatedAt(t, db, valid.ID, oldUpdatedAt)
}

func TestCreateGroupConversationProjectGroupInsertFailureUsesTransactionAndRollsBack(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "group-link-failure-owner@example.com", "Group Link Failure Owner", store.UserStatusActive, oldUpdatedAt)
	member := insertTestUser(t, db, "group-link-failure-member@example.com", "Group Link Failure Member", store.UserStatusActive, oldUpdatedAt)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Link Failure", UpdatedAt: oldUpdatedAt})
	projectGroupErr := errors.New("forced project group insert failure")
	var callbackCalled atomic.Bool
	var usedTransaction atomic.Bool
	const callbackName = "test:fail_group_create_project_group"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "project_groups" {
			return
		}
		callbackCalled.Store(true)
		_, inTransaction := tx.Statement.ConnPool.(*sql.Tx)
		usedTransaction.Store(inTransaction)
		tx.AddError(projectGroupErr)
	}); err != nil {
		t.Fatalf("register project group create callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove project group create callback: %v", err)
		}
	})

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Project Link Failure",
		"member_ids":  []string{member.ID},
		"project_ids": []string{project.ID},
	}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "internal_error")
	if !callbackCalled.Load() {
		t.Fatal("project group create callback was not called")
	}
	if !usedTransaction.Load() {
		t.Fatal("project group insert did not use the group creation transaction")
	}
	requireNoGroupCreationWrites(t, db)
	requireProjectUpdatedAt(t, db, project.ID, oldUpdatedAt)
}

func TestCreateGroupConversationProjectTimestampUpdateFailureRollsBack(t *testing.T) {
	for _, zeroRows := range []bool{false, true} {
		name := "database error"
		if zeroRows {
			name = "zero rows"
		}
		t.Run(name, func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()

			oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
			owner := insertTestUser(t, db, "group-project-update-owner@example.com", "Group Project Update Owner", store.UserStatusActive, oldUpdatedAt)
			member := insertTestUser(t, db, "group-project-update-member@example.com", "Group Project Update Member", store.UserStatusActive, oldUpdatedAt)
			project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Update Failure", UpdatedAt: oldUpdatedAt})
			cookie := loginAsUser(t, server, owner.Email)
			if zeroRows {
				registerProjectZeroRowsCallback(t, db, "test:zero_group_create_project_update", false)
			} else {
				failUpdatesForTable(t, db, "test:fail_group_create_project_update", "projects", errors.New("forced project timestamp update failure"))
			}

			resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
				"name":        "Project Update Failure",
				"member_ids":  []string{member.ID},
				"project_ids": []string{project.ID},
			}, cookie)
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500, body = %#v", resp.StatusCode, body)
			}
			requireError(t, body, "internal_error")
			requireNoGroupCreationWrites(t, db)
			requireProjectUpdatedAt(t, db, project.ID, oldUpdatedAt)
		})
	}
}

func TestCreateGroupConversationCancellationRollsBackAllWrites(t *testing.T) {
	_, db := newTestRouter(t)

	oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "group-canceled-owner@example.com", "Group Canceled Owner", store.UserStatusActive, oldUpdatedAt)
	member := insertTestUser(t, db, "group-canceled-member@example.com", "Group Canceled Member", store.UserStatusActive, oldUpdatedAt)
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Canceled Project", UpdatedAt: oldUpdatedAt})
	ctx, cancel := context.WithCancel(context.Background())
	const callbackName = "test:cancel_group_creation_after_member_load"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "users" {
			cancel()
		}
	}); err != nil {
		t.Fatalf("register group creation cancellation callback: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove group creation cancellation callback: %v", err)
		}
	})

	_, err := conversationapp.NewService(conversationapp.Dependencies{DB: db}).CreateGroup(ctx, conversationapp.CreateGroupCommand{
		Actor: conversationActorFromUser(owner), Name: "Canceled Group",
		MemberIDs: []string{member.ID}, ProjectIDs: []string{project.ID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("create group error = %v, want context.Canceled", err)
	}
	requireNoGroupCreationWrites(t, db)
	requireProjectUpdatedAt(t, db, project.ID, oldUpdatedAt)
}

func TestCreateGroupConversationProjectRowsLockInSortedOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "group-project-lock-owner@example.com", "Group Project Lock Owner", store.UserStatusActive, now)
	low := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000001", Owner: owner, Name: "Low", UpdatedAt: now})
	high := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000002", Owner: owner, Name: "High", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	recorder := registerGroupLifecycleLockRecorder(t, db, "test:record_group_create_project_locks")

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Project Lock Group",
		"project_ids": []string{high.ID, low.ID},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}

	records := recorder.snapshot()
	if len(records) != 1 {
		t.Fatalf("project lock records = %#v, want one batch lock", records)
	}
	record := records[0]
	if record.Table != "projects" || len(record.IDs) != 2 || record.IDs[0] != low.ID || record.IDs[1] != high.ID || !record.Ordered || !record.InTransaction {
		t.Fatalf("project lock record = %#v, want one ordered batch FOR UPDATE in transaction for [%s %s]", record, low.ID, high.ID)
	}
}

func TestDissolveGroupConversationRemovesProjectsAndUpdatesTimestampsAtomically(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "dissolve-project-owner@example.com", "Dissolve Project Owner", store.UserStatusActive, oldUpdatedAt)
	member := insertTestUser(t, db, "dissolve-project-member@example.com", "Dissolve Project Member", store.UserStatusActive, oldUpdatedAt)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID, member.ID},
		name:            "Dissolve Linked Group",
		now:             oldUpdatedAt,
	})
	projects := []store.Project{
		insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Dissolve One", UpdatedAt: oldUpdatedAt}),
		insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Dissolve Two", UpdatedAt: oldUpdatedAt}),
	}
	for _, project := range projects {
		insertProjectGroupFixture(t, db, project.ID, conversation.ID, owner.ID, oldUpdatedAt)
	}

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, loginAsUser(t, server, owner.Email))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
	}
	requireSuccess(t, body)
	requireRowCount(t, db, &store.ProjectGroup{}, 0, "conversation_id = ?", conversation.ID)

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find dissolved conversation: %v", err)
	}
	if storedConversation.Status != store.ConversationStatusDissolved || storedConversation.DissolvedAt == nil {
		t.Fatalf("conversation status = %s dissolved_at = %v, want dissolved with timestamp", storedConversation.Status, storedConversation.DissolvedAt)
	}
	if !storedConversation.UpdatedAt.Equal(*storedConversation.DissolvedAt) {
		t.Fatalf("conversation updated_at = %v, want dissolution time %v", storedConversation.UpdatedAt, *storedConversation.DissolvedAt)
	}
	for _, project := range projects {
		storedProject := requireProjectByID(t, db, project.ID)
		if !storedProject.UpdatedAt.Equal(*storedConversation.DissolvedAt) {
			t.Fatalf("project %s updated_at = %v, want dissolution time %v", project.ID, storedProject.UpdatedAt, *storedConversation.DissolvedAt)
		}
	}
}

func TestDissolveGroupConversationProjectQueryCountIsConstant(t *testing.T) {
	for _, projectCount := range []int{1, 5} {
		t.Run(string(rune('0'+projectCount))+" projects", func(t *testing.T) {
			server, db := newTestRouter(t)
			defer server.Close()

			now := time.Now().UTC().Add(-time.Hour)
			owner := insertTestUser(t, db, "dissolve-project-query-count@example.com", "Dissolve Project Query Count", store.UserStatusActive, now)
			conversation := insertTestConversation(t, db, testConversationInput{
				createdByUserID: owner.ID,
				kind:            store.ConversationKindGroup,
				memberIDs:       []string{owner.ID},
				name:            "Dissolve Constant Project Work",
				now:             now,
			})
			for index := range projectCount {
				project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Dissolve Counted " + string(rune('A'+index)), UpdatedAt: now})
				insertProjectGroupFixture(t, db, project.ID, conversation.ID, owner.ID, now)
			}
			cookie := loginAsUser(t, server, owner.Email)
			counter := registerGroupProjectStatementCounter(t, db, "test:count_group_dissolve_project_statements")

			resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, cookie)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %#v", resp.StatusCode, body)
			}
			if got := counter.queries.Load(); got != 1 {
				t.Fatalf("project SELECT count = %d, want 1", got)
			}
			if got := counter.updates.Load(); got != 1 {
				t.Fatalf("project UPDATE count = %d, want 1", got)
			}
		})
	}
}

func TestDissolveGroupConversationUnauthorizedPreflightDoesNotLockProjects(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "dissolve-preflight-owner@example.com", "Dissolve Preflight Owner", store.UserStatusActive, now)
	member := insertTestUser(t, db, "dissolve-preflight-member@example.com", "Dissolve Preflight Member", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID, member.ID},
		name:            "Dissolve Preflight Group",
		now:             now,
	})
	project := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Preflight Project", UpdatedAt: now})
	insertProjectGroupFixture(t, db, project.ID, conversation.ID, owner.ID, now)
	cookie := loginAsUser(t, server, member.Email)
	recorder := registerGroupLifecycleLockRecorder(t, db, "test:record_unauthorized_dissolve_locks")

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "forbidden")
	if records := recorder.snapshot(); len(records) != 0 {
		t.Fatalf("unauthorized dissolution acquired locks: %#v", records)
	}
}

func TestDissolveGroupConversationRetriesOneRelationSetChange(t *testing.T) {
	_, db := newTestRouter(t)

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "dissolve-retry-owner@example.com", "Dissolve Retry Owner", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolve Retry Group",
		now:             now,
	})
	initialProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Initial Retry Project", UpdatedAt: now})
	extraProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Injected Retry Project", UpdatedAt: now})
	insertProjectGroupFixture(t, db, initialProject.ID, conversation.ID, owner.ID, now)
	changer := registerDissolveRelationSetChanger(t, db, "test:change_dissolve_relations_once", extraProject.ID, conversation.ID, owner.ID, 1)

	if _, err := conversationapp.NewService(conversationapp.Dependencies{DB: db}).Dissolve(context.Background(), conversationapp.DissolveCommand{Actor: conversationActorFromUser(owner), ConversationID: conversation.ID}); err != nil {
		t.Fatalf("dissolve group conversation after one retry: %v", err)
	}
	if got := changer.discoveries.Load(); got != 2 {
		t.Fatalf("dissolution attempts = %d, want 2", got)
	}
	if got := changer.changes.Load(); got != 1 {
		t.Fatalf("relation set changes = %d, want 1", got)
	}
	requireRowCount(t, db, &store.ProjectGroup{}, 0, "conversation_id = ?", conversation.ID)
}

func TestDissolveGroupConversationRelationSetChangeExhaustionReturnsConflict(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "dissolve-conflict-owner@example.com", "Dissolve Conflict Owner", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolve Conflict Group",
		now:             now,
	})
	initialProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Initial Conflict Project", UpdatedAt: now})
	extraProject := insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Injected Conflict Project", UpdatedAt: now})
	insertProjectGroupFixture(t, db, initialProject.ID, conversation.ID, owner.ID, now)
	cookie := loginAsUser(t, server, owner.Email)
	changer := registerDissolveRelationSetChanger(t, db, "test:always_change_dissolve_relations", extraProject.ID, conversation.ID, owner.ID, -1)

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "conflict")
	if got := changer.discoveries.Load(); got != 3 {
		t.Fatalf("dissolution attempts = %d, want 3", got)
	}
	if got := changer.changes.Load(); got != 3 {
		t.Fatalf("relation set changes = %d, want 3", got)
	}
	requireRowCount(t, db, &store.ProjectGroup{}, 1, "conversation_id = ?", conversation.ID)
}

func TestDissolveGroupConversationCanceledContextStopsBeforeQueries(t *testing.T) {
	_, db := newTestRouter(t)

	now := time.Now().UTC()
	owner := insertTestUser(t, db, "dissolve-canceled-owner@example.com", "Dissolve Canceled Owner", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolve Canceled Group",
		now:             now,
	})
	var queryCount atomic.Int32
	const callbackName = "test:count_canceled_dissolve_queries"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queryCount.Add(1)
	}); err != nil {
		t.Fatalf("register canceled query callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove canceled query callback: %v", err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := conversationapp.NewService(conversationapp.Dependencies{DB: db}).Dissolve(ctx, conversationapp.DissolveCommand{Actor: conversationActorFromUser(owner), ConversationID: conversation.ID})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dissolve error = %v, want context.Canceled", err)
	}
	if got := queryCount.Load(); got != 0 {
		t.Fatalf("queries after cancellation = %d, want 0", got)
	}
}

func TestDissolveGroupConversationProjectFailureRollsBackLinksAndStatus(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	oldUpdatedAt := time.Now().UTC().Add(-24 * time.Hour)
	owner := insertTestUser(t, db, "dissolve-project-failure@example.com", "Dissolve Project Failure", store.UserStatusActive, oldUpdatedAt)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolve Rollback Group",
		now:             oldUpdatedAt,
	})
	projects := []store.Project{
		insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Rollback One", UpdatedAt: oldUpdatedAt}),
		insertProjectFixture(t, db, projectFixtureInput{Owner: owner, Name: "Rollback Two", UpdatedAt: oldUpdatedAt}),
	}
	for _, project := range projects {
		insertProjectGroupFixture(t, db, project.ID, conversation.ID, owner.ID, oldUpdatedAt)
	}
	cookie := loginAsUser(t, server, owner.Email)
	var relationDeleteCalled atomic.Bool
	var relationDeleteUsedTransaction atomic.Bool
	const deleteCallbackName = "test:record_group_dissolve_project_group_delete"
	if err := db.Callback().Delete().After("gorm:delete").Register(deleteCallbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "project_groups" {
			return
		}
		relationDeleteCalled.Store(true)
		_, inTransaction := tx.Statement.ConnPool.(*sql.Tx)
		relationDeleteUsedTransaction.Store(inTransaction)
	}); err != nil {
		t.Fatalf("register project group delete callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Delete().Remove(deleteCallbackName); err != nil {
			t.Errorf("remove project group delete callback: %v", err)
		}
	})

	forcedErr := errors.New("forced dissolution status update failure")
	const updateCallbackName = "test:fail_group_dissolve_status_update"
	if err := db.Callback().Update().Before("gorm:update").Register(updateCallbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "conversations" {
			return
		}
		if !relationDeleteCalled.Load() {
			tx.AddError(errors.New("conversation updated before project group deletion"))
			return
		}
		tx.AddError(forcedErr)
	}); err != nil {
		t.Fatalf("register conversation update callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Update().Remove(updateCallbackName); err != nil {
			t.Errorf("remove conversation update callback: %v", err)
		}
	})

	resp, body := requestJSON(t, server, http.MethodDelete, "/api/client/conversations/groups/"+conversation.ID, map[string]any{}, cookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %#v", resp.StatusCode, body)
	}
	requireError(t, body, "internal_error")
	if !relationDeleteCalled.Load() {
		t.Fatal("project group delete callback was not called")
	}
	if !relationDeleteUsedTransaction.Load() {
		t.Fatal("project group deletion did not use the dissolution transaction")
	}
	requireRowCount(t, db, &store.ProjectGroup{}, int64(len(projects)), "conversation_id = ?", conversation.ID)

	var storedConversation store.Conversation
	if err := db.First(&storedConversation, "id = ?", conversation.ID).Error; err != nil {
		t.Fatalf("find rolled-back conversation: %v", err)
	}
	if storedConversation.Status != store.ConversationStatusActive || storedConversation.DissolvedAt != nil {
		t.Fatalf("conversation status = %s dissolved_at = %v, want active with nil timestamp", storedConversation.Status, storedConversation.DissolvedAt)
	}
	if !storedConversation.UpdatedAt.Equal(oldUpdatedAt) {
		t.Fatalf("conversation updated_at = %v, want unchanged %v", storedConversation.UpdatedAt, oldUpdatedAt)
	}
	for _, project := range projects {
		requireProjectUpdatedAt(t, db, project.ID, oldUpdatedAt)
	}
}

func TestDissolveGroupConversationProjectRowsLockBeforeConversation(t *testing.T) {
	_, db := newTestRouter(t)

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "dissolve-project-lock@example.com", "Dissolve Project Lock", store.UserStatusActive, now)
	conversation := insertTestConversation(t, db, testConversationInput{
		createdByUserID: owner.ID,
		kind:            store.ConversationKindGroup,
		memberIDs:       []string{owner.ID},
		name:            "Dissolve Lock Group",
		now:             now,
	})
	low := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000001", Owner: owner, Name: "Low", UpdatedAt: now})
	high := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000002", Owner: owner, Name: "High", UpdatedAt: now})
	insertProjectGroupFixture(t, db, high.ID, conversation.ID, owner.ID, now)
	insertProjectGroupFixture(t, db, low.ID, conversation.ID, owner.ID, now)
	recorder := registerGroupLifecycleLockRecorder(t, db, "test:record_group_dissolve_lock_order")

	if _, err := conversationapp.NewService(conversationapp.Dependencies{DB: db}).Dissolve(context.Background(), conversationapp.DissolveCommand{Actor: conversationActorFromUser(owner), ConversationID: conversation.ID}); err != nil {
		t.Fatalf("dissolve group conversation: %v", err)
	}
	records := recorder.snapshot()
	if len(records) != 2 {
		t.Fatalf("lock records = %#v, want project batch then conversation", records)
	}
	projectLock := records[0]
	if projectLock.Table != "projects" || len(projectLock.IDs) != 2 || projectLock.IDs[0] != low.ID || projectLock.IDs[1] != high.ID || !projectLock.Ordered || !projectLock.InTransaction {
		t.Fatalf("project lock = %#v, want one ordered batch FOR UPDATE in transaction", projectLock)
	}
	conversationLock := records[1]
	if conversationLock.Table != "conversations" || len(conversationLock.IDs) != 1 || conversationLock.IDs[0] != conversation.ID || !conversationLock.InTransaction {
		t.Fatalf("conversation lock = %#v, want conversation FOR UPDATE after projects", conversationLock)
	}
}

func requireNoGroupCreationWrites(t *testing.T, db *gorm.DB) {
	t.Helper()

	requireRowCount(t, db, &store.Conversation{}, 0, "1 = 1")
	requireRowCount(t, db, &store.ConversationMember{}, 0, "1 = 1")
	requireRowCount(t, db, &store.Message{}, 0, "1 = 1")
	requireRowCount(t, db, &store.ProjectGroup{}, 0, "1 = 1")
}

func requireProjectUpdatedAt(t *testing.T, db *gorm.DB, projectID string, want time.Time) {
	t.Helper()

	var project store.Project
	if err := db.Unscoped().First(&project, "id = ?", projectID).Error; err != nil {
		t.Fatalf("find project %s: %v", projectID, err)
	}
	if !project.UpdatedAt.Equal(want) {
		t.Fatalf("project %s updated_at = %v, want %v", projectID, project.UpdatedAt, want)
	}
}

type groupProjectStatementCounter struct {
	queries atomic.Int32
	updates atomic.Int32
}

func registerGroupProjectStatementCounter(t *testing.T, db *gorm.DB, callbackName string) *groupProjectStatementCounter {
	t.Helper()

	counter := &groupProjectStatementCounter{}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName+":query", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "projects" {
			counter.queries.Add(1)
		}
	}); err != nil {
		t.Fatalf("register project query counter: %v", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register(callbackName+":update", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "projects" {
			counter.updates.Add(1)
		}
	}); err != nil {
		t.Fatalf("register project update counter: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName + ":query"); err != nil {
			t.Errorf("remove project query counter: %v", err)
		}
		if err := db.Callback().Update().Remove(callbackName + ":update"); err != nil {
			t.Errorf("remove project update counter: %v", err)
		}
	})
	return counter
}

type groupLifecycleLockRecord struct {
	Table         string
	IDs           []string
	Ordered       bool
	InTransaction bool
}

type groupLifecycleLockRecorder struct {
	mu      sync.Mutex
	records []groupLifecycleLockRecord
}

func (recorder *groupLifecycleLockRecorder) snapshot() []groupLifecycleLockRecord {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]groupLifecycleLockRecord(nil), recorder.records...)
}

func registerGroupLifecycleLockRecorder(t *testing.T, db *gorm.DB, callbackName string) *groupLifecycleLockRecorder {
	t.Helper()

	recorder := &groupLifecycleLockRecorder{}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil {
			return
		}
		locking, locked := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if !locked || locking.Strength != "UPDATE" {
			return
		}
		ids := make([]string, 0, len(tx.Statement.Vars))
		for _, variable := range tx.Statement.Vars {
			value, ok := variable.(string)
			if !ok {
				continue
			}
			if _, err := uuid.Parse(value); err == nil {
				ids = append(ids, value)
			}
		}
		ordered := false
		if orderBy, ok := tx.Statement.Clauses["ORDER BY"].Expression.(clause.OrderBy); ok {
			for _, column := range orderBy.Columns {
				columnName := strings.TrimSpace(column.Column.Name)
				if (columnName == "id" || strings.EqualFold(columnName, "id ASC")) && !column.Desc {
					ordered = true
				}
			}
		}
		_, inTransaction := tx.Statement.ConnPool.(*sql.Tx)
		recorder.mu.Lock()
		recorder.records = append(recorder.records, groupLifecycleLockRecord{
			Table:         tx.Statement.Schema.Table,
			IDs:           ids,
			Ordered:       ordered,
			InTransaction: inTransaction,
		})
		recorder.mu.Unlock()
	}); err != nil {
		t.Fatalf("register group lifecycle lock recorder: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove group lifecycle lock recorder: %v", err)
		}
	})
	return recorder
}

type dissolveRelationSetChanger struct {
	discoveries atomic.Int32
	changes     atomic.Int32
	queries     atomic.Int32
}

func registerDissolveRelationSetChanger(t *testing.T, db *gorm.DB, callbackName string, projectID string, conversationID string, userID string, maxChanges int32) *dissolveRelationSetChanger {
	t.Helper()

	changer := &dissolveRelationSetChanger{}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "project_groups" {
			return
		}
		if _, ok := tx.Statement.ConnPool.(*sql.Tx); !ok {
			return
		}
		queryNumber := changer.queries.Add(1)
		if queryNumber%2 == 0 {
			return
		}
		discovery := changer.discoveries.Add(1)
		if maxChanges >= 0 && discovery > maxChanges {
			return
		}
		changer.changes.Add(1)
		callbackDB := tx.Session(&gorm.Session{NewDB: true})
		if _, ok := callbackDB.Statement.ConnPool.(*sql.Tx); !ok {
			tx.AddError(errors.New("relation set changer is not in the dissolution transaction"))
			return
		}
		if err := callbackDB.Create(&store.ProjectGroup{
			ProjectID:      projectID,
			ConversationID: conversationID,
			LinkedByUserID: userID,
			CreatedAt:      time.Now().UTC(),
		}).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatalf("register dissolve relation set changer: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove dissolve relation set changer: %v", err)
		}
	})
	return changer
}

func failUpdatesForTable(t *testing.T, db *gorm.DB, callbackName string, table string, updateErr error) {
	t.Helper()

	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			tx.AddError(updateErr)
		}
	}); err != nil {
		t.Fatalf("register %s update callback: %v", table, err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Update().Remove(callbackName); err != nil {
			t.Errorf("remove %s update callback: %v", table, err)
		}
	})
}
