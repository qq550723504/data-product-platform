package main

// Fixed preparation profiles for the independently reviewed remaining-D1
// contract. These are not sender profiles and grant no disposal authority.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/v3/policy"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type remainingObject struct {
	Case, Namespace, Key, Content string
	Hold, Retained, FutureSend    bool
}

func remainingLayout(group string) ([]remainingObject, error) {
	a, b := namespaces[0], namespaces[1]
	control := remainingObject{"CONTROL", a, "fixture/control-hold.csv", "control", true, false, false}
	switch group {
	case "business-races":
		return []remainingObject{
			{"P1", a, "fixture/p1.csv", "p1", false, false, true},
			{"P2", a, "fixture/p2.csv", "p2", false, false, true},
			{"T1", a, "fixture/t1.csv", "t1", false, false, true},
			{"T2", b, "fixture/t2.csv", "t2", false, false, true},
			control, {"CONTROL", b, "fixture/control-retention.csv", "retained", false, true, false},
		}, nil
	case "late-hold":
		return []remainingObject{
			{"H0", a, "fixture/h0.csv", "h0", false, false, false},
			{"H1", a, "fixture/h1-a.csv", "h1", false, false, false},
			{"H1", b, "fixture/h1-b.csv", "h1", false, false, false},
			{"H2", a, "fixture/h2-a.csv", "h2", false, false, true},
			{"H2", b, "fixture/h2-b.csv", "h2", false, false, false}, control,
		}, nil
	case "mixed":
		return []remainingObject{
			{"M1", a, "fixture/m1-a.csv", "m1", false, false, true},
			{"M1", b, "fixture/m1-b.csv", "m1", false, false, true},
			{"M1", b, "fixture/m1-c.csv", "m1", false, true, false}, control,
		}, nil
	default:
		return nil, errors.New("unknown remaining-D1 preparation group")
	}
}

func remainingPayload(group, content string) []byte {
	// Real native processing inputs can be derived in later case preparation;
	// use the existing synthetic source contract, never customer data.
	if group == "business-races" && (content == "p1" || content == "p2") {
		return pilot.PlaneSyntheticPayloads()[0]
	}
	if content == "t1" {
		return pilot.PlaneSyntheticPayloads()[1] // actual TCP backpressure input
	}
	return []byte(fmt.Sprintf("id,value\n1,synthetic-remaining-%s-%s\n", group, content))
}

func remainingPolicy(access string, objects []object, layout []remainingObject) ([]byte, error) {
	if len(objects) != len(layout) || access == "" {
		return nil, errors.New("remaining IAM scope invalid")
	}
	statements := []any{}
	for i, o := range objects {
		s := layout[i]
		if o.Namespace != s.Namespace || o.Key != s.Key || o.Version == "" || o.Version == "null" {
			return nil, errors.New("remaining IAM identity mismatch")
		}
		if s.FutureSend {
			// Native handler requires both actions; the version condition still
			// rejects a bare delete. Reuse the corrected v15 IAM contract.
			statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:DeleteObject", "s3:DeleteObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + o.Namespace + "/" + o.Key}, "Condition": map[string]any{"StringEquals": map[string]any{"s3:versionid": []string{o.Version}}}})
		}
	}
	// No unversioned delete, payload read/write, list, protection change or bypass.
	return json.MarshalIndent(map[string]any{"Version": "2012-10-17", "Statement": statements}, "", "  ")
}

func prepareProfilePlans(ctx context.Context, c config, manager *transaction.Manager, repo *infrastructure.PostgresRepository, a authority, objects []object, ids []uuid.UUID) (domain.Plan, []map[string]any, error) {
	svc := lc.NewPlanService(manager, repo, a)
	if c.RemainingGroup == "" {
		p, e := svc.PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: ids, IdempotencyKey: "fixture-plan:" + c.RunID.String()})
		return p, nil, e
	}
	layout, _ := remainingLayout(c.RemainingGroup)
	byCase := map[string][]uuid.UUID{}
	var cases []string
	for i, s := range layout {
		if s.Case == "CONTROL" {
			continue
		}
		if _, ok := byCase[s.Case]; !ok {
			cases = append(cases, s.Case)
		}
		byCase[s.Case] = append(byCase[s.Case], objects[i].CopyID)
	}
	var plan domain.Plan
	var plans []map[string]any
	for _, name := range cases {
		p, e := svc.PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: byCase[name], IdempotencyKey: "remaining:" + c.RunID.String() + ":" + name})
		if e != nil {
			return p, nil, e
		}
		plan = p
		plans = append(plans, map[string]any{"case": name, "id": p.ID, "scope_hash": p.ContentHash, "hash_algorithm": domain.PlanHashAlgorithm, "status": p.Status, "targets": len(p.Targets)})
	}
	return plan, plans, nil
}

func finishRemaining(ctx context.Context, c config, objects []object, plans []map[string]any, manifest map[string]any) error {
	manifest["schema"] = "lifecycle307-remaining-preparation-v1"
	manifest["group"] = c.RemainingGroup
	manifest["plans"] = plans
	delete(manifest, "plan")
	layout, _ := remainingLayout(c.RemainingGroup)
	deleteAccess := "delete" + c.RunID.String()[:8]
	policy, e := remainingPolicy(deleteAccess, objects, layout)
	if e != nil {
		return e
	}
	policyName := "lifecycle307-remaining-delete-" + c.RunID.String()
	t := &bootstrapTransport{host: c.Endpoint, receipts: filepath.Join(c.OutputDirectory, "bootstrap-receipts.jsonl"), base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second}}
	admin, e := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Secure: false, Transport: t})
	if e != nil {
		return e
	}
	for _, op := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"create-delete-identity-without-policy", func(ctx context.Context) error { return admin.AddUser(ctx, deleteAccess, c.Token) }},
		{"create-exact-delete-policy", func(ctx context.Context) error { return admin.AddCannedPolicy(ctx, policyName, policy) }},
		{"attach-disabled-exact-delete-policy", func(ctx context.Context) error {
			_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{policyName}, User: deleteAccess})
			return e
		}},
		{"disable-delete-identity", func(ctx context.Context) error { return admin.SetUserStatus(ctx, deleteAccess, madmin.AccountDisabled) }},
		{"verify-disabled-delete-identity", func(ctx context.Context) error {
			u, e := admin.GetUserInfo(ctx, deleteAccess)
			if e == nil && u.Status != madmin.AccountDisabled {
				return errors.New("delete identity not disabled")
			}
			return e
		}},
		{"disable-inspection-identity", func(ctx context.Context) error {
			return admin.SetUserStatus(ctx, c.InspectAccess, madmin.AccountDisabled)
		}},
		{"verify-disabled-inspection-identity", func(ctx context.Context) error {
			u, e := admin.GetUserInfo(ctx, c.InspectAccess)
			if e == nil && u.Status != madmin.AccountDisabled {
				return errors.New("inspection identity not disabled")
			}
			return e
		}},
	} {
		if e = one(ctx, op.name, op.run); e != nil {
			return e
		}
	}
	if e = os.WriteFile(filepath.Join(c.OutputDirectory, "delete-policy.json"), policy, 0600); e != nil {
		return e
	}
	manifest["iam"] = map[string]any{"delete_access": deleteAccess, "delete_policy": policyName, "delete_status": "disabled", "inspection_status": "disabled", "root_scope": "private bootstrap only; no sender mount", "delete_policy_sha256": fmt.Sprintf("%x", sha256.Sum256(policy))}
	manifest["sender_profile"] = "NOT_IMPLEMENTED_FOR_REMAINING_CASES"
	manifest["current_delete_budget"] = 0
	return nil
}

func remainingVersions(layout []remainingObject) ([]uuid.UUID, map[string]uuid.UUID) {
	var versions []uuid.UUID
	byContent := map[string]uuid.UUID{}
	for _, s := range layout {
		if _, ok := byContent[s.Content]; !ok {
			id := uuid.New()
			versions = append(versions, id)
			byContent[s.Content] = id
		}
	}
	return versions, byContent
}

// Recovery verification is a separate one-shot, GET/HEAD-only bootstrap probe.
// It neither connects to Core nor enables a product/inspection/send authority.
// Its physical request journal is not product business acceptance or CostEvent.
func verifyRemaining(ctx context.Context, c config) error {
	layout, e := remainingLayout(c.RemainingGroup)
	if e != nil || c.Enabled {
		return errors.New("disabled fixed remaining profile required")
	}
	var manifest struct {
		Schema  string    `json:"schema"`
		Run     uuid.UUID `json:"run_id"`
		Group   string    `json:"group"`
		Objects []object  `json:"objects"`
	}
	raw, e := os.ReadFile("/run/manifest.json")
	if e != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Schema != "lifecycle307-remaining-preparation-v1" || manifest.Run != c.RunID || manifest.Group != c.RemainingGroup || len(manifest.Objects) != len(layout) {
		return errors.New("fixed remaining manifest required")
	}
	if e = os.Mkdir(c.OutputDirectory, 0700); e != nil {
		return e
	}
	t := &bootstrapTransport{host: c.Endpoint, receipts: filepath.Join(c.OutputDirectory, "readonly-requests.jsonl"), base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second}}
	client, e := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: remainingReadTransport{t}})
	if e != nil {
		return e
	}
	for i, o := range manifest.Objects {
		s := layout[i]
		if o.Namespace != s.Namespace || o.Key != s.Key || o.Version == "" || o.Version == "null" || o.LegalHold != s.Hold || (o.Retention == "GOVERNANCE") != s.Retained {
			return errors.New("remaining exact membership/protection mismatch")
		}
		payload := remainingPayload(c.RemainingGroup, s.Content)
		if fmt.Sprintf("%x", sha256.Sum256(payload)) != o.SHA256 || int64(len(payload)) != o.Bytes {
			return errors.New("remaining content contract mismatch")
		}
		if e = one(ctx, "readonly-exact-bytes:"+o.Namespace+":"+o.Key, func(ctx context.Context) error {
			r, e := client.GetObject(ctx, o.Namespace, o.Key, minio.GetObjectOptions{VersionID: o.Version})
			if e != nil {
				return e
			}
			defer r.Close()
			b, e := io.ReadAll(io.LimitReader(r, o.Bytes+1))
			if e != nil {
				return e
			}
			info, e := r.Stat()
			if e != nil {
				return e
			}
			if !bytes.Equal(b, payload) || info.VersionID != o.Version || info.Size != o.Bytes {
				return errors.New("native restored bytes/identity mismatch")
			}
			return nil
		}); e != nil {
			return e
		}
		if e = one(ctx, "readonly-retention:"+o.Key, func(ctx context.Context) error {
			mode, until, e := client.GetObjectRetention(ctx, o.Namespace, o.Key, o.Version)
			if e != nil {
				native := minio.ToErrorResponse(e)
				if o.Retention == "NONE" && native.StatusCode == 400 && native.Code == "NoSuchObjectLockConfiguration" {
					return nil
				}
				return e
			}
			actual := "NONE"
			if mode != nil {
				actual = string(*mode)
			}
			if actual != o.Retention || (until == nil) != (o.RetainUntil == nil) || (until != nil && !until.Equal(*o.RetainUntil)) {
				return errors.New("native retention drift")
			}
			return nil
		}); e != nil {
			return e
		}
		if e = one(ctx, "readonly-hold:"+o.Key, func(ctx context.Context) error {
			h, e := client.GetObjectLegalHold(ctx, o.Namespace, o.Key, minio.GetObjectLegalHoldOptions{VersionID: o.Version})
			if e != nil {
				native := minio.ToErrorResponse(e)
				if !o.LegalHold && native.StatusCode == 400 && native.Code == "NoSuchObjectLockConfiguration" {
					return nil
				}
				return e
			}
			if (h != nil && *h == minio.LegalHoldEnabled) != o.LegalHold {
				return errors.New("native hold drift")
			}
			return nil
		}); e != nil {
			return e
		}
	}
	for _, ns := range namespaces {
		expected := map[string]int64{}
		for _, o := range manifest.Objects {
			if o.Namespace == ns {
				expected[o.Key+"\x00"+o.Version] = o.Bytes
			}
		}
		if e = one(ctx, "readonly-full-inventory:"+ns, func(ctx context.Context) error {
			seen := map[string]bool{}
			for o := range client.ListObjects(ctx, ns, minio.ListObjectsOptions{Recursive: true, WithVersions: true, MaxKeys: 1000}) {
				if o.Err != nil {
					return o.Err
				}
				key := o.Key + "\x00" + o.VersionID
				size, ok := expected[key]
				if !ok || seen[key] || o.IsDeleteMarker || o.Size != size {
					return errors.New("native inventory drift")
				}
				seen[key] = true
			}
			if len(seen) != len(expected) {
				return errors.New("native inventory incomplete")
			}
			return nil
		}); e != nil {
			return e
		}
	}
	admin, e := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Secure: false, Transport: remainingReadTransport{t}})
	if e != nil {
		return e
	}
	for _, access := range []string{c.InspectAccess, "delete" + c.RunID.String()[:8]} {
		if e = one(ctx, "readonly-disabled-user:"+access, func(ctx context.Context) error {
			u, e := admin.GetUserInfo(ctx, access)
			if e == nil && (u.Status != madmin.AccountDisabled || u.PolicyName == "") {
				return errors.New("native IAM disabled/policy contract drift")
			}
			return e
		}); e != nil {
			return e
		}
	}
	expected, e := remainingPolicy("delete"+c.RunID.String()[:8], manifest.Objects, layout)
	if e != nil {
		return e
	}
	if e = one(ctx, "readonly-exact-policy", func(ctx context.Context) error {
		actual, e := admin.InfoCannedPolicy(ctx, "lifecycle307-remaining-delete-"+c.RunID.String())
		if e != nil {
			return e
		}
		a, err := policy.ParseConfig(bytes.NewReader(actual))
		if err != nil {
			return err
		}
		b, err := policy.ParseConfig(bytes.NewReader(expected))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(c.OutputDirectory, "native-delete-policy.json"), actual, 0600); err != nil {
			return err
		}
		if !a.Equals(*b) {
			return errors.New("IAM exact policy drift")
		}
		return nil
	}); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(map[string]any{"status": "PASS_NATIVE_READONLY", "run_id": c.RunID, "group": c.RemainingGroup, "objects": manifest.Objects, "version_count": len(manifest.Objects), "delete": 0, "mutations": 0, "root_management_probe": true, "business_acceptance": "NOT_RUN", "disabled_users": 2, "exact_iam_policy": "PASS"}, "", "  ")
	return os.WriteFile(filepath.Join(c.OutputDirectory, "verification.json"), append(b, '\n'), 0600)
}

type remainingReadTransport struct{ base http.RoundTripper }

func (t remainingReadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != "GET" && r.Method != "HEAD" {
		return nil, errors.New("remaining verification is GET/HEAD only")
	}
	return t.base.RoundTrip(r)
}
