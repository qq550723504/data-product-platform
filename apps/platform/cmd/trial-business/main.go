// trial-business composes existing Core Commands for one retained synthetic run.
// It never starts services, reads objects, issues credentials or processes data.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	certapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	"gopkg.in/yaml.v3"
)

const businessContract = "issue325/business-prerequisites-v1"
const purpose = "ENTERPRISE_CREDIT_RISK_SUPPORT"
const contractRef = "examples/enterprise-activity/contract/data-contract-v1.yaml"

type config struct {
	DSN, Repo, Private string
	Workspace, Actor   uuid.UUID
	Profile            struct{ Subject, Bucket string }
}

type registry struct {
	PreparationBinding fileBinding    `json:"businessPrerequisiteBinding"`
	RenewalBinding     fileBinding    `json:"businessRightsRenewalBinding"`
	ActivationBinding  fileBinding    `json:"activationBinding"`
	RunID              string         `json:"runId"`
	Root               string         `json:"root"`
	Private            string         `json:"private"`
	Workspace          uuid.UUID      `json:"workspaceId"`
	Actor              uuid.UUID      `json:"actorId"`
	Phase              string         `json:"phase"`
	Ports              map[string]int `json:"ports"`
	Activation         struct {
		AttemptID string `json:"attemptId"`
		Status    string `json:"status"`
	} `json:"activation"`
}

type version struct {
	ID     uuid.UUID `json:"id"`
	Kind   string    `json:"kind"`
	SHA256 string    `json:"sha256"`
	URI    string    `json:"uri"`
	Bytes  int64     `json:"bytes"`
	Rows   int64     `json:"rows"`
}

type spec struct {
	Contract          string    `json:"contract"`
	RunID             string    `json:"runId"`
	Workspace         uuid.UUID `json:"workspaceId"`
	Actor             uuid.UUID `json:"actorId"`
	ActivationAttempt string    `json:"activationAttemptId"`
	Target            uuid.UUID `json:"targetDatasetVersionId"`
	Versions          []version `json:"versions"`
	Consumer          string    `json:"consumerRef"`
	Purpose           string    `json:"purpose"`
	ValidFrom         time.Time `json:"validFrom"`
	ValidTo           time.Time `json:"validTo"`
	Reason            string    `json:"verificationReason"`
}

type plan struct {
	Contract             string                               `json:"contract"`
	Spec                 spec                                 `json:"spec"`
	Principal            string                               `json:"principalRef"`
	Resources            []uuid.UUID                          `json:"sourceResourceIds"`
	Authorization        rightsapp.CreateAuthorizationCommand `json:"authorizationCommand"`
	Declarations         []rightsdomain.RightsDeclarationSpec `json:"declarationSpecs"`
	BindingAuthorityMode string                               `json:"bindingAuthorityMode"`
	Original             map[string]json.RawMessage           `json:"originalFacts"`
	Files                map[string]string                    `json:"sourceFiles"`
	Profile              certdomain.CertificationProfile      `json:"profile"`
	ContractYAML         string                               `json:"contractYAML"`
	Commands             []string                             `json:"commands"`
}

type receipt struct {
	Contract           string               `json:"contract"`
	Status             string               `json:"status"`
	RunID              string               `json:"runId"`
	Workspace          uuid.UUID            `json:"workspaceId"`
	Actor              uuid.UUID            `json:"actorId"`
	Principal          string               `json:"principalRef"`
	PlanSHA256         string               `json:"planSha256"`
	Objects            map[string]uuid.UUID `json:"objects"`
	DeliveryAuthorized bool                 `json:"deliveryAuthorized"`
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encoded(v any) []byte { b, _ := json.Marshal(v); return b }
func decode(path string, v any, strict bool) error {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 1<<20 {
		return fmt.Errorf("bounded private input unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if strict {
		d.DisallowUnknownFields()
	}
	if err = d.Decode(v); err != nil {
		return fmt.Errorf("invalid private input")
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("single JSON input required")
	}
	return nil
}

func validate(c config, r registry, s spec) error {
	u, err := url.Parse(c.DSN)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" || u.Path != "/dpp_trial_core" ||
		u.Port() != strconv.Itoa(r.Ports["core"]) || c.Repo != r.Root || c.Private != r.Private ||
		!filepath.IsAbs(c.Repo) || !filepath.IsAbs(c.Private) || filepath.Base(c.Private) != "dpp-trial-"+r.RunID ||
		c.Workspace == uuid.Nil || c.Actor == uuid.Nil || c.Workspace != r.Workspace || c.Actor != r.Actor ||
		c.Profile.Subject != "trial-operator-"+r.RunID || c.Profile.Bucket != "raw" || r.Phase != "RUNNING" || r.Activation.Status != "PASS" {
		return fmt.Errorf("same activated retained Core and operator identity required")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(r.RunID) || s.Contract != businessContract || s.RunID != r.RunID ||
		s.Workspace != c.Workspace || s.Actor != c.Actor || s.ActivationAttempt != r.Activation.AttemptID || s.Target == uuid.Nil ||
		!regexp.MustCompile(`^[A-Z][A-Z0-9_-]{2,127}$`).MatchString(s.Consumer) || s.Purpose != purpose ||
		len(strings.TrimSpace(s.Reason)) < 10 || len(s.Reason) > 1024 || s.ValidFrom.IsZero() ||
		!s.ValidTo.After(s.ValidFrom) || s.ValidTo.Sub(s.ValidFrom) > 24*time.Hour || len(s.Versions) != 5 {
		return fmt.Errorf("explicit finite synthetic business context required")
	}
	seen, kinds := map[uuid.UUID]bool{}, map[string]int{}
	for _, v := range s.Versions {
		if v.ID == uuid.Nil || seen[v.ID] || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(v.SHA256) ||
			v.Bytes <= 0 || v.Rows < 0 || !strings.HasPrefix(v.URI, "s3://raw/") {
			return fmt.Errorf("five exact retained versions required")
		}
		seen[v.ID] = true
		kinds[v.Kind]++
		if v.ID == s.Target && v.Kind != "CURATED" {
			return fmt.Errorf("retained CURATED target required")
		}
	}
	if !seen[s.Target] || kinds["RAW"] != 3 || kinds["STANDARDIZED"] != 1 || kinds["CURATED"] != 1 {
		return fmt.Errorf("original three RAW and two outputs required")
	}
	return nil
}

// makePlan has only a REPEATABLE READ READ ONLY transaction and local file reads.
func makePlan(ctx context.Context, pool *pgxpool.Pool, c config, r registry, s spec) (plan, error) {
	p := plan{Contract: businessContract, Spec: s, Principal: c.Profile.Subject, Original: map[string]json.RawMessage{}, Files: map[string]string{}}
	if err := validate(c, r, s); err != nil {
		return p, err
	}
	sort.Slice(p.Spec.Versions, func(i, j int) bool { return p.Spec.Versions[i].ID.String() < p.Spec.Versions[j].ID.String() })
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return p, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var schema, count int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(version),0) FROM schema_migration`).Scan(&schema); err != nil || schema != 71 {
		return p, fmt.Errorf("schema71 required")
	}
	ids := []uuid.UUID{}
	for _, want := range p.Spec.Versions {
		var got version
		var owner, workspace uuid.UUID
		var resource *uuid.UUID
		var state string
		err = tx.QueryRow(ctx, `SELECT v.id,d.dataset_type,v.checksum_value,v.storage_uri,v.byte_size,v.row_count,d.owner_id,d.workspace_id,d.source_resource_id,v.status FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=$1 AND d.deleted_at IS NULL AND v.checksum_algorithm='SHA256'`, want.ID).Scan(&got.ID, &got.Kind, &got.SHA256, &got.URI, &got.Bytes, &got.Rows, &owner, &workspace, &resource, &state)
		if err != nil || got != want || owner != c.Actor || workspace != c.Workspace || state != "READY" {
			return p, fmt.Errorf("retained version or ownership drift")
		}
		ids = append(ids, want.ID)
		if want.Kind == "RAW" {
			if resource == nil {
				return p, fmt.Errorf("RAW source resource required")
			}
			p.Resources = append(p.Resources, *resource)
		}
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1`, c.Workspace).Scan(&count); err != nil || count != 5 {
		return p, fmt.Errorf("exact original version set required")
	}
	var closure []uuid.UUID
	err = tx.QueryRow(ctx, `WITH RECURSIVE inputs(id) AS (SELECT $1::uuid UNION SELECT l.input_version_id FROM dataset_version_lineage l JOIN inputs i ON i.id=l.output_version_id) SELECT array_agg(id ORDER BY id) FROM inputs`, s.Target).Scan(&closure)
	if err != nil || string(encoded(closure)) != string(encoded(ids)) {
		return p, fmt.Errorf("complete original lineage closure required")
	}
	sort.Slice(p.Resources, func(i, j int) bool { return p.Resources[i].String() < p.Resources[j].String() })
	if p.Resources[0] == p.Resources[1] || p.Resources[1] == p.Resources[2] {
		return p, fmt.Errorf("three distinct RAW resources required")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM data_resource WHERE workspace_id=$1 AND id=ANY($2) AND owner_id=$3 AND deleted_at IS NULL`, c.Workspace, p.Resources, c.Actor).Scan(&count); err != nil || count != 3 {
		return p, fmt.Errorf("source resource workspace drift")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ingestion_operation WHERE workspace_id=$1 AND actor_id=$2 AND principal_ref=$3 AND state='ACCEPTED'`, c.Workspace, c.Actor, p.Principal).Scan(&count); err != nil || count != 1 {
		return p, fmt.Errorf("original accepted batch required")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ingestion_receipt_slot rs JOIN ingestion_receipt r ON r.id=rs.receipt_id JOIN ingestion_operation o ON o.id=r.operation_id JOIN dataset_version v ON v.id=rs.version_id JOIN dataset d ON d.id=v.dataset_id WHERE o.workspace_id=$1 AND o.state='ACCEPTED' AND rs.version_id=ANY($2) AND d.dataset_type='RAW' AND rs.content_hash=v.checksum_value AND rs.storage_uri=v.storage_uri`, c.Workspace, ids).Scan(&count); err != nil || count != 3 {
		return p, fmt.Errorf("original three RAW receipt bindings required")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM execution e JOIN dataset_version v ON v.id=e.output_dataset_version_id WHERE e.workspace_id=$1 AND v.id=$2 AND v.generated_by_execution_id=e.id AND e.status='SUCCEEDED' AND e.attempt=1`, c.Workspace, s.Target).Scan(&count); err != nil || count != 1 {
		return p, fmt.Errorf("original successful execution required")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM execution_input i JOIN execution e ON e.id=i.execution_id JOIN workflow_version wv ON wv.id=e.workflow_version_id JOIN workflow w ON w.id=wv.workflow_id WHERE e.output_dataset_version_id=$1 AND e.workspace_id=$2 AND w.workspace_id=$2 AND i.dataset_version_id=ANY($3) AND i.dataset_version_id<>$1`, s.Target, c.Workspace, ids).Scan(&count); err != nil || count != 4 {
		return p, fmt.Errorf("four original frozen execution inputs required")
	}
	queries := map[string]string{
		"versions":     `SELECT v.* FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1`,
		"datasets":     `SELECT * FROM dataset WHERE workspace_id=$1`,
		"batch":        `SELECT * FROM ingestion_operation WHERE workspace_id=$1`,
		"receipts":     `SELECT r.* FROM ingestion_receipt r JOIN ingestion_operation o ON o.id=r.operation_id WHERE o.workspace_id=$1`,
		"receiptSlots": `SELECT s.* FROM ingestion_receipt_slot s JOIN ingestion_receipt r ON r.id=s.receipt_id JOIN ingestion_operation o ON o.id=r.operation_id WHERE o.workspace_id=$1`,
		"executions":   `SELECT * FROM execution WHERE workspace_id=$1`,
		"inputs":       `SELECT i.* FROM execution_input i JOIN execution e ON e.id=i.execution_id WHERE e.workspace_id=$1`,
		"quarantine":   `SELECT q.* FROM execution_quarantine_record q JOIN execution e ON e.id=q.execution_id WHERE e.workspace_id=$1`,
		"lineage":      `SELECT l.* FROM dataset_version_lineage l JOIN dataset_version v ON v.id=l.output_version_id JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1`,
		"workflows":    `SELECT v.* FROM workflow_version v JOIN workflow w ON w.id=v.workflow_id WHERE w.workspace_id=$1`,
	}
	for name, query := range queries {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text),'[]'::jsonb) FROM (`+query+`) x`, c.Workspace).Scan(&raw); err != nil {
			return p, err
		}
		var canonical any
		if json.Unmarshal(raw, &canonical) != nil {
			return p, fmt.Errorf("invalid retained fact projection")
		}
		p.Original[name] = encoded(canonical)
	}
	for _, ref := range []string{contractRef, "industry-packs/park/quality/enterprise-activity-quality-v1.yaml", "industry-packs/park/quality/energy-raw-nonnegative-v1.yaml", "industry-packs/park/compliance/enterprise-activity-compliance-v1.yaml", "industry-packs/park/remediation/energy-quarantine-v1.yaml"} {
		b, readErr := os.ReadFile(filepath.Join(c.Repo, filepath.FromSlash(ref)))
		if readErr != nil {
			return p, fmt.Errorf("reviewed business rule file unavailable")
		}
		p.Files[ref] = hash(b)
		if ref == contractRef {
			var doc map[string]any
			if yaml.Unmarshal(b, &doc) != nil {
				return p, fmt.Errorf("reference contract invalid")
			}
			meta, metaOK := doc["metadata"].(map[string]any)
			body, bodyOK := doc["spec"].(map[string]any)
			product, productOK := body["product"].(map[string]any)
			if !metaOK || !bodyOK || !productOK {
				return p, fmt.Errorf("reference contract structure invalid")
			}
			meta["name"] = "trial-enterprise-activity-" + s.RunID
			product["code"] = "TRIAL-EA-" + s.RunID
			product["name"] = "Synthetic retained trial enterprise activity"
			body["consumers"] = []string{s.Consumer}
			body["purposes"] = []string{s.Purpose}
			b, err = yaml.Marshal(doc)
			if err != nil {
				return p, err
			}
			p.ContractYAML = string(b)
		}
	}
	explicit := func(v string) certdomain.Applicability {
		return certdomain.Applicability{Mode: certdomain.ApplicabilityExplicit, Values: []string{v}}
	}
	scopes := []certdomain.ScopeRef{}
	p.Authorization = rightsapp.CreateAuthorizationCommand{
		WorkspaceID: s.Workspace, Code: "TRIAL-EA-" + s.RunID, GrantorRef: "SYNTHETIC-TRIAL-" + s.RunID,
		GranteeRef: s.Consumer, Purpose: s.Purpose, ValidFrom: &s.ValidFrom, ValidTo: &s.ValidTo, ActorID: &s.Actor,
	}
	p.BindingAuthorityMode = rightsdomain.AuthorityDirect
	for _, id := range p.Resources {
		scopes = append(scopes, certdomain.ScopeRef{Type: "ALL_RESOURCE", Ref: id.String()})
		p.Authorization.Resources = append(p.Authorization.Resources, rightsdomain.ResourceGrantSpec{
			DataResourceID: id, Actions: []string{"READ"}, ScopeType: "ALL_RESOURCE", ScopeRef: id.String(),
		})
		p.Declarations = append(p.Declarations, rightsdomain.RightsDeclarationSpec{
			WorkspaceID: s.Workspace, DataResourceID: id, ClaimantRef: p.Authorization.GrantorRef,
			BasisType: "SYNTHETIC_TRIAL", BasisRef: businessContract + ":" + s.RunID,
			ConsumerScopeType: "EXPLICIT", ConsumerRef: s.Consumer, EffectiveFrom: &s.ValidFrom, EffectiveTo: &s.ValidTo,
			Parties:     []rightsdomain.RightsParty{{PartyRef: p.Authorization.GrantorRef, Role: "RIGHTS_HOLDER"}},
			Permissions: []rightsdomain.RightsPermission{{Kind: rightsdomain.PermissionGrant, Action: "READ", Purpose: s.Purpose, Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: id.String()}}}, ActorID: &s.Actor,
		})
	}
	p.Profile = certdomain.CertificationProfile{ProfileRef: "park/trial-" + s.RunID + "/certified-v1", Code: "TRIAL-EA-" + s.RunID, Name: "Synthetic retained trial certification", Version: "1.0.0", Purpose: explicit(s.Purpose), Actions: explicit("READ"), Consumers: explicit(s.Consumer), Delivery: explicit("DIRECT_DATA"), RequiredCriticalRules: []string{"QA-COMPANY-ID-COMPLETE", "QA-ENERGY-NONNEGATIVE"}, QualityGateRequired: true, Rights: certdomain.RightsRequirement{Required: true, Purpose: explicit(s.Purpose), Actions: explicit("READ"), Consumers: explicit(s.Consumer), Scopes: certdomain.ScopeApplicability{Mode: certdomain.ApplicabilityExplicit, Values: scopes}}, ComplianceRequired: true, ContractRequired: true, ContractCode: "TRIAL-EA-" + s.RunID, TraceabilityRequired: true, EvidenceRequired: true}
	if _, err = p.Profile.Snapshot(); err != nil {
		return p, err
	}
	p.Commands = []string{"CreateAuthorization(READ, three RAW resources)", "SubmitAuthorization", "ApproveAuthorization", "ActivateAuthorization", "CreateRightsDeclaration + VerifyRightsDeclaration + BindAuthorizationProvenance (each RAW)", "CreateContractVersion + PublishContractVersion", "CreateCertificationProfile"}
	return p, tx.Commit(ctx)
}

// Each Command commits its own business facts and AuditEvent together. A stable
// trace restores its exact object after a lost response, with no duplicate call.
type executor struct {
	pool           *pgxpool.Pool
	manager        *transaction.Manager
	p              plan
	digest         string
	afterStep      func(string) error
	beforeStep     func(string) error
	readOnlyReplay bool
}

func (e *executor) trace(name string) string {
	return "trial-business:" + hash([]byte(e.p.Spec.RunID+":"+e.digest+":"+name))
}
func (e *executor) step(ctx context.Context, name, action string, call func(string) (uuid.UUID, error)) (uuid.UUID, error) {
	rows, err := e.pool.Query(ctx, `SELECT object_id FROM audit_event WHERE workspace_id=$1 AND actor_id=$2 AND trace_id=$3 AND action=$4 ORDER BY id LIMIT 2`, e.p.Spec.Workspace, e.p.Spec.Actor, e.trace(name), action)
	if err != nil {
		return uuid.Nil, err
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err != nil {
		return uuid.Nil, err
	}
	if err = rows.Err(); err != nil {
		return uuid.Nil, err
	}
	if len(ids) > 1 {
		return uuid.Nil, fmt.Errorf("ambiguous committed command receipt")
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if e.readOnlyReplay {
		return uuid.Nil, fmt.Errorf("completed preparation lacks committed command receipt")
	}
	if e.beforeStep != nil {
		if err = e.beforeStep(name); err != nil {
			return uuid.Nil, err
		}
	}
	id, err := call(e.trace(name))
	if err != nil {
		return uuid.Nil, fmt.Errorf("Core prerequisite command failed: %s", name)
	}
	if e.afterStep != nil {
		if err = e.afterStep(name); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

func (e *executor) rights(ctx context.Context, objects map[string]uuid.UUID) error {
	p, m := e.p, e.manager
	s := p.Spec
	rights := rightsapp.NewService(m, rightsinfra.NewPostgresRepository(e.pool))
	actor := s.Actor
	auth, err := e.step(ctx, "authorization", "AUTHORIZATION_CREATED", func(trace string) (uuid.UUID, error) {
		command := p.Authorization
		command.TraceID = trace
		v, err := rights.Create(ctx, command)
		return v.ID, err
	})
	if err != nil {
		return err
	}
	objects["authorization"] = auth
	transitions := []struct {
		name, action string
		call         func(context.Context, rightsapp.TransitionCommand) (rightsdomain.Authorization, error)
	}{{"submit", "AUTHORIZATION_SUBMITTED", rights.Submit}, {"approve", "AUTHORIZATION_APPROVED", rights.Approve}, {"activate", "AUTHORIZATION_ACTIVATED", rights.Activate}}
	for _, t := range transitions {
		_, err = e.step(ctx, t.name, t.action, func(trace string) (uuid.UUID, error) {
			v, err := t.call(ctx, rightsapp.TransitionCommand{AuthorizationID: auth, ActorID: &actor, At: time.Now().UTC(), TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
	}
	for _, declaration := range p.Declarations {
		resource := declaration.DataResourceID
		key := resource.String()
		activity := uuid.NewSHA1(uuid.NameSpaceURL, []byte(e.trace("declaration-"+key)))
		dec, err := e.step(ctx, "declaration-"+key, "RIGHTS_DECLARATION_CREATED", func(trace string) (uuid.UUID, error) {
			v, err := rights.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{Spec: declaration, ActivityID: &activity, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		objects["declaration-"+key] = dec
		activity = uuid.NewSHA1(uuid.NameSpaceURL, []byte(e.trace("verification-"+key)))
		verification, err := e.step(ctx, "verification-"+key, "RIGHTS_DECLARATION_VERIFIED", func(trace string) (uuid.UUID, error) {
			v, err := rights.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{DeclarationID: dec, Outcome: rightsdomain.DeclarationVerified, Reason: s.Reason, ActivityID: &activity, ActorID: &actor, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		objects["verification-"+key] = verification
		activity = uuid.NewSHA1(uuid.NameSpaceURL, []byte(e.trace("binding-"+key)))
		binding, err := e.step(ctx, "binding-"+key, "AUTHORIZATION_PROVENANCE_BOUND", func(trace string) (uuid.UUID, error) {
			v, err := rights.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{WorkspaceID: s.Workspace, AuthorizationID: auth, DataResourceID: resource, DeclarationID: dec, GrantorRef: p.Authorization.GrantorRef, AuthorityMode: p.BindingAuthorityMode, ActivityID: &activity, ActorID: &actor, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		objects["binding-"+key] = binding
	}
	return nil
}

func prepare(ctx context.Context, pool *pgxpool.Pool, p plan, afterStep func(string) error) (receipt, error) {
	digest := hash(encoded(p))
	s := p.Spec
	result := receipt{Contract: businessContract, Status: "PREPARED", RunID: s.RunID, Workspace: s.Workspace, Actor: s.Actor, Principal: p.Principal, PlanSHA256: digest, Objects: map[string]uuid.UUID{}}
	m := transaction.NewManager(pool)
	e := executor{pool: pool, manager: m, p: p, digest: digest, afterStep: afterStep}
	err := m.WithAdvisoryLock(ctx, "trial-business:"+s.Workspace.String()+":"+s.RunID, func(ctx context.Context) error {
		var fingerprint string
		var completed *uuid.UUID
		err := pool.QueryRow(ctx, `SELECT request_fingerprint,result_ref FROM command_idempotency WHERE workspace_id=$1 AND command_type='TRIAL.BUSINESS_PREREQUISITES' AND idempotency_key=$2`, s.Workspace, s.RunID).Scan(&fingerprint, &completed)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && fingerprint != digest {
			return fmt.Errorf("preparation idempotency input conflict")
		}
		e.readOnlyReplay = completed != nil
		if completed == nil && (time.Now().Before(s.ValidFrom) || !time.Now().Before(s.ValidTo)) {
			return fmt.Errorf("preparation validity window is not current; no automatic renewal")
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = m.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO command_idempotency(workspace_id,command_type,idempotency_key,object_id,request_fingerprint) VALUES ($1,'TRIAL.BUSINESS_PREREQUISITES',$2,$3,$4)`, s.Workspace, s.RunID, s.Target, digest)
				return err
			})
			if err != nil {
				return err
			}
		}
		if err := e.rights(ctx, result.Objects); err != nil {
			return err
		}
		actor := s.Actor
		contracts := contractapp.NewService(m, contractinfra.NewPostgresRepository(pool))
		cv, err := e.step(ctx, "contract", "CONTRACT_VERSION_CREATED", func(trace string) (uuid.UUID, error) {
			v, err := contracts.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{WorkspaceID: s.Workspace, SourceRef: businessContract + ":" + contractRef, DocumentYAML: []byte(p.ContractYAML), ActorID: &actor, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		result.Objects["contractVersion"] = cv
		_, err = e.step(ctx, "publish-contract", "CONTRACT_VERSION_PUBLISHED", func(trace string) (uuid.UUID, error) {
			v, err := contracts.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: cv, ActorID: &actor, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		profile, err := e.step(ctx, "profile", "CERTIFICATION_PROFILE_CREATED", func(trace string) (uuid.UUID, error) {
			v, err := certapp.NewProfileService(m, certinfra.NewProfileRepository(pool)).Create(ctx, certapp.CreateProfileCommand{WorkspaceID: s.Workspace, Profile: p.Profile, ActorID: &actor, TraceID: trace})
			return v.ID, err
		})
		if err != nil {
			return err
		}
		result.Objects["profile"] = profile
		if completed != nil {
			if *completed != profile {
				return fmt.Errorf("completed preparation receipt drift")
			}
			return nil
		}
		return m.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE command_idempotency SET result_ref=$3 WHERE workspace_id=$1 AND command_type='TRIAL.BUSINESS_PREREQUISITES' AND idempotency_key=$2 AND result_ref IS NULL`, s.Workspace, s.RunID, profile)
			return err
		})
	})
	return result, err
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "renewal-plan" || args[0] == "renew-rights") {
		return runRenewal(args)
	}
	if len(args) == 0 || (args[0] != "plan" && args[0] != "prepare") {
		return fmt.Errorf("explicit plan|prepare required")
	}
	flags := flag.NewFlagSet("trial-business", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	specFile := flags.String("spec", "", "private specification")
	planFile := flags.String("plan", "", "reviewed plan")
	sha := flags.String("sha256", "", "reviewed plan SHA256")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *specFile == "" || (args[0] == "plan" && (*planFile != "" || *sha != "")) || (args[0] == "prepare" && (*planFile == "" || len(*sha) != 64)) {
		return fmt.Errorf("bounded explicit plan arguments required")
	}
	var c config
	var r registry
	var s spec
	configFile := os.Getenv("DPP_TRIAL_CONFIG")
	if decode(configFile, &c, false) != nil || decode(filepath.Join(c.Repo, ".data/trial/state.json"), &r, false) != nil || decode(*specFile, &s, true) != nil || validate(c, r, s) != nil {
		return fmt.Errorf("same retained private inputs required")
	}
	if configFile != filepath.Join(c.Private, "config.json") {
		return fmt.Errorf("original private config path required")
	}
	for _, path := range []string{configFile, *specFile} {
		rel, err := filepath.Rel(c.Private, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || pilot.CheckCustodyPrivatePath(path, false) != nil {
			return fmt.Errorf("operator-private inputs required")
		}
	}
	if pilot.CheckCustodyPrivatePath(c.Private, true) != nil {
		return fmt.Errorf("operator-private root required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, c.DSN)
	if err != nil {
		return fmt.Errorf("Core unavailable")
	}
	defer pool.Close()
	p, err := makePlan(ctx, pool, c, r, s)
	if err != nil {
		return fmt.Errorf("read-only business admission refused")
	}
	if args[0] == "plan" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"plan": p, "planSha256": hash(encoded(p))})
	}
	var reviewed plan
	if decode(*planFile, &reviewed, true) != nil || hash(encoded(reviewed)) != *sha || !bytes.Equal(encoded(reviewed), encoded(p)) {
		return fmt.Errorf("reviewed business plan drift")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return fmt.Errorf("Core routing unavailable")
	}
	outbox.ConfigureAppendObligation(router)
	result, err := prepare(ctx, pool, p, nil)
	if err != nil {
		return fmt.Errorf("business preparation refused; preserve the original claim and observations")
	}
	check, err := makePlan(ctx, pool, c, r, s)
	if err != nil || !bytes.Equal(encoded(p), encoded(check)) {
		return fmt.Errorf("retained business facts changed; preserve UNKNOWN observation")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "REFUSED:", err)
		os.Exit(1)
	}
}
