package externalauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	appauth "app/internal/application/externalauth"
	"app/internal/application/identityprovider"
)

func profileFromClaims(provider identityprovider.Provider, claims map[string]any) (appauth.Profile, error) {
	externalID := stringField(claims, identityprovider.StringConfigValue(provider.Config, "external_id_field"))
	if externalID == "" {
		externalID = fallbackExternalUserID(provider.Type, claims)
	}
	if externalID == "" {
		return appauth.Profile{}, errors.New("external user id is empty")
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return appauth.Profile{}, err
	}
	return appauth.Profile{
		ExternalUserID: externalID,
		Email:          emailFromClaims(provider, claims),
		Name:           stringField(claims, identityprovider.StringConfigValue(provider.Config, "name_field")),
		Nickname:       stringField(claims, identityprovider.StringConfigValue(provider.Config, "nickname_field")),
		Phone:          stringField(claims, identityprovider.StringConfigValue(provider.Config, "phone_field")),
		Avatar:         stringField(claims, identityprovider.StringConfigValue(provider.Config, "avatar_field")),
		Raw:            raw,
	}, nil
}

func emailFromClaims(provider identityprovider.Provider, claims map[string]any) string {
	emailField := identityprovider.StringConfigValue(provider.Config, "email_field")
	if provider.Type == identityprovider.TypeDingTalk {
		return firstNonEmptyField(claims, "org_email", emailField, "email")
	}
	if provider.Type == identityprovider.TypeFeishu {
		return firstNonEmptyField(claims, "enterprise_email", emailField, "email")
	}
	return stringField(claims, emailField)
}

func fallbackExternalUserID(providerType string, claims map[string]any) string {
	switch providerType {
	case identityprovider.TypeDingTalk:
		return firstNonEmptyField(claims, "unionId", "openId", "userid", "userId")
	case identityprovider.TypeFeishu:
		return firstNonEmptyField(claims, "union_id", "open_id", "user_id")
	case identityprovider.TypeGitHub:
		return firstNonEmptyField(claims, "id", "node_id", "login")
	case identityprovider.TypeGoogle:
		return stringField(claims, "sub")
	default:
		return firstNonEmptyField(claims, "sub", "id", "user_id", "open_id", "union_id")
	}
}

func unwrapDataClaims(claims map[string]any) map[string]any {
	data, ok := claims["data"].(map[string]any)
	if !ok || len(data) == 0 {
		return claims
	}
	return data
}

func mergeClaims(base, extra map[string]any) map[string]any {
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyField(claims map[string]any, fields ...string) string {
	for _, field := range fields {
		if value := stringField(claims, field); value != "" {
			return value
		}
	}
	return ""
}

func stringField(claims map[string]any, field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	var current any = claims
	for _, part := range strings.Split(field, ".") {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = currentMap[part]
	}
	switch value := current.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	case float64:
		return fmt.Sprintf("%.0f", value)
	default:
		return ""
	}
}

func int64Field(claims map[string]any, field string) int64 {
	value := stringField(claims, field)
	if value == "" {
		return 0
	}
	parsed, err := json.Number(value).Int64()
	if err != nil {
		return 0
	}
	return parsed
}

func boolField(claims map[string]any, field string) bool {
	value, ok := claims[field].(bool)
	return ok && value
}
