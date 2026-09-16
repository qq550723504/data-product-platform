package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Environment string
	HTTPAddr    string
	PostgresDSN string
	Redis       RedisConfig
	Storage     StorageConfig
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

func Load() (Config, error) {
	redisDB, err := intEnv("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	useSSL, err := boolEnv("OBJECT_STORAGE_USE_SSL", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Environment: stringEnv("APP_ENV", "development"),
		HTTPAddr:    stringEnv("APP_HTTP_ADDR", ":8080"),
		PostgresDSN: stringEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/data_product_platform?sslmode=disable"),
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
