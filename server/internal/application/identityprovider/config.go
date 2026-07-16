package identityprovider

import (
	"errors"
	"net/url"
	"strings"
)

func NormalizeType(providerType string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(providerType)) {
	case TypeDingTalk:
		return TypeDingTalk, nil
	case TypeWeCom:
		return TypeWeCom, nil
	case TypeFeishu:
		return TypeFeishu, nil
	case TypeGitHub:
		return TypeGitHub, nil
	case TypeGoogle:
		return TypeGoogle, nil
	case TypeOIDC:
		return TypeOIDC, nil
	case "":
		return "", errors.New("登录方式类型不能为空")
	default:
		return "", errors.New("登录方式类型不支持")
	}
}

func NormalizeScopes(providerType string, rawScopes []string) ([]string, error) {
	if len(rawScopes) == 0 {
		rawScopes = defaultScopes(providerType)
	}
	seen := map[string]struct{}{}
	scopes := make([]string, 0, len(rawScopes))
	for _, rawScope := range rawScopes {
		scope := strings.TrimSpace(rawScope)
		if scope == "" {
			continue
		}
		if strings.ContainsAny(scope, " \t\r\n") {
			return nil, errors.New("Scope 不能包含空白字符")
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	if scopes == nil {
		scopes = []string{}
	}
	return scopes, nil
}

func NormalizeConfig(providerType string, rawConfig map[string]any) (map[string]any, error) {
	config := defaultConfig(providerType)
	for key, rawValue := range rawConfig {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		switch value := rawValue.(type) {
		case string:
			config[normalizedKey] = strings.TrimSpace(value)
		default:
			config[normalizedKey] = value
		}
	}
	for _, key := range []string{"authorize_url", "token_url", "userinfo_url", "emails_url", "app_token_url", "userid_by_unionid_url", "userdetail_url"} {
		value := StringConfigValue(config, key)
		if value == "" {
			continue
		}
		normalizedURL, err := normalizeHTTPURL(value, configURLMessage(key))
		if err != nil {
			return nil, err
		}
		config[key] = normalizedURL
	}
	for _, key := range []string{"authorize_url", "token_url", "userinfo_url"} {
		if StringConfigValue(config, key) == "" {
			return nil, errors.New(configURLMessage(key))
		}
	}
	if providerType == TypeWeCom && StringConfigValue(config, "agent_id") == "" {
		return nil, errors.New("企业微信 Agent ID 不能为空")
	}
	return config, nil
}

func StringConfigValue(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func defaultScopes(providerType string) []string {
	switch providerType {
	case TypeDingTalk:
		return []string{"openid"}
	case TypeWeCom:
		return []string{"snsapi_base"}
	case TypeFeishu:
		return []string{}
	case TypeGitHub:
		return []string{"read:user", "user:email"}
	case TypeGoogle:
		return []string{"openid", "email", "profile"}
	default:
		return []string{"openid", "email", "profile"}
	}
}

func defaultConfig(providerType string) map[string]any {
	switch providerType {
	case TypeDingTalk:
		return map[string]any{
			"authorize_url": "https://login.dingtalk.com/oauth2/auth?prompt=consent", "token_url": "https://api.dingtalk.com/v1.0/oauth2/userAccessToken",
			"userinfo_url": "https://api.dingtalk.com/v1.0/contact/users/me", "app_token_url": "https://api.dingtalk.com/v1.0/oauth2/accessToken",
			"userid_by_unionid_url": "https://oapi.dingtalk.com/user/getUseridByUnionid", "userdetail_url": "https://oapi.dingtalk.com/topapi/v2/user/get",
			"external_id_field": "unionId", "email_field": "org_email", "phone_field": "mobile", "name_field": "name",
			"nickname_field": "nick", "avatar_field": "avatarUrl",
		}
	case TypeWeCom:
		return map[string]any{
			"authorize_url": "https://login.work.weixin.qq.com/wwlogin/sso/login", "token_url": "https://qyapi.weixin.qq.com/cgi-bin/gettoken",
			"userinfo_url": "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo", "userdetail_url": "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserdetail",
			"login_type": "CorpApp",
		}
	case TypeFeishu:
		return map[string]any{
			"authorize_url": "https://accounts.feishu.cn/open-apis/authen/v1/authorize", "token_url": "https://accounts.feishu.cn/oauth/v3/token",
			"userinfo_url": "https://open.feishu.cn/open-apis/authen/v1/user_info", "external_id_field": "union_id",
			"email_field": "enterprise_email", "name_field": "name", "nickname_field": "en_name", "avatar_field": "avatar_url",
		}
	case TypeGitHub:
		return map[string]any{
			"authorize_url": "https://github.com/login/oauth/authorize", "token_url": "https://github.com/login/oauth/access_token",
			"userinfo_url": "https://api.github.com/user", "emails_url": "https://api.github.com/user/emails", "external_id_field": "id",
			"email_field": "email", "name_field": "name", "nickname_field": "login", "avatar_field": "avatar_url",
		}
	case TypeGoogle:
		return map[string]any{
			"authorize_url": "https://accounts.google.com/o/oauth2/v2/auth", "token_url": "https://oauth2.googleapis.com/token",
			"userinfo_url": "https://openidconnect.googleapis.com/v1/userinfo", "external_id_field": "sub", "email_field": "email",
			"name_field": "name", "avatar_field": "picture",
		}
	default:
		return map[string]any{
			"external_id_field": "sub", "email_field": "email", "phone_field": "phone", "name_field": "name",
			"nickname_field": "nickname", "avatar_field": "picture",
		}
	}
}

func configURLMessage(key string) string {
	switch key {
	case "authorize_url":
		return "Authorize URL 格式错误"
	case "token_url":
		return "Access Token URL 格式错误"
	case "userinfo_url":
		return "用户信息 URL 格式错误"
	case "emails_url":
		return "邮箱 API URL 格式错误"
	case "app_token_url":
		return "应用 Access Token URL 格式错误"
	case "userid_by_unionid_url":
		return "UnionID 查询 UserID URL 格式错误"
	case "userdetail_url":
		return "用户详情 URL 格式错误"
	default:
		return "URL 格式错误"
	}
}

func normalizeHTTPURL(value string, message string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New(message)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New(message)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New(message)
	}
	return trimmed, nil
}
