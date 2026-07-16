package identityprovider

import (
	"context"
	"fmt"
	"testing"

	"app/internal/store"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestServiceManagesProviderConfigurationAndOrder(t *testing.T) {
	db := openIdentityProviderTestDB(t)
	ids := []string{
		"10000000-0000-0000-0000-000000000001",
		"10000000-0000-0000-0000-000000000002",
	}
	service := NewService(Dependencies{
		DB: db,
		NewID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
		GenerateSuffix: func() (string, error) { return "deadbeef", nil },
	})

	first, err := service.Create(context.Background(), WriteCommand{
		Name: " Enterprise SSO ", Type: TypeOIDC, ClientID: " client-id ", ClientSecret: " secret ",
		Config: testOIDCConfig(),
	})
	if err != nil {
		t.Fatalf("create first provider: %v", err)
	}
	if first.Name != "Enterprise SSO" || first.Key != "enterprise-sso" || !first.Enabled || first.SortOrder != 10 {
		t.Fatalf("first provider = %#v", first)
	}
	if len(first.Scopes) != 3 || first.Scopes[0] != "openid" || first.ClientID != "client-id" {
		t.Fatalf("normalized first provider = %#v", first)
	}

	second, err := service.Create(context.Background(), WriteCommand{
		Name: "Enterprise SSO", Type: TypeGitHub, ClientID: "github-id", ClientSecret: "github-secret",
		Config: map[string]any{
			"authorize_url": "https://github.test/authorize", "token_url": "https://github.test/token",
			"userinfo_url": "https://github.test/user",
		},
	})
	if err != nil {
		t.Fatalf("create second provider: %v", err)
	}
	if second.Key != "enterprise-sso-deadbeef" || second.SortOrder != 20 {
		t.Fatalf("second provider = %#v", second)
	}

	moved, err := service.Move(context.Background(), MoveCommand{ProviderID: second.ID, Direction: "up"})
	if err != nil {
		t.Fatalf("move provider: %v", err)
	}
	if len(moved) != 2 || moved[0].ID != second.ID || moved[0].SortOrder != 10 || moved[1].SortOrder != 20 {
		t.Fatalf("moved providers = %#v", moved)
	}

	updated, err := service.Update(context.Background(), UpdateCommand{
		ProviderID: first.ID,
		WriteCommand: WriteCommand{
			Name: "Corporate SSO", Type: TypeOIDC, ClientID: "new-id", ClientSecret: "new-secret",
			Scopes: []string{"openid", "openid", "email"}, Config: testOIDCConfig(),
		},
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if updated.Key != first.Key || updated.Name != "Corporate SSO" || len(updated.Scopes) != 2 {
		t.Fatalf("updated provider = %#v", updated)
	}

	disabled, err := service.SetEnabled(context.Background(), SetEnabledCommand{ProviderID: first.ID, Enabled: false})
	if err != nil || disabled.Enabled {
		t.Fatalf("disable provider = %#v, err = %v", disabled, err)
	}
	if _, err := service.GetEnabledByKey(context.Background(), first.Key); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("get disabled provider error = %v, code = %q", err, ErrorCodeOf(err))
	}
	if err := service.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("delete provider: %v", err)
	}
	if _, err := service.Get(context.Background(), first.ID); ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("get deleted provider error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func TestServiceRejectsInvalidProviderConfiguration(t *testing.T) {
	service := NewService(Dependencies{DB: openIdentityProviderTestDB(t)})
	_, err := service.Create(context.Background(), WriteCommand{
		Name: "Broken", Type: TypeOIDC, ClientID: "id", ClientSecret: "secret",
		Scopes: []string{"openid email"}, Config: testOIDCConfig(),
	})
	if ErrorCodeOf(err) != CodeInvalidRequest || ErrorMessage(err) != "Scope 不能包含空白字符" {
		t.Fatalf("invalid scopes error = %v, code = %q", err, ErrorCodeOf(err))
	}
	_, err = service.Create(context.Background(), WriteCommand{
		Name: "Broken", Type: TypeOIDC, ClientID: "id", ClientSecret: "secret",
		Config: map[string]any{"authorize_url": "javascript:alert(1)"},
	})
	if ErrorCodeOf(err) != CodeInvalidRequest {
		t.Fatalf("invalid URL error = %v, code = %q", err, ErrorCodeOf(err))
	}
}

func testOIDCConfig() map[string]any {
	return map[string]any{
		"authorize_url": "https://sso.test/authorize", "token_url": "https://sso.test/token",
		"userinfo_url": "https://sso.test/userinfo",
	}
}

func openIdentityProviderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:identity-provider-%p?mode=memory&cache=shared", t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&store.ThirdPartyLoginProvider{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}
