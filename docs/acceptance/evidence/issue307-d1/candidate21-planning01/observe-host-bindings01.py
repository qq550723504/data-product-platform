"""Record containment Git anchor and actual service identities; no execution authority."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
PACKET=BASE/'remaining-candidate21'
PREP=BASE/'remaining-candidate21-preparation01'
CONTROL=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')
ANCHOR=json.loads((HERE/'packet-commit01.json').read_bytes())['head']
load=lambda p:json.loads(p.read_bytes())
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def need(ok,message):
 if not ok:raise ValueError(message)
def save(path,value):
 with path.open('xb') as f:f.write((json.dumps(value,indent=2)+'\n').encode('utf-8'))
def git(*args):return subprocess.check_output(['git','-c','core.longpaths=true',*args],cwd=ROOT)
def main():
 c=load(PACKET/'candidate.json');s=load(PACKET/'status.json');a=load(PREP/'allocation.json')
 for name in ('candidate.json','bindings.json','confirmation-template.txt'):
  need(git('show',ANCHOR+':'+(PACKET/name).relative_to(ROOT).as_posix())==(PACKET/name).read_bytes(),'containment Git snapshot bytes differ')
 anchor={'profile':c['Profile'],'execution':c['Execution'],'candidate_sha256':s['candidate_sha256'],
  'bindings_sha256':s['bindings_sha256'],'template_sha256':s['confirmation_sha256'],'packet_commit':ANCHOR}
 save(CONTROL/'host-anchor.json',anchor)
 save(HERE/'host-anchor-readback01.json',{'status':'PASS_HOST_ANCHOR_FILE_BINDING_ONLY','anchor':anchor,
  'private_anchor_sha256':sha(CONTROL/'host-anchor.json'),'authorization':'NOT_RECEIVED',
  'Host_check':'NOT_RUN_REQUIRES_NEW_HUMAN_AUTHORIZATION','source_Docker_provider_DELETE':0})
 need(load(BASE/'remaining-candidate21-readonly01/complete.json')['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY','fresh full read missing')
 services=[]
 for g in c['Groups']:
  for role in ('Core','Native','Gateway'):
   x=json.loads(subprocess.check_output(['docker','inspect',g[role]]))[0]
   need(not x['State']['Running'] and x['State']['Pid']==0 and x['HostConfig']['RestartPolicy']['Name']=='no','service not closed')
   services.append({'group':g['Group'],'role':role,'cid':x['Id'],'image_id':x['Image'],
    'configured_image':x['Config']['Image'],'PID':0,'restart':'no'})
 save(HERE/'engine-service-bindings01.json',{'status':'PASS_CURRENT_SERVICE_IMAGE_CID_BINDINGS_READONLY',
  'at':dt.datetime.now(dt.timezone.utc).isoformat(),'services':services,'source_start':0,'native_delete':0})
 need(len(a['owners'])==86 and len(a['resources'])==18 and len(a['build_files'])==12,'allocation readback differs')
 save(HERE/'allocation-readback01.json',{'status':'OBSERVED_SUCCESSFUL_STOPPED_ALLOCATION',
  'observed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':c['Execution'],'source':c['Source'],
  'owners':86,'networks':3,'new_volumes':15,'frozen_build_members':12,'allocation_sha256':sha(PREP/'allocation.json'),
  'allocation_closure_sha256':sha(PREP/'allocation-closed.json'),'provider_io':0,'native_delete':0,'authority':False,
  'note':'Original allocation receipts retained; current stopped state is separately observed.'})
 print(json.dumps({'status':'PASS_HOST_BINDINGS_READBACK','containment_anchor':ANCHOR,'services':len(services),'authority':False}))
if __name__=='__main__':main()
