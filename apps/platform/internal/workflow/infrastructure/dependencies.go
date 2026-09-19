package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type dependencyQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *PostgresRepository) GetDependencyPreparation(ctx context.Context, executionID uuid.UUID) (domain.DependencyPreparation, bool, error) {
	return getDependencyPreparation(ctx, r.pool, executionID)
}

func (r *PostgresRepository) GetDependencyPreparationTx(ctx context.Context, tx pgx.Tx, executionID uuid.UUID) (domain.DependencyPreparation, bool, error) {
	return getDependencyPreparation(ctx, tx, executionID)
}

func getDependencyPreparation(ctx context.Context, q dependencyQuerier, executionID uuid.UUID) (domain.DependencyPreparation, bool, error) {
	var preparation domain.DependencyPreparation
	err := q.QueryRow(ctx, `
		SELECT execution_id, workspace_id, binding_fingerprint, status
		FROM execution_dependency_preparation
		WHERE execution_id=$1
	`, executionID).Scan(&preparation.ExecutionID, &preparation.WorkspaceID, &preparation.BindingFingerprint, &preparation.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.DependencyPreparation{}, false, nil
		}
		return domain.DependencyPreparation{}, false, fmt.Errorf("get execution dependency preparation: %w", err)
	}

	dependencies, err := q.Query(ctx, `
		SELECT id, execution_id, workspace_id, dependency_name, dataset_version_id,
		       reference, version, content_sha256, content
		FROM execution_dependency_binding
		WHERE execution_id=$1
		ORDER BY dependency_name
	`, executionID)
	if err != nil {
		return domain.DependencyPreparation{}, false, fmt.Errorf("list execution dependency bindings: %w", err)
	}
	for dependencies.Next() {
		var binding domain.DependencyBinding
		if err := dependencies.Scan(
			&binding.ID, &binding.ExecutionID, &binding.WorkspaceID, &binding.Name,
			&binding.DatasetVersionID, &binding.Reference, &binding.Version,
			&binding.ContentSHA256, &binding.Content,
		); err != nil {
			dependencies.Close()
			return domain.DependencyPreparation{}, false, fmt.Errorf("scan execution dependency binding: %w", err)
		}
		preparation.Dependencies = append(preparation.Dependencies, binding)
	}
	if err := dependencies.Err(); err != nil {
		dependencies.Close()
		return domain.DependencyPreparation{}, false, fmt.Errorf("iterate execution dependency bindings: %w", err)
	}
	dependencies.Close()

	usages, err := q.Query(ctx, `
		SELECT id, execution_id, workspace_id, input_name, input_dataset_version_id,
		       resolution_dataset_version_id, source_type, source_ref, source_key,
		       decision_id, entity_id
		FROM execution_mapping_usage
		WHERE execution_id=$1
		ORDER BY input_name, source_ref, source_key
	`, executionID)
	if err != nil {
		return domain.DependencyPreparation{}, false, fmt.Errorf("list execution mapping usages: %w", err)
	}
	for usages.Next() {
		var usage domain.MappingUsage
		if err := usages.Scan(
			&usage.ID, &usage.ExecutionID, &usage.WorkspaceID, &usage.InputName,
			&usage.InputDatasetVersionID, &usage.ResolutionDatasetVersionID,
			&usage.SourceType, &usage.SourceRef, &usage.SourceKey,
			&usage.DecisionID, &usage.EntityID,
		); err != nil {
			usages.Close()
			return domain.DependencyPreparation{}, false, fmt.Errorf("scan execution mapping usage: %w", err)
		}
		preparation.MappingUsages = append(preparation.MappingUsages, usage)
	}
	if err := usages.Err(); err != nil {
		usages.Close()
		return domain.DependencyPreparation{}, false, fmt.Errorf("iterate execution mapping usages: %w", err)
	}
	usages.Close()
	return preparation, true, nil
}

func (r *PostgresRepository) InsertDependencyPreparation(ctx context.Context, tx pgx.Tx, preparation domain.DependencyPreparation) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO execution_dependency_preparation (
			execution_id, workspace_id, binding_fingerprint, status
		) VALUES ($1,$2,$3,$4)
	`, preparation.ExecutionID, preparation.WorkspaceID, preparation.BindingFingerprint, preparation.Status); err != nil {
		return fmt.Errorf("insert execution dependency preparation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertDependencyBinding(ctx context.Context, tx pgx.Tx, binding domain.DependencyBinding) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO execution_dependency_binding (
			id, execution_id, workspace_id, dependency_name, dataset_version_id,
			reference, version, content_sha256, content
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, binding.ID, binding.ExecutionID, binding.WorkspaceID, binding.Name, binding.DatasetVersionID,
		binding.Reference, binding.Version, binding.ContentSHA256, binding.Content); err != nil {
		return fmt.Errorf("insert execution dependency binding %s: %w", binding.Name, err)
	}
	return nil
}

func (r *PostgresRepository) InsertMappingUsage(ctx context.Context, tx pgx.Tx, usage domain.MappingUsage) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO execution_mapping_usage (
			id, execution_id, workspace_id, input_name, input_dataset_version_id,
			resolution_dataset_version_id, source_type, source_ref, source_key,
			decision_id, entity_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, usage.ID, usage.ExecutionID, usage.WorkspaceID, usage.InputName,
		usage.InputDatasetVersionID, usage.ResolutionDatasetVersionID,
		usage.SourceType, usage.SourceRef, usage.SourceKey, usage.DecisionID, usage.EntityID); err != nil {
		return fmt.Errorf("insert execution mapping usage %s/%s: %w", usage.InputName, usage.SourceKey, err)
	}
	return nil
}
