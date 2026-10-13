// Package contentreference defines reference identities; each domain module owns
// its controlled-fixture byte requirement and the loss accepted on release.
package contentreference

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrReference = errors.New("unknown or mismatched controlled fixture reference")

type Reference struct {
	Kind      string
	SubjectID uuid.UUID
	InputName string
}
type Spec struct {
	Kind, Table, Column, FactColumn, Loss, Module string
}

var specs = []Spec{
	{"LINEAGE", "dataset_version_lineage", "input_version_id", "lineage_id", "SOURCE_REPLAY_AND_CONTENT_REVERIFICATION_UNAVAILABLE", "dataset"},
	{"ENTITY_MATCH", "entity_match_job", "input_dataset_version_id", "entity_match_job_id", "ENTITY_MATCH_REPLAY_UNAVAILABLE", "entity"},
	{"EXECUTION_INPUT", "execution_input", "dataset_version_id", "execution_id", "EXECUTION_REPLAY_UNAVAILABLE", "workflow"},
	{"EXECUTION_MAPPING", "execution_mapping_usage", "input_dataset_version_id", "execution_mapping_usage_id", "MAPPING_INPUT_REVERIFICATION_UNAVAILABLE", "workflow"},
	{"QUALITY_RESULT", "quality_result", "dataset_version_id", "quality_result_id", "QUALITY_RECOMPUTATION_UNAVAILABLE", "quality"},
	{"QUALITY_ATTEMPT", "quality_assessment_attempt", "dataset_version_id", "quality_assessment_attempt_id", "QUALITY_ATTEMPT_REPLAY_UNAVAILABLE", "quality"},
	{"CERTIFICATION", "dataset_certification", "dataset_version_id", "dataset_certification_id", "CERTIFICATION_CONTENT_REVERIFICATION_UNAVAILABLE", "certification"},
	{"DELIVERY", "delivery_operation", "dataset_version_id", "delivery_operation_id", "DELIVERY_PAYLOAD_REPLAY_UNAVAILABLE", "delivery"},
}

func Lookup(kind string) (Spec, bool) {
	for _, s := range specs {
		if s.Kind == kind {
			return s, true
		}
	}
	return Spec{}, false
}
func ForRelation(schema, table, column string) (Spec, bool) {
	if schema != "public" {
		return Spec{}, false
	}
	for _, s := range specs {
		if s.Table == table && s.Column == column {
			return s, true
		}
	}
	return Spec{}, false
}
func (r Reference) Validate() error {
	_, ok := Lookup(r.Kind)
	if !ok || r.SubjectID == uuid.Nil || (r.Kind == "EXECUTION_INPUT") != (r.InputName != "") || len(r.InputName) > 128 {
		return ErrReference
	}
	return nil
}

// Owner is installed only by the closed server composition, never selected by
// caller-provided table names. It validates original source identity/workspace;
// lifecycle cannot declare another module's historical relation expendable.
type Owner interface {
	Module() string
	Check(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, Reference) (Spec, error)
}
