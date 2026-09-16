package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WorkflowStatus string
type ExecutionStatus string

const (
	WorkflowActive   WorkflowStatus = "ACTIVE"
	WorkflowArchived WorkflowStatus = "ARCHIVED"

	ExecutionQueued    ExecutionStatus = "QUEUED"
	ExecutionRunning   ExecutionStatus = "RUNNING"
	ExecutionSucceeded ExecutionStatus = "SUCCEEDED"
	ExecutionFailed    ExecutionStatus = "FAILED"
	ExecutionCancelled ExecutionStatus = "CANCELLED"
)

var (
	ErrInvalidWorkspace        = errors.New("workspace id is required")
	ErrInvalidWorkflowCode     = errors.New("workflow code is required")
	ErrInvalidWorkflowName     = errors.New("workflow name is required")
	ErrInvalidWorkflowVersion  = errors.New("workflow version is required")
	ErrInvalidDefinition       = errors.New("workflow definition reference and sha256 are required")
	ErrInvalidTargetPeriod     = errors.New("target period must be YYYY-MM")
	ErrInvalidExecutionInput   = errors.New("execution requires at least one input DatasetVersion")
	ErrInvalidExecutionOutput  = errors.New("execution output Dataset is required")
	ErrInvalidTransition       = errors.New("invalid execution state transition")
	ErrRetryRequiresFailure    = errors.New("only failed or cancelled execution can be retried")
)

type Workflow struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Code        string
	Name        string
	Description string
	Status      WorkflowStatus
	CreatedAt   time.Time
	CreatedBy   *uuid.UUID
}

type WorkflowVersion struct {
	ID               uuid.UUID
	WorkflowID       uuid.UUID
	Version          string
	DefinitionRef    string
	DefinitionSHA256 string
	Definition       map[string]any
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

type InputBinding struct {
	Name             string
	DatasetVersionID uuid.UUID
}

type Execution struct {
	ID                     uuid.UUID
	WorkspaceID            uuid.UUID
	WorkflowVersionID      uuid.UUID
	OutputDatasetID        uuid.UUID
	OutputDatasetVersionID *uuid.UUID
	TargetPeriod           string
	Status                 ExecutionStatus
	Attempt                int
	RetryOfExecutionID     *uuid.UUID
	EngineType             string
	EngineExecutionID      string
	ErrorCode              string
	ErrorMessage           string
	Metrics                map[string]any
	Inputs                 []InputBinding
	CreatedAt              time.Time
	CreatedBy              *uuid.UUID
	StartedAt              *time.Time
	FinishedAt             *time.Time
}

func NewWorkflow(workspaceID uuid.UUID, code, name, description string, createdBy *uuid.UUID) (Workflow, error) {
	if workspaceID == uuid.Nil {
		return Workflow{}, ErrInvalidWorkspace
	}
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if code == "" {
		return Workflow{}, ErrInvalidWorkflowCode
	}
	if name == "" {
		return Workflow{}, ErrInvalidWorkflowName
	}
	return Workflow{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Code:        code,
		Name:        name,
		Description: strings.TrimSpace(description),
		Status:      WorkflowActive,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   createdBy,
	}, nil
}

func NewWorkflowVersion(workflowID uuid.UUID, version, definitionRef, definitionSHA256 string, definition map[string]any, createdBy *uuid.UUID) (WorkflowVersion, error) {
	if workflowID == uuid.Nil {
		return WorkflowVersion{}, ErrInvalidWorkflowVersion
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return WorkflowVersion{}, ErrInvalidWorkflowVersion
	}
	definitionRef = strings.TrimSpace(definitionRef)
	definitionSHA256 = strings.TrimSpace(definitionSHA256)
	if definitionRef == "" || definitionSHA256 == "" || len(definition) == 0 {
		return WorkflowVersion{}, ErrInvalidDefinition
	}
	return WorkflowVersion{
		ID:               uuid.New(),
		WorkflowID:       workflowID,
		Version:          version,
		DefinitionRef:    definitionRef,
		DefinitionSHA256: definitionSHA256,
		Definition:       definition,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        createdBy,
	}, nil
}

func NewExecution(workspaceID, workflowVersionID, outputDatasetID uuid.UUID, targetPeriod string, inputs []InputBinding, createdBy *uuid.UUID) (Execution, error) {
	if workspaceID == uuid.Nil || workflowVersionID == uuid.Nil {
		return Execution{}, ErrInvalidWorkspace
	}
	if outputDatasetID == uuid.Nil {
		return Execution{}, ErrInvalidExecutionOutput
	}
	if !validTargetPeriod(targetPeriod) {
		return Execution{}, ErrInvalidTargetPeriod
	}
	if len(inputs) == 0 {
		return Execution{}, ErrInvalidExecutionInput
	}
	seen := map[string]struct{}{}
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" || input.DatasetVersionID == uuid.Nil {
			return Execution{}, ErrInvalidExecutionInput
		}
		if _, exists := seen[name]; exists {
			return Execution{}, ErrInvalidExecutionInput
		}
		seen[name] = struct{}{}
	}
	return Execution{
		ID:                uuid.New(),
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersionID,
		OutputDatasetID:   outputDatasetID,
		TargetPeriod:      targetPeriod,
		Status:            ExecutionQueued,
		Attempt:           1,
		EngineType:        "NATIVE",
		Metrics:           map[string]any{},
		Inputs:            append([]InputBinding(nil), inputs...),
		CreatedAt:         time.Now().UTC(),
		CreatedBy:         createdBy,
	}, nil
}

func (e *Execution) Start(engineExecutionID string) error {
	if e.Status != ExecutionQueued {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	e.Status = ExecutionRunning
	e.StartedAt = &now
	e.EngineExecutionID = strings.TrimSpace(engineExecutionID)
	return nil
}

func (e *Execution) Succeed(outputVersionID uuid.UUID, metrics map[string]any) error {
	if e.Status != ExecutionRunning || outputVersionID == uuid.Nil {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	e.Status = ExecutionSucceeded
	e.OutputDatasetVersionID = &outputVersionID
	e.FinishedAt = &now
	e.ErrorCode = ""
	e.ErrorMessage = ""
	if metrics == nil {
		metrics = map[string]any{}
	}
	e.Metrics = metrics
	return nil
}

func (e *Execution) Fail(code, message string, metrics map[string]any) error {
	if e.Status != ExecutionRunning && e.Status != ExecutionQueued {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	e.Status = ExecutionFailed
	e.FinishedAt = &now
	e.ErrorCode = strings.TrimSpace(code)
	e.ErrorMessage = strings.TrimSpace(message)
	if metrics == nil {
		metrics = map[string]any{}
	}
	e.Metrics = metrics
	return nil
}

func (e *Execution) Cancel() error {
	if e.Status != ExecutionQueued && e.Status != ExecutionRunning {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	e.Status = ExecutionCancelled
	e.FinishedAt = &now
	return nil
}

func (e Execution) Retry(createdBy *uuid.UUID) (Execution, error) {
	if e.Status != ExecutionFailed && e.Status != ExecutionCancelled {
		return Execution{}, ErrRetryRequiresFailure
	}
	retryOf := e.ID
	return Execution{
		ID:                 uuid.New(),
		WorkspaceID:        e.WorkspaceID,
		WorkflowVersionID:  e.WorkflowVersionID,
		OutputDatasetID:    e.OutputDatasetID,
		TargetPeriod:       e.TargetPeriod,
		Status:             ExecutionQueued,
		Attempt:            e.Attempt + 1,
		RetryOfExecutionID: &retryOf,
		EngineType:         e.EngineType,
		Metrics:            map[string]any{},
		Inputs:             append([]InputBinding(nil), e.Inputs...),
		CreatedAt:          time.Now().UTC(),
		CreatedBy:          createdBy,
	}, nil
}

func validTargetPeriod(value string) bool {
	if len(value) != 7 || value[4] != '-' {
		return false
	}
	_, err := time.Parse("2006-01", value)
	return err == nil
}
