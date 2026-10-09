package infrastructure

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"reflect"
	"sort"
)

func (r *Repository) PrepareSubmissionBatchTx(ctx context.Context, tx pgx.Tx, campaignID, workspaceID uuid.UUID, expected []annotationdomain.SubmissionExpectation, budget int) (bool, error) {
	expected = append([]annotationdomain.SubmissionExpectation(nil), expected...)
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].TaskID != expected[j].TaskID {
			return expected[i].TaskID.String() < expected[j].TaskID.String()
		}
		if expected[i].AssignmentID != expected[j].AssignmentID {
			return expected[i].AssignmentID < expected[j].AssignmentID
		}
		return expected[i].Revision < expected[j].Revision
	})
	tasks := make([]uuid.UUID, 0, len(expected))
	assignments := make([]string, 0, len(expected))
	revisions := make([]int64, 0, len(expected))
	for _, e := range expected {
		tasks = append(tasks, e.TaskID)
		assignments = append(assignments, e.AssignmentID)
		revisions = append(revisions, e.Revision)
	}
	var oldTasks []uuid.UUID
	var oldAssignments []string
	var oldRevisions []int64
	var oldBudget int
	var oldWorkspace uuid.UUID
	err := tx.QueryRow(ctx, `SELECT workspace_id,expected_task_ids,expected_assignment_ids,expected_revisions,page_budget FROM annotation_submission_batch WHERE campaign_id=$1`, campaignID).Scan(&oldWorkspace, &oldTasks, &oldAssignments, &oldRevisions, &oldBudget)
	if err == nil {
		if oldWorkspace != workspaceID || oldBudget != budget || !reflect.DeepEqual(tasks, oldTasks) || !reflect.DeepEqual(assignments, oldAssignments) || !reflect.DeepEqual(revisions, oldRevisions) {
			return false, annotationdomain.ErrSourceConflict
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO annotation_submission_batch(campaign_id,workspace_id,expected_task_ids,expected_assignment_ids,expected_revisions,page_budget) VALUES($1,$2,$3,$4,$5,$6)`, campaignID, workspaceID, tasks, assignments, revisions, budget)
	return err == nil, err
}
func (r *Repository) SubmissionBatch(ctx context.Context, campaignID uuid.UUID) ([]annotationdomain.SubmissionExpectation, int, error) {
	var tasks []uuid.UUID
	var assignments []string
	var revisions []int64
	var budget int
	err := r.pool.QueryRow(ctx, `SELECT expected_task_ids,expected_assignment_ids,expected_revisions,page_budget FROM annotation_submission_batch WHERE campaign_id=$1`, campaignID).Scan(&tasks, &assignments, &revisions, &budget)
	if err != nil {
		return nil, 0, err
	}
	expected := make([]annotationdomain.SubmissionExpectation, 0, len(tasks))
	for i, id := range tasks {
		expected = append(expected, annotationdomain.SubmissionExpectation{TaskID: id, AssignmentID: assignments[i], Revision: revisions[i]})
	}
	return expected, budget, nil
}
func (r *Repository) AppendBatchReceiptTx(ctx context.Context, tx pgx.Tx, campaignID uuid.UUID, complete bool, scanID uuid.UUID, sources []annotationdomain.SourceObservation) (uuid.UUID, error) {
	if _, err := r.LockCampaignTx(ctx, tx, campaignID); err != nil {
		return uuid.Nil, err
	}
	ids := make([]uuid.UUID, 0, len(sources))
	fingerprints := make([]string, 0, len(sources))
	for _, s := range sources {
		ids = append(ids, s.ID)
		fingerprints = append(fingerprints, s.Hash())
	}
	outcome := "UNRESOLVED"
	var started any
	if complete {
		outcome = "COMPLETE"
		started = scanID
	}
	id := uuid.New()
	_, err := tx.Exec(ctx, `INSERT INTO annotation_submission_batch_receipt(id,campaign_id,sequence,outcome,scan_started_receipt_id,source_ids,fingerprints) SELECT $1,$2,COALESCE(max(sequence),0)+1,$3,$4,$5,$6 FROM annotation_submission_batch_receipt WHERE campaign_id=$2`, id, campaignID, outcome, started, ids, fingerprints)
	return id, err
}

type SourceReceiptAppend struct {
	ID          uuid.UUID
	Created     bool
	Disposition string
}

func (r *Repository) AppendSourceReceiptTx(ctx context.Context, tx pgx.Tx, s annotationdomain.SourceObservation, attemptID uuid.UUID, disposition string) (SourceReceiptAppend, error) {
	if _, err := r.LockCampaignTx(ctx, tx, s.CampaignID); err != nil {
		return SourceReceiptAppend{}, err
	}
	if disposition == "OBSERVED" {
		task, err := r.GetTaskTx(ctx, tx, s.TaskID)
		if err != nil {
			return SourceReceiptAppend{}, err
		}
		campaign, err := r.GetCampaignTx(ctx, tx, s.CampaignID)
		if err != nil {
			return SourceReceiptAppend{}, err
		}
		if task.Status == annotationdomain.TaskReviewed || campaign.Status == annotationdomain.CampaignSealed {
			disposition = "LATE"
		}
	}
	var sourceID any = s.ID
	if err := r.StoreSourceTx(ctx, tx, s); err != nil {
		if !errors.Is(err, annotationdomain.ErrSourceConflict) {
			return SourceReceiptAppend{}, err
		}
		sourceID = nil
		disposition = "CONFLICT"
	}
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(attemptID.String()+":"+s.ID.String()+":"+s.Hash()+":"+disposition))
	var physicalID any = attemptID
	if attemptID == uuid.Nil {
		physicalID = nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO annotation_source_receipt(id,workspace_id,campaign_id,task_id,physical_attempt_id,source_id,incoming_fingerprint_sha256,snapshot,canonical_payload,disposition) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(id) DO NOTHING`, id, s.WorkspaceID, s.CampaignID, s.TaskID, physicalID, sourceID, s.Hash(), s.Snapshot, s.CanonicalPayload, disposition)
	return SourceReceiptAppend{ID: id, Created: tag.RowsAffected() == 1, Disposition: disposition}, err
}
