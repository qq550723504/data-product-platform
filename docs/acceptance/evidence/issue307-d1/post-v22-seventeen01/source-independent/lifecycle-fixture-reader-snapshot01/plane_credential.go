package main

// Closed fixture adapter. Native secrets stay in the private composition;
// delivery returns only a platform redemption handle, never an S3 credential.
import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	iampolicy "github.com/minio/pkg/v3/policy"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func credentialPolicy(cp ld.Copy) []byte {
	p, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cp.ProviderVersion}}}}})
	return p
}

type nativeCredential struct {
	c                                   config
	pool                                *pgxpool.Pool
	copy                                ld.Copy
	occupancy                           *lc.OccupancyService
	loseRevoke                          bool
	forensicRoot, originalInvalidRecipe bool // exact retained failed native attempts only
}

func (p *nativeCredential) keys(k string) (string, string, string) {
	mac := func(s string) string {
		h := hmac.New(sha256.New, []byte(p.c.IssuerSecret))
		h.Write([]byte(k + ":" + s))
		return hex.EncodeToString(h.Sum(nil))
	}
	if p.originalInvalidRecipe {
		return "lc13cap" + mac("access")[:24], mac("secret"), mac("redemption")
	}
	return "lc13cap" + mac("access")[:12], mac("secret")[:40], mac("redemption")
}
func (p *nativeCredential) admin(ctx context.Context) (*madmin.AdminClient, context.Context, error) {
	if p.forensicRoot && p.c.RootAccess != "" && p.c.RootSecret != "" {
		a, e := madmin.New(p.c.Endpoint, p.c.RootAccess, p.c.RootSecret, false)
		if e != nil {
			return nil, ctx, e
		}
		a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, p.c})
		return a, context.WithValue(ctx, adminKey{}, &adminCall{}), nil
	}
	if p.c.RootAccess != "" || p.c.RootSecret != "" || p.c.IssuerAccess == "" || p.c.IssuerSecret == "" {
		return nil, ctx, lc.ErrStorageScope
	}
	a, e := madmin.New(p.c.Endpoint, p.c.IssuerAccess, p.c.IssuerSecret, false)
	if e != nil {
		return nil, ctx, e
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, p.c})
	return a, context.WithValue(ctx, adminKey{}, &adminCall{}), nil
}
func (p *nativeCredential) operation(ctx context.Context, k string) (dd.Operation, error) {
	tx, e := p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if e != nil {
		return dd.Operation{}, e
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	if e = tx.QueryRow(ctx, `SELECT id FROM delivery_operation WHERE workspace_id=$1 AND provider_request_key=$2 AND provider_name='LC13_MINIO_REDEMPTION'`, p.c.WorkspaceID, k).Scan(&id); e != nil {
		return dd.Operation{}, e
	}
	return deliveryinfra.NewPostgresRepository(p.pool).GetOperation(ctx, tx, id, false)
}
func (p *nativeCredential) capability(o dd.Operation, expiry time.Time) dd.Capability {
	_, _, handle := p.keys(o.ProviderRequestKey)
	ref := sha256.Sum256([]byte(o.ProviderRequestKey + ":" + handle))
	h := sha256.Sum256([]byte(handle))
	return dd.Capability{Credential: handle, CapabilityRef: "lc13-redemption:" + hex.EncodeToString(ref[:]), CapabilityHash: hex.EncodeToString(h[:]), ProviderCredentialExpiresAt: expiry, DatasetVersionID: o.DatasetVersionID, ConsumerRef: o.EffectiveConsumerRef, Action: o.Action, ScopeRef: o.ScopeRef, DeliveryChannel: o.DeliveryChannel, DeliveryMode: o.DeliveryMode, AuthoritativelyVerified: true}
}
func (p *nativeCredential) Issue(ctx context.Context, r deliveryapp.ProviderRequest) (dd.Capability, error) {
	if p.forensicRoot {
		return dd.Capability{}, lc.ErrStorageUnsupported
	}
	o, e := p.operation(ctx, r.ProviderRequestKey)
	if e != nil {
		return dd.Capability{}, e
	}
	if o.DatasetVersionID != p.copy.DatasetVersionID || r.ConsumerRef != pilot.PlaneConsumer || r.Purpose != pilot.PlanePurpose || r.Action != "READ" || r.ScopeRef != p.copy.DatasetVersionID.String() || r.DeliveryChannel != "REDEMPTION" || r.DeliveryMode != "PLATFORM_REDEMPTION" {
		return dd.Capability{}, lc.ErrStorageScope
	}
	refs := lc.NewFixtureReferenceService(p.occupancy, deliveryapp.FixtureReferenceOwner{})
	if _, e = refs.DeclareFixtureReference(ctx, lc.DeclareFixtureReferenceCommand{CommandHeader: lc.CommandHeader{WorkspaceID: p.c.WorkspaceID, IdempotencyKey: "credential-material:" + o.ID.String()}, CopyID: p.copy.ID, Reference: contentreference.Reference{Kind: "DELIVERY", SubjectID: o.ID}, BasisRef: "Native service-account ownership; required until trusted containment"}); e != nil {
		return dd.Capability{}, e
	}
	a, x, e := p.admin(ctx)
	if e != nil {
		return dd.Capability{}, e
	}
	access, secret, _ := p.keys(r.ProviderRequestKey)
	expiry := r.ExpiresAt.UTC().Truncate(time.Second)
	result, e := a.AddServiceAccount(x, madmin.AddServiceAccountReq{TargetUser: p.c.IssuerAccess, AccessKey: access, SecretKey: secret, Policy: credentialPolicy(p.copy), Expiration: &expiry})
	if e != nil {
		message := madmin.ToErrorResponse(e).Message
		for _, s := range []string{access, secret, p.c.IssuerAccess, p.c.IssuerSecret, p.c.Token, p.c.Access, p.c.Secret, p.c.InspectAccess, p.c.InspectSecret, p.c.ProducerAccess, p.c.ProducerSecret} {
			if s != "" {
				message = strings.ReplaceAll(message, s, "[redacted]")
			}
		}
		_ = write(p.c, "credential-native-error-"+uuid.NewString()+".json", map[string]any{"code": madmin.ToErrorResponse(e).Code, "sanitized_message": message})
		return dd.Capability{}, deliveryapp.ErrUnknownProviderOutcome
	}
	if result.AccessKey != access || result.SecretKey != secret || !result.Expiration.Equal(expiry) {
		return dd.Capability{}, deliveryapp.ErrUnknownProviderOutcome
	}
	return p.capability(o, result.Expiration), nil
}
func canonicalPolicy(s []byte) string {
	var x any
	if json.Unmarshal(s, &x) != nil {
		return ""
	}
	b, _ := json.Marshal(x)
	return string(b)
}
func sameNativePolicy(actual string, expected []byte) bool {
	a, e := iampolicy.ParseConfig(strings.NewReader(actual))
	if e != nil {
		return false
	}
	b, e := iampolicy.ParseConfig(bytes.NewReader(expected))
	return e == nil && a.Equals(*b)
}
func (p *nativeCredential) Recover(ctx context.Context, k string) (dd.Capability, error) {
	o, e := p.operation(ctx, k)
	if e != nil {
		return dd.Capability{}, e
	}
	a, x, e := p.admin(ctx)
	if e != nil {
		return dd.Capability{}, e
	}
	access, _, _ := p.keys(k)
	info, e := a.InfoServiceAccount(x, access)
	if e != nil {
		if madmin.ToErrorResponse(e).Code == "XMinioInvalidIAMCredentials" {
			return dd.Capability{}, deliveryapp.ErrCapabilityNotFound
		}
		return dd.Capability{}, deliveryapp.ErrUnknownProviderOutcome
	}
	if info.AccountStatus != "on" || info.Expiration == nil || !info.Expiration.After(time.Now().UTC()) {
		return dd.Capability{}, deliveryapp.ErrCapabilityNotFound
	}
	if info.ParentUser != p.c.IssuerAccess || info.ImpliedPolicy || !sameNativePolicy(info.Policy, credentialPolicy(p.copy)) {
		return dd.Capability{}, dd.ErrCapabilityUnverifiable
	}
	return p.capability(o, *info.Expiration), nil
}
func (p *nativeCredential) Revoke(ctx context.Context, k string) error {
	a, x, e := p.admin(ctx)
	if e != nil {
		return e
	}
	access, _, _ := p.keys(k)
	if e = a.UpdateServiceAccount(x, access, madmin.UpdateServiceAccountReq{NewStatus: "off"}); e != nil {
		if madmin.ToErrorResponse(e).Code == "XMinioInvalidIAMCredentials" {
			return deliveryapp.ErrCapabilityNotFound
		}
		return deliveryapp.ErrUnknownProviderOutcome
	}
	if p.loseRevoke {
		return deliveryapp.ErrUnknownProviderOutcome
	}
	return nil
}

// Recovery of real retained failed provider attempts is business reconciliation,
// not compatibility with imagined legacy rows. Root is restricted to the admin
// gateway; no root data-plane route exists. Never creates a service account.
func credentialForensic(ctx context.Context, c config, pool *pgxpool.Pool, cs []ld.Copy) error {
	if !c.LC13Continuation || c.RunID.String() != "f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3" || c.RootAccess == "" {
		return lc.ErrStorageScope
	}
	madmin.MaxRetry = 1
	state, e := loadPlaneState(c)
	if e != nil {
		return e
	}
	state.ProfileID, e = credentialProfile(ctx, c, pool, state.ProfileID, cs[1], false)
	if e != nil {
		return e
	}
	all, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, policy, _, _, e := planeServices(ctx, c, pool, all)
	if e != nil {
		return e
	}
	tx := transaction.NewManager(pool)
	profiles := certificationinfra.NewProfileRepository(pool)
	cert := certificationapp.NewCertificationService(tx, profiles, certificationinfra.NewCertificationRepository(pool), certificationapp.NewReferenceEvidenceResolver())
	elig := certificationapp.NewEligibilityService(cert, datasetinfra.NewPostgresRepository(pool), rightsinfra.NewPostgresRepository(pool))
	var commands []deliveryapp.IssueCredentialCommand
	b, e := os.ReadFile("/run/private/credential-forensic-commands.json")
	if e != nil {
		return e
	}
	if json.Unmarshal(b, &commands) != nil || len(commands) != 4 {
		return lc.ErrStorageScope
	}
	facts := []map[string]any{}
	for _, cmd := range commands {
		if cmd.IdempotencyKey != "lc13-native-expiry" && cmd.IdempotencyKey != "lc13-native-expiry-02" && cmd.IdempotencyKey != "lc13-native-expiry-03" && cmd.IdempotencyKey != "lc13-native-expiry-04" {
			return lc.ErrStorageScope
		}
		var key string
		if e = pool.QueryRow(ctx, `SELECT provider_request_key FROM delivery_operation WHERE workspace_id=$1 AND idempotency_key=$2 AND provider_name='LC13_MINIO_REDEMPTION'`, c.WorkspaceID, cmd.IdempotencyKey).Scan(&key); e != nil {
			return e
		}
		p := &nativeCredential{c: c, pool: pool, copy: cs[1], occupancy: o, forensicRoot: true, originalInvalidRecipe: cmd.IdempotencyKey == "lc13-native-expiry"}
		service := deliveryapp.NewService(tx, deliveryinfra.NewPostgresRepository(pool), credentialGate{c, cs[1], policy, elig, state.ProfileID}, p)
		result, e := service.IssueCredential(ctx, cmd)
		if e != nil && !errors.Is(e, deliveryapp.ErrCredentialReplay) {
			return e
		}
		if result.Capability != nil || (result.Operation.Status != dd.StatusBlocked && result.Operation.Status != dd.StatusFailed) {
			return errors.New("failed native attempt remains unresolved")
		}
		data, call, e := credentialRead(ctx, p, key)
		if !errors.Is(e, lc.ErrStorageDenied) || len(data) != 0 {
			return errors.New("forensic native capability still reads")
		}
		facts = append(facts, map[string]any{"operation": result.Operation.ID, "status": result.Operation.Status, "native_get_denied": true, "call": call, "original_attempts_preserved": true, "native_secret_output": 0})
	}
	return write(c, "credential-forensic.json", map[string]any{"facts": facts, "native_delete": 0, "creates": 0})
}

func credentialSeal(ctx context.Context, c config) error {
	if !c.LC13Continuation || c.RootAccess == "" || c.IssuerAccess == "" {
		return lc.ErrStorageScope
	}
	a, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true}, c})
	return a.SetUserStatus(context.WithValue(ctx, adminKey{}, &adminCall{}), c.IssuerAccess, madmin.AccountDisabled)
}

type credentialGate struct {
	c           config
	copy        ld.Copy
	policy      *lc.CopyService
	eligibility *certificationapp.EligibilityService
	profile     uuid.UUID
}

func (g credentialGate) Evaluate(ctx context.Context, tx pgx.Tx, r dd.GateRequest) (dd.GateEvaluation, error) {
	ev := dd.GateEvaluation{PrincipalRef: r.PrincipalRef, EffectiveConsumerRef: r.EffectiveConsumerRef, CertificationProfileID: &g.profile}
	caller, e := (authority{}).AuthorizeLifecycle(ctx, tx, g.c.WorkspaceID, lc.CapabilityAdmitReader, time.Now().UTC())
	if e != nil || caller.Subject != r.PrincipalRef || r.WorkspaceID != g.c.WorkspaceID || r.DatasetVersionID != g.copy.DatasetVersionID || r.EffectiveConsumerRef != pilot.PlaneConsumer || r.Purpose != pilot.PlanePurpose || r.Action != "READ" || r.ScopeRef != g.copy.DatasetVersionID.String() || r.DeliveryChannel != "REDEMPTION" || r.DeliveryMode != "PLATFORM_REDEMPTION" {
		ev.Blockers = []string{"IDENTITY_CONTEXT"}
		return ev, nil
	}
	var restricted bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_content_use_restriction WHERE workspace_id=$1 AND dataset_version_id=$2)`, g.c.WorkspaceID, g.copy.DatasetVersionID).Scan(&restricted); e != nil {
		return ev, e
	}
	if restricted {
		ev.Blockers = []string{"CONTENT_USE_RESTRICTED"}
		return ev, nil
	}
	now := time.Now().UTC()
	d, e := g.policy.EvaluateFixtureUse(ctx, tx, lc.UseGateRequest{Copy: g.copy, ReaderID: r.OperationID, Caller: caller, AsOf: now})
	if e != nil {
		ev.Blockers = []string{"CONTENT_USE_WINDOW"}
		return ev, nil
	}
	result, e := g.eligibility.CheckTx(ctx, tx, certificationapp.DeliveryEligibilityQuery{WorkspaceID: r.WorkspaceID, DatasetVersionID: r.DatasetVersionID, ProfileID: g.profile, Consumer: pilot.PlaneConsumer, Purpose: r.Purpose, Action: r.Action, Delivery: r.DeliveryChannel, ScopeType: "ALL_RESOURCE", ScopeRef: r.ScopeRef, AsOf: now})
	if e != nil {
		return ev, e
	}
	if !result.Allowed {
		ev.Blockers = []string{"CURRENT_CERTIFICATION_ENTITLEMENT"}
		return ev, nil
	}
	cap := r.RequestedExpiresAt.UTC()
	for _, t := range []*time.Time{&g.c.ExpiresAt, d.UseUntil} {
		if t != nil && t.Before(cap) {
			cap = *t
		}
	}
	if !cap.After(now) {
		ev.Blockers = []string{"EXPIRED"}
		return ev, nil
	}
	ev.Allowed = true
	ev.FreshCapExpiresAt = &cap
	return ev, nil
}
func credentialProvision(ctx context.Context, c config) error {
	_, cs, e := copies(c)
	if e != nil {
		return e
	}
	if c.RootAccess == "" || c.IssuerAccess == "" {
		return lc.ErrStorageScope
	}
	a, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true}, c})
	call := func(f func(context.Context) error) error { return f(context.WithValue(ctx, adminKey{}, &adminCall{})) }
	policy := credentialPolicy(cs[1])
	var scoped map[string]any
	_ = json.Unmarshal(policy, &scoped)
	// MinIO's update API does not permit a user to disable its own service
	// accounts without this management permission. This identity exists only
	// inside this isolated synthetic inventory; the fixed gateway/adapter
	// exposes only own derived accounts and never returns it to consumers.
	scoped["Statement"] = append(scoped["Statement"].([]any), map[string]any{"Effect": "Allow", "Action": []string{"admin:ListServiceAccounts", "admin:UpdateServiceAccount"}, "Resource": []string{"*"}})
	policy, _ = json.Marshal(scoped)
	name := "lc13-issuer-" + c.RunID.String()
	if e = call(func(x context.Context) error { return a.AddCannedPolicy(x, name, policy) }); e != nil {
		return e
	}
	if e = call(func(x context.Context) error { return a.AddUser(x, c.IssuerAccess, c.IssuerSecret) }); e != nil {
		return e
	}
	if e = call(func(x context.Context) error {
		_, e := a.AttachPolicy(x, madmin.PolicyAssociationReq{Policies: []string{name}, User: c.IssuerAccess})
		return e
	}); e != nil {
		return e
	}
	return write(c, "issuer-policy.json", json.RawMessage(policy))
}
func credentialRead(ctx context.Context, p *nativeCredential, key string) ([]byte, uuid.UUID, error) {
	access, secret, _ := p.keys(key)
	reader, e := storage.NewLifecycleCheckpointReader(storage.LifecycleCopyConfig{Endpoint: p.c.Endpoint, AccessKey: access, SecretKey: secret, WorkspaceID: p.c.WorkspaceID, ProviderInstanceID: p.c.ProviderInstanceID, IncarnationID: p.c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(p.occupancy)}, []ld.Copy{p.copy})
	if e != nil {
		return nil, uuid.Nil, e
	}
	return reader.ReadExact(ctx, p.copy)
}
func credentialCommands(c config, cp ld.Copy) ([]deliveryapp.IssueCredentialCommand, error) {
	b, e := os.ReadFile("/run/private/credential-commands.json")
	if e != nil {
		return nil, e
	}
	var cmds []deliveryapp.IssueCredentialCommand
	e = json.Unmarshal(b, &cmds)
	return cmds, e
}
func credentialProfile(ctx context.Context, c config, pool *pgxpool.Pool, old uuid.UUID, cp ld.Copy, create bool) (uuid.UUID, error) {
	path := filepath.Join(c.OutputDirectory, "credential-profile.json")
	if b, e := os.ReadFile(path); e == nil {
		var fact struct{ Profile uuid.UUID }
		if json.Unmarshal(b, &fact) != nil {
			return uuid.Nil, lc.ErrStorageContract
		}
		return fact.Profile, nil
	}
	if !create {
		return uuid.Nil, lc.ErrStorageScope
	}
	tx := transaction.NewManager(pool)
	repo := certificationinfra.NewProfileRepository(pool)
	previous, e := repo.GetProfile(ctx, old)
	if e != nil {
		return uuid.Nil, e
	}
	spec := previous.CertificationProfile
	spec.ProfileRef = "fixture/lc13-redemption"
	spec.Code = "LC13-REDEMPTION"
	spec.Delivery = certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"DIRECT_DATA", "REDEMPTION"}}
	profile, e := certificationapp.NewProfileService(tx, repo).Create(ctx, certificationapp.CreateProfileCommand{WorkspaceID: c.WorkspaceID, ActorID: &c.ActorID, Profile: spec})
	if e != nil {
		return uuid.Nil, e
	}
	var quality uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM quality_result WHERE dataset_version_id=$1 ORDER BY created_at LIMIT 1`, cp.DatasetVersionID).Scan(&quality); e != nil {
		return uuid.Nil, e
	}
	cert, e := certificationapp.NewCertificationService(tx, repo, certificationinfra.NewCertificationRepository(pool), certificationapp.NewReferenceEvidenceResolver()).Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: cp.DatasetVersionID, ProfileID: profile.ID, QualityAssessmentID: quality, IdempotencyKey: "lc13-redemption-certification", ActorID: &c.ActorID})
	if e != nil {
		return uuid.Nil, e
	}
	if string(cert.Decision) != "CERTIFIED" {
		return uuid.Nil, lc.ErrStorageDenied
	}
	return profile.ID, write(c, "credential-profile.json", map[string]any{"Profile": profile.ID, "Certification": cert.ID, "previous_profile_preserved": old})
}

type credentialDirectGate struct {
	inner  deliveryapp.DirectDataGate
	expiry time.Time
}

func (g credentialDirectGate) EvaluateDirectData(ctx context.Context, tx pgx.Tx, r deliveryapp.DirectDataGateRequest) (deliveryapp.DirectDataGateResult, error) {
	if r.RequestedExpiresAt.After(g.expiry) {
		r.RequestedExpiresAt = g.expiry
	}
	res, e := g.inner.EvaluateDirectData(ctx, tx, r)
	if e == nil && !g.expiry.After(time.Now().UTC()) {
		res.Evaluation.Allowed = false
		res.Evaluation.FreshCapExpiresAt = nil
		res.Evaluation.Blockers = []string{"NATIVE_CREDENTIAL_EXPIRED"}
	}
	return res, e
}
func credentialRedemption(ctx context.Context, p *nativeCredential, svc *deliveryapp.Service, cmd deliveryapp.IssueCredentialCommand, profile uuid.UUID, key, token string) (int, string, error) {
	var operation uuid.UUID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, e := (authority{}).Resolve(r, p.c.WorkspaceID, lc.CapabilityAdmitReader)
		if e != nil {
			http.Error(w, "denied", 403)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/fixture/redeem" || r.ContentLength != 0 {
			http.Error(w, "denied", 403)
			return
		}
		result, e := svc.IssueCredential(r.Context(), cmd)
		if e != nil || result.Capability == nil {
			http.Error(w, "denied", 403)
			return
		}
		// The handle is never sufficient authority: trusted caller/context and fresh
		// certification + lifecycle + native expiry are checked at actual redemption.
		if caller.Subject != result.Operation.PrincipalRef || result.Capability.ConsumerRef != pilot.PlaneConsumer {
			http.Error(w, "denied", 403)
			return
		}
		access, secret, _ := p.keys(result.Operation.ProviderRequestKey)
		c := p.c
		c.Access = access
		c.Secret = secret
		all := []ld.Copy{p.copy}
		_, _, reader, pins, e := planeServices(r.Context(), c, p.pool, all)
		if e != nil {
			http.Error(w, "denied", 403)
			return
		}
		current := planeCurrentDeliveryGate(p.pool)
		direct := deliveryapp.NewDirectDataService(transaction.NewManager(p.pool), current.repo, credentialDirectGate{current.gate, result.Capability.ProviderCredentialExpiresAt}, datasetinfra.NewPostgresRepository(p.pool)).WithTransferPins(pins)
		owned := lc.WithFixtureReaderCaller(r.Context(), caller)
		d, e := direct.Deliver(owned, deliveryapp.DirectDataCommand{WorkspaceID: p.c.WorkspaceID, DatasetVersionID: p.copy.DatasetVersionID, ProfileID: profile, PrincipalRef: caller.Subject, EffectiveConsumerRef: pilot.PlaneConsumer, Purpose: pilot.PlanePurpose, Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: key})
		if e != nil || !d.PayloadReady {
			http.Error(w, "denied", 403)
			return
		}
		operation = d.Operation.ID
		out := &planeHTTPWriter{ResponseWriter: w, ctx: owned}
		_, e = reader.ReadPinnedTo(owned, p.c.WorkspaceID, d.TransferPinID, out)
		if e != nil && out.written == 0 {
			http.Error(w, "denied", 403)
		}
	})
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, "", e
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	r, _ := http.NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String()+"/fixture/redeem", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	response, e := (&http.Client{Timeout: 15 * time.Second}).Do(r)
	if e != nil {
		return 0, "", e
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if e != nil {
		return 0, "", e
	}
	if response.StatusCode == 200 && !bytes.Equal(data, pilot.PlaneSyntheticPayloads()[1]) {
		return 0, "", lc.ErrStorageContract
	}
	return response.StatusCode, operation.String(), nil
}
func planeCredential(ctx context.Context, c config, pool *pgxpool.Pool, cs []ld.Copy, mode string) error {
	if !c.LC13Continuation || c.RootAccess != "" || c.IssuerAccess == "" {
		return lc.ErrStorageScope
	}
	madmin.MaxRetry = 1
	state, e := loadPlaneState(c)
	if e != nil {
		return e
	}
	state.ProfileID, e = credentialProfile(ctx, c, pool, state.ProfileID, cs[1], mode == "credential-issue")
	if e != nil {
		return e
	}
	all, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, policy, _, _, e := planeServices(ctx, c, pool, all)
	if e != nil {
		return e
	}
	tx := transaction.NewManager(pool)
	profiles := certificationinfra.NewProfileRepository(pool)
	cert := certificationapp.NewCertificationService(tx, profiles, certificationinfra.NewCertificationRepository(pool), certificationapp.NewReferenceEvidenceResolver())
	elig := certificationapp.NewEligibilityService(cert, datasetinfra.NewPostgresRepository(pool), rightsinfra.NewPostgresRepository(pool))
	p := &nativeCredential{c: c, pool: pool, copy: cs[1], occupancy: o}
	service := deliveryapp.NewService(tx, deliveryinfra.NewPostgresRepository(pool), credentialGate{c, cs[1], policy, elig, state.ProfileID}, p)
	cmds, e := credentialCommands(c, cs[1])
	if e != nil || len(cmds) != 2 {
		return errors.New("two fixed credential commands required")
	}
	facts := []map[string]any{}
	if mode == "credential-issue" {
		// Use the original immutable policy. Its actual use_until participates in
		// every fresh gate; acceptance never edits an existing policy/deadline.
		for _, cmd := range cmds {
			result, e := service.IssueCredential(ctx, cmd)
			if e != nil {
				return e
			}
			if result.Capability == nil || result.Operation.Status != dd.StatusIssued {
				return errors.New("native credential not issued")
			}
			replay, e := service.IssueCredential(ctx, cmd)
			if e != nil || replay.Capability == nil || replay.Capability.CapabilityRef != result.Capability.CapabilityRef || replay.Capability.Credential != result.Capability.Credential {
				return errors.New("same native capability replay failed")
			}
			data, call, e := credentialRead(ctx, p, result.Operation.ProviderRequestKey)
			if e != nil || !bytes.Equal(data, pilot.PlaneSyntheticPayloads()[1]) {
				return errors.New("native credential exact read failed")
			}
			status, delivery, e := credentialRedemption(ctx, p, service, cmd, state.ProfileID, "redeem:"+cmd.IdempotencyKey, c.Token)
			if e != nil || status != 200 {
				return errors.New("fresh authorized redemption failed")
			}
			denied, _, e := credentialRedemption(ctx, p, service, cmd, state.ProfileID, "untrusted:"+cmd.IdempotencyKey, "untrusted")
			if e != nil || denied != 403 {
				return errors.New("untrusted redemption accepted")
			}
			facts = append(facts, map[string]any{"operation": result.Operation.ID, "capability_ref": result.Capability.CapabilityRef, "capability_hash": result.Capability.CapabilityHash, "native_expiry": result.Capability.ProviderCredentialExpiresAt, "request_expiry": cmd.RequestedExpiresAt, "native_get_call": call, "same_capability_replay": true, "native_secret_returned": false, "redemption_http": status, "redemption_operation": delivery, "untrusted_http": denied})
		}
	} else if mode == "credential-expiry" {
		wait := time.Until(cmds[0].RequestedExpiresAt.Add(2 * time.Second))
		if wait > 10*time.Second {
			return errors.New("host must observe the natural native deadline before launching this short helper")
		}
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		var key string
		var id uuid.UUID
		if e = pool.QueryRow(ctx, `SELECT id,provider_request_key FROM delivery_operation WHERE workspace_id=$1 AND idempotency_key=$2`, c.WorkspaceID, cmds[0].IdempotencyKey).Scan(&id, &key); e != nil {
			return e
		}
		data, call, e := credentialRead(ctx, p, key)
		if !errors.Is(e, lc.ErrStorageDenied) || len(data) != 0 {
			return errors.New("expired native credential still reads")
		}
		var active int
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_reference_requirement r WHERE delivery_operation_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_release x WHERE x.requirement_id=r.id)`, id).Scan(&active); e != nil || active != 1 {
			return errors.New("expiry incorrectly released material demand")
		}
		facts = append(facts, map[string]any{"operation": id, "native_expired_get_denied": true, "call": call, "material_demand_retained": active})
	} else if mode == "credential-contain" || mode == "credential-confirm" {
		if mode == "credential-contain" {
			if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "credential-current-restriction"}, DatasetVersionID: cs[1].DatasetVersionID, BasisRef: "Closed native containment acceptance"}); e != nil {
				return e
			}
			p.loseRevoke = true
		}
		for _, cmd := range cmds {
			result, e := service.IssueCredential(ctx, cmd)
			if e != nil && !errors.Is(e, deliveryapp.ErrCredentialReplay) {
				return e
			}
			if result.Capability != nil || result.Operation.Status != dd.StatusIssued {
				return errors.New("blocked replay leaked capability or rewrote issued")
			}
			data, call, e := credentialRead(ctx, p, result.Operation.ProviderRequestKey)
			if !errors.Is(e, lc.ErrStorageDenied) || len(data) != 0 {
				return errors.New("contained native credential still reads")
			}
			facts = append(facts, map[string]any{"operation": result.Operation.ID, "original_status": result.Operation.Status, "credential_output": 0, "native_get_denied": true, "call": call, "revoke_response_unknown": p.loseRevoke})
		}
	} else {
		return fmt.Errorf("unknown credential phase %s", mode)
	}
	return write(c, mode+".json", map[string]any{"facts": facts, "native_delete": 0, "channel": "PLATFORM_REDEMPTION_ONLY", "raw_s3_delivery": "UNSUPPORTED"})
}
