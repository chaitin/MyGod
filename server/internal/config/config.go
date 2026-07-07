package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Admin    AdminConfig    `yaml:"admin"`
	Storage  StorageConfig  `yaml:"storage"`
}

type ServerConfig struct {
	Addr           string `yaml:"addr"`
	ClientHostname string `yaml:"-"`
	AdminHostname  string `yaml:"-"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type AdminConfig struct {
	Password string `yaml:"password"`
}

type StorageConfig struct {
	Provider        string                 `yaml:"provider"`
	Endpoint        string                 `yaml:"endpoint"`
	Region          string                 `yaml:"region"`
	AccessKeyID     string                 `yaml:"access_key_id"`
	SecretAccessKey string                 `yaml:"secret_access_key"`
	ForcePathStyle  bool                   `yaml:"force_path_style"`
	Buckets         StorageBucketsConfig   `yaml:"buckets"`
	Lifecycle       StorageLifecycleConfig `yaml:"lifecycle"`
	AssetsHostname  string                 `yaml:"-"`
}

type StorageBucketsConfig struct {
	Public    string `yaml:"public"`
	Private   string `yaml:"private"`
	Temporary string `yaml:"temporary"`
}

type StorageLifecycleConfig struct {
	TemporaryExpireDays int32 `yaml:"temporary_expire_days"`
	AbortMultipartDays  int32 `yaml:"abort_multipart_days"`
}

func Load() (Config, error) {
	path := os.Getenv("CONFIG")
	if strings.TrimSpace(path) == "" {
		path = "config.yaml"
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg := Config{
		Server: ServerConfig{Addr: ":20080"},
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	if strings.TrimSpace(cfg.Server.Addr) == "" {
		cfg.Server.Addr = ":20080"
	}
	if strings.TrimSpace(cfg.Database.DSN) == "" {
		return Config{}, fmt.Errorf("database.dsn is required")
	}
	if strings.TrimSpace(cfg.Admin.Password) == "" {
		return Config{}, fmt.Errorf("admin.password is required")
	}
	if err := normalizePublicHostnames(&cfg); err != nil {
		return Config{}, err
	}
	if err := normalizeStorageConfig(&cfg.Storage); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func normalizePublicHostnames(cfg *Config) error {
	cfg.Server.ClientHostname = strings.TrimSpace(firstNonEmptyEnv("CLIENT_HOSTNAME"))
	cfg.Server.AdminHostname = strings.TrimSpace(firstNonEmptyEnv("ADMIN_HOSTNAME"))
	cfg.Storage.AssetsHostname = strings.TrimSpace(firstNonEmptyEnv("ASSETS_HOSTNAME"))

	if err := validateHostnameEnv("CLIENT_HOSTNAME", cfg.Server.ClientHostname); err != nil {
		return err
	}
	if err := validateHostnameEnv("ADMIN_HOSTNAME", cfg.Server.AdminHostname); err != nil {
		return err
	}
	if err := validateHostnameEnv("ASSETS_HOSTNAME", cfg.Storage.AssetsHostname); err != nil {
		return err
	}

	return nil
}

func validateHostnameEnv(name string, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.Contains(value, "://") || strings.Contains(value, "/") {
		return fmt.Errorf("%s must be a hostname without scheme or path", name)
	}

	return nil
}

func normalizeStorageConfig(cfg *StorageConfig) error {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Provider == "" {
		return nil
	}
	if cfg.Provider != "s3" {
		return fmt.Errorf("storage.provider must be s3")
	}

	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	cfg.Buckets.Public = strings.TrimSpace(cfg.Buckets.Public)
	cfg.Buckets.Private = strings.TrimSpace(cfg.Buckets.Private)
	cfg.Buckets.Temporary = strings.TrimSpace(cfg.Buckets.Temporary)

	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.AccessKeyID == "" {
		cfg.AccessKeyID = firstNonEmptyEnv("RUSTFS_ACCESS_KEY", "AWS_ACCESS_KEY_ID")
	}
	if cfg.SecretAccessKey == "" {
		cfg.SecretAccessKey = firstNonEmptyEnv("RUSTFS_SECRET_KEY", "AWS_SECRET_ACCESS_KEY")
	}
	if cfg.AccessKeyID == "" {
		return fmt.Errorf("storage.access_key_id is required")
	}
	if cfg.SecretAccessKey == "" {
		return fmt.Errorf("storage.secret_access_key is required")
	}
	if cfg.Buckets.Public == "" {
		return fmt.Errorf("storage.buckets.public is required")
	}
	if cfg.Buckets.Private == "" {
		return fmt.Errorf("storage.buckets.private is required")
	}
	if cfg.Buckets.Temporary == "" {
		return fmt.Errorf("storage.buckets.temporary is required")
	}
	if cfg.Lifecycle.TemporaryExpireDays <= 0 {
		cfg.Lifecycle.TemporaryExpireDays = 180
	}
	if cfg.Lifecycle.AbortMultipartDays <= 0 {
		cfg.Lifecycle.AbortMultipartDays = 7
	}

	return nil
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}

	return ""
}
