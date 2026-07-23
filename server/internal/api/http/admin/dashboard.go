package admin

import (
	"net/http"

	"app/internal/application/dashboard"

	"github.com/labstack/echo/v4"
)

type DashboardAPI struct {
	dashboard dashboard.AdminService
}

type dashboardStatsResponse struct {
	TotalUsers                 int64 `json:"total_users" example:"120"`
	VisitedUsers24Hours        int64 `json:"visited_users_24_hours" example:"32"`
	VisitedUsers7Days          int64 `json:"visited_users_7_days" example:"86"`
	OnlineUsers                int64 `json:"online_users" example:"18"`
	Messages24Hours            int64 `json:"messages_24_hours" example:"326"`
	Messages7Days              int64 `json:"messages_7_days" example:"1842"`
	ActiveConversations24Hours int64 `json:"active_conversations_24_hours" example:"27"`
	ActiveConversations7Days   int64 `json:"active_conversations_7_days" example:"74"`
}

func NewDashboardAPI(service dashboard.AdminService) *DashboardAPI {
	return &DashboardAPI{dashboard: service}
}

func (a *DashboardAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/dashboard", a.stats)
}

// stats godoc
//
// @Summary 获取管理仪表盘统计
// @Description 返回用户访问、实时在线、消息和活跃会话的 24 小时及 7 天统计。
// @Tags 管理仪表盘
// @Produce json
// @Success 200 {object} successEnvelope{data=dashboardStatsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/dashboard [get]
func (a *DashboardAPI) stats(c echo.Context) error {
	stats, err := a.dashboard.GetStats(c.Request().Context())
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, "internal_error", "加载仪表盘统计失败")
	}
	return writeSuccess(c, http.StatusOK, dashboardStatsResponse{
		TotalUsers:                 stats.TotalUsers,
		VisitedUsers24Hours:        stats.VisitedUsers24Hours,
		VisitedUsers7Days:          stats.VisitedUsers7Days,
		OnlineUsers:                stats.OnlineUsers,
		Messages24Hours:            stats.Messages24Hours,
		Messages7Days:              stats.Messages7Days,
		ActiveConversations24Hours: stats.ActiveConversations24H,
		ActiveConversations7Days:   stats.ActiveConversations7D,
	})
}
