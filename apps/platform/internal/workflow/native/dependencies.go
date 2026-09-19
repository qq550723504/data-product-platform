package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/matching"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/indicator"
)

const (
	enterpriseResolutionInput = "enterprise_resolution"
	companyPolicyDependency   = "company_match_policy"
	indicatorPolicyDependency = "indicator_policy"
)

type mappingUsageKey struct {
	InputName string
	SourceRef string
	SourceKey string
}

type preparedNativeDependencies struct {
	ResolutionVersionID uuid.UUID
	Mappings            map[mappingUsageKey]entitydomain.MappingDecision
	CompanyPolicy       matching.Policy
	IndicatorPolicy     indicator.Policy
}

var errDependencyPreparationAlreadyExists = errors.New("execution dependency preparation already exists")

func (e *Engine) prepareDependencies(ctx context.Context, request workflowapp.ProcessingRequest, bindings map[string]uuid.UUID, enterpriseRows, leaseRows, energyRows []map[string]string, enterpriseRef, leaseRef, energyRef string) (preparedNativeDependencies, error) {
	resolutionVersionID := bindings[enterpriseResolutionInput]
	if resolutionVersionID == uuid.Nil {
		return preparedNativeDependencies{}, fmt.Errorf("native enterprise activity workflow requires input %s", enterpriseResolutionInput)
	}

	if existing, found, err := e.workflowRepo.GetDependencyPreparation(ctx, request.ExecutionID); err != nil {
		return preparedNativeDependencies{}, err
	} else if found {
		return e.restorePreparedDependencies(existing, resolutionVersionID)
	}

	companyPolicy, companyContent, err := loadMatchingPolicy(e.industryPackRoot, companyPolicyRef)
	if err != nil {
		return preparedNativeDependencies{}, err
	}
	indicatorPolicy, indicatorContent, err := loadIndicatorPolicy(e.industryPackRoot, indicatorPolicyRef)
	if err != nil {
		return preparedNativeDependencies{}, err
	}

	job, err := e.entityRepo.GetSuccessfulResolutionJobForOutput(
		ctx, request.WorkspaceID, bindings["enterprise_raw"], resolutionVersionID, "CSV", enterpriseRef,
	)
	if err != nil {
		return preparedNativeDependencies{}, fmt.Errorf("validate enterprise_resolution: %w", err)
	}
	if len(job.PolicyContent) == 0 || job.PolicyContentSHA256 == "" || hashBytes(job.PolicyContent) != job.PolicyContentSHA256 {
		return preparedNativeDependencies{}, fmt.Errorf("enterprise_resolution %s has no verifiable matching policy content", resolutionVersionID)
	}
	resolutionDecisions, err := e.entityRepo.ListResolutionOutputDecisions(ctx, request.WorkspaceID, resolutionVersionID)
	if err != nil {
		return preparedNativeDependencies{}, fmt.Errorf("load enterprise_resolution decisions: %w", err)
	}

	enterpriseMappings := make(map[mappingUsageKey]entitydomain.MappingDecision, len(resolutionDecisions))
	for sourceKey, decision := range resolutionDecisions {
		enterpriseMappings[mappingUsageKey{InputName: "enterprise_raw", SourceRef: enterpriseRef, SourceKey: sourceKey}] = decision
	}
	companies, _, err := e.buildCanonicalCompanies(enterpriseRows, enterpriseRef, companyPolicy, enterpriseMappings)
	if err != nil {
		return preparedNativeDependencies{}, err
	}
	mappings := enterpriseMappings
	usages := make([]workflowdomain.MappingUsage, 0)
	usageKeys := make(map[mappingUsageKey]struct{})
	addUsage := func(inputName string, inputVersionID uuid.UUID, sourceRef, sourceKey string, decision entitydomain.MappingDecision) error {
		key := mappingUsageKey{InputName: inputName, SourceRef: sourceRef, SourceKey: sourceKey}
		if previous, exists := mappings[key]; exists {
			if previous.ID != decision.ID {
				return fmt.Errorf("source %s/%s resolved to conflicting decisions", sourceRef, sourceKey)
			}
		} else {
			mappings[key] = decision
		}
		if decision.WorkspaceID != request.WorkspaceID || decision.EntityID == uuid.Nil || decision.SourceOrigin == entitydomain.OriginUnknown ||
			(decision.Status != entitydomain.MappingAutoMatched && decision.Status != entitydomain.MappingConfirmed) {
			return fmt.Errorf("source %s/%s has an unusable mapping decision", sourceRef, sourceKey)
		}
		if _, exists := usageKeys[key]; exists {
			return nil
		}
		usageKeys[key] = struct{}{}
		usages = append(usages, workflowdomain.MappingUsage{
			ID:                         uuid.New(),
			ExecutionID:                request.ExecutionID,
			WorkspaceID:                request.WorkspaceID,
			InputName:                  inputName,
			InputDatasetVersionID:      inputVersionID,
			ResolutionDatasetVersionID: resolutionVersionID,
			SourceType:                 decision.SourceType,
			SourceRef:                  decision.SourceRef,
			SourceKey:                  decision.SourceKey,
			DecisionID:                 decision.ID,
			EntityID:                   decision.EntityID,
		})
		return nil
	}

	for _, row := range enterpriseRows {
		sourceKey := strings.TrimSpace(row["source_company_id"])
		decision, ok := resolutionDecisions[sourceKey]
		if !ok || decision.SourceJobID == nil || *decision.SourceJobID != job.ID {
			return preparedNativeDependencies{}, fmt.Errorf("enterprise source %s has no exact decision in enterprise_resolution %s", sourceKey, resolutionVersionID)
		}
		if decision.SourceType != "CSV" || decision.SourceRef != enterpriseRef {
			return preparedNativeDependencies{}, fmt.Errorf("enterprise source %s decision scope does not match enterprise_raw", sourceKey)
		}
		if err := addUsage("enterprise_raw", bindings["enterprise_raw"], enterpriseRef, sourceKey, decision); err != nil {
			return preparedNativeDependencies{}, err
		}
	}

	type aliasSource struct {
		inputName      string
		inputVersionID uuid.UUID
		sourceRef      string
		sourceKey      string
		companyName    string
	}
	aliasSources := make([]aliasSource, 0, len(leaseRows)+len(energyRows))
	appendAliasSources := func(inputName string, inputVersionID uuid.UUID, rows []map[string]string, sourceRef string) error {
		for _, row := range rows {
			sourceKey := strings.TrimSpace(row["source_company_id"])
			if sourceKey == "" {
				return fmt.Errorf("%s record is missing source_company_id", inputName)
			}
			aliasSources = append(aliasSources, aliasSource{
				inputName: inputName, inputVersionID: inputVersionID, sourceRef: sourceRef,
				sourceKey: sourceKey, companyName: row["company_name"],
			})
		}
		return nil
	}
	if err := appendAliasSources("lease_raw", bindings["lease_raw"], leaseRows, leaseRef); err != nil {
		return preparedNativeDependencies{}, err
	}
	if err := appendAliasSources("energy_raw", bindings["energy_raw"], energyRows, energyRef); err != nil {
		return preparedNativeDependencies{}, err
	}
	sort.SliceStable(aliasSources, func(i, j int) bool {
		if aliasSources[i].sourceRef != aliasSources[j].sourceRef {
			return aliasSources[i].sourceRef < aliasSources[j].sourceRef
		}
		if aliasSources[i].sourceKey != aliasSources[j].sourceKey {
			return aliasSources[i].sourceKey < aliasSources[j].sourceKey
		}
		if aliasSources[i].inputName != aliasSources[j].inputName {
			return aliasSources[i].inputName < aliasSources[j].inputName
		}
		return aliasSources[i].companyName < aliasSources[j].companyName
	})

	// Alias decisions and all usage rows are written under one transaction. A
	// concurrent worker for this Execution rechecks the preparation row while
	// holding the same advisory transaction lock and reuses the committed set.
	preparation := workflowdomain.DependencyPreparation{
		ExecutionID:        request.ExecutionID,
		WorkspaceID:        request.WorkspaceID,
		BindingFingerprint: dependencyFingerprint(request, resolutionVersionID, companyContent, indicatorContent),
		MappingUsageCount:  len(usages),
		Status:             "PREPARED",
	}
	bindingsToPersist := []workflowdomain.DependencyBinding{
		{
			ID: uuid.New(), Name: enterpriseResolutionInput, ExecutionID: request.ExecutionID, WorkspaceID: request.WorkspaceID,
			DatasetVersionID: &resolutionVersionID, Reference: job.PolicyRef + " (entity_match_job:" + job.ID.String() + ")",
			Version: job.PolicyVersion, ContentSHA256: job.PolicyContentSHA256, Content: job.PolicyContent,
		},
		{
			ID: uuid.New(), Name: companyPolicyDependency, ExecutionID: request.ExecutionID, WorkspaceID: request.WorkspaceID,
			Reference: companyPolicyRef, Version: companyPolicy.Metadata.Version,
			ContentSHA256: hashBytes(companyContent), Content: companyContent,
		},
		{
			ID: uuid.New(), Name: indicatorPolicyDependency, ExecutionID: request.ExecutionID, WorkspaceID: request.WorkspaceID,
			Reference: indicatorPolicyRef, Version: indicatorPolicy.Metadata.Version,
			ContentSHA256: hashBytes(indicatorContent), Content: indicatorContent,
		},
	}

	var committed workflowdomain.DependencyPreparation
	err = e.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, "execution-dependencies", request.ExecutionID.String()); err != nil {
			return fmt.Errorf("lock execution dependency preparation: %w", err)
		}
		if existing, found, err := e.workflowRepo.GetDependencyPreparationTx(ctx, tx, request.ExecutionID); err != nil {
			return err
		} else if found {
			committed = existing
			return errDependencyPreparationAlreadyExists
		}
		for _, source := range aliasSources {
			key := mappingUsageKey{InputName: source.inputName, SourceRef: source.sourceRef, SourceKey: source.sourceKey}
			decision, exists := mappings[key]
			if !exists {
				var err error
				decision, err = e.prepareSourceDecision(ctx, tx, request, source.inputName, source.inputVersionID, source.sourceRef, source.sourceKey, source.companyName, companyPolicy, companies)
				if err != nil {
					return err
				}
			}
			if err := addUsage(source.inputName, source.inputVersionID, source.sourceRef, source.sourceKey, decision); err != nil {
				return err
			}
		}
		preparation.MappingUsageCount = len(usages)
		for _, binding := range bindingsToPersist {
			if err := e.workflowRepo.InsertDependencyBinding(ctx, tx, binding); err != nil {
				return err
			}
		}
		for _, usage := range usages {
			if err := e.workflowRepo.InsertMappingUsage(ctx, tx, usage); err != nil {
				return err
			}
		}
		if err := e.workflowRepo.InsertDependencyPreparation(ctx, tx, preparation); err != nil {
			return err
		}
		if err := appendDependencyPreparationFacts(ctx, tx, preparation, bindingsToPersist, usages); err != nil {
			return err
		}
		committed = preparation
		committed.Dependencies = bindingsToPersist
		committed.MappingUsages = usages
		return nil
	})
	if errors.Is(err, errDependencyPreparationAlreadyExists) {
		return e.restorePreparedDependencies(committed, resolutionVersionID)
	}
	if err != nil {
		return preparedNativeDependencies{}, fmt.Errorf("prepare execution dependencies: %w", err)
	}
	return preparedNativeDependencies{
		ResolutionVersionID: resolutionVersionID,
		Mappings:            mappings,
		CompanyPolicy:       companyPolicy,
		IndicatorPolicy:     indicatorPolicy,
	}, nil
}

func (e *Engine) restorePreparedDependencies(preparation workflowdomain.DependencyPreparation, resolutionVersionID uuid.UUID) (preparedNativeDependencies, error) {
	if preparation.Status != "PREPARED" || len(preparation.Dependencies) != 3 || preparation.MappingUsageCount != len(preparation.MappingUsages) {
		return preparedNativeDependencies{}, fmt.Errorf("execution dependency preparation is incomplete")
	}
	mappings := make(map[mappingUsageKey]entitydomain.MappingDecision, len(preparation.MappingUsages))
	var companyContent, indicatorContent []byte
	dependencyNames := make(map[string]struct{}, len(preparation.Dependencies))
	for _, binding := range preparation.Dependencies {
		if _, exists := dependencyNames[binding.Name]; exists {
			return preparedNativeDependencies{}, fmt.Errorf("execution dependency %s is duplicated", binding.Name)
		}
		dependencyNames[binding.Name] = struct{}{}
		if hashBytes(binding.Content) != binding.ContentSHA256 {
			return preparedNativeDependencies{}, fmt.Errorf("dependency %s content hash is invalid", binding.Name)
		}
		switch binding.Name {
		case enterpriseResolutionInput:
			if binding.DatasetVersionID == nil || *binding.DatasetVersionID != resolutionVersionID {
				return preparedNativeDependencies{}, fmt.Errorf("enterprise_resolution does not match prepared DatasetVersion")
			}
		case companyPolicyDependency:
			companyContent = binding.Content
		case indicatorPolicyDependency:
			indicatorContent = binding.Content
		}
	}
	_, hasResolution := dependencyNames[enterpriseResolutionInput]
	_, hasCompanyPolicy := dependencyNames[companyPolicyDependency]
	_, hasIndicatorPolicy := dependencyNames[indicatorPolicyDependency]
	if !hasResolution || !hasCompanyPolicy || !hasIndicatorPolicy {
		return preparedNativeDependencies{}, fmt.Errorf("execution dependency preparation is missing a required dependency")
	}
	companyPolicy, err := matching.LoadPolicyBytes(companyContent, companyPolicyDependency)
	if err != nil {
		return preparedNativeDependencies{}, fmt.Errorf("restore company policy snapshot: %w", err)
	}
	indicatorPolicy, err := indicator.LoadPolicyBytes(indicatorContent, indicatorPolicyDependency)
	if err != nil {
		return preparedNativeDependencies{}, fmt.Errorf("restore indicator policy snapshot: %w", err)
	}
	for _, usage := range preparation.MappingUsages {
		if usage.ResolutionDatasetVersionID != resolutionVersionID || usage.WorkspaceID != preparation.WorkspaceID ||
			usage.InputName == "" || usage.SourceType == "" || usage.SourceRef == "" || usage.SourceKey == "" ||
			usage.DecisionID == uuid.Nil || usage.EntityID == uuid.Nil {
			return preparedNativeDependencies{}, fmt.Errorf("execution dependency mapping usage is out of scope")
		}
		key := mappingUsageKey{InputName: usage.InputName, SourceRef: usage.SourceRef, SourceKey: usage.SourceKey}
		if _, exists := mappings[key]; exists {
			return preparedNativeDependencies{}, fmt.Errorf("execution dependency mapping usage is duplicated for %s/%s", usage.SourceRef, usage.SourceKey)
		}
		mappings[key] = entitydomain.MappingDecision{
			ID: usage.DecisionID, WorkspaceID: usage.WorkspaceID, EntityID: usage.EntityID,
			SourceType: usage.SourceType, SourceRef: usage.SourceRef, SourceKey: usage.SourceKey,
			Status: entitydomain.MappingAutoMatched,
		}
	}
	return preparedNativeDependencies{
		ResolutionVersionID: resolutionVersionID,
		Mappings:            mappings,
		CompanyPolicy:       companyPolicy,
		IndicatorPolicy:     indicatorPolicy,
	}, nil
}

func (e *Engine) prepareSourceDecision(ctx context.Context, tx pgx.Tx, request workflowapp.ProcessingRequest, inputName string, inputVersionID uuid.UUID, sourceRef, sourceKey, companyName string, policy matching.Policy, companies map[uuid.UUID]*canonicalCompany) (entitydomain.MappingDecision, error) {
	returnValue := entitydomain.MappingDecision{}
	err := func() error {
		if err := e.entityRepo.LockMappingSourceTx(ctx, tx, request.WorkspaceID, "CSV", sourceRef, sourceKey); err != nil {
			return err
		}
		mapping, err := e.entityRepo.GetMappingBySourceTx(ctx, tx, request.WorkspaceID, "CSV", sourceRef, sourceKey)
		if err == nil {
			if mapping.CurrentDecisionID == nil {
				return fmt.Errorf("%s source %s has no current decision", inputName, sourceKey)
			}
			decision, err := e.entityRepo.GetMappingDecisionByIDTx(ctx, tx, request.WorkspaceID, *mapping.CurrentDecisionID)
			if err != nil {
				return err
			}
			if decision.SourceType != "CSV" || decision.SourceRef != sourceRef || decision.SourceKey != sourceKey {
				return fmt.Errorf("%s source %s current decision scope is invalid", inputName, sourceKey)
			}
			returnValue = decision
			return nil
		}
		if !errors.Is(err, entityinfra.ErrNotFound) {
			return err
		}
		normalized := matching.NormalizeCompany(matching.CompanyRecord{CompanyName: companyName}, policy).CompanyName
		entityID, method, confidence, err := resolveAlias(normalized, companies)
		if err != nil {
			return fmt.Errorf("resolve %s source %s company %q: %w", inputName, sourceKey, companyName, err)
		}
		aliasMapping := entitydomain.EntityMapping{
			ID: uuid.New(), WorkspaceID: request.WorkspaceID, EntityID: entityID, SourceType: "CSV",
			SourceRef: sourceRef, SourceKey: sourceKey, SourceName: strings.TrimSpace(companyName),
			MatchMethod: method, MatchRuleID: "WORKFLOW-CANONICAL-ALIAS", MatchPolicyVersion: policy.Metadata.Version,
			Confidence: confidence, Status: entitydomain.MappingAutoMatched, CreatedAt: time.Now().UTC(),
		}
		aliasEvidence, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  request.WorkspaceID,
			EvidenceType: "ENTITY_MAPPING_WORKFLOW_ALIAS",
			Title:        "Workflow alias mapping decision",
			SourceType:   "ENTITY_MAPPING",
			SourceID:     &aliasMapping.ID,
			Metadata: map[string]any{
				"executionId":  request.ExecutionID,
				"inputName":    inputName,
				"inputVersion": inputVersionID,
				"sourceType":   "CSV",
				"sourceRef":    sourceRef,
				"sourceKey":    sourceKey,
				"entityId":     entityID,
			},
		}, evidence.Relation{ObjectType: "ENTITY_MAPPING", ObjectID: aliasMapping.ID, RelationType: "SUPPORTS"})
		if err != nil {
			return err
		}
		aliasMapping.EvidenceID = &aliasEvidence.ID
		key := stableAliasIdempotencyKey(request.ExecutionID, inputName, inputVersionID, sourceRef, sourceKey)
		decision, err := e.entityRepo.RecordMappingDecision(ctx, tx, entitydomain.MappingDecisionCommand{
			Mapping: aliasMapping, SourceOrigin: entitydomain.OriginWorkflowAlias, IdempotencyKey: key,
		})
		if err != nil {
			return err
		}
		if err := appendMappingDecisionFacts(ctx, tx, decision, request); err != nil {
			return err
		}
		returnValue = decision
		return nil
	}()
	return returnValue, err
}

func appendDependencyPreparationFacts(ctx context.Context, tx pgx.Tx, preparation workflowdomain.DependencyPreparation, dependencies []workflowdomain.DependencyBinding, usages []workflowdomain.MappingUsage) error {
	dependencyNames := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		dependencyNames = append(dependencyNames, dependency.Name)
	}
	usageCount := len(usages)
	if _, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  preparation.WorkspaceID,
		EvidenceType: "EXECUTION_DEPENDENCY_PREPARED",
		Title:        "Execution dependencies prepared",
		SourceType:   "EXECUTION",
		SourceID:     &preparation.ExecutionID,
		Metadata: map[string]any{
			"bindingFingerprint": preparation.BindingFingerprint,
			"dependencies":       dependencyNames,
			"mappingUsageCount":  usageCount,
		},
	}, evidence.Relation{ObjectType: "EXECUTION", ObjectID: preparation.ExecutionID, RelationType: "SUPPORTS"}); err != nil {
		return err
	}
	event, err := outbox.NewEvent("EXECUTION", preparation.ExecutionID, "ExecutionDependenciesPrepared", map[string]any{
		"executionId":        preparation.ExecutionID,
		"workspaceId":        preparation.WorkspaceID,
		"bindingFingerprint": preparation.BindingFingerprint,
		"dependencyNames":    dependencyNames,
		"mappingUsageCount":  usageCount,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	return audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &preparation.WorkspaceID,
		ActorType:   "SERVICE",
		Action:      "EXECUTION_DEPENDENCIES_PREPARED",
		ObjectType:  "EXECUTION",
		ObjectID:    preparation.ExecutionID,
		AfterState: map[string]any{
			"status":             preparation.Status,
			"bindingFingerprint": preparation.BindingFingerprint,
			"dependencyNames":    dependencyNames,
			"mappingUsageCount":  usageCount,
		},
		TraceID: preparation.ExecutionID.String(),
	})
}

func appendMappingDecisionFacts(ctx context.Context, tx pgx.Tx, decision entitydomain.MappingDecision, request workflowapp.ProcessingRequest) error {
	event, err := outbox.NewEvent("ENTITY_MAPPING", decision.MappingID, "EntityMappingDecisionRecorded", map[string]any{
		"decisionId":   decision.ID,
		"mappingId":    decision.MappingID,
		"entityId":     decision.EntityID,
		"sourceType":   decision.SourceType,
		"sourceRef":    decision.SourceRef,
		"sourceKey":    decision.SourceKey,
		"status":       decision.Status,
		"sourceOrigin": decision.SourceOrigin,
		"sourceJobId":  decision.SourceJobID,
	})
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	return audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &request.WorkspaceID,
		ActorType:   "SERVICE",
		Action:      "ENTITY_MAPPING_DECISION_RECORDED",
		ObjectType:  "ENTITY_MAPPING_DECISION",
		ObjectID:    decision.ID,
		AfterState: map[string]any{
			"mappingId":    decision.MappingID,
			"entityId":     decision.EntityID,
			"sourceType":   decision.SourceType,
			"sourceRef":    decision.SourceRef,
			"sourceKey":    decision.SourceKey,
			"sourceOrigin": decision.SourceOrigin,
		},
		TraceID: request.ExecutionID.String(),
	})
}

func loadMatchingPolicy(root, ref string) (matching.Policy, []byte, error) {
	path, err := matching.ResolvePolicyPath(root, ref)
	if err != nil {
		return matching.Policy{}, nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return matching.Policy{}, nil, err
	}
	policy, err := matching.LoadPolicyBytes(content, path)
	return policy, content, err
}

func loadIndicatorPolicy(root, ref string) (indicator.Policy, []byte, error) {
	path, err := matching.ResolvePolicyPath(root, ref)
	if err != nil {
		return indicator.Policy{}, nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return indicator.Policy{}, nil, err
	}
	policy, err := indicator.LoadPolicyBytes(content, path)
	return policy, content, err
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func dependencyFingerprint(request workflowapp.ProcessingRequest, resolutionVersionID uuid.UUID, companyPolicy, indicatorPolicy []byte) string {
	type fingerprintInput struct {
		ExecutionID           uuid.UUID
		WorkspaceID           uuid.UUID
		Inputs                []workflowdomain.InputBinding
		ResolutionVersionID   uuid.UUID
		CompanyPolicySHA256   string
		IndicatorPolicySHA256 string
	}
	payload, _ := json.Marshal(fingerprintInput{
		ExecutionID: request.ExecutionID, WorkspaceID: request.WorkspaceID, Inputs: request.Inputs,
		ResolutionVersionID: resolutionVersionID, CompanyPolicySHA256: hashBytes(companyPolicy),
		IndicatorPolicySHA256: hashBytes(indicatorPolicy),
	})
	return hashBytes(payload)
}

func stableAliasIdempotencyKey(executionID uuid.UUID, inputName string, inputVersionID uuid.UUID, sourceRef, sourceKey string) string {
	payload := executionID.String() + "\x1f" + inputName + "\x1f" + inputVersionID.String() + "\x1f" + sourceRef + "\x1f" + sourceKey
	return "workflow-alias:" + hashBytes([]byte(payload))
}
