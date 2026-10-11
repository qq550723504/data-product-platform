// Controlled synthetic fixture preparation only. Extraction, review, execution,
// Quality, certification and delivery are performed by the real browser/API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	certapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const purpose = "ENTERPRISE_CREDIT_RISK_SUPPORT"

type config struct {
	DSN, Private, Output, Migrations, RootKey, RootSecret, Repo string
	Profile                                                     pilot.Profile
}
type facts struct{ Workspace, Actor, Workflow, Authorization, Profile, Rights, Contract uuid.UUID }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string, v any) { b, e := os.ReadFile(path); must(e); must(json.Unmarshal(b, v)) }
func write(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, b, 0600))
}
func main() {
	if len(os.Args) != 2 || os.Getenv("TEST_INGESTION_B3_CONFIG") == "" {
		panic("isolated B3 fixture config and command required")
	}
	var c config
	read(os.Getenv("TEST_INGESTION_B3_CONFIG"), &c)
	if !strings.HasPrefix(filepath.Base(c.Private), "dpp-pgi-b3-") {
		panic("isolated private directory required")
	}
	uri, err := url.Parse(c.DSN)
	if err != nil || uri.Hostname() != "127.0.0.1" || uri.Path != "/dpp_ingestion_core" {
		panic("isolated loopback dpp_ingestion_core database required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, c.DSN)
	must(e)
	defer pool.Close()
	router, e := routing.NewRouter(false)
	must(e)
	outbox.ConfigureAppendObligation(router)
	tx := transaction.NewManager(pool)
	var f facts
	path := filepath.Join(c.Private, "facts.json")
	if os.Args[1] == "bootstrap" {
		var n int
		must(pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name='data_resource'`).Scan(&n))
		if n != 0 {
			panic("bootstrap requires fresh Core")
		}
		must(migration.NewRunner(pool, c.Migrations).Up(ctx))
		f.Workspace = uuid.New()
		f.Actor = uuid.New()
		p := c.Profile
		p.ActorID = f.Actor.String()
		p.Workspaces = []string{f.Workspace.String()}
		p.Capabilities = []string{"INGEST_RAW"}
		root, e := storage.New(p.StorageEndpoint, c.RootKey, c.RootSecret, p.Bucket, false)
		must(e)
		must(root.EnsureBucket(ctx))
		allow := func(prefix string, actions ...string) any {
			return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
		}
		stage := "staging/" + f.Workspace.String() + "/"
		core := "core/" + f.Workspace.String() + "/"
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow(stage, "s3:GetObject"), allow(core, "s3:GetObject", "s3:PutObject")}})
		creds, e := credentials.NewSTSAssumeRole("http://"+p.StorageEndpoint, credentials.STSAssumeRoleOptions{AccessKey: c.RootKey, SecretKey: c.RootSecret, Policy: string(policy)})
		must(e)
		cv, e := creds.Get()
		must(e)
		write(p.CoreCredentialFile, pilot.Identity{AccessKey: cv.AccessKeyID, SecretKey: cv.SecretAccessKey, SessionToken: cv.SessionToken})
		admin, e := madmin.New(p.StorageEndpoint, c.RootKey, c.RootSecret, false)
		must(e)
		publisher := pilot.Identity{AccessKey: "publisher-" + uuid.NewString(), SecretKey: uuid.NewString() + uuid.NewString()}
		policy, _ = json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow(stage, "s3:GetObject", "s3:PutObject")}})
		pn := "b3-stage-" + f.Workspace.String()
		must(admin.AddCannedPolicy(ctx, pn, policy))
		must(admin.SetUser(ctx, publisher.AccessKey, publisher.SecretKey, madmin.AccountEnabled))
		_, e = admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{pn}, User: publisher.AccessKey})
		must(e)
		write(p.PublisherCredentialFile, publisher)
		write(filepath.Join(c.Private, "profile.json"), p)
		definition, e := os.ReadFile(filepath.Join(c.Repo, "examples/enterprise-activity/workflow/workflow-v1.yaml"))
		must(e)
		workflow, e := workflowapp.NewWorkflowVersionService(tx, workflowinfra.NewPostgresRepository(pool)).Create(ctx, workflowapp.CreateWorkflowVersionCommand{WorkspaceID: f.Workspace, Code: "PGI-B3-PRODUCTION", Name: "既有企业活动加工", Version: "1.0.0", DefinitionRef: "examples/enterprise-activity/workflow/workflow-v1.yaml", DefinitionYAML: definition, ActorID: &f.Actor})
		must(e)
		f.Workflow = workflow.ID
		write(path, f)
		write(filepath.Join(c.Private, "public.json"), map[string]any{"workspaceId": f.Workspace, "actorId": f.Actor, "workflowVersionId": f.Workflow})
	} else {
		read(path, &f)
		rights := rightsapp.NewService(tx, rightsinfra.NewPostgresRepository(pool))
		switch os.Args[1] {
		case "renew-rights":
			original, e := rightsinfra.NewPostgresRepository(pool).GetAuthorization(ctx, f.Authorization)
			must(e)
			if original.WorkspaceID != f.Workspace || original.Status != rightsdomain.StatusRevoked {
				panic("only this fixture's revoked authorization may be replaced")
			}
			declarations := map[uuid.UUID]uuid.UUID{}
			grants := make([]rightsdomain.ResourceGrantSpec, 0, len(original.Resources))
			for _, g := range original.Resources {
				var declaration uuid.UUID
				must(pool.QueryRow(ctx, `SELECT rights_declaration_id FROM authorization_provenance_binding WHERE authorization_id=$1 AND data_resource_id=$2`, original.ID, g.DataResourceID).Scan(&declaration))
				declarations[g.DataResourceID] = declaration
				grants = append(grants, rightsdomain.ResourceGrantSpec{DataResourceID: g.DataResourceID, Actions: g.Actions, ScopeType: g.ScopeType, ScopeRef: g.ScopeRef})
			}
			from, to := time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour)
			replacement, e := rights.Create(ctx, rightsapp.CreateAuthorizationCommand{WorkspaceID: f.Workspace, Code: "PGI-B3-REPLACEMENT-" + uuid.NewString(), GrantorRef: original.GrantorRef, GranteeRef: original.GranteeRef, Purpose: original.Purpose, ValidFrom: &from, ValidTo: &to, Resources: grants, ActorID: &f.Actor})
			must(e)
			command := rightsapp.TransitionCommand{AuthorizationID: replacement.ID, ActorID: &f.Actor, At: time.Now().UTC()}
			_, e = rights.Submit(ctx, command)
			must(e)
			_, e = rights.Approve(ctx, command)
			must(e)
			_, e = rights.Activate(ctx, command)
			must(e)
			for _, g := range grants {
				declaration := declarations[g.DataResourceID]
				_, e = rights.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{WorkspaceID: f.Workspace, AuthorizationID: replacement.ID, DataResourceID: g.DataResourceID, DeclarationID: declaration, GrantorRef: replacement.GrantorRef, AuthorityMode: rightsdomain.AuthorityDirect, AsOf: time.Now().UTC(), ActorID: &f.Actor})
				must(e)
			}
			f.Authorization = replacement.ID
			write(path, f)
		case "rights":
			if f.Authorization != uuid.Nil {
				panic("rights fixture already prepared")
			}
			rows, e := pool.Query(ctx, `SELECT source_resource_id FROM dataset WHERE workspace_id=$1 AND dataset_type='RAW' ORDER BY code`, f.Workspace)
			must(e)
			grants := []rightsdomain.ResourceGrantSpec{}
			scopes := []certdomain.ScopeRef{}
			for rows.Next() {
				var id uuid.UUID
				must(rows.Scan(&id))
				grants = append(grants, rightsdomain.ResourceGrantSpec{DataResourceID: id, Actions: []string{"READ", "PROCESS", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, ScopeType: "ALL_RESOURCE", ScopeRef: id.String()})
				scopes = append(scopes, certdomain.ScopeRef{Type: "ALL_RESOURCE", Ref: id.String()})
			}
			must(rows.Err())
			rows.Close()
			if len(grants) != 3 {
				panic("browser must first register three RAW sources")
			}
			from, to := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(24*time.Hour)
			auth, e := rights.Create(ctx, rightsapp.CreateAuthorizationCommand{WorkspaceID: f.Workspace, Code: "PGI-B3-SYNTHETIC-AUTH", GrantorRef: "SYNTHETIC-PARK-OPERATOR", GranteeRef: "LICENSED_BANK", Purpose: purpose, ValidFrom: &from, ValidTo: &to, Resources: grants, ActorID: &f.Actor})
			must(e)
			cmd := rightsapp.TransitionCommand{AuthorizationID: auth.ID, ActorID: &f.Actor, At: time.Now().UTC()}
			_, e = rights.Submit(ctx, cmd)
			must(e)
			_, e = rights.Approve(ctx, cmd)
			must(e)
			_, e = rights.Activate(ctx, cmd)
			must(e)
			f.Authorization = auth.ID
			for _, g := range grants {
				permissions := []rightsdomain.RightsPermission{}
				for _, action := range g.Actions {
					permissions = append(permissions, rightsdomain.RightsPermission{Kind: rightsdomain.PermissionGrant, Action: action, Purpose: purpose, Scope: rightsdomain.NormalizedScope{Type: g.ScopeType, Ref: g.ScopeRef}})
				}
				dec, e := rights.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{Spec: rightsdomain.RightsDeclarationSpec{WorkspaceID: f.Workspace, DataResourceID: g.DataResourceID, ClaimantRef: auth.GrantorRef, BasisType: "LICENSE", BasisRef: "synthetic-b3-fixture", Parties: []rightsdomain.RightsParty{{PartyRef: auth.GrantorRef, Role: "RIGHTS_HOLDER"}}, Permissions: permissions, ActorID: &f.Actor}})
				must(e)
				_, e = rights.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{DeclarationID: dec.ID, Outcome: rightsdomain.DeclarationVerified, ActorID: &f.Actor})
				must(e)
				_, e = rights.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{WorkspaceID: f.Workspace, AuthorizationID: auth.ID, DataResourceID: g.DataResourceID, DeclarationID: dec.ID, GrantorRef: auth.GrantorRef, AuthorityMode: rightsdomain.AuthorityDirect, AsOf: time.Now().UTC(), ActorID: &f.Actor})
				must(e)
			}
			snap, e := rights.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{WorkspaceID: f.Workspace, Purpose: purpose, ConsumerRef: "LICENSED_BANK", AuthorizationIDs: []uuid.UUID{auth.ID}, ActorID: &f.Actor})
			must(e)
			f.Rights = snap.ID
			explicit := func(v string) certdomain.Applicability {
				return certdomain.Applicability{Mode: certdomain.ApplicabilityExplicit, Values: []string{v}}
			}
			profile, e := certapp.NewProfileService(tx, certinfra.NewProfileRepository(pool)).Create(ctx, certapp.CreateProfileCommand{WorkspaceID: f.Workspace, Profile: certdomain.CertificationProfile{ProfileRef: "park/postgres-b3-certified-v1", Code: "PGI-B3-CERTIFIED", Name: "PostgreSQL 企业活动认证", Version: "1.0.0", Purpose: explicit(purpose), Actions: explicit("READ"), Consumers: explicit("LICENSED_BANK"), Delivery: explicit("DIRECT_DATA"), RequiredCriticalRules: []string{"QA-COMPANY-ID-COMPLETE"}, QualityGateRequired: true, Rights: certdomain.RightsRequirement{Required: true, Purpose: explicit(purpose), Actions: explicit("READ"), Consumers: explicit("LICENSED_BANK"), Scopes: certdomain.ScopeApplicability{Mode: certdomain.ApplicabilityExplicit, Values: scopes}}, ComplianceRequired: true, ContractRequired: true, ContractCode: "DP-ENTERPRISE-ACTIVITY", TraceabilityRequired: true, EvidenceRequired: true}, ActorID: &f.Actor})
			must(e)
			f.Profile = profile.ID
			content, e := os.ReadFile(filepath.Join(c.Repo, "examples/enterprise-activity/contract/data-contract-v1.yaml"))
			must(e)
			contracts := contractapp.NewService(tx, contractinfra.NewPostgresRepository(pool))
			cv, e := contracts.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{WorkspaceID: f.Workspace, SourceRef: "examples/enterprise-activity/contract/data-contract-v1.yaml", DocumentYAML: content, ActorID: &f.Actor})
			must(e)
			cv, e = contracts.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: cv.ID, ActorID: &f.Actor})
			must(e)
			f.Contract = cv.ID
			write(path, f)
		default:
			panic("bootstrap|rights|renew-rights required")
		}
	}
	fmt.Println("B3 synthetic prerequisites prepared; browser business Commands remain independent")
}
