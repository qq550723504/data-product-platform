package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

type sourceQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const sourceColumns = "id,workspace_id,connection_id,campaign_id,task_id,input_version_id,campaign_binding_id,task_binding_id,actor_binding_id,provider_instance,provider_incarnation,source_kind,external_id,assignment_id,external_project_id,external_task_id,external_annotation_id,external_author_ref,core_author_ref,source_item_ref,input_sha256,source_sha256,task_text_sha256,config_sha256,schema_sha256,taxonomy_sha256,rubric_sha256,renderer_sha256,review_policy_sha256,mapping_sha256,normalizer_version,snapshot_sha256,canonical_payload_sha256,submission_revision,fingerprint_payload,fingerprint_sha256,snapshot,canonical_payload"

func getSource(ctx context.Context, q sourceQuerier, id uuid.UUID) (annotationdomain.SourceObservation, error) {
	var s annotationdomain.SourceObservation
	var payload []byte
	var fingerprint string
	err := q.QueryRow(ctx, "SELECT "+sourceColumns+" FROM annotation_source_observation WHERE id=$1", id).Scan(
		&s.ID, &s.WorkspaceID, &s.ConnectionID, &s.CampaignID, &s.TaskID, &s.InputVersionID, &s.CampaignBindingID, &s.TaskBindingID, &s.ActorBindingID, &s.ProviderInstance, &s.ProviderIncarnation, &s.SourceKind, &s.ExternalID, &s.AssignmentID, &s.ExternalProjectID, &s.ExternalTaskID, &s.ExternalAnnotationID, &s.ExternalAuthorRef, &s.CoreAuthorRef, &s.SourceItemRef, &s.InputSHA256, &s.SourceSHA256, &s.TaskTextSHA256, &s.ConfigSHA256, &s.SchemaSHA256, &s.TaxonomySHA256, &s.RubricSHA256, &s.RendererSHA256, &s.ReviewPolicySHA256, &s.MappingSHA256, &s.NormalizerVersion, &s.SnapshotSHA256, &s.CanonicalPayloadSHA256, &s.SubmissionRevision, &payload, &fingerprint, &s.Snapshot, &s.CanonicalPayload)
	if err != nil {
		return s, err
	}
	if err := s.Validate(); err != nil {
		return s, err
	}
	if fingerprint != s.Hash() || !bytes.Equal(payload, s.Payload()) {
		return s, annotationdomain.ErrSourceIntegrity
	}

	var contextIntact bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM annotation_source_observation s
 JOIN annotation_campaign c ON c.id=s.campaign_id
 JOIN annotation_task t ON t.id=s.task_id
 JOIN annotation_engine_campaign_binding b ON b.id=s.campaign_binding_id
 JOIN annotation_engine_task_binding tb ON tb.id=s.task_binding_id
 JOIN annotation_engine_actor_binding ab ON ab.id=s.actor_binding_id
 JOIN annotation_source_connection_owner co ON co.id=s.connection_id
 WHERE s.id=$1 AND c.workspace_id=s.workspace_id AND t.workspace_id=s.workspace_id AND t.campaign_id=c.id
 AND b.workspace_id=s.workspace_id AND b.campaign_id=c.id AND b.admission_protocol='controlled-fork-submission-v1'
 AND co.workspace_id=s.workspace_id AND b.connection_id=s.connection_id
 AND b.provider_instance_ref=s.provider_instance AND b.provider_incarnation=s.provider_incarnation
 AND b.external_project_id=s.external_project_id AND b.config_sha256=s.config_sha256 AND b.normalizer_version=s.normalizer_version
 AND tb.workspace_id=s.workspace_id AND tb.campaign_id=c.id AND tb.campaign_binding_id=b.id AND tb.task_id=t.id AND tb.external_task_id=s.external_task_id
 AND ab.workspace_id=s.workspace_id AND ab.provider=b.provider AND ab.provider_instance_ref=s.provider_instance
 AND ab.external_actor_ref=s.external_author_ref AND ab.core_actor_ref=s.core_author_ref AND t.primary_annotator_ref=s.core_author_ref
 AND c.input_dataset_version_id=s.input_version_id AND c.input_checksum_sha256=s.input_sha256
 AND c.schema_content_sha256=s.schema_sha256 AND c.taxonomy_content_sha256=s.taxonomy_sha256
 AND c.rubric_content_sha256=s.rubric_sha256 AND c.renderer_content_sha256=s.renderer_sha256 AND c.review_policy_content_sha256=s.review_policy_sha256
 AND t.source_item_ref=s.source_item_ref AND t.source_content_sha256=s.source_sha256 AND t.task_text_sha256=s.task_text_sha256)`, id).Scan(&contextIntact); err != nil {
		return s, err
	}
	if !contextIntact {
		return s, annotationdomain.ErrSourceIntegrity
	}
	return s, nil
}
func (r *Repository) StoreSourceTx(ctx context.Context, tx pgx.Tx, s annotationdomain.SourceObservation) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if _, err := r.LockCampaignTx(ctx, tx, s.CampaignID); err != nil {
		return err
	}
	if old, err := getSource(ctx, tx, s.ID); err == nil {
		if old.Hash() != s.Hash() || !bytes.Equal(old.Snapshot, s.Snapshot) || !bytes.Equal(old.CanonicalPayload, s.CanonicalPayload) {
			return annotationdomain.ErrSourceConflict
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO annotation_source_observation(`+sourceColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38) ON CONFLICT DO NOTHING`,
		s.ID, s.WorkspaceID, s.ConnectionID, s.CampaignID, s.TaskID, s.InputVersionID, s.CampaignBindingID, s.TaskBindingID, s.ActorBindingID, s.ProviderInstance, s.ProviderIncarnation, s.SourceKind, s.ExternalID, s.AssignmentID, s.ExternalProjectID, s.ExternalTaskID, s.ExternalAnnotationID, s.ExternalAuthorRef, s.CoreAuthorRef, s.SourceItemRef, s.InputSHA256, s.SourceSHA256, s.TaskTextSHA256, s.ConfigSHA256, s.SchemaSHA256, s.TaxonomySHA256, s.RubricSHA256, s.RendererSHA256, s.ReviewPolicySHA256, s.MappingSHA256, s.NormalizerVersion, s.SnapshotSHA256, s.CanonicalPayloadSHA256, s.SubmissionRevision, s.Payload(), s.Hash(), s.Snapshot, s.CanonicalPayload)
	if err != nil {
		return err
	}
	old, err := getSource(ctx, tx, s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.ErrSourceConflict
	}
	if err != nil {
		return err
	}
	if old.Hash() != s.Hash() || !bytes.Equal(old.Snapshot, s.Snapshot) ||
		!bytes.Equal(old.CanonicalPayload, s.CanonicalPayload) {
		return annotationdomain.ErrSourceConflict
	}
	return nil
}
func (r *Repository) BindSourceResultTx(ctx context.Context, tx pgx.Tx, resultID uuid.UUID, s annotationdomain.SourceObservation) error {
	_, err := tx.Exec(ctx, `INSERT INTO annotation_source_result_binding(result_id,source_id,fingerprint_sha256) VALUES ($1,$2,$3)`, resultID, s.ID, s.Hash())
	return err
}
func sourceForResult(ctx context.Context, q sourceQuerier, resultID uuid.UUID) (annotationdomain.SourceObservation, error) {
	var id uuid.UUID
	var hash string
	err := q.QueryRow(ctx, `SELECT source_id,fingerprint_sha256 FROM annotation_source_result_binding WHERE result_id=$1`, resultID).Scan(&id, &hash)
	if err != nil {
		return annotationdomain.SourceObservation{}, annotationdomain.ErrSourceIntegrity
	}
	s, err := getSource(ctx, q, id)
	if err != nil {
		return s, err
	}
	if s.Hash() != hash {
		return s, annotationdomain.ErrSourceIntegrity
	}
	return s, nil
}
func (r *Repository) VerifySourceResultTx(ctx context.Context, tx pgx.Tx, result annotationdomain.Result, incoming annotationdomain.SourceObservation) error {
	stored, err := sourceForResult(ctx, tx, result.ID)
	if err != nil {
		return err
	}
	if err := incoming.Validate(); err != nil {
		return err
	}
	if stored.ID != incoming.ID || stored.Hash() != incoming.Hash() ||
		!bytes.Equal(stored.Snapshot, incoming.Snapshot) ||
		!bytes.Equal(stored.CanonicalPayload, incoming.CanonicalPayload) ||
		!sourceMatchesResult(stored, result) {
		return annotationdomain.ErrSourceConflict
	}
	return nil
}
func sourceMatchesResult(s annotationdomain.SourceObservation, r annotationdomain.Result) bool {
	return s.WorkspaceID == r.WorkspaceID && s.CampaignID == r.CampaignID && s.TaskID == r.TaskID &&
		s.CoreAuthorRef == r.AuthorRef && s.CampaignBindingID.String() == r.ProviderBindingRef &&
		s.ExternalTaskID == r.ExternalTaskID && s.ExternalAnnotationID == r.ExternalAnnotationID &&
		s.ExternalRevision() == r.ExternalRevision && s.CanonicalPayloadSHA256 == r.CanonicalPayloadSHA256 &&
		s.NormalizerVersion == r.NormalizerVersion && bytes.Equal(s.CanonicalPayload, r.CanonicalPayload)
}

type SourceClosure struct {
	ResultID          uuid.UUID `json:"resultId"`
	OriginalResultID  uuid.UUID `json:"originalResultId"`
	SourceID          uuid.UUID `json:"sourceId"`
	FingerprintSHA256 string    `json:"fingerprintSha256"`
	SnapshotSHA256    string    `json:"snapshotSha256"`
}

func (r *Repository) SourceClosureTx(ctx context.Context, tx pgx.Tx, campaignID uuid.UUID) ([]SourceClosure, error) {
	return sourceClosure(ctx, tx, campaignID)
}
func sourceClosure(ctx context.Context, q sourceQuerier, campaignID uuid.UUID) ([]SourceClosure, error) {
	var protocol string
	err := q.QueryRow(ctx, "SELECT admission_protocol FROM annotation_engine_campaign_binding WHERE campaign_id=$1", campaignID).Scan(&protocol)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && protocol == annotationdomain.OfficialCEProtocol {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if protocol != annotationdomain.ControlledSubmissionProtocol {
		return nil, annotationdomain.ErrSourceIntegrity
	}
	rows, err := q.Query(ctx, `SELECT id,workspace_id,campaign_id,task_id,author_ref,
 COALESCE(provider_binding_ref,''),COALESCE(external_task_id,''),COALESCE(external_annotation_id,''),
 COALESCE(external_revision,''),observation_key,canonical_payload,canonical_payload_sha256,
 normalizer_version,corrected_from_result_id FROM annotation_result WHERE campaign_id=$1 ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]annotationdomain.Result{}
	for rows.Next() {
		var v annotationdomain.Result
		if err := rows.Scan(&v.ID, &v.WorkspaceID, &v.CampaignID, &v.TaskID, &v.AuthorRef, &v.ProviderBindingRef,
			&v.ExternalTaskID, &v.ExternalAnnotationID, &v.ExternalRevision, &v.ObservationKey, &v.CanonicalPayload,
			&v.CanonicalPayloadSHA256, &v.NormalizerVersion, &v.CorrectedFromResultID); err != nil {
			rows.Close()
			return nil, err
		}
		if annotationdomain.SourceDigest(v.CanonicalPayload) != v.CanonicalPayloadSHA256 {
			rows.Close()
			return nil, annotationdomain.ErrSourceIntegrity
		}
		byID[v.ID] = v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	closure := make([]SourceClosure, 0, len(byID))
	for _, result := range byID {
		current := result
		seen := map[uuid.UUID]bool{}
		for current.CorrectedFromResultID != nil {
			if seen[current.ID] {
				return nil, annotationdomain.ErrSourceIntegrity
			}
			seen[current.ID] = true
			parent, ok := byID[*current.CorrectedFromResultID]
			if !ok || parent.WorkspaceID != result.WorkspaceID || parent.CampaignID != result.CampaignID ||
				parent.TaskID != result.TaskID {
				return nil, annotationdomain.ErrSourceIntegrity
			}
			current = parent
		}
		source, err := sourceForResult(ctx, q, current.ID)
		if err != nil {
			return nil, err
		}
		if !sourceMatchesResult(source, current) {
			return nil, annotationdomain.ErrSourceIntegrity
		}
		closure = append(closure, SourceClosure{result.ID, current.ID, source.ID, source.Hash(), source.SnapshotSHA256})
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].ResultID.String() < closure[j].ResultID.String() })
	return closure, nil
}
func (r *Repository) verifySnapshotSourceIntegrity(ctx context.Context, snapshotID uuid.UUID) (bool, error) {
	var campaignID uuid.UUID
	var manifest []byte
	if err := r.pool.QueryRow(ctx, "SELECT campaign_id,manifest FROM annotation_snapshot WHERE id=$1", snapshotID).Scan(&campaignID, &manifest); err != nil {
		return false, err
	}
	closure, err := sourceClosure(ctx, r.pool, campaignID)
	if err != nil {
		return false, err
	}
	if closure == nil {
		return true, nil
	}
	var frozen struct {
		SourceProtocol string          `json:"sourceProtocol"`
		SourceClosure  []SourceClosure `json:"sourceClosure"`
		BatchReceiptID uuid.UUID       `json:"submissionBatchReceiptId"`
	}
	if err := json.Unmarshal(manifest, &frozen); err != nil {
		return false, err
	}
	actual, _ := json.Marshal(closure)
	expected, _ := json.Marshal(frozen.SourceClosure)
	if frozen.SourceProtocol != annotationdomain.ControlledSubmissionProtocol || !bytes.Equal(actual, expected) || frozen.BatchReceiptID == uuid.Nil {
		return false, nil
	}
	var outcome string
	if err := r.pool.QueryRow(ctx, "SELECT outcome FROM annotation_submission_batch_receipt WHERE id=$1 AND campaign_id=$2", frozen.BatchReceiptID, campaignID).Scan(&outcome); err != nil {
		return false, err
	}
	return outcome == "COMPLETE", nil
}
func (r *Repository) LatestCompleteBatchTx(ctx context.Context, tx pgx.Tx, campaignID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	var outcome string
	err := tx.QueryRow(ctx, `SELECT id,outcome FROM annotation_submission_batch_receipt WHERE campaign_id=$1 ORDER BY sequence DESC LIMIT 1`, campaignID).Scan(&id, &outcome)
	if err != nil || outcome != "COMPLETE" {
		return uuid.Nil, fmt.Errorf("controlled batch is unresolved: %w", annotationdomain.ErrSourceIntegrity)
	}
	return id, nil
}
