package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const renewalContract = "issue325/business-rights-renewal-v1"
const renewalCommand = "TRIAL.BUSINESS_RIGHTS_RENEWAL"
const preparationCommand = "TRIAL.BUSINESS_PREREQUISITES"

type fileBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type renewalSpec struct {
	Contract          string    `json:"contract"`
	RenewalID         uuid.UUID `json:"renewalId"`
	RunID             string    `json:"runId"`
	Workspace         uuid.UUID `json:"workspaceId"`
	Actor             uuid.UUID `json:"actorId"`
	PreparationSHA256 string    `json:"preparationSha256"`
	ValidFrom         time.Time `json:"validFrom"`
	ValidTo           time.Time `json:"validTo"`
	Reason            string    `json:"verificationReason"`
}
type rightsWindow struct {
	PlanSHA256 string               `json:"planSha256"`
	Command    string               `json:"command"`
	ClaimKey   string               `json:"claimKey"`
	ValidFrom  time.Time            `json:"validFrom"`
	ValidTo    time.Time            `json:"validTo"`
	Objects    map[string]uuid.UUID `json:"objects"`
}
type renewalPlan struct {
	Contract        string       `json:"contract"`
	Spec            renewalSpec  `json:"spec"`
	Preparation     fileBinding  `json:"preparationBinding"`
	Original        plan         `json:"originalPlan"`
	OriginalReceipt receipt      `json:"originalReceipt"`
	Parent          rightsWindow `json:"parent"`
	Next            plan         `json:"next"`
}
type renewalReceipt struct {
	Contract           string               `json:"contract"`
	Status             string               `json:"status"`
	RunID              string               `json:"runId"`
	Workspace          uuid.UUID            `json:"workspaceId"`
	Actor              uuid.UUID            `json:"actorId"`
	Principal          string               `json:"principalRef"`
	RenewalID          uuid.UUID            `json:"renewalId"`
	PlanSHA256         string               `json:"planSha256"`
	ParentPlanSHA256   string               `json:"parentPlanSha256"`
	Objects            map[string]uuid.UUID `json:"objects"`
	DeliveryAuthorized bool                 `json:"deliveryAuthorized"`
}
type preparationManifest struct {
	Contract       string      `json:"contract"`
	Status         string      `json:"status"`
	RunID          string      `json:"runId"`
	Workspace      uuid.UUID   `json:"workspaceId"`
	Actor          uuid.UUID   `json:"actorId"`
	Root           string      `json:"root"`
	Activation     fileBinding `json:"activationBinding"`
	CorePlanSHA256 string      `json:"corePlanSha256"`
	Receipt        receipt     `json:"receipt"`
}
type renewalManifest struct {
	Contract       string         `json:"contract"`
	Status         string         `json:"status"`
	RunID          string         `json:"runId"`
	Workspace      uuid.UUID      `json:"workspaceId"`
	Actor          uuid.UUID      `json:"actorId"`
	Root           string         `json:"root"`
	Preparation    fileBinding    `json:"preparationBinding"`
	CorePlanSHA256 string         `json:"corePlanSha256"`
	Receipt        renewalReceipt `json:"receipt"`
}

func renewalKey(run, parent string) string { return run + ":" + parent }
func renewalCode(run string, id uuid.UUID) string {
	return "TRIAL-RENEW-" + hash([]byte(run + ":" + id.String()))[:48]
}

func privateDocument(c config, path string, dst any) error {
	rel, err := filepath.Rel(c.Private, path)
	if err != nil || !filepath.IsAbs(path) || rel == "." || strings.HasPrefix(rel, "..") || pilot.CheckCustodyPrivatePath(path, false) != nil {
		return fmt.Errorf("bound operator-private document required")
	}
	return decode(path, dst, false)
}
func boundDocument(c config, ref fileBinding, stage string, dst any) error {
	rel, err := filepath.Rel(filepath.Join(c.Private, stage), ref.Path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || pilot.CheckCustodyPrivatePath(ref.Path, false) != nil {
		return fmt.Errorf("bound historical manifest path drift")
	}
	b, readErr := os.ReadFile(ref.Path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || readErr != nil || hash(b) != ref.SHA256 {
		return fmt.Errorf("bound historical manifest drift")
	}
	return privateDocument(c, ref.Path, dst)
}

// Only immutable private documents and SQL reads are used for plan construction.
func loadRenewalBasis(c config, r registry, s renewalSpec) (plan, receipt, rightsWindow, error) {
	var manifest preparationManifest
	var original plan
	var none rightsWindow
	if boundDocument(c, r.PreparationBinding, "business-preparations", &manifest) != nil || manifest.Contract != businessContract || manifest.Status != "PREPARED" || manifest.RunID != r.RunID || manifest.Workspace != r.Workspace || manifest.Actor != r.Actor || manifest.Root != r.Root || manifest.Activation != r.ActivationBinding ||
		privateDocument(c, filepath.Join(filepath.Dir(r.PreparationBinding.Path), "core-plan.json"), &original) != nil || hash(encoded(original)) != manifest.CorePlanSHA256 || manifest.Receipt.PlanSHA256 != manifest.CorePlanSHA256 || manifest.Receipt.Status != "PREPARED" || manifest.Receipt.DeliveryAuthorized || manifest.Receipt.RunID != r.RunID || manifest.Receipt.Workspace != r.Workspace || manifest.Receipt.Actor != r.Actor || manifest.Receipt.Principal != c.Profile.Subject {
		return original, manifest.Receipt, none, fmt.Errorf("original completed preparation binding required")
	}
	parent := rightsWindow{PlanSHA256: manifest.CorePlanSHA256, Command: preparationCommand, ClaimKey: r.RunID, ValidFrom: original.Spec.ValidFrom, ValidTo: original.Spec.ValidTo, Objects: manifest.Receipt.Objects}
	if r.RenewalBinding.Path != "" {
		var latest renewalManifest
		var previous renewalPlan
		if boundDocument(c, r.RenewalBinding, "business-rights-renewals", &latest) != nil || latest.Contract != renewalContract || latest.Status != "RENEWED" || latest.RunID != r.RunID || latest.Workspace != r.Workspace || latest.Actor != r.Actor || latest.Root != r.Root || latest.Preparation != r.PreparationBinding ||
			privateDocument(c, filepath.Join(filepath.Dir(r.RenewalBinding.Path), "core-plan.json"), &previous) != nil || hash(encoded(previous)) != latest.CorePlanSHA256 || latest.Receipt.PlanSHA256 != latest.CorePlanSHA256 || latest.Receipt.Contract != renewalContract || latest.Receipt.Status != "RENEWED" || latest.Receipt.DeliveryAuthorized || latest.Receipt.RunID != r.RunID || latest.Receipt.Workspace != r.Workspace || latest.Receipt.Actor != r.Actor || latest.Receipt.Principal != c.Profile.Subject || latest.Receipt.RenewalID != previous.Spec.RenewalID || latest.Receipt.ParentPlanSHA256 != previous.Parent.PlanSHA256 || previous.Preparation != r.PreparationBinding || !bytes.Equal(encoded(previous.Original), encoded(original)) || !bytes.Equal(encoded(previous.OriginalReceipt), encoded(manifest.Receipt)) {
			return original, manifest.Receipt, none, fmt.Errorf("latest explicit renewal binding drift")
		}
		parent = rightsWindow{PlanSHA256: latest.CorePlanSHA256, Command: renewalCommand, ClaimKey: renewalKey(r.RunID, previous.Parent.PlanSHA256), ValidFrom: previous.Spec.ValidFrom, ValidTo: previous.Spec.ValidTo, Objects: latest.Receipt.Objects}
		if previous.Spec.RenewalID == s.RenewalID {
			parent = previous.Parent
		}
	}
	return original, manifest.Receipt, parent, nil
}

func auditRightsObjects(ctx context.Context, tx pgx.Tx, base plan, digest string, objects map[string]uuid.UUID, prepared bool) error {
	e := executor{p: base, digest: digest}
	actions := map[string]string{"authorization": "AUTHORIZATION_CREATED", "submit": "AUTHORIZATION_SUBMITTED", "approve": "AUTHORIZATION_APPROVED", "activate": "AUTHORIZATION_ACTIVATED"}
	for _, resource := range base.Resources {
		key := resource.String()
		actions["declaration-"+key] = "RIGHTS_DECLARATION_CREATED"
		actions["verification-"+key] = "RIGHTS_DECLARATION_VERIFIED"
		actions["binding-"+key] = "AUTHORIZATION_PROVENANCE_BOUND"
	}
	if prepared {
		actions["contract"] = "CONTRACT_VERSION_CREATED"
		actions["publish-contract"] = "CONTRACT_VERSION_PUBLISHED"
		actions["profile"] = "CERTIFICATION_PROFILE_CREATED"
	}
	for name, action := range actions {
		key := name
		if name == "submit" || name == "approve" || name == "activate" {
			key = "authorization"
		}
		if name == "contract" || name == "publish-contract" {
			key = "contractVersion"
		}
		id := objects[key]
		var n int
		if id == uuid.Nil {
			return fmt.Errorf("exact committed object identity required")
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND actor_id=$2 AND trace_id=$3 AND action=$4 AND object_id=$5`, base.Spec.Workspace, base.Spec.Actor, e.trace(name), action, id).Scan(&n); err != nil || n != 1 {
			return fmt.Errorf("committed audit identity drift")
		}
	}
	return nil
}
func committedWindow(ctx context.Context, tx pgx.Tx, base plan, w rightsWindow) error {
	if len(w.PlanSHA256) != 64 || (w.Command != preparationCommand && w.Command != renewalCommand) || !w.ValidTo.After(w.ValidFrom) {
		return fmt.Errorf("fixed predecessor identity required")
	}
	if w.Command == preparationCommand && (w.ClaimKey != base.Spec.RunID || w.PlanSHA256 != hash(encoded(base)) || !w.ValidFrom.Equal(base.Spec.ValidFrom) || !w.ValidTo.Equal(base.Spec.ValidTo)) {
		return fmt.Errorf("original predecessor identity drift")
	}
	if w.Command == renewalCommand && !strings.HasPrefix(w.ClaimKey, base.Spec.RunID+":") {
		return fmt.Errorf("same-run predecessor key required")
	}
	var fingerprint string
	var completed *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT request_fingerprint,result_ref FROM command_idempotency WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3 AND object_id=$4`, base.Spec.Workspace, w.Command, w.ClaimKey, base.Spec.Target).Scan(&fingerprint, &completed)
	result := w.Objects["authorization"]
	if w.Command == preparationCommand {
		result = w.Objects["profile"]
	}
	if err != nil || fingerprint != w.PlanSHA256 || completed == nil || *completed != result {
		return fmt.Errorf("completed predecessor Core claim required")
	}
	return auditRightsObjects(ctx, tx, base, w.PlanSHA256, w.Objects, w.Command == preparationCommand)
}
func verifyWindowScope(ctx context.Context, tx pgx.Tx, base plan, w rightsWindow) error {
	var n int
	cmd := base.Authorization
	err := tx.QueryRow(ctx, `SELECT count(*) FROM data_authorization WHERE id=$1 AND workspace_id=$2 AND created_by=$3 AND grantor_ref=$4 AND grantee_ref=$5 AND purpose=$6 AND valid_from=$7 AND valid_to=$8 AND status IN ('ACTIVE','EXPIRED')`, w.Objects["authorization"], base.Spec.Workspace, base.Spec.Actor, cmd.GrantorRef, cmd.GranteeRef, cmd.Purpose, w.ValidFrom, w.ValidTo).Scan(&n)
	if err != nil || n != 1 {
		return fmt.Errorf("predecessor rights were changed, suspended or revoked")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM authorization_resource WHERE authorization_id=$1`, w.Objects["authorization"]).Scan(&n); err != nil || n != 3 {
		return fmt.Errorf("exact predecessor resource count required")
	}
	for _, resource := range base.Resources {
		key := resource.String()
		declaration, binding := w.Objects["declaration-"+key], w.Objects["binding-"+key]
		err = tx.QueryRow(ctx, `SELECT count(*) FROM authorization_resource WHERE authorization_id=$1 AND data_resource_id=$2 AND actions=ARRAY['READ']::text[] AND scope_type='ALL_RESOURCE' AND scope_ref=$2::text AND NOT raw_export_allowed`, w.Objects["authorization"], resource).Scan(&n)
		if err != nil || n != 1 {
			return fmt.Errorf("predecessor normalized READ scope drift")
		}
		err = tx.QueryRow(ctx, `SELECT count(*) FROM rights_declaration d JOIN rights_declaration_verification v ON v.declaration_id=d.id WHERE d.id=$1 AND d.workspace_id=$2 AND d.data_resource_id=$3 AND d.claimant_ref=$4 AND d.basis_type='SYNTHETIC_TRIAL' AND d.consumer_scope_type='EXPLICIT' AND d.consumer_ref=$5 AND d.effective_from=$6 AND d.effective_to=$7 AND v.id=$8 AND v.outcome='VERIFIED' AND NOT EXISTS(SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id)`, declaration, base.Spec.Workspace, resource, cmd.GrantorRef, base.Spec.Consumer, w.ValidFrom, w.ValidTo, w.Objects["verification-"+key]).Scan(&n)
		if err != nil || n != 1 {
			return fmt.Errorf("predecessor declaration verification/disposition drift")
		}
		err = tx.QueryRow(ctx, `SELECT count(*) FROM authorization_provenance_binding b WHERE b.id=$1 AND b.workspace_id=$2 AND b.authorization_id=$3 AND b.data_resource_id=$4 AND b.rights_declaration_id=$5 AND b.grantor_ref=$6 AND b.grantor_authority_mode='DIRECT_DECLARATION_PARTY' AND b.delegation_chain_id IS NULL AND b.delegation_chain_hash IS NULL AND NOT EXISTS(SELECT 1 FROM authorization_provenance_binding_disposition x WHERE x.binding_id=b.id)`, binding, base.Spec.Workspace, w.Objects["authorization"], resource, declaration, cmd.GrantorRef).Scan(&n)
		if err != nil || n != 1 {
			return fmt.Errorf("predecessor provenance binding drift")
		}
	}
	return nil
}

func makeRenewalPlan(ctx context.Context, pool *pgxpool.Pool, c config, r registry, s renewalSpec, original plan, old receipt, parent rightsWindow) (renewalPlan, error) {
	p := renewalPlan{Contract: renewalContract, Spec: s, Preparation: r.PreparationBinding, Original: original, OriginalReceipt: old, Parent: parent}
	// Core declarations and PostgreSQL persist microseconds. Reject lossy input
	// before freezing a plan so its window can remain an exact future predecessor.
	_, fromOffset := s.ValidFrom.Zone()
	_, toOffset := s.ValidTo.Zone()
	if fromOffset != 0 || toOffset != 0 || s.ValidFrom.Nanosecond()%1000 != 0 || s.ValidTo.Nanosecond()%1000 != 0 {
		return p, fmt.Errorf("renewal window requires exact UTC microseconds")
	}
	if s.Contract != renewalContract || s.RenewalID == uuid.Nil || s.RunID != r.RunID || s.Workspace != r.Workspace || s.Actor != r.Actor || s.PreparationSHA256 != r.PreparationBinding.SHA256 || len(strings.TrimSpace(s.Reason)) < 10 || len(s.Reason) > 1024 || s.ValidFrom.IsZero() || !s.ValidTo.After(s.ValidFrom) || s.ValidTo.Sub(s.ValidFrom) > original.Spec.ValidTo.Sub(original.Spec.ValidFrom) || s.ValidTo.Sub(s.ValidFrom) > 24*time.Hour || s.ValidFrom.Before(parent.ValidTo) {
		return p, fmt.Errorf("explicit same-scope finite successor required")
	}
	check, err := makePlan(ctx, pool, c, r, original.Spec)
	if err != nil || !bytes.Equal(encoded(check), encoded(original)) || old.PlanSHA256 != hash(encoded(original)) || old.Contract != businessContract || old.Status != "PREPARED" || old.RunID != r.RunID || old.Workspace != r.Workspace || old.Actor != r.Actor || old.Principal != c.Profile.Subject || old.DeliveryAuthorized {
		return p, fmt.Errorf("original preparation content/coverage drift")
	}
	nextSpec := original.Spec
	nextSpec.ValidFrom = s.ValidFrom
	nextSpec.ValidTo = s.ValidTo
	nextSpec.Reason = s.Reason
	next, err := makePlan(ctx, pool, c, r, nextSpec)
	if err != nil {
		return p, err
	}
	next.Authorization.Code = renewalCode(s.RunID, s.RenewalID)
	p.Next = next
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return p, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root := rightsWindow{PlanSHA256: old.PlanSHA256, Command: preparationCommand, ClaimKey: r.RunID, ValidFrom: original.Spec.ValidFrom, ValidTo: original.Spec.ValidTo, Objects: old.Objects}
	if err = committedWindow(ctx, tx, original, root); err != nil {
		return p, err
	}
	if err = committedWindow(ctx, tx, original, parent); err != nil {
		return p, err
	}
	if parent.Command == renewalCommand && !strings.HasPrefix(parent.ClaimKey, r.RunID+":") {
		return p, fmt.Errorf("same-run predecessor key required")
	}
	if err = verifyWindowScope(ctx, tx, original, parent); err != nil {
		return p, err
	}
	profile, err := original.Profile.Snapshot()
	if err != nil {
		return p, err
	}
	var n int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM certification_profile WHERE id=$1 AND workspace_id=$2 AND membership_state='FINALIZED' AND content_sha256=$3 AND content_snapshot=$4`, old.Objects["profile"], r.Workspace, profile.ContentSHA256, string(profile.Content)).Scan(&n); err != nil || n != 1 {
		return p, fmt.Errorf("reuse exact original profile required")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM contract_version v JOIN data_contract d ON d.id=v.contract_id WHERE v.id=$1 AND d.workspace_id=$2 AND v.status='PUBLISHED' AND v.source_sha256=$3`, old.Objects["contractVersion"], r.Workspace, hash([]byte(original.ContractYAML))).Scan(&n); err != nil || n != 1 {
		return p, fmt.Errorf("reuse exact original published contract required")
	}
	return p, tx.Commit(ctx)
}

func renewRights(ctx context.Context, pool *pgxpool.Pool, p renewalPlan, recheck func() error, afterStep func(string) error) (renewalReceipt, error) {
	digest := hash(encoded(p))
	s := p.Spec
	result := renewalReceipt{Contract: renewalContract, Status: "RENEWED", RunID: s.RunID, Workspace: s.Workspace, Actor: s.Actor, Principal: p.Next.Principal, RenewalID: s.RenewalID, PlanSHA256: digest, ParentPlanSHA256: p.Parent.PlanSHA256, Objects: map[string]uuid.UUID{}}
	m := transaction.NewManager(pool)
	e := executor{pool: pool, manager: m, p: p.Next, digest: digest, afterStep: afterStep}
	e.beforeStep = func(string) error {
		at := time.Now().UTC()
		if at.Before(s.ValidFrom) || !at.Before(s.ValidTo) || at.Before(p.Parent.ValidTo) {
			return fmt.Errorf("renewal window is no longer current")
		}
		if recheck != nil {
			return recheck()
		}
		return nil
	}
	err := m.WithAdvisoryLock(ctx, "trial-business:"+s.Workspace.String()+":"+s.RunID, func(ctx context.Context) error {
		key := renewalKey(s.RunID, p.Parent.PlanSHA256)
		var fingerprint string
		var completed *uuid.UUID
		err := pool.QueryRow(ctx, `SELECT request_fingerprint,result_ref FROM command_idempotency WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3`, s.Workspace, renewalCommand, key).Scan(&fingerprint, &completed)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && fingerprint != digest {
			return fmt.Errorf("predecessor already claimed by a different renewal")
		}
		e.readOnlyReplay = completed != nil
		if completed == nil {
			at := time.Now().UTC()
			if at.Before(s.ValidFrom) || !at.Before(s.ValidTo) || at.Before(p.Parent.ValidTo) {
				return fmt.Errorf("renewal window is not current or predecessor not expired")
			}
			if recheck != nil {
				if err := recheck(); err != nil {
					return err
				}
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			if err = m.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO command_idempotency(workspace_id,command_type,idempotency_key,object_id,request_fingerprint) VALUES($1,$2,$3,$4,$5)`, s.Workspace, renewalCommand, key, p.Next.Spec.Target, digest)
				return err
			}); err != nil {
				return err
			}
		}
		if err = e.rights(ctx, result.Objects); err != nil {
			return err
		}
		result.Objects["contractVersion"] = p.OriginalReceipt.Objects["contractVersion"]
		result.Objects["profile"] = p.OriginalReceipt.Objects["profile"]
		if completed != nil {
			if *completed != result.Objects["authorization"] {
				return fmt.Errorf("renewal receipt drift")
			}
			return nil
		}
		return m.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE command_idempotency SET result_ref=$4 WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3 AND result_ref IS NULL`, s.Workspace, renewalCommand, key, result.Objects["authorization"])
			return err
		})
	})
	return result, err
}

func runRenewal(args []string) error {
	flags := flag.NewFlagSet("trial-business renewal", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	specFile := flags.String("spec", "", "private renewal specification")
	planFile := flags.String("plan", "", "reviewed plan")
	sha := flags.String("sha256", "", "reviewed SHA256")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *specFile == "" || (args[0] == "renewal-plan" && (*planFile != "" || *sha != "")) || (args[0] == "renew-rights" && (*planFile == "" || len(*sha) != 64)) {
		return fmt.Errorf("explicit bounded renewal arguments required")
	}
	var c config
	var r registry
	var s renewalSpec
	configFile := os.Getenv("DPP_TRIAL_CONFIG")
	if decode(configFile, &c, false) != nil || decode(filepath.Join(c.Repo, ".data/trial/state.json"), &r, false) != nil || decode(*specFile, &s, true) != nil || configFile != filepath.Join(c.Private, "config.json") || pilot.CheckCustodyPrivatePath(c.Private, true) != nil || privateDocument(c, configFile, &config{}) != nil || privateDocument(c, *specFile, &renewalSpec{}) != nil {
		return fmt.Errorf("same retained private renewal inputs required")
	}
	original, old, parent, err := loadRenewalBasis(c, r, s)
	if err != nil {
		return err
	}
	if validate(c, r, original.Spec) != nil {
		return fmt.Errorf("same original preparation identity required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, c.DSN)
	if err != nil {
		return fmt.Errorf("Core unavailable")
	}
	defer pool.Close()
	makeCurrent := func() (renewalPlan, error) { return makeRenewalPlan(ctx, pool, c, r, s, original, old, parent) }
	p, err := makeCurrent()
	if err != nil {
		return fmt.Errorf("readonly renewal admission refused: %w", err)
	}
	if args[0] == "renewal-plan" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"plan": p, "planSha256": hash(encoded(p))})
	}
	var reviewed renewalPlan
	if privateDocument(c, *planFile, &reviewed) != nil || decode(*planFile, &reviewed, true) != nil || hash(encoded(reviewed)) != *sha || !bytes.Equal(encoded(reviewed), encoded(p)) {
		return fmt.Errorf("reviewed renewal plan drift")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(router)
	result, err := renewRights(ctx, pool, p, func() error {
		current, err := makeCurrent()
		if err != nil || !bytes.Equal(encoded(current), encoded(p)) {
			return fmt.Errorf("renewal facts drift")
		}
		return nil
	}, nil)
	if err != nil {
		return fmt.Errorf("renewal refused; preserve original claims and observations: %w", err)
	}
	check, err := makeCurrent()
	if err != nil || !bytes.Equal(encoded(check), encoded(p)) {
		return fmt.Errorf("retained renewal facts changed; preserve UNKNOWN")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
