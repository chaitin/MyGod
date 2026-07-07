package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsConfigFromCONFIGPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Addr != ":20080" {
		t.Fatalf("Server.Addr = %q, want :20080", cfg.Server.Addr)
	}
	if cfg.Database.DSN != "postgres://app:app@localhost:5432/app?sslmode=disable" {
		t.Fatalf("Database.DSN = %q", cfg.Database.DSN)
	}
	if cfg.Admin.Password != "secret-admin-password" {
		t.Fatalf("Admin.Password = %q", cfg.Admin.Password)
	}
	if cfg.Storage.Provider != "" {
		t.Fatalf("Storage.Provider = %q, want empty", cfg.Storage.Provider)
	}
}

func TestLoadReadsPublicHostnamesFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	t.Setenv("CLIENT_HOSTNAME", "client.example.com")
	t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
	t.Setenv("ASSETS_HOSTNAME", "assets.example.com")
	t.Setenv("GODDESS_APP_SECRET", "env-goddess-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.ClientHostname != "client.example.com" {
		t.Fatalf("Server.ClientHostname = %q, want client.example.com", cfg.Server.ClientHostname)
	}
	if cfg.Server.AdminHostname != "admin.example.com" {
		t.Fatalf("Server.AdminHostname = %q, want admin.example.com", cfg.Server.AdminHostname)
	}
	if cfg.Storage.AssetsHostname != "assets.example.com" {
		t.Fatalf("Storage.AssetsHostname = %q, want assets.example.com", cfg.Storage.AssetsHostname)
	}
	if cfg.Apps.GoddessSecret != "env-goddess-secret" {
		t.Fatalf("Apps.GoddessSecret = %q, want env-goddess-secret", cfg.Apps.GoddessSecret)
	}
}

func TestLoadReadsGoddessSecretFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
apps:
  goddess_secret: "file-goddess-secret"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	t.Setenv("CLIENT_HOSTNAME", "client.example.com")
	t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
	t.Setenv("ASSETS_HOSTNAME", "assets.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Apps.GoddessSecret != "file-goddess-secret" {
		t.Fatalf("Apps.GoddessSecret = %q, want file-goddess-secret", cfg.Apps.GoddessSecret)
	}
}

func TestLoadRejectsMissingGoddessSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	t.Setenv("CLIENT_HOSTNAME", "client.example.com")
	t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
	t.Setenv("ASSETS_HOSTNAME", "assets.example.com")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing goddess secret error")
	}
	if !strings.Contains(err.Error(), "apps.goddess_secret") {
		t.Fatalf("Load() error = %q, want apps.goddess_secret", err.Error())
	}
}

func TestLoadRejectsMissingPublicHostnames(t *testing.T) {
	for _, input := range []struct {
		name       string
		missingEnv string
	}{
		{name: "client", missingEnv: "CLIENT_HOSTNAME"},
		{name: "admin", missingEnv: "ADMIN_HOSTNAME"},
		{name: "assets", missingEnv: "ASSETS_HOSTNAME"},
	} {
		t.Run(input.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
`)

			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}

			t.Setenv("CONFIG", path)
			if input.missingEnv != "CLIENT_HOSTNAME" {
				t.Setenv("CLIENT_HOSTNAME", "client.example.com")
			}
			if input.missingEnv != "ADMIN_HOSTNAME" {
				t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
			}
			if input.missingEnv != "ASSETS_HOSTNAME" {
				t.Setenv("ASSETS_HOSTNAME", "assets.example.com")
			}

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want missing %s error", input.missingEnv)
			}
			if !strings.Contains(err.Error(), input.missingEnv) {
				t.Fatalf("Load() error = %q, want %s", err.Error(), input.missingEnv)
			}
		})
	}
}

func TestLoadReadsStorageConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  endpoint: "http://rustfs:9000"
  access_key_id: "mygod"
  secret_access_key: "storage-secret"
  force_path_style: true
  buckets:
    public: "mygod-public"
    private: "mygod-private"
    temporary: "mygod-temporary"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Storage.Provider != "s3" {
		t.Fatalf("Storage.Provider = %q, want s3", cfg.Storage.Provider)
	}
	if cfg.Storage.Endpoint != "http://rustfs:9000" {
		t.Fatalf("Storage.Endpoint = %q", cfg.Storage.Endpoint)
	}
	if cfg.Storage.Region != "us-east-1" {
		t.Fatalf("Storage.Region = %q, want us-east-1 default", cfg.Storage.Region)
	}
	if !cfg.Storage.ForcePathStyle {
		t.Fatal("Storage.ForcePathStyle = false, want true")
	}
	if cfg.Storage.Buckets.Public != "mygod-public" {
		t.Fatalf("Storage.Buckets.Public = %q", cfg.Storage.Buckets.Public)
	}
	if cfg.Storage.AssetsHostname != "assets.example.com" {
		t.Fatalf("Storage.AssetsHostname = %q, want assets.example.com", cfg.Storage.AssetsHostname)
	}
	if cfg.Storage.Lifecycle.TemporaryExpireDays != 180 {
		t.Fatalf("Storage.Lifecycle.TemporaryExpireDays = %d, want 180 default", cfg.Storage.Lifecycle.TemporaryExpireDays)
	}
	if cfg.Storage.Lifecycle.AbortMultipartDays != 7 {
		t.Fatalf("Storage.Lifecycle.AbortMultipartDays = %d, want 7 default", cfg.Storage.Lifecycle.AbortMultipartDays)
	}
}

func TestLoadReadsStorageCredentialsFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  endpoint: "http://rustfs:9000"
  buckets:
    public: "mygod-public"
    private: "mygod-private"
    temporary: "mygod-temporary"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)
	t.Setenv("RUSTFS_ACCESS_KEY", "env-access-key")
	t.Setenv("RUSTFS_SECRET_KEY", "env-secret-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Storage.AccessKeyID != "env-access-key" {
		t.Fatalf("Storage.AccessKeyID = %q, want env-access-key", cfg.Storage.AccessKeyID)
	}
	if cfg.Storage.SecretAccessKey != "env-secret-key" {
		t.Fatalf("Storage.SecretAccessKey = %q, want env-secret-key", cfg.Storage.SecretAccessKey)
	}
}

func TestLoadRejectsMissingAssetsHostname(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  endpoint: "http://rustfs:9000"
  access_key_id: "mygod"
  secret_access_key: "storage-secret"
  buckets:
    public: "mygod-public"
    private: "mygod-private"
    temporary: "mygod-temporary"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	t.Setenv("CLIENT_HOSTNAME", "client.example.com")
	t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
	t.Setenv("GODDESS_APP_SECRET", "test-goddess-secret")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing assets hostname error")
	}
	if !strings.Contains(err.Error(), "ASSETS_HOSTNAME") {
		t.Fatalf("Load() error = %q, want ASSETS_HOSTNAME", err.Error())
	}
}

func TestLoadRejectsInvalidPublicHostnames(t *testing.T) {
	for _, input := range []struct {
		name         string
		invalidEnv   string
		invalidValue string
	}{
		{name: "client scheme", invalidEnv: "CLIENT_HOSTNAME", invalidValue: "https://client.example.com"},
		{name: "admin query", invalidEnv: "ADMIN_HOSTNAME", invalidValue: "admin.example.com?tenant=1"},
		{name: "assets fragment", invalidEnv: "ASSETS_HOSTNAME", invalidValue: "assets.example.com#public"},
	} {
		t.Run(input.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  endpoint: "http://rustfs:9000"
  access_key_id: "mygod"
  secret_access_key: "storage-secret"
  buckets:
    public: "mygod-public"
    private: "mygod-private"
    temporary: "mygod-temporary"
`)

			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}

			t.Setenv("CONFIG", path)
			t.Setenv("CLIENT_HOSTNAME", "client.example.com")
			t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
			t.Setenv("ASSETS_HOSTNAME", "assets.example.com")
			t.Setenv("GODDESS_APP_SECRET", "test-goddess-secret")
			t.Setenv(input.invalidEnv, input.invalidValue)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want invalid %s error", input.invalidEnv)
			}
			if !strings.Contains(err.Error(), input.invalidEnv) {
				t.Fatalf("Load() error = %q, want %s", err.Error(), input.invalidEnv)
			}
		})
	}
}

func TestLoadRejectsMissingAdminPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: ""
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing admin password error")
	}
	if !strings.Contains(err.Error(), "admin.password") {
		t.Fatalf("Load() error = %q, want admin.password", err.Error())
	}
}

func TestLoadRejectsMissingDatabaseDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: ""
admin:
  password: "secret-admin-password"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing database dsn error")
	}
	if !strings.Contains(err.Error(), "database.dsn") {
		t.Fatalf("Load() error = %q, want database.dsn", err.Error())
	}
}

func TestLoadRejectsIncompleteStorageConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  access_key_id: "mygod"
  secret_access_key: "storage-secret"
  buckets:
    public: "mygod-public"
    private: "mygod-private"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing temporary bucket error")
	}
	if !strings.Contains(err.Error(), "storage.buckets.temporary") {
		t.Fatalf("Load() error = %q, want storage.buckets.temporary", err.Error())
	}
}

func TestLoadRejectsMissingStorageCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
database:
  dsn: "postgres://app:app@localhost:5432/app?sslmode=disable"
admin:
  password: "secret-admin-password"
storage:
  provider: "s3"
  buckets:
    public: "mygod-public"
    private: "mygod-private"
    temporary: "mygod-temporary"
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG", path)
	setRequiredPublicHostnames(t)
	t.Setenv("RUSTFS_ACCESS_KEY", "")
	t.Setenv("RUSTFS_SECRET_KEY", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing storage access key error")
	}
	if !strings.Contains(err.Error(), "storage.access_key_id") {
		t.Fatalf("Load() error = %q, want storage.access_key_id", err.Error())
	}
}

func setRequiredPublicHostnames(t *testing.T) {
	t.Helper()

	t.Setenv("CLIENT_HOSTNAME", "client.example.com")
	t.Setenv("ADMIN_HOSTNAME", "admin.example.com")
	t.Setenv("ASSETS_HOSTNAME", "assets.example.com")
	t.Setenv("GODDESS_APP_SECRET", "test-goddess-secret")
}
