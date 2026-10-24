package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const postV22Purpose = "POST_V22_SEVENTEEN_NATIVE_READONLY"
const postV22Candidate = "f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243"
const absentP1 = "5a4c9789-3e67-4e54-ab58-128d111cbd65"

// Root credentials retain provider-side root authority. Only this fixed helper's
// GET/HEAD transport enforces read-only access. No business credentials/DB live here.
type postV22ReadProfile struct {
	Enabled                                                 bool
	AuditID                                                 uuid.UUID
	Purpose, Group, CandidateSHA256, BinarySHA256           string
	Core, Native, Network, Endpoint, RootAccess, RootSecret string
	FrozenInventorySHA256, PresentInventorySHA256           string
	NotBefore, ExpiresAt                                    time.Time
	MaxProviderDeletes                                      int
}

// Exact retained sources and original inventory bytes; no profile can select
// another environment, omit an arbitrary object, or redefine the old policy.
var postV22Sources = pilot.RemainingEnvironmentSources()

func strictPostV22(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing post-v22 input")
	}
	return nil
}
func postV22Hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func parsePostV22Profile(raw []byte, now time.Time) (postV22ReadProfile, error) {
	var p postV22ReadProfile
	if e := strictPostV22(raw, &p); e != nil {
		return p, e
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	source, ok := postV22Sources[p.Group]
	if len(fields) != 17 || string(fields["MaxProviderDeletes"]) != "0" || !ok || !p.Enabled || p.AuditID == uuid.Nil || p.AuditID.String() == "8e41cfba-a41e-44dd-9ce8-d6365b3bb90e" ||
		p.Purpose != postV22Purpose || p.CandidateSHA256 != postV22Candidate ||
		p.Core != source[0] || p.Native != source[1] || p.Network != source[2] || p.FrozenInventorySHA256 != source[3] ||
		p.Endpoint != "127.0.0.1:9000" || p.RootAccess == "" || p.RootSecret == "" ||
		!hexDigest(p.BinarySHA256) || !hexDigest(p.PresentInventorySHA256) ||
		p.NotBefore.IsZero() || p.ExpiresAt.Sub(p.NotBefore) <= 0 || p.ExpiresAt.Sub(p.NotBefore) > 55*time.Minute ||
		now.Before(p.NotBefore) || !now.Before(p.ExpiresAt) {
		return p, errors.New("post-v22 profile authority, scope or lifetime differs")
	}
	return p, nil
}
func hexDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}

func postV22Inventories(p postV22ReadProfile, frozen, present []byte) ([]remainingExpected, []remainingExpected, *domain.Copy, error) {
	var full, actual []remainingExpected
	if postV22Hash(frozen) != p.FrozenInventorySHA256 || postV22Hash(present) != p.PresentInventorySHA256 ||
		strictPostV22(frozen, &full) != nil || strictPostV22(present, &actual) != nil || validateRemainingInventory(p.Group, full) != nil {
		return nil, nil, nil, errors.New("post-v22 exact inventory bytes differ")
	}
	wanted := []remainingExpected{}
	var absent *domain.Copy
	for _, o := range full {
		if o.Copy.ID.String() == absentP1 {
			if p.Group != "business-races" || absent != nil {
				return nil, nil, nil, errors.New("invalid P1 scope")
			}
			cp := o.Copy
			absent = &cp
		} else {
			wanted = append(wanted, o)
		}
	}
	if (p.Group == "business-races") != (absent != nil) || !reflect.DeepEqual(wanted, actual) {
		return nil, nil, nil, errors.New("present inventory must be exactly frozen inventory minus unique P1")
	}
	return full, actual, absent, nil
}

type postV22VerifyTransport struct {
	c          config
	base       http.RoundTripper
	p1         *domain.Copy
	p1Attempts []uuid.UUID
	p1Results  map[uuid.UUID]postV22ProviderResult
	auditID    uuid.UUID
	lastID     uuid.UUID
	lastStatus int
}
type postV22ProviderResult struct {
	Code, RequestID string
	Status          int
}

func (t *postV22VerifyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "http" || req.URL.Host != t.c.Endpoint || req.URL.User != nil || req.URL.Fragment != "" || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
		return nil, errors.New("post-v22 verifier permits only bound GET/HEAD")
	}
	id := uuid.New()
	if e := write(t.c, "readonly-start-"+id.String()+".json", map[string]any{"id": id, "audit_id": t.auditID, "method": req.Method, "path": req.URL.Path, "query": req.URL.RawQuery, "at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	t.lastID, t.lastStatus = id, 0
	if t.p1 != nil && req.Method == "HEAD" && req.URL.Path == "/"+t.p1.Namespace+"/"+t.p1.Key && req.URL.Query().Get("versionId") == t.p1.ProviderVersion && len(req.URL.Query()) == 1 {
		t.p1Attempts = append(t.p1Attempts, id)
	}
	response, e := t.base.RoundTrip(req)
	status := 0
	providerCode, requestID := "", ""
	if response != nil {
		status = response.StatusCode
		providerCode = response.Header.Get("X-Minio-Error-Code")
		requestID = response.Header.Get("X-Amz-Request-Id")
	}
	t.lastStatus = status
	if len(t.p1Attempts) > 0 && t.p1Attempts[len(t.p1Attempts)-1] == id {
		if t.p1Results == nil {
			t.p1Results = map[uuid.UUID]postV22ProviderResult{}
		}
		t.p1Results[id] = postV22ProviderResult{providerCode, requestID, status}
	}
	if j := write(t.c, "readonly-observation-"+id.String()+".json", map[string]any{"id": id, "audit_id": t.auditID, "status": status, "provider_error_code": providerCode, "provider_request_id": requestID, "transport_error": e != nil, "at": time.Now().UTC()}); j != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, j
	}
	return response, e
}

func postV22Absence(ctx context.Context, c config, cp *domain.Copy, t *postV22VerifyTransport) (any, error) {
	if cp == nil {
		return nil, nil
	}
	client, e := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: t})
	if e != nil {
		return nil, e
	}
	_, e = client.StatObject(ctx, cp.Namespace, cp.Key, minio.StatObjectOptions{VersionID: cp.ProviderVersion})
	native := minio.ToErrorResponse(e)
	if e == nil || native.StatusCode != 404 || (native.Code != "NoSuchVersion" && native.Code != "NoSuchKey") || len(t.p1Attempts) != 1 {
		return nil, errors.New("P1 requires one exact version HEAD with official absence")
	}
	provider := t.p1Results[t.p1Attempts[0]]
	if provider.Status != 404 || provider.Code != native.Code || provider.RequestID == "" {
		return nil, errors.New("P1 absence requires original provider error code and request identity")
	}
	return map[string]any{"copy": cp, "attempt_id": t.p1Attempts[0], "status": native.StatusCode, "code": native.Code}, nil
}

func postV22Verify(ctx context.Context) error {
	raw, e := os.ReadFile("/run/private/post-v22-readonly.json")
	if e != nil {
		return e
	}
	started := time.Now().UTC()
	p, e := parsePostV22Profile(raw, started)
	if e != nil {
		return e
	}
	return verifyNativeAudit(ctx, p, raw, started, "post-v22-readonly.json", "post-v22-native-verification.json", false)
}

func verifyNativeAudit(ctx context.Context, p postV22ReadProfile, raw []byte, started time.Time, profileFile, receiptFile string, six bool) error {
	return verifyNativeAuditPhase(ctx, p, raw, started, profileFile, receiptFile, six, nil)
}

func verifyNativeAuditPhase(ctx context.Context, p postV22ReadProfile, raw []byte, started time.Time, profileFile, receiptFile string, six bool, maintenance *retentionReadProfile) error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(executable)
	if e != nil || postV22Hash(binary) != p.BinarySHA256 {
		return errors.New("post-v22 executable differs")
	}
	frozen, e := os.ReadFile("/run/private/frozen-inventory.json")
	if e != nil {
		return e
	}
	present, e := os.ReadFile("/run/private/present-inventory.json")
	if e != nil {
		return e
	}
	full, actual, absent, e := postV22Inventories(p, frozen, present)
	if e != nil {
		return e
	}
	if maintenance != nil {
		effective, err := os.ReadFile("/run/private/effective-inventory.json")
		if err != nil {
			return err
		}
		actual, err = retentionEffectiveInventory(*maintenance, actual, effective)
		if err != nil {
			return err
		}
		p.PresentInventorySHA256 = maintenance.EffectiveInventorySHA256
	}
	scope, e := pilot.RemainingScopeFor(p.Group)
	if e != nil {
		return e
	}
	c := config{RunID: scope.Run, Endpoint: p.Endpoint, RootAccess: p.RootAccess, RootSecret: p.RootSecret, InspectAccess: "inspect" + scope.Run.String()[:8] + scope.Run.String()[9:13], OutputDirectory: "/run/output"}
	ctx, cancel := context.WithDeadline(ctx, p.ExpiresAt)
	defer cancel()
	transport := &postV22VerifyTransport{c: c, base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, p1: absent, auditID: p.AuditID}
	absence, e := postV22Absence(ctx, c, absent, transport)
	if e != nil {
		return e
	}
	users, policies, e := legacyNativePolicies(c, p.Group, full)
	if e != nil {
		return e
	}
	if six {
		copies := []domain.Copy{}
		for _, item := range actual {
			copies = append(copies, item.Copy)
		}
		newUsers, newPolicies, err := pilot.RemainingAuditPolicies(p.Group, copies)
		if err != nil {
			return err
		}
		for k, v := range newUsers {
			users[k] = v
		}
		for k, v := range newPolicies {
			policies[k] = v
		}
	}
	verification, e := collectNativeWithPolicies(ctx, c, p.Group, actual, users, policies, transport)
	if e != nil {
		return e
	}
	if six {
		if e = sixUncreatedUsers(ctx, c, p.Group, transport); e != nil {
			return e
		}
	}
	verification["inventory_sha256"] = p.PresentInventorySHA256
	verification["source_config"] = profileFile
	finished := time.Now().UTC()
	if !finished.Before(p.ExpiresAt) {
		return errors.New("post-v22 read deadline exceeded")
	}
	fact := map[string]any{"schema": p.Purpose, "audit_id": p.AuditID, "profile_sha256": postV22Hash(raw), "binary_sha256": p.BinarySHA256, "candidate_sha256": p.CandidateSHA256, "frozen_inventory_sha256": p.FrozenInventorySHA256, "core": p.Core, "native": p.Native, "network": p.Network, "not_before": p.NotBefore, "expires_at": p.ExpiresAt, "started_at": started, "finished_at": finished, "p1_absence": absence, "verification": verification}
	if maintenance != nil {
		fact["maintenance_id"] = maintenance.MaintenanceID
		fact["phase"] = maintenance.Phase
		fact["bindings_sha256"] = maintenance.BindingsSHA256
		fact["retain_until"] = maintenance.RetainUntil
		fact["maintenance_end"] = maintenance.MaintenanceEnd
	}
	return write(c, receiptFile, fact)
}
