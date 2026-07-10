package httpserver

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

func TestCreateGroupConversationProjectRowsLockInSortedOrder(t *testing.T) {
	server, db := newTestRouter(t)
	defer server.Close()

	now := time.Now().UTC().Add(-time.Hour)
	owner := insertTestUser(t, db, "group-project-lock-owner@example.com", "Group Project Lock Owner", store.UserStatusActive, now)
	low := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000001", Owner: owner, Name: "Low", UpdatedAt: now})
	high := insertProjectFixture(t, db, projectFixtureInput{ID: "00000000-0000-0000-0000-000000000002", Owner: owner, Name: "High", UpdatedAt: now})
	cookie := loginAsUser(t, server, owner.Email)
	recorder := registerProjectQueryLockRecorder(t, db, "test:record_group_create_project_locks", map[string]struct{}{
		low.ID: {}, high.ID: {},
	})

	resp, body := postJSON(t, server, "/api/client/conversations/groups", map[string]any{
		"name":        "Project Lock Group",
		"project_ids": []string{high.ID, low.ID},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %#v", resp.StatusCode, body)
	}

	records := recorder.snapshot()
	wantIDs := []string{low.ID, high.ID}
	if len(records) != len(wantIDs) {
		t.Fatalf("project lock records = %#v, want IDs %v", records, wantIDs)
	}
	for index, record := range records {
		if record.ID != wantIDs[index] || !record.Locked || !record.InTransaction {
			t.Fatalf("project lock record %d = %#v, want ID %s FOR UPDATE in transaction", index, record, wantIDs[index])
		}
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
	recorder := registerProjectQueryLockRecorder(t, db, "test:record_group_dissolve_lock_order", map[string]struct{}{
		low.ID: {}, high.ID: {}, conversation.ID: {},
	})

	if _, err := (&Server{db: db}).dissolveUserGroupConversation(owner, conversation.ID); err != nil {
		t.Fatalf("dissolve group conversation: %v", err)
	}
	records := recorder.snapshot()
	wantIDs := []string{low.ID, high.ID, conversation.ID}
	if len(records) != len(wantIDs) {
		t.Fatalf("lock records = %#v, want IDs %v", records, wantIDs)
	}
	for index, record := range records {
		if record.ID != wantIDs[index] || !record.Locked || !record.InTransaction {
			t.Fatalf("lock record %d = %#v, want ID %s FOR UPDATE in transaction", index, record, wantIDs[index])
		}
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
