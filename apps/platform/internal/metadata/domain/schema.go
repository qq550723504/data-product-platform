package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SchemaPlan is a finite server-owned observation contract, not a connector or
// a DatasetVersion quality rule. It cannot grant certification or delivery.
type SchemaPlan struct {
	Enabled         bool           `json:"enabled"`
	WorkspaceID     uuid.UUID      `json:"workspaceId"`
	ResourceID      uuid.UUID      `json:"resourceId"`
	ActorID         uuid.UUID      `json:"actorId"`
	AuthorizationID uuid.UUID      `json:"authorizationId"`
	ConsumerRef     string         `json:"consumerRef"`
	Purpose         string         `json:"purpose"`
	ExternalFQN     string         `json:"externalFqn"`
	RuleRef         string         `json:"ruleRef"`
	RuleVersion     string         `json:"ruleVersion"`
	RuleHash        string         `json:"ruleHash"`
	ConnectorHash   string         `json:"connectorHash"`
	Columns         []SchemaColumn `json:"columns"`
}

type SchemaColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Constraint string `json:"constraint"`
}

func (p SchemaPlan) Validate() error {
	if p.RuleVersion == "" || !isHash(p.RuleHash) || !isHash(p.ConnectorHash) {
		return errors.New("frozen source schema rule identity is required")
	}
	if !p.Enabled || p.WorkspaceID == uuid.Nil || p.ResourceID == uuid.Nil || p.ActorID == uuid.Nil || p.AuthorizationID == uuid.Nil || p.ConsumerRef == "" || p.Purpose == "" || p.ExternalFQN == "" || p.RuleRef == "" {
		return errors.New("source schema plan is incomplete or disabled")
	}
	return ValidateColumns(p.Columns)
}
func isHash(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && value == strings.ToLower(value)
}
func ValidateColumns(columns []SchemaColumn) error {
	if len(columns) == 0 || len(columns) > 100 {
		return errors.New("bounded structural schema required")
	}
	names := map[string]bool{}
	for _, c := range columns {
		if c.Name == "" || c.Type == "" || c.Constraint == "" || names[c.Name] {
			return errors.New("invalid or duplicate schema column")
		}
		names[c.Name] = true
	}
	return nil
}
func Hash(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type SchemaRequest struct {
	ID          uuid.UUID  `json:"id"`
	Plan        SchemaPlan `json:"plan"`
	PlanHash    string     `json:"planHash"`
	BindingID   uuid.UUID  `json:"bindingId"`
	ExternalID  string     `json:"externalId"`
	Key         string     `json:"key"`
	RequestedAt time.Time  `json:"requestedAt"`
}
type SchemaObservation struct {
	ID          uuid.UUID      `json:"id"`
	RequestID   uuid.UUID      `json:"requestId"`
	AttemptID   *uuid.UUID     `json:"attemptId"`
	Status      string         `json:"status"`
	Reason      string         `json:"reason"`
	Columns     []SchemaColumn `json:"columns"`
	SchemaHash  string         `json:"schemaHash"`
	StartedAt   *time.Time     `json:"startedAt"`
	CompletedAt *time.Time     `json:"completedAt"`
	RecordedAt  time.Time      `json:"recordedAt"`
}
