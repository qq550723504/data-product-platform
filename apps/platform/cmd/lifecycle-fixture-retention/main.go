// Exact-two retained-fixture maintenance, outside Core. No DELETE/IAM/hold API.
// A production --run requires a newly compiled host-key anchor and a fresh,
// human-referenced signed grant. The default build cannot perform a write.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const deadline = "2026-10-24T12:40:00Z"
const kind = "RETENTION_EXTENSION_EXACT_TWO_V1"
const approvedPlanSHA256 = "ae85a69fee49b556b88c9437be4c4923a370213b9c9c0e92466328932f826d2f"

// Set with -X only when the separately reviewed maintenance bindings are frozen.
var trustedHostPublicSHA256 string
var denied = errors.New("bounded retention maintenance rejected; preserve all evidence; no automatic retry")

type target struct {
	Group            string
	Copy             domain.Copy
	Native, OldUntil string
}

func fixed(group string) (target, error) {
	key, old, native, version := "", "", "", ""
	switch group {
	case "business-races":
		key = "fixture/control-retention.csv"
		old = "2026-10-23T06:25:44Z"
		native = "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce"
		version = "2606d71f-207b-42a7-b671-4b7f4ca4d194"
	case "mixed":
		key = "fixture/m1-c.csv"
		old = "2026-10-23T06:33:33Z"
		native = "66c17878149ff1959339bc60e319f3b0494786e62da33b77017bfc8aab13613d"
		version = "81a8402e-390f-4247-9d34-552df5eadeb3"
	default:
		return target{}, denied
	}
	scope, e := pilot.RemainingScopeFor(group)
	if e != nil {
		return target{}, denied
	}
	for _, cp := range scope.Copies {
		if cp.Key == key && cp.ProviderVersion == version {
			return target{group, cp, native, old}, nil
		}
	}
	return target{}, denied
}

type owner struct{ Group, CID, Native, DescriptorSHA256, ConfigSHA256 string }
type bindings struct {
	Kind                                                 string
	MaintenanceID                                        uuid.UUID
	NotBefore, NotAfter                                  time.Time
	PlanSHA256, BinarySHA256, HostPublicSHA256, BudgetID string
	Owners                                               []owner
}
type grant struct {
	Kind                               string
	MaintenanceID                      uuid.UUID
	BindingsSHA256, HumanMessageSHA256 string
	NotBefore, NotAfter, ConfirmedAt   time.Time
	MaxPerTarget, MaxTotal             int
	OldAuthorizationReused             bool
}
type proof struct {
	Kind                                                                           string
	MaintenanceID                                                                  uuid.UUID
	BindingsSHA256, Group, Owner, Native, DescriptorSHA256, ConfigSHA256, BudgetID string
	At                                                                             time.Time
}
type signed struct{ Body, Signature []byte }
type permission struct {
	Enabled        bool
	BindingsSHA256 string
}
type config struct{ Group, Endpoint, RootAccess, RootSecret string }
type ownership struct {
	Kind                     string
	MaintenanceID            uuid.UUID
	BindingsSHA256, BudgetID string
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func hash(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && strings.ToLower(v) == v
}
func strict(raw []byte, v any) error {
	// Token traversal rejects duplicate keys before the typed decoder can hide one.
	d := json.NewDecoder(bytes.NewReader(raw))
	if e := jsonValue(d); e != nil {
		return denied
	}
	if _, e := d.Token(); e != io.EOF {
		return denied
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return denied
	}
	return nil
}
func jsonValue(d *json.Decoder) error {
	t, e := d.Token()
	if e != nil {
		return e
	}
	v, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch v {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			s, ok := k.(string)
			if e != nil || !ok || seen[s] {
				return denied
			}
			seen[s] = true
			if e = jsonValue(d); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = jsonValue(d); e != nil {
				return e
			}
		}
	default:
		return denied
	}
	_, e = d.Token()
	return e
}
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 1<<20 {
		return denied
	}
	return strict(b, v)
}
func checkedBindings(b bindings) error {
	end, _ := time.Parse(time.RFC3339, deadline)
	if b.Kind != kind || b.MaintenanceID == uuid.Nil || b.MaintenanceID.String() == pilot.RemainingExecution || b.NotBefore.IsZero() || !b.NotBefore.Before(b.NotAfter) || b.NotAfter.Sub(b.NotBefore) > 40*time.Minute || b.NotAfter.After(end) || b.PlanSHA256 != approvedPlanSHA256 || !hash(b.BinarySHA256) || !hash(b.HostPublicSHA256) || !hash(b.BudgetID) || len(b.Owners) != 2 {
		return denied
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, o := range b.Owners {
		t, e := fixed(o.Group)
		if e != nil || seen[o.Group] || ids[o.CID] || !hash(o.CID) || o.CID == t.Native || o.Native != t.Native || !hash(o.DescriptorSHA256) || !hash(o.ConfigSHA256) {
			return denied
		}
		seen[o.Group] = true
		ids[o.CID] = true
	}
	return nil
}

type engine struct {
	b                       bindings
	bindingSHA              string
	group                   string
	selected                owner
	private, budget, output string
	public                  ed25519.PublicKey
	now                     func() time.Time
	client                  *minio.Client
	endpoint                string
	base                    http.RoundTripper
	syncDir                 func(string) error
}

func (r *engine) save(dir, name string, v any) error {
	root, e := os.OpenRoot(dir)
	if e != nil {
		return denied
	}
	defer root.Close()
	f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return denied
	}
	if e = json.NewEncoder(f).Encode(v); e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return denied
	}
	if r.syncDir(dir) != nil {
		return denied
	}
	return nil
}
func syncDirectory(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func (r *engine) authorize() error {
	at := r.now().UTC()
	end, _ := time.Parse(time.RFC3339, deadline)
	if checkedBindings(r.b) != nil || len(r.public) != ed25519.PublicKeySize || r.b.HostPublicSHA256 != trustedHostPublicSHA256 || !hash(trustedHostPublicSHA256) || digest(r.public) != trustedHostPublicSHA256 || at.Before(r.b.NotBefore) || !at.Before(r.b.NotAfter) || !at.Before(end) {
		return denied
	}
	var s signed
	var g grant
	var p permission
	var h proof
	if read(filepath.Join(r.private, "authorization.json"), &s) != nil || !ed25519.Verify(r.public, s.Body, s.Signature) || strict(s.Body, &g) != nil {
		return denied
	}
	human, e := os.ReadFile(filepath.Join(r.private, "user-message-verbatim.txt"))
	if e != nil || len(human) == 0 || g.Kind != kind || g.MaintenanceID != r.b.MaintenanceID || g.BindingsSHA256 != r.bindingSHA || !hash(g.HumanMessageSHA256) || digest(human) != g.HumanMessageSHA256 || !g.NotBefore.Equal(r.b.NotBefore) || !g.NotAfter.Equal(r.b.NotAfter) || g.ConfirmedAt.IsZero() || at.Before(g.ConfirmedAt) || g.MaxPerTarget != 1 || g.MaxTotal != 2 || g.OldAuthorizationReused {
		return denied
	}
	if read(filepath.Join(r.private, "permission.json"), &p) != nil || !p.Enabled || p.BindingsSHA256 != r.bindingSHA {
		return denied
	}
	if read(filepath.Join(r.private, "topology.json"), &s) != nil || !ed25519.Verify(r.public, s.Body, s.Signature) || strict(s.Body, &h) != nil || h.Kind != kind || h.MaintenanceID != r.b.MaintenanceID || h.BindingsSHA256 != r.bindingSHA || h.Group != r.group || h.Owner != r.selected.CID || h.Native != r.selected.Native || h.DescriptorSHA256 != r.selected.DescriptorSHA256 || h.ConfigSHA256 != r.selected.ConfigSHA256 || h.BudgetID != r.b.BudgetID || at.Before(h.At) || at.Sub(h.At) > time.Minute {
		return denied
	}
	// The fixed shared budget, not a host progress projection, controls continuation.
	var own ownership
	if read(filepath.Join(r.budget, "ownership.json"), &own) != nil || own != (ownership{kind, r.b.MaintenanceID, r.bindingSHA, r.b.BudgetID}) {
		return denied
	}
	entries, e := os.ReadDir(r.budget)
	if e != nil {
		return denied
	}
	allowed := map[string]bool{"ownership.json": true}
	for _, group := range []string{"business-races", "mixed"} {
		for _, suffix := range []string{"-invocation.json", "-slot.json", "-success.json"} {
			allowed[group+suffix] = true
		}
	}
	for _, x := range entries {
		if x.IsDir() || !allowed[x.Name()] {
			return denied
		}
	}
	if r.group == "mixed" {
		var previous struct {
			MaintenanceID          uuid.UUID
			BindingsSHA256, Status string
		}
		if read(filepath.Join(r.budget, "business-races-success.json"), &previous) != nil || previous.MaintenanceID != r.b.MaintenanceID || previous.BindingsSHA256 != r.bindingSHA || previous.Status != "PASS_EXACT_RETENTION_READBACK" {
			return denied
		}
	}
	return nil
}

type callKey struct{}
type call struct{ Operation string }

func (r *engine) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.authorize() != nil {
		return nil, denied
	}
	cp, _ := fixed(r.group)
	q := req.URL.Query()
	op, ok := req.Context().Value(callKey{}).(call)
	if !ok || req.URL.Scheme != "http" || req.URL.Host != r.endpoint || req.URL.User != nil || req.URL.Fragment != "" || req.URL.EscapedPath() != "/"+cp.Copy.Namespace+"/"+cp.Copy.Key || q.Get("versionId") != cp.Copy.ProviderVersion || q.Encode() != req.URL.RawQuery || len(req.Header.Values("X-Amz-Bypass-Governance-Retention")) != 0 || len(req.Header.Values("X-Amz-Copy-Source")) != 0 {
		return nil, denied
	}
	want := ""
	method := "GET"
	switch op.Operation {
	case "content":
		want = ""
	case "retention":
		want = "retention"
	case "hold":
		want = "legal-hold"
	case "extend":
		want = "retention"
		method = "PUT"
	default:
		return nil, denied
	}
	if req.Method != method || len(q) != 1+map[bool]int{true: 1}[want != ""] || (want != "" && (!q.Has(want) || len(q[want]) != 1 || q.Get(want) != "")) || len(q["versionId"]) != 1 {
		return nil, denied
	}
	if method == "PUT" {
		raw, e := io.ReadAll(io.LimitReader(req.Body, 1025))
		req.Body.Close()
		wanted := []byte("<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>" + deadline + "</RetainUntilDate></Retention>")
		if e != nil || !bytes.Equal(raw, wanted) || req.ContentLength != int64(len(raw)) {
			return nil, denied
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
	}
	id := uuid.New()
	at := r.now().UTC()
	start := map[string]any{"kind": kind, "maintenance_id": r.b.MaintenanceID, "physical_attempt_id": id, "group": r.group, "copy": cp.Copy, "operation": op.Operation, "method": method, "started_at": at, "provider_invocation": "NOT_YET_CONFIRMED"}
	if method == "PUT" {
		if e := r.save(r.budget, r.group+"-slot.json", start); e != nil {
			return nil, e
		}
	}
	if e := r.save(r.output, "start-"+id.String()+".json", start); e != nil {
		return nil, e
	}
	// Re-read revocable authority and freshness after all durable start writes.
	if r.authorize() != nil {
		return nil, denied
	}
	response, e := r.base.RoundTrip(req)
	status := 0
	invoked := "UNKNOWN"
	outcome := "UNKNOWN"
	if response != nil {
		status = response.StatusCode
		invoked = "CONFIRMED"
		outcome = "HTTP_FAILURE"
		if status == 200 || status == 204 {
			outcome = "ACCEPTED"
		} else if status == 403 {
			outcome = "DENIED"
		}
	}
	responseHash, errorCode := "", ""
	if response != nil && response.Body != nil {
		body, re := io.ReadAll(io.LimitReader(response.Body, 65537))
		response.Body.Close()
		responseHash = digest(body)
		if re != nil || len(body) > 65536 {
			e = denied
		} else {
			response.Body = io.NopCloser(bytes.NewReader(body))
		}
		if status >= 400 {
			var failure struct{ Code string }
			if xml.Unmarshal(body, &failure) == nil {
				switch failure.Code {
				case "AccessDenied", "NoSuchVersion", "InvalidRequest", "InvalidArgument", "InternalError", "RequestTimeTooSkewed", "SignatureDoesNotMatch":
					errorCode = failure.Code
				default:
					errorCode = "UNRECOGNIZED_PROVIDER_ERROR"
				}
			}
		}
	}
	obs := map[string]any{"physical_attempt_id": id, "group": r.group, "operation": op.Operation, "observed_at": r.now().UTC(), "http_status": status, "error_code": errorCode, "response_sha256": responseHash, "transport_error": e != nil, "provider_invocation": invoked, "outcome": outcome, "quantity": 1, "unit": "HTTP_TRANSPORT_INVOCATION", "amount": "NOT_ASSESSED", "automatic_retry": false}
	if j := r.save(r.output, "observation-"+id.String()+".json", obs); j != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, j
	}
	return response, e
}
func (r *engine) checkObject(ctx context.Context, expiry string) error {
	t, _ := fixed(r.group)
	cp := t.Copy
	c := func(op string) context.Context { return context.WithValue(ctx, callKey{}, call{op}) }
	obj, e := r.client.GetObject(c("content"), cp.Namespace, cp.Key, minio.GetObjectOptions{VersionID: cp.ProviderVersion})
	if e != nil {
		return denied
	}
	raw, e := io.ReadAll(io.LimitReader(obj, cp.ByteSize+1))
	info, statErr := obj.Stat()
	closeErr := obj.Close()
	if e != nil || statErr != nil || info.VersionID != cp.ProviderVersion || info.Size != cp.ByteSize || info.IsDeleteMarker || closeErr != nil || int64(len(raw)) != cp.ByteSize || digest(raw) != cp.ContentSHA256 {
		return denied
	}
	mode, until, e := r.client.GetObjectRetention(c("retention"), cp.Namespace, cp.Key, cp.ProviderVersion)
	if e != nil || mode == nil || string(*mode) != "GOVERNANCE" || until == nil || until.UTC().Format(time.RFC3339Nano) != expiry {
		return denied
	}
	hold, e := r.client.GetObjectLegalHold(c("hold"), cp.Namespace, cp.Key, minio.GetObjectLegalHoldOptions{VersionID: cp.ProviderVersion})
	if e != nil || hold == nil || *hold != minio.LegalHoldDisabled {
		return denied
	}
	return nil
}
func (r *engine) run(ctx context.Context) (result error) {
	if r.authorize() != nil {
		return denied
	}
	// A failed or interrupted preflight also permanently closes this invocation.
	if e := r.save(r.budget, r.group+"-invocation.json", map[string]any{"maintenance_id": r.b.MaintenanceID, "bindings_sha256": r.bindingSHA, "at": r.now().UTC()}); e != nil {
		return e
	}
	defer func() {
		readback := result == nil
		fact := map[string]any{"MaintenanceID": r.b.MaintenanceID, "BindingsSHA256": r.bindingSHA, "Status": "FAILED_NO_AUTOMATIC_RETRY", "readback_confirmed": readback, "group": r.group, "at": r.now().UTC(), "delete": 0, "hold_change": 0, "bypass": 0}
		if result == nil {
			if e := r.save(r.budget, r.group+"-success.json", map[string]any{"MaintenanceID": r.b.MaintenanceID, "BindingsSHA256": r.bindingSHA, "Status": "PASS_EXACT_RETENTION_READBACK"}); e != nil {
				result = denied
			}
		}
		if result == nil {
			fact["Status"] = "PASS_EXACT_RETENTION_READBACK"
		}
		if e := r.save(r.output, "terminal.json", fact); e != nil {
			result = denied
		}
		if result != nil {
			fact["Status"] = "FAILED_NO_AUTOMATIC_RETRY"
			// The host also requires exit 0 and this owner's terminal receipt
			// before attesting the next owner. A partial checkpoint is not closure.
			_ = r.save(r.budget, r.group+"-failed.json", fact)
		}
	}()
	t, _ := fixed(r.group)
	if r.checkObject(ctx, t.OldUntil) != nil {
		return denied
	}
	until, _ := time.Parse(time.RFC3339, deadline)
	mode := minio.Governance
	if e := r.client.PutObjectRetention(context.WithValue(ctx, callKey{}, call{"extend"}), t.Copy.Namespace, t.Copy.Key, minio.PutObjectRetentionOptions{VersionID: t.Copy.ProviderVersion, Mode: &mode, RetainUntilDate: &until, GovernanceBypass: false}); e != nil {
		return denied
	}
	if r.checkObject(ctx, deadline) != nil {
		return denied
	}
	return nil
}
func loadEngine(private, budget, output string) (*engine, error) {
	var b bindings
	raw, e := os.ReadFile(filepath.Join(private, "bindings.json"))
	if e != nil || strict(raw, &b) != nil || checkedBindings(b) != nil {
		return nil, denied
	}
	var cfg config
	if read(filepath.Join(private, "root.json"), &cfg) != nil || cfg.Endpoint != "127.0.0.1:9000" || cfg.RootAccess == "" || cfg.RootSecret == "" {
		return nil, denied
	}
	var selected owner
	for _, o := range b.Owners {
		if o.Group == cfg.Group {
			selected = o
		}
	}
	configRaw, e := os.ReadFile(filepath.Join(private, "root.json"))
	if e != nil || digest(configRaw) != selected.ConfigSHA256 {
		return nil, denied
	}
	plan, e := os.ReadFile(filepath.Join(private, "approved-proposal.json"))
	if e != nil || digest(plan) != b.PlanSHA256 {
		return nil, denied
	}
	self, e := os.Executable()
	if e != nil {
		return nil, denied
	}
	binary, e := os.ReadFile(self)
	if e != nil || digest(binary) != b.BinarySHA256 {
		return nil, denied
	}
	key, e := os.ReadFile(filepath.Join(private, "host-public.key"))
	if e != nil || len(key) != ed25519.PublicKeySize {
		return nil, denied
	}
	r := &engine{b: b, bindingSHA: digest(raw), group: cfg.Group, selected: selected, private: private, budget: budget, output: output, public: key, now: time.Now, endpoint: cfg.Endpoint, syncDir: syncDirectory}
	r.base = &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	r.client, e = minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.RootAccess, cfg.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: r})
	if e != nil {
		return nil, denied
	}
	return r, nil
}
func mainRun(check bool) error {
	var b bindings
	raw, e := os.ReadFile("/run/private/bindings.json")
	if e != nil || strict(raw, &b) != nil || checkedBindings(b) != nil {
		return denied
	}
	if check {
		fmt.Println("PASS_FINITE_BINDINGS_ONLY; NOT_AUTHORIZED; no source/provider I/O")
		return nil
	}
	if runtime.GOOS != "linux" || !hash(trustedHostPublicSHA256) {
		return denied
	}
	for _, dir := range []string{"/run/private", "/run/budget", "/run/output"} {
		if pilot.CheckCustodyPrivatePath(dir, true) != nil {
			return denied
		}
	}
	r, e := loadEngine("/run/private", "/run/budget", "/run/output")
	if e != nil {
		return denied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return r.run(ctx)
}
func main() {
	check := flag.Bool("check", false, "check finite bindings only; never contacts a provider")
	flag.Parse()
	if flag.NArg() != 0 || mainRun(*check) != nil {
		fmt.Fprintln(os.Stderr, denied)
		os.Exit(1)
	}
}
