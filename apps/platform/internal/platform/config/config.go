package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Environment      string
	HTTPAddr         string
	PostgresDSN      string
	IndustryPackRoot string
	Redis            RedisConfig
	Storage          StorageConfig
	OpenMetadata     OpenMetadataConfig
	Hop              HopConfig
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type StorageConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type OpenMetadataConfig struct {
	Enabled bool
	BaseURL string
	Token   string
	Domain  string
}

type HopConfig struct {
	Enabled  bool
	BaseURL  string
	Username string
	Password string
}

func Load() (Config, error) {
	redisDB, err := intEnv("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	useSSL, err := boolEnv("OBJECT_STORAGE_USE_SSL", false)
	if err != nil {
		return Config{}, err
	}
	openMetadataEnabled, err := boolEnv("OPENMETADATA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	hopEnabled, err := boolEnv("HOP_ENABLED", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Environment:      stringEnv("APP_ENV", "development"),
		HTTPAddr:         stringEnv("APP_HTTP_ADDR", ":8080"),
		PostgresDSN:      stringEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/data_product_platform?sslmode=disable"),
		IndustryPackRoot: stringEnv("INDUSTRY_PACK_ROOT", "../../industry-packs"),
		Redis: RedisConfig{
			Addr:     stringEnv("REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       redisDB,
		},
		Storage: StorageConfig{
			Endpoint:  stringEnv("OBJECT_STORAGE_ENDPOINT", "localhost:9000"),
			AccessKey: stringEnv("OBJECT_STORAGE_ACCESS_KEY", "minio"),
			SecretKey: stringEnv("OBJECT_STORAGE_SECRET_KEY", "minio123"),
			Bucket:    stringEnv("OBJECT_STORAGE_BUCKET", "data-product-platform"),
			UseSSL:    useSSL,
		},
		OpenMetadata: OpenMetadataConfig{
			Enabled: openMetadataEnabled,
			BaseURL: os.Getenv("OPENMETADATA_BASE_URL"),
			Token:   os.Getenv("OPENMETADATA_TOKEN"),
			Domain:  os.Getenv("OPENMETADATA_DOMAIN"),
		},
		Hop: HopConfig{
			Enabled:  hopEnabled,
			BaseURL:  os.Getenv("HOP_SERVER_URL"),
			Username: os.Getenv("HOP_SERVER_USERNAME"),
			Password: os.Getenv("HOP_SERVER_PASSWORD"),
		},
	}

	if cfg.PostgresDSN == "" {
		return Config{}, fmt.Errorf("POSTGRES_DSN must not be empty")
	}
	if cfg.Redis.Addr == "" {
		return Config{}, fmt.Errorf("REDIS_ADDR must not be empty")
	}
	if cfg.Storage.Endpoint == "" || cfg.Storage.Bucket == "" {
		return Config{}, fmt.Errorf("object storage endpoint and bucket must not be empty")
	}
	if cfg.IndustryPackRoot == "" {
		return Config{}, fmt.Errorf("INDUSTRY_PACK_ROOT must not be empty")
	}
	if cfg.OpenMetadata.Enabled {
		if cfg.OpenMetadata.BaseURL == "" {
			return Config{}, fmt.Errorf("OPENMETADATA_BASE_URL must not be empty when OpenMetadata is enabled")
		}
		if cfg.OpenMetadata.Domain == "" {
			return Config{}, fmt.Errorf("OPENMETADATA_DOMAIN must not be empty when OpenMetadata is enabled")
		}
	}
	if cfg.Hop.Enabled {
		if cfg.Hop.BaseURL == "" {
			return Config{}, fmt.Errorf("HOP_SERVER_URL must not be empty when Apache Hop is enabled")
		}
		if cfg.Hop.Username == "" || cfg.Hop.Password == "" {
			return Config{}, fmt.Errorf("HOP_SERVER_USERNAME and HOP_SERVER_PASSWORD must not be empty when Apache Hop is enabled")
		}
	}

	return cfg, nil
}

func stringEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
