"""Publish reviewed preparation to the existing Draft PR, without activation."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
OUT=HERE/'publication02'
SOURCE='a260cfd25c164652ee0e908c6e8fb65d1be9ec38'
REMOTE_HEAD='4d126c60687830416b2697ab4b120e4323682eda'
BRANCH='codex/issue307-post-v22-dispatch-closure'
REPO='qq550723504/data-product-platform'
load=lambda p:json.loads(p.read_bytes())
def need(ok,message):
 if not ok:raise ValueError(message)
def save(name,value):
 with (OUT/name).open('xb') as f:f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))
def call(argv):
 r=subprocess.run(argv,cwd=ROOT,capture_output=True)
 need(r.returncode==0,r.stderr.decode('utf-8','replace'))
 return r.stdout
def git(*argv):return call(['git','-c','core.longpaths=true',*argv])
def gh(*argv):return json.loads(call(['gh',*argv]))
def main():
 review=load(BASE/'candidate19-review01/independent-final-review01.json')
 need(review['status']=='PASS_BOUNDED_C19_FROZEN_NONDELETE_PREPARATION_REVIEW' and not review['remaining_blockers'],'final independent review incomplete')
 need(load(HERE/'final-static-audit01/evidence-audit.json')['status']=='PASS_EXPLICIT_EVIDENCE_INDEX_AUDIT','fixed final audit incomplete')
 need(load(HERE/'publication-secret-scan03.json')['findings']==[],'publication credentials scan failed')
 need(git('diff',SOURCE,'--','apps/platform','examples/lifecycle-d1','migrations')==b'','runtime changed since frozen build')
 pr=gh('pr','view','332','--repo',REPO,'--json','headRefOid,headRefName,isDraft,state,mergedAt')
 need(pr=={'headRefName':BRANCH,'headRefOid':REMOTE_HEAD,'isDraft':True,'mergedAt':None,'state':'OPEN'},'PR ownership/head/Draft state changed')
 need(gh('issue','view','307','--repo',REPO,'--json','state')['state']=='OPEN','Issue307 closed')
 git('merge-base','--is-ancestor',REMOTE_HEAD,'HEAD')
 OUT.mkdir(exist_ok=False)
 save('intent.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'runtime_source':SOURCE,'previous_PR_head':REMOTE_HEAD,
  'remote_branch':BRANCH,'PR332':'DRAFT','Issue307':'OPEN','native_delete':0,'merge':False,'execution_authority':False})
 folders=['post-v28-core-audit01','post-v28-restored-core01','post-v28-restored-core02','post-v28-restored-core03',
  'post-v28-business-closure01','post-v28-seventeen01','candidate19-planning01','candidate19-source-review01',
  'candidate19-review01','remaining-candidate19-freeze-failure01','remaining-candidate19-preparation01',
  'remaining-candidate19','remaining-candidate19-readonly01']
 paths=[(BASE/x).relative_to(ROOT).as_posix() for x in folders]
 paths+=['docs/acceptance/issue307-d1-candidate19-confirmation.md','docs/acceptance/issue307-d1-v29-window-runbook.md',
  'docs/acceptance/issue307-d1-v29-preparation.md']
 git('add','--','.gitattributes',*paths)
 raw=git('ls-files','-s','-z','--',*paths);count=0;failures=[]
 for entry in raw.split(b'\0'):
  if not entry:continue
  header,name=entry.split(b'\t',1);mode,oid,stage=header.split();relative=name.decode('utf-8');p=ROOT/relative
  need(stage==b'0' and mode==b'100644' and p.is_file() and not p.is_symlink() and not p.is_junction(),'unexpected staged evidence type')
  content=p.read_bytes();actual=hashlib.sha1(b'blob '+str(len(content)).encode()+b'\0'+content).hexdigest()
  if actual!=oid.decode():failures.append(relative)
  count+=1
 need(not failures,'Indexed evidence bytes changed through Git filters: '+str(failures))
 save('index-bytes-audit01.json',{'status':'PASS_STAGED_EVIDENCE_RAW_EQUALS_GIT_BLOB','files':count,'failures':[],
  'scope':paths,'self_excluded':True,'runtime_raw_LF_Git_ZIP_semantics_separate':True,'native_delete':0})
 git('add','--',(OUT/'index-bytes-audit01.json').relative_to(ROOT).as_posix())
 git('diff','--cached','--check')
 git('commit','-q','-m','docs(lifecycle): complete candidate19 native preparation and independent review')
 head=git('rev-parse','HEAD').decode().strip()
 save('commit-result01.json',{'head':head,'runtime_source':SOURCE,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'native_delete':0})
 pushed=subprocess.run(['git','-c','core.longpaths=true','push','origin','HEAD:refs/heads/'+BRANCH],cwd=ROOT,capture_output=True)
 for name in ('stdout','stderr'):
  with (OUT/('push-'+name+'.txt')).open('xb') as f:f.write(getattr(pushed,name))
 save('push-result01.json',{'exit':pushed.returncode,'head':head,'at':dt.datetime.now(dt.timezone.utc).isoformat()})
 need(pushed.returncode==0,'Push failed; retained raw channels')
 current=gh('pr','view','332','--repo',REPO,'--json','headRefOid,isDraft,state,mergedAt')
 need(current['headRefOid']==head and current['isDraft'] and current['state']=='OPEN' and current['mergedAt'] is None,'Published PR state differs')
 call(['gh','pr','edit','332','--repo',REPO,'--title','#307 D1: close retained v28 dispatches and freeze candidate19/v29',
  '--body-file',str(HERE/'pull-request-body01.md')])
 save('published-readback01.json',{'head':head,'PR':current,'Issue307':'OPEN','CI':'PENDING_EXACT_HEAD',
  'at':dt.datetime.now(dt.timezone.utc).isoformat(),'main_ci_claimed':False,'native_delete':0,'execution_authority':False})
 print(json.dumps({'status':'PUBLISHED_DRAFT_PREPARATION_PENDING_EXACT_HEAD_CI','head':head,'staged_evidence_files':count,'native_delete':0}))
if __name__=='__main__':main()
