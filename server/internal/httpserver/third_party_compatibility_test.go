package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	externalauthapp "app/internal/application/externalauth"
	"app/internal/application/identityprovider"
	"app/internal/store"
)

type externalUserProfile = externalauthapp.Profile

type thirdPartyUserError struct {
	status  int
	code    string
	message string
}

func (err thirdPartyUserError) Error() string {
	return err.message
}

func (s *Server) findOrCreateThirdPartyUser(provider store.ThirdPartyLoginProvider, profile externalUserProfile) (store.User, error) {
	domainProvider, err := legacyIdentityProvider(provider)
	if err != nil {
		return store.User{}, err
	}
	service := externalauthapp.NewService(externalauthapp.Dependencies{DB: s.db})
	user, err := service.ResolveUser(context.Background(), domainProvider, profile)
	if err == nil || externalauthapp.ErrorCodeOf(err) == externalauthapp.CodeInternal {
		return user, err
	}
	status := http.StatusBadRequest
	if externalauthapp.ErrorCodeOf(err) == externalauthapp.CodeInvalidCredentials {
		status = http.StatusUnauthorized
	}
	if externalauthapp.ErrorCodeOf(err) == externalauthapp.CodeConflict {
		status = http.StatusConflict
	}
	return store.User{}, thirdPartyUserError{
		status: status, code: string(externalauthapp.ErrorCodeOf(err)), message: externalauthapp.ErrorMessage(err),
	}
}

func legacyIdentityProvider(value store.ThirdPartyLoginProvider) (identityprovider.Provider, error) {
	var scopes []string
	if len(value.Scopes) > 0 {
		if err := json.Unmarshal(value.Scopes, &scopes); err != nil {
			return identityprovider.Provider{}, err
		}
	}
	var config map[string]any
	if len(value.Config) > 0 {
		if err := json.Unmarshal(value.Config, &config); err != nil {
			return identityprovider.Provider{}, err
		}
	}
	return identityprovider.Provider{
		ID: value.ID, Name: value.Name, Key: value.Key, Type: value.Type, Enabled: value.Enabled,
		ClientID: value.ClientID, ClientSecret: value.ClientSecret, Scopes: scopes, Config: config, SortOrder: value.SortOrder,
	}, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
