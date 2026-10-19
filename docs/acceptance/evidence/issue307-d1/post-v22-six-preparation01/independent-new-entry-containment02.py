import copy,datetime as dt,hashlib,importlib.util,json,subprocess,sys,tempfile
from contextlib import ExitStack
from pathlib import Path
from unittest.mock import patch
sys.dont_write_bytecode=True
R=Path(__file__).resolve().parents[5];D=Path(__file__).parent
sys.path.insert(0,str(R/'examples/lifecycle-d1'))
def refuse(*a,**k):raise RuntimeError('independent harness refuses unmodeled external subprocess')
class RefusedPopen(subprocess.Popen):
    def __init__(self,*a,**k):refuse(*a,**k)
subprocess.run=refuse;subprocess.Popen=RefusedPopen
spec=importlib.util.spec_from_file_location('independent_new_support',R/'examples/lifecycle-d1/window_six_support.py');w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
v=w.v;r=w.r;p=w.p
sha=lambda x:hashlib.sha256(x.read_bytes()).hexdigest()
class UnknownStop(w.Host):
    def __init__(self):self.commands=[]
    def inspect(self,cid):raise RuntimeError('recorded inspect unavailable')
    def docker(self,*args,**kw):self.commands.append(list(args));return b''
h=UnknownStop();error=None
try:h.stop('a'*64)
except Exception as e:error=type(e).__name__
assert error and h.commands==[['stop','--time','15','a'*64]]
stop={'inspect_unavailable':True,'fixed_stop_requested':True,'actual_modeled_commands':h.commands,'error':error}
with tempfile.TemporaryDirectory() as tmp,ExitStack() as s:
    root=Path(tmp);control=root/'private';control.mkdir();packet=root/'packet';packet.mkdir()
    group={'Group':'business-races','Core':'c'*64,'Native':'n'*64,'Network':'g'*64}
    allocation={'group':'business-races'}
    s.enter_context(patch.object(r,'CONTROL',control))
    s.enter_context(patch.object(v,'load',lambda path: {'groups':[allocation]} if Path(path).name=='allocation.json' else ({'Groups':[group]} if Path(path).name=='candidate.json' else {'baseline':True})))
    s.enter_context(patch.object(r,'profile',lambda *a:{'AuditID':'11111111-1111-4111-8111-111111111111','Enabled':False}))
    folder=control/'business-races';folder.mkdir()
    for name in ('frozen-inventory.json','present-inventory.json'):(folder/name).write_bytes(b'{}')
    (control/'six-map.json').write_bytes(b'{}')
    s.enter_context(patch.object(p.prior.closure,'source_contract',lambda value:None))
    s.enter_context(patch.object(p.native,'wait_pg',lambda *a:None))
    s.enter_context(patch.object(p.native,'fingerprints',lambda cid:{'baseline':True}))
    s.enter_context(patch.object(v.time,'sleep',lambda n:None))
    commands=[];closed=[]
    def docker(*args,**kw):
        commands.append(list(args))
        if args[0]=='start':return b''
        if args[:2]==('network','inspect'):return json.dumps([{'Internal':True,'Containers':{group['Core']:{},group['Native']:{}}}]).encode()
        if args[0]=='create':raise RuntimeError('recorded create transport outcome unknown')
        raise AssertionError('unbound Docker command')
    s.enter_context(patch.object(r,'docker',docker))
    s.enter_context(patch.object(r,'inspect',lambda name: (_ for _ in ()).throw(RuntimeError('recorded exact-name inspect unavailable'))))
    s.enter_context(patch.object(p.p1,'close_known',lambda cid:closed.append(cid) or {'cid':cid,'closed':True}))
    def replace(path,value):path.write_text(json.dumps(value),encoding='utf-8')
    s.enter_context(patch.object(r,'replace',replace))
    at=r.now().replace(microsecond=0);error=None
    try:v.collect(group,{},packet,control/'fresh-read',root/'out',at+dt.timedelta(minutes=50))
    except Exception as e:error=type(e).__name__
    receipt=json.loads((root/'out/closure.json').read_text('utf-8'))
    assert error and receipt['all_closed'] is False and len(closed)==2 and receipt['unknown_temporary_owner']['state']=='UNCONFIRMED'
    unknown={'raised':error,'receipt':receipt,'closed_known_cids':closed,'create_names':[args[args.index('--name')+1] for args in commands if args[0]=='create'],'unconfirmed_created_name_recorded':True,'model_only':True}
result={'at':dt.datetime.now(dt.timezone.utc).isoformat(),'findings':{'E1_inherited_stop':stop,'E2_unknown_create_closure':unknown},'source_files':{name:sha(R/'examples/lifecycle-d1'/name) for name in ('window_six_support.py','verify-six-native.py')},'actual_Docker':0,'actual_source_native_provider':0}
path=D/'independent-new-entry-containment02.json'
with path.open('x',encoding='utf-8',newline='\n') as f:json.dump(result,f,indent=2);f.write('\n')
print(json.dumps({'receipt':str(path),'sha256':sha(path),'findings':result['findings']}))
