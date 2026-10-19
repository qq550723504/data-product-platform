package main

import (
	"github.com/google/uuid"
	"net/http"
	"strings"
	"testing"
	"time"
)

func validReadOnlyPermission(at time.Time) p1ReadOnlyPermission {
	return p1ReadOnlyPermission{Enabled: true, AuditID: uuid.New(), Group: "business-races", Purpose: "POST_V22_P1_READONLY_RECONCILIATION",
		SourceCandidateSHA256: "f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243", BinarySHA256: strings.Repeat("a", 64),
		Core: "9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0", Native: "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce",
		CopyID: uuid.MustParse("5a4c9789-3e67-4e54-ab58-128d111cbd65"), AttemptID: uuid.MustParse("009f4317-8855-4f99-80ec-cafc225f654e"),
		NotBefore: at.Add(-time.Second), ExpiresAt: at.Add(30 * time.Minute)}
}
func TestP1ReadOnlyPermissionRejectsScopeAndTimeExpansion(t *testing.T) {
	at := time.Now().UTC()
	valid := validReadOnlyPermission(at)
	if err := validateReadOnlyPermission(valid, valid.SourceCandidateSHA256, valid.BinarySHA256, at); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*p1ReadOnlyPermission){
		"disabled": func(p *p1ReadOnlyPermission) { p.Enabled = false }, "old-execution": func(p *p1ReadOnlyPermission) { p.AuditID = uuid.MustParse(execution) },
		"deletes": func(p *p1ReadOnlyPermission) { p.MaxProviderDeletes = 1 }, "purpose": func(p *p1ReadOnlyPermission) { p.Purpose = "DELETE" },
		"group": func(p *p1ReadOnlyPermission) { p.Group = "mixed" }, "source": func(p *p1ReadOnlyPermission) { p.SourceCandidateSHA256 = strings.Repeat("b", 64) },
		"binary": func(p *p1ReadOnlyPermission) { p.BinarySHA256 = strings.Repeat("b", 64) }, "Core": func(p *p1ReadOnlyPermission) { p.Core = strings.Repeat("b", 64) },
		"native": func(p *p1ReadOnlyPermission) { p.Native = strings.Repeat("b", 64) }, "copy": func(p *p1ReadOnlyPermission) { p.CopyID = uuid.New() },
		"attempt": func(p *p1ReadOnlyPermission) { p.AttemptID = uuid.New() }, "future": func(p *p1ReadOnlyPermission) { p.NotBefore = at.Add(time.Second) },
		"expired": func(p *p1ReadOnlyPermission) { p.ExpiresAt = at }, "too-long": func(p *p1ReadOnlyPermission) { p.ExpiresAt = p.NotBefore.Add(time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			p := valid
			mutate(&p)
			if validateReadOnlyPermission(p, valid.SourceCandidateSHA256, valid.BinarySHA256, at) == nil {
				t.Fatal("expanded read-only permission admitted")
			}
		})
	}
}
func TestReadOnlyBootstrapCannotEnableDeleteOrBusinessUsers(t *testing.T) {
	for _, user := range []string{"delete808a6960", "remainread808a6960", "remainput808a6960", "foreign", "inspect808a69606c36"} {
		r, _ := http.NewRequest("PUT", "http://127.0.0.1:9000/minio/admin/v3/set-user-status?accessKey="+user+"&status=enabled", nil)
		// Even inspector cannot pass without the separate fresh host permission.
		if _, err := (transport{shouldNotCall{t}, map[string]string{user: "policy"}, true, true}).RoundTrip(r); err == nil {
			t.Fatal("unconfirmed or non-inspector enable admitted", user)
		}
	}
}
