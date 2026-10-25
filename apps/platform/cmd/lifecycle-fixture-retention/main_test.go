package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fixture struct {
	b              bindings
	key            ed25519.PrivateKey
	at             time.Time
	budget         string
	dirs           map[string]string
	outputs        map[string]string
	server         *httptest.Server
	requests, puts int
	failStatus     int
	unknown        bool
	wrongVersion   bool
	beforePut      func()
	expiries       map[string]string
	bodies         map[string][]byte
	root           string
}

func testFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{at: time.Date(2026, 10, 23, 14, 0, 0, 0, time.UTC), root: t.TempDir(), dirs: map[string]string{}, outputs: map[string]string{}, expiries: map[string]string{}, bodies: map[string][]byte{}}
	public, key, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	f.key = key
	old := trustedHostPublicSHA256
	trustedHostPublicSHA256 = digest(public)
	t.Cleanup(func() { trustedHostPublicSHA256 = old })
	f.budget = filepath.Join(f.root, "budget")
	os.Mkdir(f.budget, 0700)
	self, _ := os.Executable()
	binary, _ := os.ReadFile(self)
	f.b = bindings{Kind: kind, MaintenanceID: uuid.New(), NotBefore: f.at.Add(-time.Minute), NotAfter: f.at.Add(30 * time.Minute), RetainUntil: time.Date(2026, 10, 24, 12, 40, 0, 0, time.UTC), BinarySHA256: digest(binary), HostPublicSHA256: digest(public), BudgetID: digest([]byte("independent-new-maintenance-budget"))}
	expected, e := proposalValue(f.b)
	if e != nil {
		t.Fatal(e)
	}
	proposal, _ := json.Marshal(expected)
	f.b.PlanSHA256 = digest(proposal)
	for i, g := range []string{"business-races", "mixed"} {
		cp, _ := fixed(g)
		folder := filepath.Join(f.root, g)
		os.Mkdir(folder, 0700)
		out := filepath.Join(f.root, g+"-output")
		os.Mkdir(out, 0700)
		f.dirs[g] = folder
		f.outputs[g] = out
		cfg := config{g, "127.0.0.1:9000", "test-access", "never-log-this-secret"}
		raw, _ := json.Marshal(cfg)
		f.b.Owners = append(f.b.Owners, owner{g, strings.Repeat(fmt.Sprint(i+1), 64), cp.Native, digest([]byte("descriptor:" + g)), digest(raw)})
		os.WriteFile(filepath.Join(folder, "root.json"), raw, 0600)
		os.WriteFile(filepath.Join(folder, "host-public.key"), public, 0600)
		os.WriteFile(filepath.Join(folder, "approved-proposal.json"), proposal, 0600)
		name := "m1"
		if g == "business-races" {
			name = "retained"
		}
		f.bodies[g] = []byte(fmt.Sprintf("id,value\n1,synthetic-remaining-%s-%s\n", g, name))
		if digest(f.bodies[g]) != cp.Copy.ContentSHA256 {
			t.Fatal("body contract differs")
		}
		f.expiries[g] = cp.OldUntil
	}
	f.writeBindings(t)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		g := "mixed"
		if strings.Contains(r.URL.Path, "control-retention.csv") {
			g = "business-races"
		}
		cp, _ := fixed(g)
		if r.URL.Path != "/"+cp.Copy.Namespace+"/"+cp.Copy.Key || r.URL.Query().Get("versionId") != cp.Copy.ProviderVersion {
			t.Error("expanded provider target")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("X-Amz-Version-Id", cp.Copy.ProviderVersion)
		if f.wrongVersion {
			w.Header().Set("X-Amz-Version-Id", "foreign-version")
		}
		w.Header().Set("Last-Modified", f.at.Format(http.TimeFormat))
		w.Header().Set("ETag", `"fixture-only"`)
		if r.Method == "PUT" {
			f.puts++
			if f.beforePut != nil {
				f.beforePut()
			}
			if f.unknown {
				h := w.(http.Hijacker)
				c, _, _ := h.Hijack()
				c.Close()
				return
			}
			if f.failStatus != 0 {
				w.WriteHeader(f.failStatus)
				fmt.Fprint(w, "<Error><Code>AccessDenied</Code><Message>never-log-this-secret</Message></Error>")
				return
			}
			f.expiries[g] = f.b.RetainUntil.UTC().Format(time.RFC3339)
			w.WriteHeader(204)
			return
		}
		if r.URL.Query().Has("retention") {
			fmt.Fprintf(w, "<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>%s</RetainUntilDate></Retention>", f.expiries[g])
		} else if r.URL.Query().Has("legal-hold") {
			fmt.Fprint(w, "<LegalHold><Status>OFF</Status></LegalHold>")
		} else {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Length", fmt.Sprint(len(f.bodies[g])))
			w.Write(f.bodies[g])
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil || os.WriteFile(path, b, 0600) != nil {
		t.Fatal("write fixture")
	}
}
func (f *fixture) writeBindings(t *testing.T) {
	raw, _ := json.Marshal(f.b)
	h := digest(raw)
	for _, o := range f.b.Owners {
		dir := f.dirs[o.Group]
		os.WriteFile(filepath.Join(dir, "bindings.json"), raw, 0600)
		human := []byte("TEST ONLY fresh protection-write grant\n")
		os.WriteFile(filepath.Join(dir, "user-message-verbatim.txt"), human, 0600)
		g := grant{kind, f.b.MaintenanceID, h, digest(human), f.b.NotBefore, f.b.NotAfter, f.at.Add(-time.Hour), 1, 2, false, f.b.RetainUntil}
		body, _ := json.Marshal(g)
		writeJSON(t, filepath.Join(dir, "authorization.json"), signed{body, ed25519.Sign(f.key, body)})
		writeJSON(t, filepath.Join(dir, "permission.json"), permission{true, h})
		p := proof{kind, f.b.MaintenanceID, h, o.Group, o.CID, o.Native, o.DescriptorSHA256, o.ConfigSHA256, f.b.BudgetID, f.at}
		body, _ = json.Marshal(p)
		writeJSON(t, filepath.Join(dir, "topology.json"), signed{body, ed25519.Sign(f.key, body)})
	}
	writeJSON(t, filepath.Join(f.budget, "ownership.json"), ownership{kind, f.b.MaintenanceID, h, f.b.BudgetID})
}
func (f *fixture) engine(t *testing.T, g string) *engine {
	t.Helper()
	r, e := loadEngine(f.dirs[g], f.budget, f.outputs[g])
	if e != nil {
		t.Fatal(e)
	}
	r.now = func() time.Time { return f.at }
	r.syncDir = func(string) error { return nil } // Windows test only; real CLI is Linux+directory fsync.
	// Production endpoint and transport allowlist remain 127.0.0.1:9000. Only the
	// test socket dial is redirected to the loopback provider double.
	r.base = &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", f.server.Listener.Addr().String())
	}}
	return r
}
func TestRetentionCompleteTwoOwnerSuccessAndPersistentReplayClosure(t *testing.T) {
	f := testFixture(t)
	if e := f.engine(t, "mixed").run(context.Background()); e == nil || f.requests != 0 {
		t.Fatal("mixed ran before immutable prior readback", e)
	}
	for _, g := range []string{"business-races", "mixed"} {
		r := f.engine(t, g)
		if e := r.run(context.Background()); e != nil {
			t.Fatal(g, e, f.requests)
		}
		prior := f.requests
		if r.run(context.Background()) == nil || f.requests != prior {
			t.Fatal("replay reached provider")
		}
	}
	if f.puts != 2 {
		t.Fatal(f.puts)
	}
	for _, g := range []string{"business-races", "mixed"} {
		if f.expiries[g] != f.b.RetainUntil.UTC().Format(time.RFC3339) {
			t.Fatal("missing readback")
		}
		var terminal map[string]any
		if read(filepath.Join(f.outputs[g], "terminal.json"), &terminal) != nil || terminal["Status"] != "PASS_EXACT_RETENTION_READBACK" {
			t.Fatal("missing terminal")
		}
	}
}
func TestRetentionGrantAndFreshTopologyDenialsBeforeIO(t *testing.T) {
	for _, fault := range []string{"disabled", "old-purpose", "bad-signature", "human-drift", "stale-proof", "future-proof", "window-end", "foreign-budget", "expanded-budget", "sync-failure", "unknown-key", "wrong-plan", "duplicate-json"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			r := f.engine(t, "business-races")
			dir := f.dirs[r.group]
			switch fault {
			case "disabled":
				writeJSON(t, filepath.Join(dir, "permission.json"), permission{false, r.bindingSHA})
			case "old-purpose", "bad-signature":
				var s signed
				read(filepath.Join(dir, "authorization.json"), &s)
				if fault == "bad-signature" {
					s.Signature[0] ^= 1
				} else {
					var g grant
					strict(s.Body, &g)
					g.Kind = "DELETE"
					s.Body, _ = json.Marshal(g)
					s.Signature = ed25519.Sign(f.key, s.Body)
				}
				writeJSON(t, filepath.Join(dir, "authorization.json"), s)
			case "human-drift":
				os.WriteFile(filepath.Join(dir, "user-message-verbatim.txt"), []byte("different"), 0600)
			case "stale-proof", "future-proof":
				var s signed
				var p proof
				read(filepath.Join(dir, "topology.json"), &s)
				strict(s.Body, &p)
				p.At = f.at.Add(-61 * time.Second)
				if fault == "future-proof" {
					p.At = f.at.Add(time.Second)
				}
				s.Body, _ = json.Marshal(p)
				s.Signature = ed25519.Sign(f.key, s.Body)
				writeJSON(t, filepath.Join(dir, "topology.json"), s)
			case "window-end":
				f.at = f.b.NotAfter
			case "foreign-budget":
				writeJSON(t, filepath.Join(f.budget, "ownership.json"), ownership{kind, uuid.New(), r.bindingSHA, r.b.BudgetID})
			case "expanded-budget":
				os.WriteFile(filepath.Join(f.budget, "unapproved.json"), []byte("{}"), 0600)
			case "sync-failure":
				r.syncDir = func(string) error { return io.ErrClosedPipe }
			case "unknown-key":
				trustedHostPublicSHA256 = strings.Repeat("0", 64)
			case "wrong-plan":
				r.b.PlanSHA256 = strings.Repeat("0", 64)
			case "duplicate-json":
				os.WriteFile(filepath.Join(dir, "permission.json"), []byte(`{"Enabled":false,"Enabled":true,"BindingsSHA256":"`+r.bindingSHA+`"}`), 0600)
			}
			if e := r.run(context.Background()); e == nil || f.requests != 0 || f.puts != 0 {
				t.Fatal("invalid authority reached provider", e, f.requests)
			}
		})
	}
}
func TestRetentionWireRejectsExpansionBeforeSlotOrProvider(t *testing.T) {
	for _, fault := range []string{"delete", "content-put", "hold-put", "wrong-version", "extra-query", "duplicate-version", "bypass", "wrong-xml", "wrong-host", "wrong-operation"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			r := f.engine(t, "business-races")
			cp, _ := fixed(r.group)
			body := "<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>" + f.b.RetainUntil.UTC().Format(time.RFC3339) + "</RetainUntilDate></Retention>"
			req, _ := http.NewRequestWithContext(context.WithValue(context.Background(), callKey{}, call{"extend"}), "PUT", "http://127.0.0.1:9000/"+cp.Copy.Namespace+"/"+cp.Copy.Key+"?retention=&versionId="+cp.Copy.ProviderVersion, strings.NewReader(body))
			switch fault {
			case "delete":
				req.Method = "DELETE"
			case "content-put":
				req.URL.RawQuery = "versionId=" + cp.Copy.ProviderVersion
			case "hold-put":
				req.URL.RawQuery = "legal-hold=&versionId=" + cp.Copy.ProviderVersion
			case "wrong-version":
				req.URL.RawQuery = "retention=&versionId=foreign"
			case "extra-query":
				req.URL.RawQuery += "&x=1"
			case "duplicate-version":
				req.URL.RawQuery += "&versionId=" + cp.Copy.ProviderVersion
			case "bypass":
				req.Header.Set("X-Amz-Bypass-Governance-Retention", "false")
			case "wrong-xml":
				req.Body = io.NopCloser(strings.NewReader(strings.Replace(body, "GOVERNANCE", "COMPLIANCE", 1)))
			case "wrong-host":
				req.URL.Host = "foreign:9000"
			case "wrong-operation":
				req = req.WithContext(context.WithValue(req.Context(), callKey{}, call{"delete"}))
			}
			if _, e := r.RoundTrip(req); e == nil || f.requests != 0 {
				t.Fatal("expanded wire passed", e)
			}
			if _, e := os.Stat(filepath.Join(f.budget, r.group+"-slot.json")); !os.IsNotExist(e) {
				t.Fatal("rejected request consumed slot")
			}
		})
	}
}
func TestRetentionFailureUnknownAndObservationLossNeverResend(t *testing.T) {
	for _, fault := range []string{"403", "500", "lost-response", "post-readback-drift", "revoke-after-start", "observation-failure"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			r := f.engine(t, "business-races")
			switch fault {
			case "403":
				f.failStatus = 403
			case "500":
				f.failStatus = 500
			case "lost-response":
				f.unknown = true
			case "post-readback-drift":
				f.beforePut = func() { f.bodies[r.group] = []byte("changed") }
			case "revoke-after-start":
				original := r.syncDir
				r.syncDir = func(dir string) error {
					if _, e := os.Stat(filepath.Join(f.budget, r.group+"-slot.json")); e == nil {
						writeJSON(t, filepath.Join(r.private, "permission.json"), permission{false, r.bindingSHA})
					}
					return original(dir)
				}
			case "observation-failure":
				f.beforePut = func() { r.syncDir = func(string) error { return io.ErrClosedPipe } }
			}
			if r.run(context.Background()) == nil {
				t.Fatal("failure accepted")
			}
			want := 1
			if fault == "revoke-after-start" {
				want = 0
			}
			if f.puts != want {
				t.Fatal("automatic retry", f.puts)
			}
			count := f.requests
			if f.engine(t, r.group).run(context.Background()) == nil || f.requests != count {
				t.Fatal("failed invocation replayed")
			}
			if f.engine(t, "mixed").run(context.Background()) == nil || f.requests != count {
				t.Fatal("next owner advanced after failure")
			}
			entries, _ := os.ReadDir(r.output)
			for _, p := range entries {
				raw, _ := os.ReadFile(filepath.Join(r.output, p.Name()))
				if bytes.Contains(raw, []byte("never-log-this-secret")) {
					t.Fatal("secret leaked")
				}
			}
		})
	}
}
func TestRetentionPreflightDriftAndJournalFailurePreserveNoMutation(t *testing.T) {
	for _, fault := range []string{"content", "retention", "journal", "response-version"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			r := f.engine(t, "business-races")
			if fault == "content" {
				f.bodies[r.group] = []byte("wrong")
			} else if fault == "retention" {
				f.expiries[r.group] = f.b.RetainUntil.UTC().Format(time.RFC3339)
			} else if fault == "response-version" {
				f.wrongVersion = true
			} else {
				os.Remove(r.output)
			}
			if r.run(context.Background()) == nil || f.puts != 0 {
				t.Fatal("precondition failed open")
			}
		})
	}
}

func TestRetentionConcurrentInvocationConsumesOneSlot(t *testing.T) {
	f := testFixture(t)
	r := f.engine(t, "business-races")
	start, done := make(chan struct{}), make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; done <- r.run(context.Background()) }()
	}
	close(start)
	one, two := <-done, <-done
	if (one == nil) == (two == nil) || f.puts != 1 {
		t.Fatal("concurrent invocation escaped O_EXCL", one, two, f.puts)
	}
}

func TestRetentionCheckpointPersistenceFailureCannotMasqueradeAsSuccess(t *testing.T) {
	for _, stage := range []string{"success", "terminal"} {
		t.Run(stage, func(t *testing.T) {
			f := testFixture(t)
			r := f.engine(t, "business-races")
			original := r.syncDir
			r.syncDir = func(dir string) error {
				name := filepath.Join(r.budget, r.group+"-success.json")
				if stage == "terminal" {
					name = filepath.Join(r.output, "terminal.json")
				}
				if _, err := os.Stat(name); err == nil {
					return io.ErrClosedPipe
				}
				return original(dir)
			}
			if r.run(context.Background()) == nil || f.puts != 1 {
				t.Fatal("checkpoint failure accepted")
			}
			// The injected directory fsync failure leaves bytes visible; the
			// failure record must still report failure, preserving readback fact.
			raw, err := os.ReadFile(filepath.Join(r.budget, r.group+"-failed.json"))
			var fact map[string]any
			if err != nil || json.Unmarshal(raw, &fact) != nil || fact["Status"] != "FAILED_NO_AUTOMATIC_RETRY" || fact["readback_confirmed"] != true {
				t.Fatal("failure confused with readback", string(raw), err)
			}
			requests := f.requests
			if f.engine(t, "mixed").run(context.Background()) == nil || f.requests != requests {
				t.Fatal("partial success advanced")
			}
		})
	}
}
