package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app/internal/application/dashboard"

	"github.com/labstack/echo/v4"
)

func TestDashboardAPIReturnsStats(t *testing.T) {
	service := dashboardServiceStub{stats: dashboard.Stats{
		TotalUsers: 120, VisitedUsers24Hours: 32, VisitedUsers7Days: 86, OnlineUsers: 18,
		Messages24Hours: 326, Messages7Days: 1842,
		ActiveConversations24H: 27, ActiveConversations7D: 74,
	}}
	router := echo.New()
	NewDashboardAPI(service).RegisterRoutes(router.Group("/api/admin"))

	request := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); body == "" || !containsAll(body,
		`"total_users":120`, `"visited_users_24_hours":32`, `"online_users":18`,
		`"messages_7_days":1842`, `"active_conversations_7_days":74`,
	) {
		t.Fatalf("body = %s", body)
	}
}

type dashboardServiceStub struct {
	stats dashboard.Stats
	err   error
}

func (s dashboardServiceStub) GetStats(context.Context) (dashboard.Stats, error) {
	return s.stats, s.err
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
