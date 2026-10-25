package main
import("os";"path/filepath";"testing";"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot")
func TestIndependentSixRejectsRehashedNewPolicyExpansion(t *testing.T){
 for _,group:=range []string{"business-races","late-hold","mixed"}{for _,role:=range []string{"SOURCE","RESTORE"}{t.Run(group+"/"+role,func(t *testing.T){
 base,fresh,expectation,inventory,mapping:=sixMock(t,group,role)
 if _,err:=checkSix(base,fresh,group,expectation,inventory,mapping);err!=nil{t.Fatal("positive setup",err)}
 name:=pilot.RemainingPolicyName(group,"delete");path:=filepath.Join(fresh,"native-policy-"+name+".json")
 mutateJSON(t,path,func(d map[string]any){d["Statement"].([]any)[0].(map[string]any)["Resource"]=[]string{"arn:aws:s3:::*"}})
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 var receipt postV22Receipt
 if _,err=strictPostFile(filepath.Join(fresh,"six-native-verification.json"),&receipt,15);err!=nil{t.Fatal(err)}
 receipt.Verification.Policies[name]=digest(raw)
 putPostJSON(t,filepath.Join(fresh,"six-native-verification.json"),receipt)
 if _,err=checkSix(base,fresh,group,expectation,inventory,mapping);err==nil{t.Fatal("rehashing expanded policy waived official policy comparison")}
 })}}
}
