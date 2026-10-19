package main
import("context";"io";"net/http";"strings";"testing")
func TestIndependentGeneric404MustBeRejected(t *testing.T){
 p,full,present:=postProfile(t,"business-races");_,_,cp,e:=postV22Inventories(p,full,present);if e!=nil{t.Fatal(e)}
 c:=config{Endpoint:p.Endpoint,RootAccess:"synthetic",RootSecret:"synthetic",OutputDirectory:t.TempDir()}
 tr:=&postV22VerifyTransport{c:c,p1:cp,base:postRoundTripper(func(r *http.Request)(*http.Response,error){return &http.Response{StatusCode:404,Header:make(http.Header),Body:io.NopCloser(strings.NewReader("")),Request:r},nil})}
 got,e:=postV22Absence(context.Background(),c,cp,tr);if e==nil{t.Fatalf("ordinary headerless/bodyless HEAD404 was promoted to official P1 absence: %+v",got)}
}
