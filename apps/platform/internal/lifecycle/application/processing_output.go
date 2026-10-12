package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

const StorageProcessingPut = "PROCESSING_PUT"

type ProcessingOutputIntent struct {
	ID, WorkspaceID, DatasetVersionID, DatasetID, NamespaceID uuid.UUID
	Source                                                    domain.Copy
	Namespace, Key, ContentSHA256                             string
	ByteSize                                                  int64
}
type ProcessingOutputSlot struct {
	DatasetID, NamespaceID uuid.UUID
	Namespace, Key         string
}
type FixtureOutputJournal struct {
	Service      *FixtureSubjectPins
	Slots        []ProcessingOutputSlot
	Observations StorageCallJournal
}

func (j FixtureOutputJournal) ObserveStorageCall(ctx context.Context, o StorageCallObservation) error {
	return j.Observations.ObserveStorageCall(ctx, o)
}
func (j FixtureOutputJournal) StartStorageCall(ctx context.Context, c StorageCallStart) error {
	if j.Service == nil || j.Observations == nil || c.OutputIntent == nil || c.Operation != StorageProcessingPut || c.CopyIntent != nil || c.ReaderPin != nil || c.CopyOperationID != uuid.Nil || c.ID == uuid.Nil {
		return fmt.Errorf("%w: output call shape", ErrStorageScope)
	}
	i := *c.OutputIntent
	allowed := false
	for _, s := range j.Slots {
		allowed = allowed || (s.DatasetID == i.DatasetID && s.NamespaceID == i.NamespaceID && s.Namespace == i.Namespace && s.Key == i.Key)
	}
	if !allowed || i.ID == uuid.Nil || i.Source != c.Copy || i.WorkspaceID != c.Copy.WorkspaceID || i.DatasetVersionID == uuid.Nil || i.ByteSize <= 0 || i.ByteSize > 128<<10 || len(i.ContentSHA256) != 64 {
		return fmt.Errorf("%w: output slot or payload identity", ErrStorageScope)
	}
	o := j.Service.reader.occupancy
	return o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, e := deliveryfence.Lock(ctx, tx, i.WorkspaceID)
		if e != nil {
			return e
		}
		cs, e := o.repo.LockFixtureCopies(ctx, tx, i.WorkspaceID, []uuid.UUID{i.Source.ID})
		if e != nil {
			return e
		}
		if cs[0] != i.Source {
			return fmt.Errorf("%w: output source copy identity", ErrStorageScope)
		}
		caller, at, e := o.authorize(ctx, tx, i.WorkspaceID, CapabilityAdmitReader, nil)
		if e != nil {
			return e
		}
		var dataset uuid.UUID
		var execution, match *uuid.UUID
		var status string
		if e = tx.QueryRow(ctx, `SELECT dataset_id,generated_by_execution_id,generated_by_entity_match_job_id,status FROM dataset_version WHERE id=$1`, i.DatasetVersionID).Scan(&dataset, &execution, &match, &status); e != nil {
			return e
		}
		if dataset != i.DatasetID || (execution == nil) == (match == nil) || (status != "CREATED" && status != "PROCESSING") {
			return fmt.Errorf("%w: allocated output producer identity", ErrStorageScope)
		}
		if execution != nil {
			var valid bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution WHERE id=$1 AND workspace_id=$2 AND status='RUNNING' AND output_dataset_id=$3 AND created_by=$4)`, *execution, i.WorkspaceID, i.DatasetID, caller.ActorID).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return ErrStorageDenied
			}
			if e = j.Service.currentProcessingSources(ctx, tx, i.WorkspaceID, *execution, rev, at); e != nil {
				return e
			}
		} else {
			var valid bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM entity_match_job WHERE id=$1 AND workspace_id=$2 AND input_dataset_version_id=$3 AND output_dataset_id=$4)`, *match, i.WorkspaceID, i.Source.DatasetVersionID, i.DatasetID).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return ErrStorageScope
			}
			if _, e = o.currentReaderDecision(ctx, tx, i.Source, *match, caller, rev, at); e != nil {
				return e
			}
		}
		_, at, e = o.authorize(ctx, tx, i.WorkspaceID, CapabilityAdmitReader, &caller)
		if e != nil {
			return e
		}
		if c.StartedAt.Before(at.Add(-time.Minute)) || c.StartedAt.After(at) {
			return ErrStorageDenied
		}
		if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_processing_output_intent(id,workspace_id,source_copy_id,dataset_version_id,namespace_id,object_key,content_sha256,byte_size,execution_id,entity_match_job_id,actor_id,principal_ref,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, i.ID, i.WorkspaceID, i.Source.ID, i.DatasetVersionID, i.NamespaceID, i.Key, i.ContentSHA256, i.ByteSize, execution, match, caller.ActorID, caller.Subject, at); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_storage_call(id,workspace_id,copy_id,operation,started_at,actor_id,principal_ref,dependency_revision,processing_output_intent_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, i.WorkspaceID, i.Source.ID, c.Operation, c.StartedAt, caller.ActorID, caller.Subject, rev, i.ID); e != nil {
			return e
		}
		return appendOccupancyFacts(ctx, tx, StorageCallStartedEvent, "LIFECYCLE_STORAGE_CALL", c.ID, caller, rev, at, map[string]any{"operation": c.Operation, "processing_output_intent_id": i.ID, "target_dataset_version_id": i.DatasetVersionID})
	})
}

func (s *FixtureSubjectPins) currentProcessingSources(ctx context.Context, tx pgx.Tx, w, execution uuid.UUID, rev int64, at time.Time) error {
	o := s.reader.occupancy
	caller, _, e := o.authorize(ctx, tx, w, CapabilityAdmitReader, nil)
	if e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT p.copy_id FROM lifecycle_reader_pin p WHERE p.execution_id=$1 AND p.workspace_id=$2 AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id) ORDER BY p.copy_id`, execution, w)
	if e != nil {
		return e
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var required int
	if e = tx.QueryRow(ctx, `SELECT count(DISTINCT dataset_version_id) FROM execution_input WHERE execution_id=$1`, execution).Scan(&required); e != nil {
		return e
	}
	if len(ids) == 0 || len(ids) != required {
		return domain.ErrTerminationUnknown
	}
	copies, e := o.repo.LockFixtureCopies(ctx, tx, w, ids)
	if e != nil {
		return e
	}
	for _, c := range copies {
		if _, e = o.currentReaderDecision(ctx, tx, c, execution, caller, rev, at); e != nil {
			return e
		}
	}
	return nil
}

// Source/workspace fence comes before the output DatasetVersion row lock.
func (j FixtureOutputJournal) LockPublication(ctx context.Context, tx pgx.Tx, version uuid.UUID) error {
	var w uuid.UUID
	if e := tx.QueryRow(ctx, `SELECT d.workspace_id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=$1`, version).Scan(&w); e != nil {
		return e
	}
	_, e := deliveryfence.Lock(ctx, tx, w)
	return e
}
func (j FixtureOutputJournal) FinalizePublication(ctx context.Context, tx pgx.Tx, v datasetdomain.DatasetVersion) error {
	o := j.Service.reader.occupancy
	var i ProcessingOutputIntent
	var execution, match *uuid.UUID
	var version string
	var callID uuid.UUID
	e := tx.QueryRow(ctx, `SELECT i.id,i.workspace_id,i.source_copy_id,i.namespace_id,n.namespace,i.object_key,i.content_sha256,i.byte_size,i.execution_id,i.entity_match_job_id,c.id,x.copy_result_version FROM lifecycle_processing_output_intent i JOIN lifecycle_fixture_namespace n ON n.id=i.namespace_id JOIN lifecycle_storage_call c ON c.processing_output_intent_id=i.id JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE i.dataset_version_id=$1 AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.copy_result_version IS NOT NULL`, v.ID).Scan(&i.ID, &i.WorkspaceID, &i.Source.ID, &i.NamespaceID, &i.Namespace, &i.Key, &i.ContentSHA256, &i.ByteSize, &execution, &match, &callID, &version)
	if e != nil {
		return errors.Join(e, ErrStorageUnknown)
	}
	if v.ChecksumValue != i.ContentSHA256 || v.ByteSize == nil || *v.ByteSize != i.ByteSize {
		return ErrStorageContract
	}
	rev, e := deliveryfence.Lock(ctx, tx, i.WorkspaceID)
	if e != nil {
		return e
	}
	caller, at, e := o.authorize(ctx, tx, i.WorkspaceID, CapabilityAdmitReader, nil)
	if e != nil {
		return e
	}
	cs, e := o.repo.LockFixtureCopies(ctx, tx, i.WorkspaceID, []uuid.UUID{i.Source.ID})
	if e != nil {
		return e
	}
	i.Source = cs[0]
	var until time.Time
	if execution != nil {
		if e = j.Service.currentProcessingSources(ctx, tx, i.WorkspaceID, *execution, rev, at); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT min(use_until) FROM lifecycle_reader_pin WHERE execution_id=$1`, *execution).Scan(&until); e != nil {
			return e
		}
	} else {
		decision, e := o.currentReaderDecision(ctx, tx, i.Source, *match, caller, rev, at)
		if e != nil {
			return e
		}
		if decision.UseUntil == nil {
			return ErrStorageDenied
		}
		until = *decision.UseUntil
	}
	if !at.Before(until) {
		return ErrStorageDenied
	}
	copyID := uuid.New()
	if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, copyID, i.WorkspaceID, i.NamespaceID, v.ID, i.Key, version, i.ContentSHA256, i.ByteSize); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_processing_output_ready(intent_id,workspace_id,copy_id,call_id,observed_at) VALUES($1,$2,$3,$4,$5)`, i.ID, i.WorkspaceID, copyID, callID, at); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at) VALUES($1,$2,$3,'CONTROLLED_SYNTHETIC_CUSTODY_AND_USE',$4,$4,$5,$6,$7)`, uuid.New(), i.WorkspaceID, v.ID, until, caller.ActorID, caller.Subject, at); e != nil {
		return e
	}
	return nil
}
