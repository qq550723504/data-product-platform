package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	"github.com/qq550723504/data-product-platform/apps/platform/tests/acceptancefixture"
	"os"
	"strings"
	"testing"
	"time"
)

func basisFixture(t *testing.T) (*pgxpool.Pool, AcceptanceIdentityReader, q.BasisQuery) {
	t.Helper()
	dsn := os.Getenv("ACCEPTANCE_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("dedicated prepared 1A basis fixture not configured")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	if db.Config().ConnConfig.Database != "acceptance_1a_20261024" {
		t.Fatal("exact independent basis fixture required")
	}
	ws := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	var issuer, subject string
	if e = db.QueryRow(ctx, `SELECT issuer,subject FROM platform_subject_binding WHERE actor_id='73000000-0000-4000-8000-000000000001' AND enabled`).Scan(&issuer, &subject); e != nil {
		t.Fatal(e)
	}
	store := bindings.NewStore(db)
	b, e := store.Lookup(ctx, oidc.Identity{Issuer: issuer, Subject: subject}, ws)
	if e != nil || !b.Active {
		t.Fatal("active fixture identity required")
	}
	p := principal.RequestPrincipal{ActorID: b.ActorID, Subject: b.Subject, WorkspaceID: ws, Capabilities: b.Capabilities, IdentityIssuer: issuer, IdentitySubject: subject, IdentityRevision: b.AuthorityRevision, IdentityExpiresAt: time.Now().Add(time.Minute)}
	delivery, version, standard := acceptancefixture.ID(1002), acceptancefixture.ID(1001), acceptancefixture.ID(1004)
	return db, AcceptanceIdentityReader{Repository: NewPostgresRepository(db), Authority: store}, q.BasisQuery{Query: q.Query{WorkspaceID: ws, Principal: &p, ActorID: p.ActorID, Subject: p.Subject, DeliveryID: &delivery, DatasetVersionID: &version, VersionID: &standard, Limit: 25}}
}
func TestBusinessBasisReadScopeIntegrityAndNoTrustPromotion(t *testing.T) {
	db, reader, query := basisFixture(t)
	ctx := context.Background()
	var before, after string
	fingerprint := `SELECT md5(string_agg(row_to_json(t)::text,'|' ORDER BY id)) FROM evidence t`
	if e := db.QueryRow(ctx, fingerprint).Scan(&before); e != nil {
		t.Fatal(e)
	}
	b, e := reader.ReadBusinessBasis(ctx, query)
	if e != nil {
		t.Fatal(e)
	}
	if b.WriteEnabled || b.Identity.DirectorySource != "UNCONFIGURED" || b.ReceiptSource != "UNCONFIGURED" || b.Context.Applicability.Status != "MATCHED" || b.Context.Delivery.Status != "BLOCKED" || len(b.Evidence) != 13 || b.Materials.Page.Total != 28 || len(b.Materials.Items) != 25 {
		t.Fatal("wrong scoped basis projection")
	}
	for _, v := range b.Evidence {
		if v.Origin != "SYNTHETIC_FIXTURE" || v.Evidence.EnvelopeStatus != "VALID" {
			t.Fatal("wrong binding evidence provenance")
		}
	}
	query.MaterialOffset = 25
	b, e = reader.ReadBusinessBasis(ctx, query)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Materials.Items) != 3 || b.Materials.Page.Total != 28 {
		t.Fatal("page/count scope lost")
	}
	query.MaterialOffset = 1000
	b, e = reader.ReadBusinessBasis(ctx, query)
	if e != nil || len(b.Materials.Items) != 0 || b.Materials.Page.Total != 28 {
		t.Fatal("empty page replaced with latest")
	}
	query.MaterialOffset = 0
	query.VersionID = nil
	for n, status := range map[int]string{1100: "VALID", 1101: "VALID", 1102: "INVALID", 1103: "NOT_CHECKED"} {
		id := acceptancefixture.ID(n)
		query.MaterialID = &id
		d, e := reader.ReadReceiptMaterial(ctx, query)
		if e != nil {
			t.Fatal(e)
		}
		if d.Material.Evidence.EnvelopeStatus != status || d.Material.BusinessUse != "BLOCKED" || d.Material.ContentStatus != "UNATTESTED" || d.Material.RecipientStatus != "UNATTESTED" {
			t.Fatalf("evidence claim promoted: %d", n)
		}
		data, _ := json.Marshal(d)
		for _, secret := range []string{"PRIVATE_", "storageUri", "metadata", "sourceType", "sourceId", "\"title\"", "\"verified\":true"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("unsafe projection: %s", secret)
			}
		}
	}
	for _, n := range []int{1104, 1105, 2100} {
		id := acceptancefixture.ID(n)
		query.MaterialID = &id
		if _, e = reader.ReadReceiptMaterial(ctx, query); !errors.Is(e, q.ErrNotFound) {
			t.Fatalf("unscoped material visible %d: %v", n, e)
		}
	}
	query.MaterialID = nil
	query.Query.ActorID = uuid.New()
	query.Subject = "forged-caller"
	b, e = reader.ReadBusinessBasis(ctx, query)
	if e != nil || b.Identity.PrincipalID != query.Principal.ActorID {
		t.Fatal("query actor became authority")
	}
	query.WorkspaceID = uuid.New()
	if _, e = reader.ReadBusinessBasis(ctx, query); !errors.Is(e, principal.ErrUnauthenticated) {
		t.Fatal("query workspace mismatch admitted")
	}
	if e = db.QueryRow(ctx, fingerprint).Scan(&after); e != nil || after != before {
		t.Fatal("basis read changed evidence")
	}
}
func TestBusinessBasisWaitsForRevocationAndRejectsOldAuthority(t *testing.T) {
	db, reader, query := basisFixture(t)
	ctx := context.Background()
	actor := query.Principal.ActorID
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `UPDATE platform_actor_state SET enabled=true,revision=revision+1 WHERE actor_id=$1 AND NOT enabled`, actor)
	})
	writer, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Rollback(ctx)
	if _, e = writer.Exec(ctx, `UPDATE platform_actor_state SET enabled=false,revision=revision+1 WHERE actor_id=$1`, actor); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := reader.ReadBusinessBasis(ctx, query); done <- e }()
	waited := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=810051001 AND NOT granted)`).Scan(&waited); e != nil {
			t.Fatal(e)
		}
		if waited {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waited {
		t.Fatal("no actual IAM fence wait observed")
	}
	if e = writer.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if !errors.Is(e, principal.ErrWorkspaceDenied) {
			t.Fatalf("revoked basis authority admitted: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("basis reader did not finish")
	}
}
