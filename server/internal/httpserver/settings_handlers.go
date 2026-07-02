package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"app/internal/store"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type infoSettingsResponse struct {
	AppName          string `json:"app_name" example:"MyGod"`
	OrganizationName string `json:"organization_name" example:"长亭科技"`
}

type updateInfoSettingsRequest struct {
	AppName          string `json:"app_name" example:"MyGod"`
	OrganizationName string `json:"organization_name" example:"长亭科技"`
}

// clientInfo godoc
//
// @Summary 获取客户端公开信息
// @Description 返回客户端启动和登录页展示所需的公开信息，不需要登录。
// @Tags 客户端信息
// @Produce json
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 500 {object} errorEnvelope
// @Router /api/client/info [get]
func (s *Server) clientInfo(c echo.Context) error {
	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, newInfoSettingsResponse(settings))
}

// getInfoSettings godoc
//
// @Summary 获取系统基础信息设置
// @Description 管理员读取 App 名称和组织名称。
// @Tags 管理员设置
// @Produce json
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/info [get]
func (s *Server) getInfoSettings(c echo.Context) error {
	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	return success(c, http.StatusOK, newInfoSettingsResponse(settings))
}

// updateInfoSettings godoc
//
// @Summary 更新系统基础信息设置
// @Description 管理员更新 App 名称和组织名称。
// @Tags 管理员设置
// @Accept json
// @Produce json
// @Param body body updateInfoSettingsRequest true "基础信息设置"
// @Success 200 {object} successEnvelope{data=infoSettingsResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/admin/settings/info [put]
func (s *Server) updateInfoSettings(c echo.Context) error {
	var req updateInfoSettingsRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	appName := strings.TrimSpace(req.AppName)
	if appName == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "App 名称不能为空")
	}
	organizationName := strings.TrimSpace(req.OrganizationName)
	if organizationName == "" {
		return failure(c, http.StatusBadRequest, "invalid_request", "组织名称不能为空")
	}

	settings, err := s.getOrCreateAppSettings()
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	if err := s.db.Model(&settings).Updates(map[string]any{
		"app_name":          appName,
		"organization_name": organizationName,
	}).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	settings.AppName = appName
	settings.OrganizationName = organizationName

	return success(c, http.StatusOK, newInfoSettingsResponse(settings))
}

func (s *Server) getOrCreateAppSettings() (store.AppSettings, error) {
	var count int64
	if err := s.db.Model(&store.AppSettings{}).Where("id = ?", store.AppSettingsID).Count(&count).Error; err != nil {
		return store.AppSettings{}, err
	}

	if count == 0 {
		settings := store.AppSettings{
			ID:               store.AppSettingsID,
			AppName:          store.DefaultAppName,
			OrganizationName: store.DefaultOrganizationName,
		}
		if err := s.db.Create(&settings).Error; err != nil {
			return store.AppSettings{}, err
		}

		return settings, nil
	}

	var settings store.AppSettings
	err := s.db.First(&settings, "id = ?", store.AppSettingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.AppSettings{}, gorm.ErrRecordNotFound
	}
	if err != nil {
		return store.AppSettings{}, err
	}

	return settings, nil
}

func newInfoSettingsResponse(settings store.AppSettings) infoSettingsResponse {
	return infoSettingsResponse{
		AppName:          settings.AppName,
		OrganizationName: settings.OrganizationName,
	}
}
