package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"strings"
)

func (r *Repository) GetResultByProviderObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, campaignID uuid.UUID,
	providerBindingRef, externalTaskID, externalAnnotationID, externalRevision, payloadSHA256 string,
) (annotationdomain.Result, error) {
	var result annotationdomain.Result
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, author_ref,
		       COALESCE(provider_binding_ref,''), COALESCE(external_task_id,''),
		       COALESCE(external_annotation_id,''), COALESCE(external_revision,''),
		       observation_key, canonical_payload, canonical_payload_sha256,
		       normalizer_version, corrected_from_result_id, created_at, created_by
		  FROM annotation_result
		 WHERE workspace_id=$1
		   AND campaign_id=$2
		   AND provider_binding_ref=$3
		   AND external_task_id=$4
		   AND external_annotation_id=$5
		   AND COALESCE(external_revision,'')=$6
		   AND canonical_payload_sha256=$7
		   AND corrected_from_result_id IS NULL
	`, workspaceID, campaignID, providerBindingRef, externalTaskID, externalAnnotationID,
		externalRevision, payloadSHA256).Scan(
		&result.ID, &result.WorkspaceID, &result.CampaignID, &result.TaskID, &result.AuthorRef,
		&result.ProviderBindingRef, &result.ExternalTaskID, &result.ExternalAnnotationID,
		&result.ExternalRevision, &result.ObservationKey, &payload, &result.CanonicalPayloadSHA256,
		&result.NormalizerVersion, &result.CorrectedFromResultID, &result.CreatedAt, &result.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Result{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.Result{}, fmt.Errorf("get annotation result by provider observation: %w", err)
	}
	result.CanonicalPayload = payload
	return result, nil
}

func (r *Repository) GetResultByObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, campaignID uuid.UUID,
	observationKey string,
) (annotationdomain.Result, error) {
	var result annotationdomain.Result
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT r.id, r.workspace_id, r.campaign_id, r.task_id, r.author_ref,
		       COALESCE(r.provider_binding_ref,''), COALESCE(r.external_task_id,''),
		       COALESCE(r.external_annotation_id,''), COALESCE(r.external_revision,''),
		       r.observation_key, r.canonical_payload, r.canonical_payload_sha256,
		       r.normalizer_version, r.corrected_from_result_id, r.created_at, r.created_by
		  FROM annotation_result_alias a
		  JOIN annotation_result r ON r.id=a.result_id
		 WHERE a.workspace_id=$1 AND a.campaign_id=$2 AND a.alias=$3
	`, workspaceID, campaignID, observationKey).Scan(
		&result.ID, &result.WorkspaceID, &result.CampaignID, &result.TaskID, &result.AuthorRef,
		&result.ProviderBindingRef, &result.ExternalTaskID, &result.ExternalAnnotationID,
		&result.ExternalRevision, &result.ObservationKey, &payload, &result.CanonicalPayloadSHA256,
		&result.NormalizerVersion, &result.CorrectedFromResultID, &result.CreatedAt, &result.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Result{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.Result{}, fmt.Errorf("get annotation result by observation alias: %w", err)
	}
	result.CanonicalPayload = payload
	return result, nil
}

func (r *Repository) ReserveResultAliasTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, campaignID uuid.UUID,
	alias string,
	resultID uuid.UUID,
) error {
	if workspaceID == uuid.Nil || campaignID == uuid.Nil || resultID == uuid.Nil || strings.TrimSpace(alias) == "" {
		return ErrResultAliasConflict
	}
	alias = strings.TrimSpace(alias)

	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_result_alias(workspace_id, campaign_id, alias, result_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (workspace_id, campaign_id, alias) DO NOTHING
	`, workspaceID, campaignID, alias, resultID); err != nil {
		return fmt.Errorf("reserve annotation result alias: %w", err)
	}

	var mappedResultID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT result_id
		  FROM annotation_result_alias
		 WHERE workspace_id=$1 AND campaign_id=$2 AND alias=$3
	`, workspaceID, campaignID, alias).Scan(&mappedResultID); err != nil {
		return fmt.Errorf("read annotation result alias reservation: %w", err)
	}
	if mappedResultID != resultID {
		return ErrResultAliasConflict
	}
	return nil
}
