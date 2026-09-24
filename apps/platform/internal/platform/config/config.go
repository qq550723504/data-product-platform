package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment      string
	HTTPAddr         string
	PostgresDSN      string
	IndustryPackRoot string
	RightsAPI        RightsAPIConfig
	DeliveryAPI      DeliveryAPIConfig
	HumanDecisionAPI HumanDecisionAPIConfig
	Redis            RedisConfig
	Storage          StorageConfig
	OpenMetadata     OpenMetadataConfig
	Hop              HopConfig
	Splink           SplinkConfig
	LabelStudio      LabelStudioConfig
}

type RightsAPIConfig struct {
	Token        string
	ActorID      string
	WorkspaceIDs []string
}

type DeliveryAPIConfig struct {
	Enabled      bool
	Token        string
	PrincipalRef string
	ConsumerRef  string
	WorkspaceIDs []string
}

type HumanDecisionAPIConfig struct {
	Enabled      bool
	Token        string
	Subject      string
	ActorID      string
	WorkspaceIDs []string
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
	PolicyRef             string
	PolicyVersion         string
	TimeoutSeconds        int
}

type LabelStudioConfig struct {
	Enabled        bool
	BaseURL        string
	Token          string
	InstanceRef    string
	TimeoutSeconds int
	PollSeconds    int
	LeaseSeconds   int
	BatchSize      int
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
	deliveryAPIEnabled, err := boolEnv("DELIVERY_API_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	humanDecisionAPIEnabled, err := boolEnv("HUMAN_DECISION_API_ENABLED", false)
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
	labelStudioEnabled, err := boolEnv("LABEL_STUDIO_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	labelStudioTimeoutSeconds, err := intEnv("LABEL_STUDIO_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	labelStudioPollSeconds, err := intEnv("LABEL_STUDIO_POLL_SECONDS", 5)
	if err != nil {
		return Config{}, err
	}
	labelStudioLeaseSeconds, err := intEnv("LABEL_STUDIO_LEASE_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	labelStudioBatchSize, err := intEnv("LABEL_STUDIO_BATCH_SIZE", 20)
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
		RightsAPI: RightsAPIConfig{
			Token:        os.Getenv("RIGHTS_API_TOKEN"),
			ActorID:      os.Getenv("RIGHTS_API_ACTOR_ID"),
			WorkspaceIDs: stringListEnv("RIGHTS_API_WORKSPACE_IDS"),
		},
		DeliveryAPI: DeliveryAPIConfig{
			Enabled:      deliveryAPIEnabled,
			Token:        os.Getenv("DELIVERY_API_TOKEN"),
			PrincipalRef: os.Getenv("DELIVERY_API_PRINCIPAL_REF"),
			ConsumerRef:  os.Getenv("DELIVERY_API_CONSUMER_REF"),
			WorkspaceIDs: stringListEnv("DELIVERY_API_WORKSPACE_IDS"),
		},
		HumanDecisionAPI: HumanDecisionAPIConfig{
			Enabled:      humanDecisionAPIEnabled,
			Token:        os.Getenv("HUMAN_DECISION_API_TOKEN"),
			Subject:      os.Getenv("HUMAN_DECISION_API_SUBJECT"),
			ActorID:      os.Getenv("HUMAN_DECISION_API_ACTOR_ID"),
			WorkspaceIDs: stringListEnv("HUMAN_DECISION_API_WORKSPACE_IDS"),
		},
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
			PolicyRef:             stringEnv("SPLINK_POLICY_REF", "park-company-match"),
			PolicyVersion:         stringEnv("SPLINK_POLICY_VERSION", "1.0.0"),
			TimeoutSeconds:        splinkTimeoutSeconds,
		},
		LabelStudio: LabelStudioConfig{
			Enabled:        labelStudioEnabled,
			BaseURL:        os.Getenv("LABEL_STUDIO_BASE_URL"),
			Token:          os.Getenv("LABEL_STUDIO_TOKEN"),
			InstanceRef:    stringEnv("LABEL_STUDIO_INSTANCE_REF", "label-studio-reference"),
			TimeoutSeconds: labelStudioTimeoutSeconds,
			PollSeconds:    labelStudioPollSeconds,
			LeaseSeconds:   labelStudioLeaseSeconds,
			BatchSize:      labelStudioBatchSize,
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
	if strings.EqualFold(cfg.Environment, "production") && (cfg.RightsAPI.Token == "" || cfg.RightsAPI.ActorID == "" || len(cfg.RightsAPI.WorkspaceIDs) == 0) {
		return Config{}, fmt.Errorf("RIGHTS_API_TOKEN, RIGHTS_API_ACTOR_ID, and RIGHTS_API_WORKSPACE_IDS must be configured in production")
	}
	if cfg.DeliveryAPI.Enabled && (strings.TrimSpace(cfg.DeliveryAPI.Token) == "" || strings.TrimSpace(cfg.DeliveryAPI.PrincipalRef) == "" || strings.TrimSpace(cfg.DeliveryAPI.ConsumerRef) == "" || len(cfg.DeliveryAPI.WorkspaceIDs) == 0) {
		return Config{}, fmt.Errorf("DELIVERY_API_TOKEN, DELIVERY_API_PRINCIPAL_REF, DELIVERY_API_CONSUMER_REF, and DELIVERY_API_WORKSPACE_IDS must be configured when direct delivery is enabled")
	}
	if cfg.HumanDecisionAPI.Enabled && (strings.TrimSpace(cfg.HumanDecisionAPI.Token) == "" || strings.TrimSpace(cfg.HumanDecisionAPI.ActorID) == "" || len(cfg.HumanDecisionAPI.WorkspaceIDs) == 0) {
		return Config{}, fmt.Errorf("HUMAN_DECISION_API_TOKEN, HUMAN_DECISION_API_ACTOR_ID, and HUMAN_DECISION_API_WORKSPACE_IDS must be configured when human decision API is enabled")
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
	if cfg.LabelStudio.Enabled {
		if strings.TrimSpace(cfg.LabelStudio.BaseURL) == "" ||
			strings.TrimSpace(cfg.LabelStudio.Token) == "" ||
			strings.TrimSpace(cfg.LabelStudio.InstanceRef) == "" {
			return Config{}, fmt.Errorf("LABEL_STUDIO_BASE_URL, LABEL_STUDIO_TOKEN, and LABEL_STUDIO_INSTANCE_REF must be configured when Label Studio is enabled")
		}
		if cfg.LabelStudio.TimeoutSeconds <= 0 || cfg.LabelStudio.PollSeconds <= 0 ||
			cfg.LabelStudio.LeaseSeconds <= 0 || cfg.LabelStudio.BatchSize <= 0 {
			return Config{}, fmt.Errorf("Label Studio timeout, poll, lease, and batch settings must be positive")
		}
	}
	if cfg.Splink.Enabled {
		if cfg.Splink.BaseURL == "" {
			return Config{}, fmt.Errorf("SPLINK_SERVICE_URL must not be empty when Splink is enabled")
		}
		if cfg.Splink.ExpectedEngineVersion == "" || cfg.Splink.ModelRef == "" || cfg.Splink.ModelVersion == "" {
			return Config{}, fmt.Errorf("Splink expected version, model ref and model version must not be empty when Splink is enabled")
		}
		if cfg.Splink.PolicyRef == "" || cfg.Splink.PolicyVersion == "" {
			return Config{}, fmt.Errorf("Splink matching policy ref and version must not be empty when Splink is enabled")
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

func stringListEnv(key string) []string {
	values := strings.Split(os.Getenv(key), ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
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
