package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "os"
 "time"
 madmin "github.com/minio/madmin-go/v3"
 "github.com/minio/minio-go/v7"
)
const user="refdelete614cd9ad"
const policyName="lifecycle307-reference-exact-614cd9ad"
type config struct { RootAccess,RootSecret,DeleteSecret,Mode string; Users map[string]string }
type ctxKey struct{}
type call struct { Used bool }
type transport struct { Base http.RoundTripper; Log *os.File }
func(t transport)RoundTrip(r *http.Request)(*http.Response,error){
 c,ok:=r.Context().Value(ctxKey{}).(*call)
 allowed:=map[string]bool{"GET /minio/admin/v3/user-info":true,"GET /minio/admin/v3/info-canned-policy":true,"PUT /minio/admin/v3/add-canned-policy":true,"PUT /minio/admin/v3/add-user":true,"PUT /minio/admin/v3/set-user-status":true,"POST /minio/admin/v3/idp/builtin/policy/attach":true}
 if !ok||c.Used||r.URL.Scheme!="http"||r.URL.Host!="127.0.0.1:9000"||!allowed[r.Method+" "+r.URL.Path]{return nil,errors.New("admin-only transport refused")};c.Used=true
 e:=json.NewEncoder(t.Log);at:=time.Now().UTC();if e.Encode(map[string]any{"fact":"START","at":at,"method":r.Method,"path":r.URL.Path})!=nil||t.Log.Sync()!=nil{return nil,errors.New("admin receipt unavailable")}
 response,err:=t.Base.RoundTrip(r);status:=0;if response!=nil{status=response.StatusCode}
 if e.Encode(map[string]any{"fact":"OBSERVATION","start":at,"at":time.Now().UTC(),"http_status":status,"unknown":err!=nil})!=nil||t.Log.Sync()!=nil{if response!=nil{response.Body.Close()};return nil,errors.New("admin receipt persistence failed")};return response,err
}
func run()error{
 var c config;b,e:=os.ReadFile("/run/private/admin.json");if e!=nil||json.Unmarshal(b,&c)!=nil||c.RootAccess==""||c.RootSecret==""||len(c.Users)!=3{return errors.New("admin configuration refused")}
 policy,e:=os.ReadFile("/run/private/delete-executor-policy.json");sum:=sha256.Sum256(policy);if e!=nil||hex.EncodeToString(sum[:])!="ab24ab862bf0e21d23ee21989f4140b3c7200cdecee22d4a708cc45621da8b54"{return errors.New("frozen exact policy refused")}
 log,e:=os.OpenFile("/run/output/admin-calls.jsonl",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{return e};defer log.Close()
 admin,e:=madmin.New("127.0.0.1:9000",c.RootAccess,c.RootSecret,false);if e!=nil{return e};minio.MaxRetry=1
 admin.SetCustomTransport(transport{&http.Transport{Proxy:nil,DisableKeepAlives:true,ResponseHeaderTimeout:10*time.Second},log})
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel();callCtx:=func()context.Context{return context.WithValue(ctx,ctxKey{},&call{})}
 facts:=[]any{};probe:=func(role,key string,want madmin.AccountStatus)error{info,e:=admin.GetUserInfo(callCtx(),key);if e!=nil||info.Status!=want{return fmt.Errorf("account state mismatch: %s",role)};facts=append(facts,map[string]any{"role":role,"status":info.Status,"policy":info.PolicyName});return nil}
 for _,role:=range []string{"reader","producer","inspector"}{want:=madmin.AccountDisabled;if role=="inspector"{want=madmin.AccountEnabled};if e=probe(role,c.Users[role],want);e!=nil{return e}}
 switch c.Mode{
 case "provision":
  if _,e=admin.GetUserInfo(callCtx(),user);madmin.ToErrorResponse(e).Code!="XMinioAdminNoSuchUser"{return errors.New("new exact executor already exists or not authoritatively absent")}
  if e=admin.AddCannedPolicy(callCtx(),policyName,policy);e!=nil{return fmt.Errorf("exact native policy rejected: %s",madmin.ToErrorResponse(e).Code)}
  if e=admin.SetUser(callCtx(),user,c.DeleteSecret,madmin.AccountDisabled);e!=nil{return errors.New("executor create unknown; no retry")}
  if _,e=admin.AttachPolicy(callCtx(),madmin.PolicyAssociationReq{Policies:[]string{policyName},User:user});e!=nil{return errors.New("exact policy attachment unknown; no retry")}
  if e=probe("executor",user,madmin.AccountDisabled);e!=nil{return e}
 case "enable","disable":
  status:=madmin.AccountDisabled;if c.Mode=="enable"{status=madmin.AccountEnabled};if e=admin.SetUserStatus(callCtx(),user,status);e!=nil{return errors.New("executor status transition unknown; no retry")};if e=probe("executor",user,status);e!=nil{return e}
 default:return errors.New("unsupported bounded administrative mode")
 }
 info,e:=admin.InfoCannedPolicyV2(callCtx(),policyName);if e!=nil{return errors.New("native policy readback unavailable")};var actual,expected any;if json.Unmarshal(info.Policy,&actual)!=nil||json.Unmarshal(policy,&expected)!=nil{return errors.New("native policy invalid")};a,_:=json.Marshal(actual);b,_=json.Marshal(expected);if string(a)!=string(b){return errors.New("native policy differs from authorized exact policy")}
 f,e:=os.OpenFile("/run/output/admin-result.json",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};defer f.Close();return json.NewEncoder(f).Encode(map[string]any{"status":"PASS_NATIVE_EXACT_IAM","mode":c.Mode,"facts":facts,"policy_sha256":hex.EncodeToString(sum[:]),"policy_semantic_match":true,"provider_object_delete":0,"root_payload_mount":false})
}
func main(){if e:=run();e!=nil{fmt.Fprintln(os.Stderr,e);os.Exit(1)}}
