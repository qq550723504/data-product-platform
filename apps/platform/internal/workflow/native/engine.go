package native

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/matching"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/indicator"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

const (
	companyPolicyRef   = "park/matching/company-match-policy-v1.yaml"
	indicatorPolicyRef = "park/indicators/enterprise-activity-v1.yaml"
)

type ObjectStore interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

type Engine struct {
	industryPackRoot string
	tx               *transaction.Manager
	datasetRepo      *datasetinfra.PostgresRepository
	entityRepo       *entityinfra.PostgresRepository
	workflowRepo     *workflowinfra.PostgresRepository
	datasetWriter       *datasetapp.UploadVersionService
	store               ObjectStore
	indicatorCalculator indicator.Calculator
}

func NewEngine(industryPackRoot string, tx *transaction.Manager, datasetRepo *datasetinfra.PostgresRepository, entityRepo *entityinfra.PostgresRepository, workflowRepo *workflowinfra.PostgresRepository, datasetWriter *datasetapp.UploadVersionService, store ObjectStore, calculators ...indicator.Calculator) *Engine {
	var calculator indicator.Calculator
	if len(calculators) > 0 {
		calculator = calculators[0]
	}
	return &Engine{
		industryPackRoot: industryPackRoot,
		tx:               tx,
		datasetRepo:      datasetRepo,
		entityRepo:       entityRepo,
		workflowRepo:     workflowRepo,
		datasetWriter:       datasetWriter,
		store:               store,
		indicatorCalculator: calculator,
	}
}

type canonicalCompany struct {
	ID          uuid.UUID
	DisplayName string
	EntryDate   *time.Time
	Aliases     map[string]struct{}
}

type sourceRecord struct {
	SourceKey   string
	CompanyName string
	Fields      map[string]string
}

func (e *Engine) Execute(ctx context.Context, request workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	if e.indicatorCalculator == nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("native workflow has no industry-pack indicator calculator")
	}
	bindings := map[string]uuid.UUID{}
	for _, input := range request.Inputs {
		bindings[input.Name] = input.DatasetVersionID
	}
	for _, required := range []string{"enterprise_raw", "lease_raw", "energy_raw"} {
		if bindings[required] == uuid.Nil {
			return workflowapp.ProcessingResult{}, fmt.Errorf("native enterprise activity workflow requires input %s", required)
		}
	}

	enterpriseVersion, enterpriseRows, enterpriseRef, err := e.readInput(ctx, bindings["enterprise_raw"])
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("read enterprise input: %w", err)
	}
	leaseVersion, leaseRows, leaseRef, err := e.readInput(ctx, bindings["lease_raw"])
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("read lease input: %w", err)
	}
	energyVersion, energyRows, energyRef, err := e.readInput(ctx, bindings["energy_raw"])
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("read energy input: %w", err)
	}

	dependencies, err := e.prepareDependencies(ctx, request, bindings, enterpriseRows, leaseRows, energyRows, enterpriseRef, leaseRef, energyRef)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	companies, inputs, err := e.buildCanonicalCompanies(enterpriseRows, enterpriseRef, dependencies.CompanyPolicy, dependencies.Mappings)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}

	leaseMapped, err := e.attachLease(leaseRows, leaseRef, dependencies.CompanyPolicy, companies, inputs, dependencies.Mappings)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	energyMapped, quarantineCount, err := e.attachEnergy(ctx, request.ExecutionID, energyRows, energyRef, dependencies.CompanyPolicy, companies, inputs, dependencies.Mappings)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339)
	rows := make([][]string, 0, len(companies)+1)
	rows = append(rows, []string{
		"company_id", "company_name", "period", "tenancy_stability",
		"rent_performance", "energy_stability", "activity_score", "activity_level", "indicator_coverage", "generated_at",
	})
	companyIDs := make([]uuid.UUID, 0, len(companies))
	for companyID := range companies {
		companyIDs = append(companyIDs, companyID)
	}
	sort.Slice(companyIDs, func(i, j int) bool { return companyIDs[i].String() < companyIDs[j].String() })
	completeScores := 0
	for _, companyID := range companyIDs {
		company := companies[companyID]
		result, err := e.indicatorCalculator.Calculate(dependencies.IndicatorPolicy, request.TargetPeriod, inputs[companyID])
		if err != nil {
			return workflowapp.ProcessingResult{}, fmt.Errorf("calculate indicators for %s: %w", companyID, err)
		}
		if result.ActivityScore != nil {
			completeScores++
		}
		rows = append(rows, []string{
			companyID.String(),
			company.DisplayName,
			request.TargetPeriod,
			formatOptional(result.TenancyStability),
			formatOptional(result.RentPerformance),
			formatOptional(result.EnergyStability),
			formatOptional(result.ActivityScore),
			result.ActivityLevel,
			fmt.Sprintf("%.2f", result.IndicatorCoverage),
			generatedAt,
		})
	}

	content, err := encodeCSV(rows)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	outputVersion, err := e.datasetWriter.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:              request.OutputDatasetID,
		Filename:               fmt.Sprintf("enterprise-activity-%s.csv", request.TargetPeriod),
		ContentType:            "text/csv; charset=utf-8",
		Content:                content,
		TraceID:                request.ExecutionID.String(),
		GeneratedByExecutionID: &request.ExecutionID,
		Metadata: map[string]any{
			"workflowVersionId":               request.WorkflowVersion.ID,
			"workflowVersion":                 request.WorkflowVersion.Version,
			"indicatorSet":                    "park-enterprise-activity@1.0.0",
			"entityPolicyVersion":             dependencies.CompanyPolicy.Metadata.Version,
			"targetPeriod":                    request.TargetPeriod,
			"quarantineCount":                 quarantineCount,
			"unresolvedEntityRate":            0.0,
			"acceptedNegativeEnergyRate":      0.0,
			"gateStatus":                      "PENDING_GOVERNANCE_GATES",
			"entityResolutionOutputVersionId": dependencies.ResolutionVersionID,
		},
	})
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("write CURATED DatasetVersion: %w", err)
	}

	lineageInputs := []uuid.UUID{enterpriseVersion.ID, leaseVersion.ID, energyVersion.ID, dependencies.ResolutionVersionID}
	if err := e.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, inputVersionID := range lineageInputs {
			if err := e.datasetRepo.AddLineage(ctx, tx, outputVersion.ID, inputVersionID, "DERIVED_FROM", &request.ExecutionID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("write DatasetVersion lineage: %w", err)
	}

	return workflowapp.ProcessingResult{
		OutputDatasetVersionID: outputVersion.ID,
		EngineExecutionID:      "native:" + request.ExecutionID.String(),
		Metrics: map[string]any{
			"enterpriseRecords": len(enterpriseRows),
			"leaseRecords":      len(leaseRows),
			"energyRecords":     len(energyRows),
			"companies":         len(companies),
			"leaseMapped":       leaseMapped,
			"energyMapped":      energyMapped,
			"completeScores":    completeScores,
			"quarantineCount":   quarantineCount,
			"outputRows":        len(companies),
		},
	}, nil
}

func (e *Engine) readInput(ctx context.Context, versionID uuid.UUID) (version struct{ ID uuid.UUID }, rows []map[string]string, sourceRef string, err error) {
	datasetVersion, err := e.datasetRepo.GetVersion(ctx, versionID)
	if err != nil {
		return version, nil, "", err
	}
	version.ID = datasetVersion.ID
	reader, err := e.store.Get(ctx, datasetVersion.StorageURI)
	if err != nil {
		return version, nil, "", err
	}
	defer reader.Close()
	rows, err = decodeCSV(reader)
	if err != nil {
		return version, nil, "", err
	}
	return version, rows, storageBasename(datasetVersion.StorageURI), nil
}

func (e *Engine) buildCanonicalCompanies(rows []map[string]string, sourceRef string, policy matching.Policy, mappings map[mappingUsageKey]entitydomain.MappingDecision) (map[uuid.UUID]*canonicalCompany, map[uuid.UUID]indicator.CompanyInput, error) {
	companies := map[uuid.UUID]*canonicalCompany{}
	inputs := map[uuid.UUID]indicator.CompanyInput{}
	for _, row := range rows {
		sourceKey := strings.TrimSpace(row["source_company_id"])
		if sourceKey == "" {
			return nil, nil, fmt.Errorf("enterprise record missing source_company_id")
		}
		mapping, ok := mappings[mappingUsageKey{InputName: "enterprise_raw", SourceRef: sourceRef, SourceKey: sourceKey}]
		if !ok {
			return nil, nil, fmt.Errorf("enterprise %s has no fixed mapping decision; run entity resolution before workflow execution", sourceKey)
		}
		normalized := matching.NormalizeCompany(matching.CompanyRecord{
			SourceCompanyID:         sourceKey,
			CompanyName:             row["company_name"],
			UnifiedSocialCreditCode: row["unified_social_credit_code"],
			LegalRepresentative:     row["legal_representative"],
			RegisteredAddress:       row["registered_address"],
			EntryDate:               row["entry_date"],
			CompanyStatus:           row["company_status"],
		}, policy)
		company := companies[mapping.EntityID]
		if company == nil {
			company = &canonicalCompany{ID: mapping.EntityID, DisplayName: strings.TrimSpace(row["company_name"]), Aliases: map[string]struct{}{}}
			companies[mapping.EntityID] = company
		}
		company.Aliases[normalized.CompanyName] = struct{}{}
		entryDate, err := parseFlexibleDate(row["entry_date"])
		if err != nil {
			return nil, nil, fmt.Errorf("enterprise %s entry_date: %w", sourceKey, err)
		}
		input := inputs[mapping.EntityID]
		if input.EntryDate == nil || strings.TrimSpace(row["unified_social_credit_code"]) != "" {
			input.EntryDate = entryDate
			company.EntryDate = entryDate
			if strings.TrimSpace(row["unified_social_credit_code"]) != "" {
				company.DisplayName = strings.TrimSpace(row["company_name"])
			}
		}
		inputs[mapping.EntityID] = input
	}
	return companies, inputs, nil
}

func (e *Engine) attachLease(rows []map[string]string, sourceRef string, policy matching.Policy, companies map[uuid.UUID]*canonicalCompany, inputs map[uuid.UUID]indicator.CompanyInput, mappings map[mappingUsageKey]entitydomain.MappingDecision) (int, error) {
	mapped := 0
	for _, row := range rows {
		companyID, err := e.resolveSourceCompany("lease_raw", sourceRef, row["source_company_id"], mappings)
		if err != nil {
			return mapped, err
		}
		start, err := parseRequiredDate(row["contract_start"])
		if err != nil {
			return mapped, fmt.Errorf("lease %s contract_start: %w", row["lease_id"], err)
		}
		end, err := parseRequiredDate(row["contract_end"])
		if err != nil {
			return mapped, fmt.Errorf("lease %s contract_end: %w", row["lease_id"], err)
		}
		due, err := parseRequiredDate(row["payment_due_date"])
		if err != nil {
			return mapped, fmt.Errorf("lease %s payment_due_date: %w", row["lease_id"], err)
		}
		paid, err := parseFlexibleDate(row["payment_date"])
		if err != nil {
			return mapped, fmt.Errorf("lease %s payment_date: %w", row["lease_id"], err)
		}
		input := inputs[companyID]
		input.LeaseEvents = append(input.LeaseEvents, indicator.LeaseEvent{
			ContractStart: start,
			ContractEnd:   end,
			LeaseStatus:   strings.TrimSpace(row["lease_status"]),
			DueDate:       due,
			PaymentDate:   paid,
		})
		inputs[companyID] = input
		mapped++
	}
	return mapped, nil
}

func (e *Engine) attachEnergy(ctx context.Context, executionID uuid.UUID, rows []map[string]string, sourceRef string, policy matching.Policy, companies map[uuid.UUID]*canonicalCompany, inputs map[uuid.UUID]indicator.CompanyInput, mappings map[mappingUsageKey]entitydomain.MappingDecision) (mapped int, quarantineCount int, err error) {
	for _, row := range rows {
		companyID, err := e.resolveSourceCompany("energy_raw", sourceRef, row["source_company_id"], mappings)
		if err != nil {
			return mapped, quarantineCount, err
		}
		readingTime, err := time.Parse(time.RFC3339, strings.TrimSpace(row["reading_time"]))
		if err != nil {
			return mapped, quarantineCount, fmt.Errorf("energy %s reading_time: %w", row["meter_id"], err)
		}
		energyKWh, err := strconv.ParseFloat(strings.TrimSpace(row["energy_kwh"]), 64)
		if err != nil {
			return mapped, quarantineCount, fmt.Errorf("energy %s energy_kwh: %w", row["meter_id"], err)
		}
		sourceKey := strings.TrimSpace(row["meter_id"]) + "@" + readingTime.Format(time.RFC3339)
		if energyKWh < 0 {
			payload := make(map[string]any, len(row))
			for key, value := range row {
				payload[key] = value
			}
			if err := e.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				return e.workflowRepo.InsertQuarantine(ctx, tx, workflowinfra.QuarantineRecord{
					ExecutionID:   executionID,
					InputName:     "energy_raw",
					SourceKey:     sourceKey,
					ReasonCode:    "NEGATIVE_ENERGY_KWH",
					ReasonMessage: "energy_kwh must be greater than or equal to zero",
					Payload:       payload,
				})
			}); err != nil {
				return mapped, quarantineCount, err
			}
			quarantineCount++
			continue
		}
		input := inputs[companyID]
		input.EnergyReadings = append(input.EnergyReadings, indicator.EnergyReading{
			ReadingTime: readingTime,
			EnergyKWh:   energyKWh,
			SourceKey:   sourceKey,
		})
		inputs[companyID] = input
		mapped++
	}
	return mapped, quarantineCount, nil
}

func (e *Engine) resolveSourceCompany(inputName, sourceRef, sourceKey string, mappings map[mappingUsageKey]entitydomain.MappingDecision) (uuid.UUID, error) {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		return uuid.Nil, fmt.Errorf("%s record is missing source_company_id", inputName)
	}
	mapping, ok := mappings[mappingUsageKey{InputName: inputName, SourceRef: sourceRef, SourceKey: sourceKey}]
	if !ok {
		return uuid.Nil, fmt.Errorf("%s source %s has no fixed mapping decision", inputName, sourceKey)
	}
	if mapping.Status != entitydomain.MappingAutoMatched && mapping.Status != entitydomain.MappingConfirmed {
		return uuid.Nil, fmt.Errorf("%s source %s has unusable mapping status %s", inputName, sourceKey, mapping.Status)
	}
	return mapping.EntityID, nil
}

func resolveAlias(normalized string, companies map[uuid.UUID]*canonicalCompany) (uuid.UUID, string, float64, error) {
	if normalized == "" {
		return uuid.Nil, "", 0, fmt.Errorf("normalized company name is empty")
	}
	exact := map[uuid.UUID]struct{}{}
	contained := map[uuid.UUID]struct{}{}
	for companyID, company := range companies {
		for alias := range company.Aliases {
			if normalized == alias {
				exact[companyID] = struct{}{}
			}
			if strings.Contains(alias, normalized) || strings.Contains(normalized, alias) {
				contained[companyID] = struct{}{}
			}
		}
	}
	if len(exact) == 1 {
		for id := range exact {
			return id, "CANONICAL_NAME_EXACT", 1, nil
		}
	}
	if len(exact) > 1 {
		return uuid.Nil, "", 0, fmt.Errorf("exact normalized name matches multiple canonical entities")
	}
	if len(contained) == 1 {
		for id := range contained {
			return id, "CANONICAL_NAME_CONTAINS", 0.96, nil
		}
	}
	if len(contained) > 1 {
		return uuid.Nil, "", 0, fmt.Errorf("contained normalized name matches multiple canonical entities")
	}
	return uuid.Nil, "", 0, fmt.Errorf("no deterministic canonical entity match")
}

func decodeCSV(reader io.Reader) ([]map[string]string, error) {
	csvReader := csv.NewReader(reader)
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	rows := make([]map[string]string, 0)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row: %w", err)
		}
		row := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(record) {
				row[header] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func encodeCSV(rows [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(rows); err != nil {
		return nil, fmt.Errorf("encode CURATED CSV: %w", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush CURATED CSV: %w", err)
	}
	return buffer.Bytes(), nil
}

func storageBasename(storageURI string) string {
	parsed, err := url.Parse(storageURI)
	if err != nil {
		return path.Base(storageURI)
	}
	return path.Base(parsed.Path)
}

func parseRequiredDate(value string) (time.Time, error) {
	parsed, err := parseFlexibleDate(value)
	if err != nil {
		return time.Time{}, err
	}
	if parsed == nil {
		return time.Time{}, fmt.Errorf("date is required")
	}
	return *parsed, nil
}

func parseFlexibleDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("unsupported date %q", value)
}

func formatOptional(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *value)
}
