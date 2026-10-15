import datetime as dt,hashlib,importlib.util,json,os,shutil,subprocess
from pathlib import Path
ROOT=Path('C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
P=Path('C:/Users/Henry/.codex/fixture-private/d1-v14-gateway-offline01');OUT=ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v14-execution01/gateway-offline';P.mkdir();OUT.mkdir()
def call(*args):return subprocess.run(list(map(str,args)),capture_output=True,check=True,cwd=ROOT).stdout
def write(p,v):
 with p.open('x',encoding='utf-8') as f:json.dump(v,f,indent=2);f.write('\n')
raw=Path('C:/Users/Henry/.codex/fixture-private/d1-lc13-continuation01/delete-candidate04/output/gateway-active.Caddyfile').read_bytes();assert hashlib.sha256(raw).hexdigest()=='7b24ad32506bfd189c9289ba85a65ab242069dfee293d61b330104a90306cd0a'
(P/'Caddyfile').write_bytes(raw.replace(b'dpp-lifecycle307-5d2274ee-minio:9000',b'127.0.0.1:9901'))
go=r'''package main
import("context";"encoding/json";"net";"net/http";"os";"sync";"time")
func main(){var mu sync.Mutex;upstream:=[]string{};server:=&http.Server{Addr:"127.0.0.1:9901",Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){mu.Lock();upstream=append(upstream,r.Method+" "+r.URL.RequestURI());mu.Unlock();w.WriteHeader(204)})};go server.ListenAndServe();defer server.Shutdown(context.Background())
for n:=0;n<100;n++{c,e:=net.DialTimeout("tcp","127.0.0.1:9000",time.Second);if e==nil{c.Close();break};time.Sleep(50*time.Millisecond)}
paths:=[]string{"/dpp-lifecycle307-online/fixture/a.csv?versionId=312365a9-f984-43de-b9d5-7e26009ccd82","/dpp-lifecycle307-quarantine/fixture/b.csv?versionId=846d0c7c-289d-4667-bc7c-aa04e44357d4","/dpp-lifecycle307-online/fixture/a.csv?versionId=EXCLUDED_VERSION"};facts:=[]any{};client:=&http.Client{Timeout:5*time.Second,Transport:&http.Transport{Proxy:nil,DisableKeepAlives:true}}
for _,path:=range paths{r,_:=http.NewRequest("DELETE","http://127.0.0.1:9000"+path,nil);r.Header.Set("Authorization","AWS4-HMAC-SHA256 Credential=refdelete614cd9ad/20261015/us-east-1/s3/aws4_request, SignedHeaders=host, Signature=FAKE_OFFLINE_ONLY");response,e:=client.Do(r);status:=0;if response!=nil{status=response.StatusCode;response.Body.Close()};facts=append(facts,map[string]any{"path":path,"status":status,"response_lost":e!=nil})};mu.Lock();defer mu.Unlock();json.NewEncoder(os.Stdout).Encode(map[string]any{"schema":"SYNTHETIC_GATEWAY_MATCH_ONLY_NOT_NATIVE_DELETE","upstream":upstream,"requests":facts,"real_provider_io":0,"credentials":"FAKE_OFFLINE_ONLY","network":"none; loopback-only responder"})}
'''
(P/'probe.go').write_text(go,encoding='utf-8');env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64');subprocess.run(['go','build','-trimpath','-o',str(P/'probe'),str(P/'probe.go')],capture_output=True,check=True,env=env,cwd=ROOT)
gateway='dpp-d1-v14-gateway-offline01';client='dpp-d1-v14-gateway-probe01';created=[]
try:
 ident=call('docker','create','--pull','never','--name',gateway,'--label','lifecycle307.confirmation-diagnostic=614cd9ad','--network','none','--read-only','--tmpfs','/config','--tmpfs','/data','--mount',f'type=bind,source={P},target=/etc/caddy,readonly','sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f').decode().strip();created.append(ident);call('docker','start',ident)
 ci=call('docker','create','--pull','never','--name',client,'--label','lifecycle307.confirmation-diagnostic=614cd9ad','--network','container:'+ident,'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={P},target=/run/private,readonly','--entrypoint','/run/private/probe','sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea').decode().strip();created.append(ci);call('docker','start',ci);assert call('docker','wait',ci).decode().strip()=='0'
 result=json.loads(call('docker','logs',ci));result.update({'at':dt.datetime.now(dt.timezone.utc).isoformat(),'candidate_gateway_sha256':hashlib.sha256(raw).hexdigest(),'offline_gateway_sha256':hashlib.sha256((P/'Caddyfile').read_bytes()).hexdigest(),'probe_source_sha256':hashlib.sha256((P/'probe.go').read_bytes()).hexdigest(),'probe_binary_sha256':hashlib.sha256((P/'probe').read_bytes()).hexdigest(),'only_configuration_change':'upstream hostname replaced by loopback-only synthetic responder','cleanup':0});write(OUT/'route-diagnostic.json',result);print(json.dumps(result))
finally:
 states=[]
 for ident in reversed(created):
  item=json.loads(call('docker','inspect',ident))[0]
  if item['State']['Running']:call('docker','stop',ident)
  item=json.loads(call('docker','inspect',ident))[0];states.append({'id':ident,'name':item['Name'],'status':item['State']['Status'],'pid':item['State']['Pid'],'network':item['HostConfig']['NetworkMode']})
 write(OUT/'retained-offline-helpers.json',{'states':states,'source_environment_starts':0,'real_provider_io':0,'delete':0,'cleanup':0})
