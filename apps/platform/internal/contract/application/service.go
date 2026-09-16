package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"gopkg.in/yaml.v3"
)

type Service struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *Service {
	return &Service{tx: tx, repo: repo}
}

type CreateVersionFromYAMLCommand struct {
	WorkspaceID uuid.UUID
	SourceRef   string
	DocumentYAML []byte
	ActorID     *uuid.UUID
	TraceID     string
}

type PublishVersionCommand struct {
	VersionID uuid.UUID
	ActorID   *uuid.UUID
	TraceID   string
}

type documentHeader struct {
	Metadata struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		Product struct {
			Code string `yaml:"code"`
			Name string `yaml:"name"`
		} `yaml:"product"`
	} `yaml:"spec"`
}

func (s *Service) CreateVersionFromYAML(ctx context.Context, cmd CreateVersionFromYAMLCommand) (domain.ContractVersion, error) {
	if cmd.WorkspaceID == uuid.Nil || len(cmd.DocumentYAML) == 0 {
		return domain.ContractVersion{}, domain.ErrInvalidContractVersion
	}
	var header documentHeader
	if err := yaml.Unmarshal(cmd.DocumentYAML, &header); err != nil {
		return domain.ContractVersion{}, fmt.Errorf("decode data contract header: %w", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(cmd.DocumentYAML, &document); err != nil {
		return domain.ContractVersion{}, fmt.Errorf("decode data contract document: %w", err)
	}

	major, minor, patch, err := parseSemver(header.Metadata.Version)
	if err != nil {
		return domain.ContractVersion{}, err
	}
	contractCode := strings.TrimSpace(header.Spec.Product.Code)
	if contractCode == "" {
		contractCode = strings.TrimSpace(header.Metadata.Name)
	}
	contractName := strings.TrimSpace(header.Spec.Product.Name)
	if contractName == "" {
		contractName = strings.TrimSpace(header.Metadata.Name)
	}
	contract, err := domain.NewDataContract(cmd.WorkspaceID, contractCode, contractName, header.Spec.Product.Code, cmd.ActorID)
	if err != nil {
		return domain.ContractVersion{}, err
	}
	digest := sha256.Sum256(cmd.DocumentYAML)
	sourceSHA256 := hex.EncodeToString(digest[:])

	var version domain.ContractVersion
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		stored, err := s.repo.EnsureContract(ctx, tx, contract)
		if err != nil {
			return err
		}
		version, err = domain.NewContractVersion(
			stored.ID,
			major,
			minor,
			patch,
			document,
			cmd.SourceRef,
			sourceSHA256,
			cmd.ActorID,
		)
		if err != nil {
			return err
		}
		if err := s.repo.InsertVersion(ctx, tx, version); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "CONTRACT_VERSION", version.ID, "ContractVersionCreated", map[string]any{
			"contractId":        stored.ID,
			"contractVersionId": version.ID,
			"version":           version.Semver(),
			"sourceSha256":      version.SourceSHA256,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &stored.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "CONTRACT_VERSION_CREATED",
			ObjectType:  "CONTRACT_VERSION",
			ObjectID:    version.ID,
			AfterState: map[string]any{
				"contractId":   stored.ID,
				"version":      version.Semver(),
				"status":       version.Status,
				"sourceRef":    version.SourceRef,
				"sourceSha256": version.SourceSHA256,
			},
			TraceID: cmd.TraceID,
		})
	})
	return version, err
}

func (s *Service) PublishVersion(ctx context.Context, cmd PublishVersionCommand) (domain.ContractVersion, error) {
	version, err := s.repo.GetVersion(ctx, cmd.VersionID)
	if err != nil {
		return domain.ContractVersion{}, err
	}
	contract, err := s.repo.GetContract(ctx, version.ContractID)
	if err != nil {
		return domain.ContractVersion{}, err
	}
	before := version.Status
	if err := version.Publish(cmd.ActorID); err != nil {
		return domain.ContractVersion{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SaveVersionState(ctx, tx, version); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "CONTRACT_VERSION", version.ID, "ContractPublished", map[string]any{
			"contractId":        version.ContractID,
			"contractVersionId": version.ID,
			"version":           version.Semver(),
			"status":            version.Status,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &contract.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "CONTRACT_VERSION_PUBLISHED",
			ObjectType:  "CONTRACT_VERSION",
			ObjectID:    version.ID,
			BeforeState: map[string]any{"status": before},
			AfterState: map[string]any{
				"status":      version.Status,
				"publishedAt": version.PublishedAt,
			},
			TraceID: cmd.TraceID,
		})
	})
	return version, err
}

func parseSemver(value string) (int, int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("contract version %q must use major.minor.patch", value)
	}
	result := make([]int, 3)
	for i, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return 0, 0, 0, fmt.Errorf("contract version %q is invalid", value)
		}
		result[i] = parsed
	}
	return result[0], result[1], result[2], nil
}

func appendEvent(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload map[string]any) error {
	event, err := outbox.NewEvent(aggregateType, aggregateID, eventType, payload)
	if err != nil {
		return fmt.Errorf("create %s event: %w", eventType, err)
	}
	return outbox.Append(ctx, tx, event)
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
