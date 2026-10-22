// Finite read-only verification composition. Uses the frozen platform inspector,
// current recovery authority and its native physical-call/cost journal.
package main
import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "net/http"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
 "encoding/json"
 "fmt"
 "os"
 "time"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
 lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)
type finiteAuthority struct{ original pilot.RecoveryConfig }
func(a finiteAuthority)AuthorizeLifecycle(_ context.Context,_ pgx.Tx,w uuid.UUID,cap string,at time.Time)(principal.RequestPrincipal,error){
 var c pilot.RecoveryConfig;b,e:=os.ReadFile("/run/private/recovery.json");if e!=nil||json.Unmarshal(b,&c)!=nil||c!=a.original||!c.Enabled||!at.Before(c.ValidUntil)||w!=pilot.ReferenceWorkspace||(cap!=lc.CapabilityInspectStorage&&cap!=lc.CapabilityCheckpointStorage){return principal.RequestPrincipal{},principal.ErrCapabilityDenied}
 resolver,e:=principal.NewStaticResolver(true,c.Token,"candidate05-survivor-checkpoint:"+c.RunID.String(),c.ActorID.String(),[]string{c.WorkspaceID.String()},[]string{lc.CapabilityInspectStorage,lc.CapabilityCheckpointStorage});if e!=nil{return principal.RequestPrincipal{},e};r,_:=http.NewRequest(http.MethodPost,"http://fixture.invalid/checkpoint",nil);r.Header.Set("Authorization","Bearer "+c.Token);return resolver.Resolve(r,w,cap)
}
func run()error{
 var c struct{PostgresDSN string};b,e:=os.ReadFile("/run/private/sql.json");if e!=nil||json.Unmarshal(b,&c)!=nil{return fmt.Errorf("SQL config unavailable")}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Minute);defer cancel();pool,e:=pgxpool.New(ctx,c.PostgresDSN);if e!=nil{return e};defer pool.Close()
 router,e:=routing.NewRouter(false);if e!=nil{return e};outbox.ConfigureAppendObligation(router);defer outbox.ConfigureAppendObligation(nil)
 var scope pilot.RecoveryConfig;b,e=os.ReadFile("/run/private/recovery.json");if e!=nil||json.Unmarshal(b,&scope)!=nil||!scope.Enabled||!scope.SyntheticOnly||scope.Endpoint!="127.0.0.1:9000"||!time.Now().Before(scope.ValidUntil){return lc.ErrStorageScope}
 o:=lc.NewOccupancyService(transaction.NewManager(pool),&infrastructure.PostgresRepository{},finiteAuthority{scope},nil,nil);journal:=lc.NewPostgresStorageCallJournal(o)
 inspector,e:=storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint:scope.Endpoint,AccessKey:scope.InspectAccess,SecretKey:scope.InspectSecret,WorkspaceID:scope.WorkspaceID,ProviderInstanceID:scope.ProviderInstanceID,IncarnationID:scope.IncarnationID,Namespaces:[]string{"dpp-lifecycle307-online","dpp-lifecycle307-quarantine"},Journal:journal});if e!=nil{return e}
 if scope.WorkspaceID!=pilot.ReferenceWorkspace||scope.ProviderInstanceID!=pilot.ReferenceProvider||scope.IncarnationID!=pilot.ReferenceIncarnation||scope.RunID.String()!=pilot.ReferenceExecution{return lc.ErrStorageScope}
 var receipts,completion int;if e=pool.QueryRow(ctx,`SELECT (SELECT count(*) FROM lifecycle_copy_absence_receipt WHERE plan_id=$1),(SELECT count(*) FROM lifecycle_plan_completion WHERE plan_id=$1)`,pilot.ReferencePlan).Scan(&receipts,&completion);e!=nil||receipts!=2||completion!=1{return fmt.Errorf("exact-scope completion unavailable")}
 var before struct{Versions []struct{Copy struct{ID string};Protection lc.CopyProtection `json:"protection"`}};b,e=os.ReadFile("/run/private/preflight.json");if e!=nil||json.Unmarshal(b,&before)!=nil||len(before.Versions)!=6{return fmt.Errorf("six-version preflight unavailable")}
 targets:=map[string]bool{};for _,c:=range pilot.ReferenceTargets(){targets[c.ID.String()]=true}
 allowed:=[]domain.Copy{};for _,copy:=range pilot.ReferenceCopies(){if !targets[copy.ID.String()]{allowed=append(allowed,copy)}};if len(allowed)!=4{return lc.ErrStorageScope}
 reader,e:=storage.NewLifecycleCheckpointReader(storage.LifecycleCopyConfig{Endpoint:scope.Endpoint,AccessKey:scope.InspectAccess,SecretKey:scope.InspectSecret,WorkspaceID:scope.WorkspaceID,ProviderInstanceID:scope.ProviderInstanceID,IncarnationID:scope.IncarnationID,Namespaces:[]string{"dpp-lifecycle307-online","dpp-lifecycle307-quarantine"},Journal:journal},allowed);if e!=nil{return e}
 inventories:=map[string]lc.NamespaceInventory{};objects:=[]any{}
 for _,copy:=range pilot.ReferenceCopies(){
  if targets[copy.ID.String()]{continue}
  presence,e:=inspector.InspectExactCopy(ctx,copy);if e!=nil{return e};protection,e:=inspector.InspectRetentionAndHold(ctx,copy);if e!=nil{return e}
  matched:=false;for _,v:=range before.Versions{if v.Copy.ID==copy.ID.String(){matched=protection.Copy==v.Protection.Copy&&protection.LegalHold==v.Protection.LegalHold&&protection.RetentionMode==v.Protection.RetentionMode&&((protection.RetainUntil==nil&&v.Protection.RetainUntil==nil)||(protection.RetainUntil!=nil&&v.Protection.RetainUntil!=nil&&protection.RetainUntil.Equal(*v.Protection.RetainUntil)))}}
  if !matched{return fmt.Errorf("survivor identity/protection changed")};data,call,e:=reader.ReadExact(ctx,copy);if e!=nil{return e};sum:=sha256.Sum256(data);if int64(len(data))!=copy.ByteSize||hex.EncodeToString(sum[:])!=copy.ContentSHA256{return lc.ErrStorageContract};objects=append(objects,map[string]any{"copy":copy,"presence":presence,"protection":protection,"read_call_id":call,"bytes":len(data),"sha256":hex.EncodeToString(sum[:])})
  if _,ok:=inventories[copy.Namespace];!ok{inv,e:=inspector.InspectNamespace(ctx,copy);if e!=nil{return e};inventories[copy.Namespace]=inv}
 }
 for ns,inv:=range inventories{expected:=map[string]int64{};for _,c:=range pilot.ReferenceCopies(){if c.Namespace==ns&&!targets[c.ID.String()]{expected[c.Key+"\x00"+c.ProviderVersion]=c.ByteSize}};if len(inv.Versions)!=len(expected){return fmt.Errorf("survivor inventory cardinality changed")};for _,v:=range inv.Versions{size,ok:=expected[v.Key+"\x00"+v.Version];if !ok||v.DeleteMarker||v.ByteSize!=size{return fmt.Errorf("survivor inventory identity changed")};delete(expected,v.Key+"\x00"+v.Version)};if len(expected)!=0{return lc.ErrStorageContract}}
 f,e:=os.OpenFile("/run/output/four-survivors.json",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};defer f.Close();return json.NewEncoder(f).Encode(map[string]any{"status":"PASS_FOUR_EXACT_SURVIVORS","objects":objects,"inventories":inventories,"absence_receipts":receipts,"completion":completion,"native_delete":0,"cleanup":0,"journal":"existing NewPostgresStorageCallJournal; finite inspect/checkpoint authority"})
}
func main(){if e:=run();e!=nil{fmt.Fprintln(os.Stderr,e);os.Exit(1)}}
