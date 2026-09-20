package hop

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
)

const maxManagedOutputBytes = 128 << 20

type ObjectStore interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
	Bucket() string
}

type Bridge struct {
	engine        workflowapp.ManagedProcessingEngine
	artifactRoot  string
	tx            *transaction.Manager
	datasetRepo   *datasetinfra.PostgresRepository
	datasetWriter *datasetapp.UploadVersionService
	store         ObjectStore
}

func NewBridge(engine workflowapp.ManagedProcessingEngine, artifactRoot string, tx *transaction.Manager, datasetRepo *datasetinfra.PostgresRepository, datasetWriter *datasetapp.UploadVersionService, store ObjectStore) (*Bridge, error) {
	if engine == nil || tx == nil || datasetRepo == nil || datasetWriter == nil || store == nil {
		return nil, fmt.Errorf("Hop bridge dependencies are required")
	}
	root, err := filepath.Abs(strings.TrimSpace(artifactRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve Hop artifact root: %w", err)
	}
	return &Bridge{
		engine:        engine,
		artifactRoot:  root,
		tx:            tx,
		datasetRepo:   datasetRepo,
		datasetWriter: datasetWriter,
		store:         store,
	}, nil
}

func (b *Bridge) EngineType() string { return "HOP" }

func (b *Bridge) Submit(ctx context.Context, request workflowapp.ProcessingRequest) (workflowapp.EngineRun, error) {
	cfg, err := b.config(request)
	if err != nil {
		return workflowapp.EngineRun{}, err
	}
	definition, err := b.loadDefinition(cfg)
	if err != nil {
		return workflowapp.EngineRun{}, err
	}
	parameters, _, err := b.executionParameters(ctx, request, cfg)
	if err != nil {
		return workflowapp.EngineRun{}, err
	}

	run, err := b.engine.Submit(ctx, workflowapp.ManagedSubmitRequest{
		Name:          cfg.Name,
		DefinitionRef: cfg.DefinitionRef,
		Definition:    wrapPipelineConfiguration(definition),
		ContentType:   cfg.ContentType,
		Parameters:    parameters,
	})
	if err != nil {
		return workflowapp.EngineRun{}, err
	}
	if run.Metrics == nil {
		run.Metrics = map[string]any{}
	}
	_, outputURI := b.stagingOutput(request, cfg)
	run.Metrics["stagingOutputUri"] = outputURI
	run.Metrics["workflowDefinitionRef"] = request.WorkflowVersion.DefinitionRef
	run.Metrics["hopDefinitionRef"] = cfg.DefinitionRef
	return run, nil
}

func (b *Bridge) Status(ctx context.Context, request workflowapp.ProcessingRequest, runID string) (workflowapp.EngineRun, error) {
	cfg, err := b.config(request)
	if err != nil {
		return workflowapp.EngineRun{}, err
	}
	return b.engine.Status(ctx, cfg.Name, runID)
}

func (b *Bridge) Finalize(ctx context.Context, request workflowapp.ProcessingRequest, run workflowapp.EngineRun) (workflowapp.ProcessingResult, error) {
	var result workflowapp.ProcessingResult
	lockKey := "managed-output-finalize:" + request.ExecutionID.String()
	err := b.tx.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
		var err error
		result, err = b.finalizeLocked(ctx, request, run)
		return err
	})
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	return result, nil
}

func (b *Bridge) finalizeLocked(ctx context.Context, request workflowapp.ProcessingRequest, run workflowapp.EngineRun) (workflowapp.ProcessingResult, error) {
	cfg, err := b.config(request)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}

	if existing, err := b.datasetRepo.FindVersionByExecution(ctx, request.ExecutionID); err == nil {
		if existing.DatasetID != request.OutputDatasetID {
			return workflowapp.ProcessingResult{}, fmt.Errorf("execution %s already generated DatasetVersion %s for dataset %s", request.ExecutionID, existing.ID, existing.DatasetID)
		}
		switch existing.Status {
		case datasetdomain.VersionReady, datasetdomain.VersionSuperseded:
			if err := b.ensureLineage(ctx, request, existing.ID); err != nil {
				return workflowapp.ProcessingResult{}, err
			}
			return workflowapp.ProcessingResult{
				OutputDatasetVersionID: existing.ID,
				EngineExecutionID:      run.ID,
				Metrics:                finalizeMetrics(existing, run, true),
			}, nil
		case datasetdomain.VersionCreated, datasetdomain.VersionProcessing, datasetdomain.VersionFailed:
			// The execution key identifies a recoverable half-product. Re-read the
			// staged engine output and send it through UploadVersionService so the
			// same version row can be published rather than retrying a permanent
			// "not usable" error forever.
		case datasetdomain.VersionInvalid:
			return workflowapp.ProcessingResult{}, fmt.Errorf("execution %s output DatasetVersion %s is not usable: %s", request.ExecutionID, existing.ID, existing.Status)
		default:
			return workflowapp.ProcessingResult{}, fmt.Errorf("execution %s output DatasetVersion %s has unsupported status: %s", request.ExecutionID, existing.ID, existing.Status)
		}
	} else if !errors.Is(err, datasetinfra.ErrNotFound) {
		return workflowapp.ProcessingResult{}, err
	}

	_, outputURI := b.stagingOutput(request, cfg)
	reader, err := b.store.Get(ctx, outputURI)
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("read Hop staged output %s: %w", outputURI, err)
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxManagedOutputBytes+1))
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("read Hop output: %w", err)
	}
	if len(content) == 0 {
		return workflowapp.ProcessingResult{}, fmt.Errorf("Hop staged output %s is empty", outputURI)
	}
	if len(content) > maxManagedOutputBytes {
		return workflowapp.ProcessingResult{}, fmt.Errorf("Hop staged output exceeds %d bytes", maxManagedOutputBytes)
	}

	outputVersion, err := b.datasetWriter.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:              request.OutputDatasetID,
		Filename:               cfg.OutputFilename,
		ContentType:            cfg.OutputContentType,
		Content:                content,
		TraceID:                request.ExecutionID.String(),
		GeneratedByExecutionID: &request.ExecutionID,
		Metadata: map[string]any{
			"workflowVersionId":   request.WorkflowVersion.ID,
			"workflowVersion":     request.WorkflowVersion.Version,
			"engineType":          "HOP",
			"engineExecutionId":   run.ID,
			"hopDefinitionRef":    cfg.DefinitionRef,
			"stagingOutputUri":    outputURI,
			"targetPeriod":        request.TargetPeriod,
			"remoteEngineMetrics": run.Metrics,
		},
	})
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("import Hop output as DatasetVersion: %w", err)
	}
	if err := b.ensureLineage(ctx, request, outputVersion.ID); err != nil {
		return workflowapp.ProcessingResult{}, err
	}

	return workflowapp.ProcessingResult{
		OutputDatasetVersionID: outputVersion.ID,
		EngineExecutionID:      run.ID,
		Metrics:                finalizeMetrics(outputVersion, run, false),
	}, nil
}

func (b *Bridge) ensureLineage(ctx context.Context, request workflowapp.ProcessingRequest, outputVersionID uuid.UUID) error {
	return b.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, input := range request.Inputs {
			if err := b.datasetRepo.AddLineage(ctx, tx, outputVersionID, input.DatasetVersionID, "DERIVED_FROM", &request.ExecutionID); err != nil {
				return err
			}
		}
		return nil
	})
}

type managedConfig struct {
	Name              string
	DefinitionRef     string
	DefinitionSHA256  string
	ContentType       string
	OutputFilename    string
	OutputContentType string
	Parameters        map[string]string
}

func (b *Bridge) config(request workflowapp.ProcessingRequest) (managedConfig, error) {
	spec, ok := asMap(request.WorkflowVersion.Definition["spec"])
	if !ok {
		return managedConfig{}, fmt.Errorf("workflow version %s has no spec", request.WorkflowVersion.ID)
	}
	managed, ok := asMap(spec["managedExecution"])
	if !ok {
		return managedConfig{}, fmt.Errorf("workflow version %s has no managedExecution config", request.WorkflowVersion.ID)
	}
	engine := strings.ToUpper(strings.TrimSpace(stringValue(managed["engine"])))
	if engine != "HOP" {
		return managedConfig{}, fmt.Errorf("managed execution engine must be HOP, got %q", engine)
	}
	cfg := managedConfig{
		Name:              strings.TrimSpace(stringValue(managed["name"])),
		DefinitionRef:     strings.TrimSpace(stringValue(managed["definitionRef"])),
		DefinitionSHA256:  strings.ToLower(strings.TrimSpace(stringValue(managed["definitionSha256"]))),
		ContentType:       strings.TrimSpace(stringValue(managed["contentType"])),
		OutputFilename:    strings.TrimSpace(stringValue(managed["outputFilename"])),
		OutputContentType: strings.TrimSpace(stringValue(managed["outputContentType"])),
		Parameters:        map[string]string{},
	}
	if cfg.Name == "" || cfg.DefinitionRef == "" || cfg.DefinitionSHA256 == "" {
		return managedConfig{}, fmt.Errorf("Hop managedExecution requires name, definitionRef and definitionSha256")
	}
	if cfg.ContentType == "" {
		cfg.ContentType = "application/xml"
	}
	if cfg.OutputFilename == "" {
		cfg.OutputFilename = "managed-output.csv"
	}
	if cfg.OutputContentType == "" {
		cfg.OutputContentType = "text/csv; charset=utf-8"
	}
	if values, ok := asMap(managed["parameters"]); ok {
		for key, value := range values {
			cfg.Parameters[key] = fmt.Sprint(value)
		}
	}
	return cfg, nil
}

func (b *Bridge) loadDefinition(cfg managedConfig) ([]byte, error) {
	if filepath.IsAbs(cfg.DefinitionRef) {
		return nil, fmt.Errorf("Hop definitionRef must be relative to the repository artifact root")
	}
	path := filepath.Join(b.artifactRoot, filepath.Clean(cfg.DefinitionRef))
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve Hop definition path: %w", err)
	}
	rootPrefix := b.artifactRoot + string(os.PathSeparator)
	if absolute != b.artifactRoot && !strings.HasPrefix(absolute, rootPrefix) {
		return nil, fmt.Errorf("Hop definitionRef escapes artifact root")
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read Hop definition %s: %w", cfg.DefinitionRef, err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	if !strings.EqualFold(checksum, cfg.DefinitionSHA256) {
		return nil, fmt.Errorf("Hop definition checksum mismatch for %s: expected %s got %s", cfg.DefinitionRef, cfg.DefinitionSHA256, checksum)
	}
	return content, nil
}

func (b *Bridge) executionParameters(ctx context.Context, request workflowapp.ProcessingRequest, cfg managedConfig) (map[string]string, string, error) {
	params := make(map[string]string, len(cfg.Parameters)+len(request.Inputs)*2+4)
	for key, value := range cfg.Parameters {
		params[key] = value
	}
	params["CORE_EXECUTION_ID"] = request.ExecutionID.String()
	params["TARGET_PERIOD"] = request.TargetPeriod
	outputBase, outputURI := b.stagingOutput(request, cfg)
	params["OUTPUT_BASE_URI"] = outputBase
	params["OUTPUT_URI"] = outputURI

	seenPrefixes := make(map[string]string, len(request.Inputs))
	for _, input := range request.Inputs {
		version, err := b.datasetRepo.GetVersion(ctx, input.DatasetVersionID)
		if err != nil {
			return nil, "", fmt.Errorf("load input %s DatasetVersion: %w", input.Name, err)
		}
		if version.Status != datasetdomain.VersionReady && version.Status != datasetdomain.VersionSuperseded {
			return nil, "", fmt.Errorf("input %s DatasetVersion %s is not usable: %s", input.Name, version.ID, version.Status)
		}
		normalized := parameterName(input.Name)
		if normalized == "" {
			return nil, "", fmt.Errorf("input name %q does not produce a valid managed parameter name", input.Name)
		}
		if previous, exists := seenPrefixes[normalized]; exists && previous != input.Name {
			return nil, "", fmt.Errorf("input names %q and %q collide after managed parameter normalization (%s)", previous, input.Name, normalized)
		}
		seenPrefixes[normalized] = input.Name
		prefix := "INPUT_" + normalized
		params[prefix+"_URI"] = version.StorageURI
		params[prefix+"_VERSION_ID"] = version.ID.String()
	}
	return params, outputURI, nil
}

func (b *Bridge) stagingOutput(request workflowapp.ProcessingRequest, cfg managedConfig) (string, string) {
	extension := filepath.Ext(cfg.OutputFilename)
	if extension == "" {
		extension = ".csv"
	}
	base := fmt.Sprintf("s3://%s/managed-executions/%s/output", b.store.Bucket(), request.ExecutionID)
	return base, base + extension
}

func finalizeMetrics(version datasetdomain.DatasetVersion, run workflowapp.EngineRun, reused bool) map[string]any {
	metrics := make(map[string]any, len(run.Metrics)+6)
	for key, value := range run.Metrics {
		metrics[key] = value
	}
	metrics["engineType"] = "HOP"
	metrics["externalExecutionId"] = run.ID
	metrics["outputDatasetVersionId"] = version.ID
	metrics["outputRowCount"] = version.RowCount
	metrics["outputByteSize"] = version.ByteSize
	metrics["outputReused"] = reused
	return metrics
}

func wrapPipelineConfiguration(definition []byte) []byte {
	trimmed := strings.TrimSpace(string(definition))
	if strings.HasPrefix(trimmed, "<pipeline_configuration") || strings.Contains(trimmed, "<pipeline_configuration>") {
		return definition
	}
	if strings.HasPrefix(trimmed, "<?xml") {
		if end := strings.Index(trimmed, "?>"); end >= 0 {
			trimmed = strings.TrimSpace(trimmed[end+2:])
		}
	}
	return []byte("<pipeline_configuration>" + trimmed + `<pipeline_execution_configuration>
<pass_export>N</pass_export>
<parameters></parameters>
<variables></variables>
<log_level>Basic</log_level>
<log_file>N</log_file>
<log_filename></log_filename>
<log_file_append>N</log_file_append>
<create_parent_folder>Y</create_parent_folder>
<clear_log>Y</clear_log>
<show_subcomponents>Y</show_subcomponents>
<run_configuration>local</run_configuration>
</pipeline_execution_configuration>
<metastore_json></metastore_json>
</pipeline_configuration>`)
}

func parameterName(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func asMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

var _ workflowapp.ManagedExecutionBridge = (*Bridge)(nil)
