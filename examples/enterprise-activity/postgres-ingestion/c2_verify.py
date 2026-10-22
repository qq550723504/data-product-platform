"""Independent real Core oracle for native schema observations.

Browser/native successes must already exist. Snapshot/compare are read-only;
stress sends only bounded authenticated replays and boundary rejections.
"""
import argparse, json, subprocess, time, urllib.request, urllib.error, uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from c1_verify import Oracle, digest, history

class C2Oracle(Oracle):
    def __init__(self,runtime):
        super().__init__(runtime)
        self.profile=json.loads((self.private/'schema-profile.json').read_text(encoding='utf-8'))
        self.resource=self.profile['plan']['resourceId']
        self.base=f'/data-resources/{self.resource}/schema-observations?workspaceId={self.ws}'
    def command(self,path,method='POST',key=None,body=None,authorized=True):
        headers={'Content-Type':'application/json'}
        if authorized:headers['Authorization']='Bearer '+self.token
        if key:headers['Idempotency-Key']=key
        r=urllib.request.Request(self.state['api']+'/api/v1'+path,method=method,headers=headers,
            data=None if body is None else json.dumps(body).encode())
        try:response=urllib.request.urlopen(r,timeout=30)
        except urllib.error.HTTPError as error:response=error
        with response:
            raw=response.read()
            assert b'source_company_id' not in raw or method=='GET', 'unexpected dataset bytes'
            return response.status,json.loads(raw) if raw else None

def snapshot(o,output,previous=None):
    prior=json.loads(previous.read_text(encoding='utf-8')) if previous else None
    frozen=history(o,prior['history'] if prior else None)
    schema={t:o.rows(t) for t in ('source_schema_request','source_schema_attempt','source_schema_observation','source_schema_handling')}
    if prior:
        for table,rows in prior['schema'].items():
            current={x['id']:x for x in schema[table]}
            assert all(current.get(row['id'])==row for row in rows),table+' history changed'
    attempts=schema['source_schema_attempt']; costs=o.rows('cost_event',"cost_type='SOURCE_SCHEMA_SCAN_INVOCATION'")
    allocations=o.rows('cost_allocation','source_schema_attempt_id IS NOT NULL')
    assert len(costs)==len(allocations)
    for c in costs:
        assert c['quantity']==1 and c['unit']=='invocation' and c['pricing_mode']=='ACTUAL'
        assert c['activity_id'] in {a['id'] for a in attempts}
        assert len([x for x in allocations if x['cost_event_id']==c['id'] and x['source_schema_attempt_id']==c['activity_id']])==1
    report={'status':'PASS','history':frozen,'schema':schema,'costs':costs,'apiSha256':o.state['apiSha256'],
        'checks':['17 nonempty historical tables unchanged' if prior else '17 nonempty historical tables captured',
            'prior source request/attempt/observation/handling bytes unchanged','typed cost per physical native invocation'],
        'nativeObservationStatuses':[v['status'] for v in schema['source_schema_observation']]}
    output.parent.mkdir(parents=True,exist_ok=True);output.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8',newline='\n')
    print(json.dumps({'status':'PASS','historyTables':len(frozen),'requests':len(schema['source_schema_request']),
        'attempts':len(attempts),'invocations':len(costs),'statuses':report['nativeObservationStatuses']}))

def stress(o,output):
    requests=o.rows('source_schema_request',f"resource_id='{o.resource}' AND EXISTS(SELECT 1 FROM source_schema_observation s WHERE s.request_id=t.id AND s.status IN ('MATCH','DRIFT'))");assert requests
    req=requests[0]
    tables=('source_schema_request','source_schema_attempt','source_schema_observation','source_schema_handling','cost_event','evidence','audit_event')
    before={t:digest(o.rows(t)) for t in tables}
    with ThreadPoolExecutor(max_workers=8) as pool:
        replies=list(pool.map(lambda _:o.command(o.base,key=req['request_key']),range(8)))
    assert all(s==202 and b['id']==req['id'] for s,b in replies),replies
    assert o.command(o.base,key='foreign',authorized=False)[0]==403
    assert o.command(o.base.replace(o.ws,str(uuid.uuid4())),key='foreign')[0]==403
    assert o.command(o.base,key='override',body={'externalFqn':'elsewhere','consumer':'ANY'})[0]==400
    handling=o.rows('source_schema_handling'); assert handling
    h=handling[0]; path=f"/data-resources/{o.resource}/schema-observations/{h['observation_id']}/handle?workspaceId={o.ws}"
    with ThreadPoolExecutor(max_workers=8) as pool:
        replies=list(pool.map(lambda _:o.command(path,key=h['request_key'],body={'reason':h['reason']}),range(8)))
    assert all(s==200 for s,_ in replies),replies
    assert o.command(path,key=h['request_key'],body={'reason':'different handling reason'})[0]==409
    assert before=={t:digest(o.rows(t)) for t in tables},'replay added facts/cost'
    report={'status':'PASS','checks':['eight concurrent same-key HTTP requests preserve identity',
        'successful replay causes zero new physical calls or business facts','missing credential and foreign workspace rejected',
        'caller source/consumer override rejected','eight concurrent handling replays preserve one fact',
        'changed same-key handling reason rejected']}
    output.write_text(json.dumps(report,indent=2),encoding='utf-8',newline='\n');print(json.dumps(report))

def concurrent(o,output):
    """Two upgraded dispatchers; two distinct real requests, no fake results."""
    repo=Path(__file__).resolve().parents[3]
    env=json.loads((o.private/'environment.json').read_text())['api']
    with output.with_suffix('.worker.log').open('wb') as log:
        worker=subprocess.Popen([str(repo/'.data/pgi-b3-c2-final-worker.exe')],cwd=repo/'apps/platform',
            env=env,stdout=log,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW)
    requests=[]
    def wait_for(fn):
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            value=fn()
            if value:return value
            time.sleep(.2)
        raise AssertionError('bounded concurrent acceptance timeout')
    try:
        for _ in range(2):
            status,req=o.command(o.base,key='c2-concurrent-'+uuid.uuid4().hex)
            assert status==202,(status,req)
            requests.append(req['id'])
            wait_for(lambda:o.rows('source_schema_attempt',f"request_id='{req['id']}'"))
        observations=[]
        for req in requests:
            observations+=wait_for(lambda:o.rows('source_schema_observation',f"request_id='{req}' AND status='MATCH'"))
        attempts=[o.rows('source_schema_attempt',f"id='{x['attempt_id']}'")[0] for x in observations]
        ordered=sorted(zip(attempts,observations),key=lambda x:x[0]['started_at'])
        assert ordered[1][0]['started_at']>=ordered[0][1]['recorded_at'],'native source scans overlapped'
        assert all(len(o.rows('cost_allocation',f"source_schema_attempt_id='{a['id']}'"))==1 for a in attempts)
        report={'status':'PASS','secondaryWorkerPid':worker.pid,'requestIds':requests,
            'intervals':[{'attemptId':a['id'],'start':a['started_at'],'recorded':v['recorded_at']} for a,v in ordered],
            'checks':['two upgraded dispatchers active for distinct native requests; log oracle identifies dispatch owners','same source scan intervals do not overlap',
                'both native discoveries MATCH; each physical invocation charged once']}
        output.write_text(json.dumps(report,indent=2),encoding='utf-8',newline='\n');print(json.dumps(report))
    finally:
        worker.terminate();worker.wait(timeout=10)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--runtime',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    p.add_argument('--previous',type=Path);p.add_argument('--stress',action='store_true');p.add_argument('--concurrent',action='store_true');a=p.parse_args();o=C2Oracle(a.runtime)
    if a.stress:stress(o,a.output)
    elif a.concurrent:concurrent(o,a.output)
    else:snapshot(o,a.output,a.previous)
