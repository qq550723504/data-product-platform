package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var referenceComposition bool

type referenceConfig struct {
	Fixture              config
	PreparationEnabled   bool
	PreparationUntil     time.Time
	AttestationPublicKey string
}
type referencePermission struct {
	permission
	ConfirmationSHA256, BinarySHA256, ProfileSHA256 string
	MaxPerTarget, MaxTotal                          int
}

func configureReference() {
	referenceComposition = true
	run = pilot.ReferenceExecution
	workspace = pilot.ReferenceWorkspace
	providerID = pilot.ReferenceProvider.String()
	incarnationID = pilot.ReferenceIncarnation.String()
	planID = pilot.ReferencePlan
	planHash = pilot.ReferencePlanHash
	inventoryPlanID = pilot.ReferenceInventoryPlan
	inventoryHash = pilot.ReferenceInventoryHash
}
func readReferenceConfig() (referenceConfig, error) {
	var c referenceConfig
	if read("/run/private/reference.json", &c) != nil {
		return c, lc.ErrStorageScope
	}
	f := c.Fixture
	if f.RunID.String() != pilot.ReferenceEnvironment || f.WorkspaceID != pilot.ReferenceWorkspace || f.ProviderInstanceID != pilot.ReferenceProvider || f.IncarnationID != pilot.ReferenceIncarnation || f.ActorID == uuid.Nil || f.Token == "" || f.InspectAccess == "" || f.InspectSecret == "" || f.RootAccess != "" || f.RootSecret != "" || f.Endpoint != "127.0.0.1:9000" || f.OutputDirectory != "/run/output" || f.PostgresDSN == "" {
		return c, lc.ErrStorageScope
	}
	return c, nil
}

type referencePreparationAuthority struct{ original referenceConfig }

func (a referencePreparationAuthority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, at time.Time) (principal.RequestPrincipal, error) {
	c, e := readReferenceConfig()
	if e != nil || c != a.original || !c.PreparationEnabled || !at.Before(c.PreparationUntil) {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	allowed := []string{lc.CapabilityObserveAttempt, lc.CapabilityInspectStorage, lc.CapabilityCheckpointStorage}
	r, e := principal.NewStaticResolver(true, c.Fixture.Token, "lifecycle-reference-preparation:"+pilot.ReferenceExecution, c.Fixture.ActorID.String(), []string{pilot.ReferenceWorkspace.String()}, allowed)
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	request, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/internal", nil)
	request.Header.Set("Authorization", "Bearer "+c.Fixture.Token)
	return r.Resolve(request, w, cap)
}
func referenceTargetsDigest() string {
	b, _ := json.Marshal(pilot.ReferenceTargets())
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func validateReferencePermission(p referencePermission, c referenceConfig, at time.Time) error {
	if p.DeleteAccess != "refdelete614cd9ad" {
		return lc.ErrStorageDenied
	}
	if ok, _ := regexp.MatchString(`^[a-zA-Z0-9]{1,64}$`, c.Fixture.InspectAccess); !ok {
		return lc.ErrStorageDenied
	}
	if !p.Enabled || p.RunID.String() != pilot.ReferenceExecution || p.WorkspaceID != pilot.ReferenceWorkspace || p.PlanHash != pilot.ReferencePlanHash || p.MaxPerTarget != 1 || p.MaxTotal != 2 || p.DeleteAccess == "" || p.DeleteSecret == "" || p.ConfirmedAt.IsZero() || at.Before(p.ConfirmedAt) || !at.Before(p.ExpiresAt) || p.ExpiresAt.Sub(p.ConfirmedAt) > 40*time.Minute || p.ExpiresAt.Sub(p.ConfirmedAt) <= 0 || len(p.TopologySHA256) != 64 || len(p.ConfirmationSHA256) != 64 || len(p.BinarySHA256) != 64 || p.ProfileSHA256 != referenceTargetsDigest() || p.BasisRef != "human-confirmed:reference-disposal:"+p.ConfirmationSHA256 || c.Fixture.RootAccess != "" || c.Fixture.RootSecret != "" {
		return lc.ErrStorageDenied
	}
	return nil
}
func loadReferenceSend() (config, permission, error) {
	c, e := readReferenceConfig()
	var p referencePermission
	if e != nil || read("/run/private/reference-permission.json", &p) != nil || validateReferencePermission(p, c, time.Now().UTC()) != nil {
		return c.Fixture, p.permission, lc.ErrStorageDenied
	}
	text, e := os.ReadFile("/run/private/human-confirmation.txt")
	hash := sha256.Sum256(text)
	binary, e2 := os.ReadFile(os.Args[0])
	binaryHash := sha256.Sum256(binary)
	if e != nil || e2 != nil || hex.EncodeToString(hash[:]) != p.ConfirmationSHA256 || hex.EncodeToString(binaryHash[:]) != p.BinarySHA256 {
		return c.Fixture, p.permission, lc.ErrStorageDenied
	}
	for _, required := range []string{"CONFIRM_DELETE_EXACT_TWO", pilot.ReferenceExecution, pilot.ReferenceProvider.String(), pilot.ReferenceIncarnation.String(), pilot.ReferencePlanHash, "cleanup=0", "hold-release=0", "bypass=0"} {
		if !strings.Contains(string(text), required) {
			return c.Fixture, p.permission, lc.ErrStorageDenied
		}
	}
	for _, copy := range pilot.ReferenceTargets() {
		if !strings.Contains(string(text), copy.Namespace+"/"+copy.Key+"@"+copy.ProviderVersion) {
			return c.Fixture, p.permission, lc.ErrStorageDenied
		}
	}
	topology, e := os.ReadFile("/run/private/reference-topology.json")
	var witness pilot.SignedClosedReferenceExecutor
	if e != nil || read("/run/private/current-topology.json", &witness) != nil || validateReferenceTopology(c, p, topology, witness, time.Now().UTC()) != nil {
		return c.Fixture, p.permission, lc.ErrStorageDenied
	}
	return c.Fixture, p.permission, nil
}
func referenceServices(ctx context.Context) (*pgxpool.Pool, *lc.OccupancyService, referenceConfig, error) {
	c, e := readReferenceConfig()
	if e != nil {
		return nil, nil, c, e
	}
	pool, e := pgxpool.New(ctx, c.Fixture.PostgresDSN)
	if e != nil {
		return nil, nil, c, lc.ErrStorageScope
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		pool.Close()
		return nil, nil, c, e
	}
	outbox.ConfigureAppendObligation(router)
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, referencePreparationAuthority{c}, nil, nil)
	return pool, o, c, nil
}
func referenceNoSend(ctx context.Context) error {
	pool, o, c, e := referenceServices(ctx)
	if e != nil {
		return e
	}
	defer pool.Close()
	defer outbox.ConfigureAppendObligation(nil)
	key, e := hex.DecodeString(c.AttestationPublicKey)
	if e != nil {
		return lc.ErrStorageContract
	}
	var proof pilot.SignedClosedReferenceExecutor
	if read("/run/private/closed-executor.json", &proof) != nil {
		return lc.ErrStorageContract
	}
	verifier := pilot.ReferenceNoSendVerifier{Key: ed25519.PublicKey(key), Proof: proof}
	var body pilot.ClosedReferenceExecutor
	if json.Unmarshal(proof.Body, &body) != nil {
		return lc.ErrStorageContract
	}
	service := lc.NewDispositionService(o, nil, nil, verifier)
	inputs := []lc.ProviderObservationInput{}
	for _, id := range []uuid.UUID{uuid.MustParse("0a86461d-d5c2-4c00-99ae-0d44ac6dde2a"), uuid.MustParse("d8a52b7b-9cad-4ab4-a8d5-286d37ceafb9")} {
		inputs = append(inputs, lc.ProviderObservationInput{AttemptID: id, ObservationRef: "closed-executor:" + body.Nonce.String()})
	}
	result, e := service.RecordProviderObservations(ctx, lc.RecordProviderObservationsCommand{CommandHeader: lc.CommandHeader{WorkspaceID: workspace, IdempotencyKey: run + ":closed-trials-atomic"}, Observations: inputs})
	if e != nil {
		return e
	}
	return write("/run/output/no-send-transaction.json", map[string]any{"status": "PASS_NOT_SENT_ATOMIC_APPEND_ONLY", "result": result, "attempts": inputs, "provider_delete": 0, "provider_io": 0, "invocation_cost_created": 0, "proof": proof})
}
func referenceInspect(ctx context.Context) error {
	pool, o, c, e := referenceServices(ctx)
	if e != nil {
		return e
	}
	defer pool.Close()
	defer outbox.ConfigureAppendObligation(nil)
	journal := lc.NewPostgresStorageCallJournal(o)
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Fixture.Endpoint, AccessKey: c.Fixture.InspectAccess, SecretKey: c.Fixture.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: pilot.ReferenceProvider, IncarnationID: pilot.ReferenceIncarnation, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: journal})
	if e != nil {
		return e
	}
	reader, e := storage.NewLifecycleCheckpointReader(storage.LifecycleCopyConfig{Endpoint: c.Fixture.Endpoint, AccessKey: c.Fixture.InspectAccess, SecretKey: c.Fixture.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: pilot.ReferenceProvider, IncarnationID: pilot.ReferenceIncarnation, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: journal}, pilot.ReferenceCopies())
	if e != nil {
		return e
	}
	results := []any{}
	inventories := map[string]lc.NamespaceInventory{}
	for _, copy := range pilot.ReferenceCopies() {
		protection, e := inspector.InspectRetentionAndHold(ctx, copy)
		if e != nil {
			return e
		}
		data, call, e := reader.ReadExact(ctx, copy)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != copy.ContentSHA256 {
			return lc.ErrStorageContract
		}
		if e = write(filepath.Join("/run/payload", copy.ID.String()+".json"), map[string]any{"bytes": data}); e != nil {
			return e
		}
		if _, ok := inventories[copy.Namespace]; !ok {
			inventory, e := inspector.InspectNamespace(ctx, copy)
			if e != nil {
				return e
			}
			inventories[copy.Namespace] = inventory
		}
		results = append(results, map[string]any{"copy": copy, "protection": protection, "read_call_id": call, "bytes": len(data), "sha256": hex.EncodeToString(sum[:])})
	}
	for ns, inventory := range inventories {
		expected := map[string]domain.Copy{}
		for _, c := range pilot.ReferenceCopies() {
			if c.Namespace == ns {
				expected[c.Key+"\x00"+c.ProviderVersion] = c
			}
		}
		if len(inventory.Versions) != len(expected) {
			return lc.ErrStorageContract
		}
		for _, v := range inventory.Versions {
			c, ok := expected[v.Key+"\x00"+v.Version]
			if !ok || v.DeleteMarker || v.ByteSize != c.ByteSize {
				return lc.ErrStorageContract
			}
			delete(expected, v.Key+"\x00"+v.Version)
		}
		if len(expected) != 0 {
			return lc.ErrStorageContract
		}
	}
	return write("/run/output/native-six-version.json", map[string]any{"status": "PASS_EXACT_SIX_NATIVE_READS_AND_PROTECTIONS", "versions": results, "inventories": inventories, "provider_delete": 0, "bytes_publicly_disclosed": 0, "business_admission": 0})
}
func referenceHostAttest(ctx context.Context) error {
	// Host-only trusted composition. Signer key never enters the payload container.
	private := `C:\Users\Henry\.codex\fixture-private\dpp-lifecycle307-5d2274ee`
	script := filepath.Join("examples", "lifecycle-d1", "prepare-reference-delete.py")
	raw, e := os.ReadFile(script)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "24e1d6d19394208016ee1d9dd0ddaa06669880d2cebae03b4c7996db0303fd1c" {
		return lc.ErrStorageContract
	}
	nonce := uuid.New()
	output := filepath.Join(private, "reference-attestation-"+nonce.String())
	command := exec.CommandContext(ctx, "python", script, "--private-directory", private, "--output-directory", output)
	if _, e = command.Output(); e != nil {
		return errors.New("canonical host verifier failed; private inputs retained")
	}
	var candidate struct {
		Status, HelperID string
		Script           string `json:"preflight_script_checkout_sha256"`
	}
	data, e := os.ReadFile(filepath.Join(output, "trial-no-send-candidate.json"))
	if e != nil {
		return e
	}
	// Script owns the frozen helper checks; verify the named output, never flags.
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil || json.Unmarshal(m["status"], &candidate.Status) != nil || candidate.Status != "VERIFIED_CANDIDATE_NOT_APPLIED" || json.Unmarshal(m["preflight_script_checkout_sha256"], &candidate.Script) != nil || candidate.Script != hex.EncodeToString(sum[:]) {
		return lc.ErrStorageContract
	}
	digest := sha256.Sum256(data)
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	body, _ := json.Marshal(pilot.ClosedReferenceExecutor{Nonce: nonce, CheckedAt: time.Now().UTC(), Environment: pilot.ReferenceEnvironment, PlanHash: pilot.ReferencePlanHash, HelperID: "fb92bdcd150127c50163448de9d918f3a2d9a18e2384274b30febfaf647bdb87", BinarySHA256: "5e04e88bbd65880a5e65da5795dcdc116021512e04d27a200ec0d03f83e972c8", ScriptSHA256: hex.EncodeToString(sum[:]), CandidateSHA256: hex.EncodeToString(digest[:])})
	if e = write(filepath.Join(output, "closed-executor.json"), pilot.SignedClosedReferenceExecutor{Body: body, Signature: ed25519.Sign(key, body)}); e != nil {
		return e
	}
	if e = write(filepath.Join(output, "attestation-public.json"), map[string]any{"public_key": hex.EncodeToString(pub), "nonce": nonce, "private_signer_key_disclosed": 0}); e != nil {
		return e
	}
	// One-use signer is not persisted or mounted; only public verifier material remains.
	println("HOST_ATTESTATION_PRIVATE_DIRECTORY=" + output)
	return nil
}
func runReference(ctx context.Context, mode string) error {
	if os.Getenv("LIFECYCLE_REFERENCE_PROFILE") != "5d2274ee-614cd9ad" {
		return lc.ErrStorageScope
	}
	configureReference()
	switch mode {
	case "attest-host":
		return referenceHostAttest(ctx)
	case "topology-host-a":
		return referenceHostTopology(ctx, "a")
	case "topology-host-b":
		return referenceHostTopology(ctx, "b")
	case "no-send":
		return referenceNoSend(ctx)
	case "inspect-source", "inspect-restored":
		return referenceInspect(ctx)
	case "render":
		return renderReference()
	case "migrate-checkpoint":
		pool, _, c, e := referenceServices(ctx)
		if e != nil {
			return e
		}
		defer pool.Close()
		defer outbox.ConfigureAppendObligation(nil)
		if !c.PreparationEnabled || !time.Now().Before(c.PreparationUntil) {
			return lc.ErrStorageDenied
		}
		var version int
		if e = pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&version); e != nil || version != 67 {
			return lc.ErrStorageScope
		}
		return migration.NewRunner(pool, "/run/migrations").Up(ctx)
	case "dry-run-send":
		var p referencePermission
		if read("/run/private/reference-permission.json", &p) != nil || p.Enabled {
			return errors.New("dry run requires explicit disabled permission")
		}
		if _, _, e := loadReferenceSend(); !errors.Is(e, lc.ErrStorageDenied) {
			return lc.ErrStorageContract
		}
		return write("/run/output/send-disabled.json", map[string]any{"status": "PASS_DISABLED_BEFORE_DB_OR_PROVIDER", "provider_io": 0, "provider_delete": 0})
	case "send-a", "send-b":
		return referenceSend(ctx, mode)
	default:
		return lc.ErrStorageUnsupported
	}
}
