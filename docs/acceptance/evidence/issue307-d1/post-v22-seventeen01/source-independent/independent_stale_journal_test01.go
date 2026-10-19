package main
import("path/filepath";"testing")
func TestIndependentNonP1StaleJournalMustBeRejected(t *testing.T){
 base,fresh,expectation,inventory:=postMock(t,"late-hold");starts,_:=filepath.Glob(filepath.Join(fresh,"readonly-start-*.json"));if len(starts)==0{t.Fatal("missing source journal")}
 mutateJSON(t,starts[0],func(d map[string]any){d["at"]="1970-01-01T00:00:00Z"})
 out,e:=checkPostV22(base,fresh,"late-hold",expectation,inventory);if e==nil{t.Fatalf("stale non-P1 source journal accepted under fresh envelope: %+v",out)}
}
