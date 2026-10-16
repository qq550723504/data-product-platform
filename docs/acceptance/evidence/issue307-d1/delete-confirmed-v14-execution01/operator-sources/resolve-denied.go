// Finite server-owned resolution of the one CONFIRMED/DENIED invocation.
// No deleter or policy mutation: fresh positive exact HEAD, then the existing
// fenced ReconcileDeniedAttempt command appends a release; never an absence.
package main
import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "time"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
 lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)
func run()error{
 var c struct{PostgresDSN string;AttemptID,WorkID uuid.UUID};b,e:=os.ReadFile("/run/private/resolution.json");if e!=nil||json.Unmarshal(b,&c)!=nil||c.AttemptID.String()!="c163dca9-4650-4efe-bdef-e3380bdc9da0"||c.WorkID.String()!="144b5fde-01f3-48a1-ba86-a41c79ebe933"{return lc.ErrStorageScope}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Minute);defer cancel();pool,e:=pgxpool.New(ctx,c.PostgresDSN);if e!=nil{return e};defer pool.Close()
 router,e:=routing.NewRouter(false);if e!=nil{return e};outbox.ConfigureAppendObligation(router);defer outbox.ConfigureAppendObligation(nil)
 execution,inspector,scope,e:=pilot.BuildRecoveryInspectorAt("/run/private/recovery.json",pool);if e!=nil||scope.WorkspaceID!=pilot.ReferenceWorkspace||scope.ProviderInstanceID!=pilot.ReferenceProvider||scope.IncarnationID!=pilot.ReferenceIncarnation{return lc.ErrStorageScope}
 copy:=pilot.ReferenceTargets()[0];for _,v:=range pilot.ReferenceTargets(){if v.Namespace=="dpp-lifecycle307-online"{copy=v}}
 var good bool;e=pool.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM lifecycle_provider_attempt a JOIN lifecycle_provider_observation o ON o.attempt_id=a.id JOIN lifecycle_reconciliation_work w ON w.attempt_id=a.id WHERE a.id=$1 AND a.plan_id=$2 AND a.copy_id=$3 AND w.id=$4 AND o.certainty='CONFIRMED' AND o.outcome='DENIED')`,c.AttemptID,pilot.ReferencePlan,copy.ID,c.WorkID).Scan(&good);if e!=nil||!good{return lc.ErrStorageScope}
 presence,e:=inspector.InspectExactCopy(ctx,copy);if e!=nil{return e};result,e:=execution.ReconcileDeniedAttempt(ctx,lc.CommandHeader{WorkspaceID:scope.WorkspaceID,IdempotencyKey:"614cd9ad:confirmed-denied-present-resolution"},c.AttemptID,presence);if e!=nil{return e}
 f,e:=os.OpenFile("/run/output/denied-present-resolution.json",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};defer f.Close();return json.NewEncoder(f).Encode(map[string]any{"status":"CONFIRMED_DENIED_EXACT_VERSION_PRESENT_RELEASED","attempt":c.AttemptID,"work":c.WorkID,"presence":presence,"result":result,"absence_claimed":false,"delete":0,"resend":"FORBIDDEN_UNDER_THIS_CONFIRMATION"})
}
func main(){if e:=run();e!=nil{fmt.Fprintln(os.Stderr,e);os.Exit(1)}}
