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
	Splink           SplinkConfig
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

type SplinkConfig struct {
	Enabled               bool
	BaseURL               string
	Token                 string
	ExpectedEngineVersion string
	ModelRef              string
	ModelVersion          string
	TimeoutSeconds        int
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
	splinkEnabled, err := boolEnv("SPLINK_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	splinkTimeoutSeconds, err := intEnv("SPLINK_TIMEOUT_SECONDS", 20)
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
		Splink: SplinkConfig{
			Enabled:               splinkEnabled,
			BaseURL:               os.Getenv("SPLINK_SERVICE_URL"),
			Token:                 os.Getenv("SPLINK_SERVICE_TOKEN"),
			ExpectedEngineVersion: stringEnv("SPLINK_EXPECTED_VERSION", "4.0.17"),
			ModelRef:              stringEnv("SPLINK_MODEL_REF", "park-company-v1"),
			ModelVersion:          stringEnv("SPLINK_MODEL_VERSION", "1.0.0"),
			TimeoutSeconds:        splinkTimeoutSeconds,
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
	if cfg.Splink.Enabled {
		if cfg.Splink.BaseURL == "" {
			return Config{}, fmt.Errorf("SPLINK_SERVICE_URL must not be empty when Splink is enabled")
		}
		if cfg.Splink.ExpectedEngineVersion == "" || cfg.Splink.ModelRef == "" || cfg.Splink.ModelVersion == "" {
			return Config{}, fmt.Errorf("Splink expected version, model ref and model version must not be empty when Splink is enabled")
		}
		if cfg.Splink.TimeoutSeconds <= 0 {
			return Config{}, fmt.Errorf("SPLINK_TIMEOUT_SECONDS must be positive")
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
