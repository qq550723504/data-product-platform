"""Actual v28 failure, consumed budget, trusted closure and new read audit.

Historical runtime and authority are retained; none can be replayed by this
module. It performs only exact file reads, Docker inspect and budget copies.
"""
import hashlib,importlib.util,json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
EVIDENCE=ROOT/'docs/acceptance/evidence/issue307-d1'
CORE_BASELINES={
 'business-races':('post-v28-business-closure01/final-Core-fingerprint.json','9fc524c5d2b84125815b8222c3d1ef70c77e3490c79428bee47e94d38f6c002e'),
 'late-hold':('remaining-candidate18/late-hold/core-readonly.json','7aafb25df2004970410fc80ebbe1206c49ebf2d3315d4294a95c7c903b10c01b'),
 'mixed':('remaining-candidate18/mixed/core-readonly.json','2c670718760a3a7306a9fdba4946e99d9fb024f7258235f4c7f6d880064afd55'),
}

def module():
 s=importlib.util.spec_from_file_location('retained28_finite_closure',Path(__file__).with_name('close-v28-business-dispatches.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def verify(root,inspect,closed,docker,ids,private,hostpath):
 c=module();need,load,sha=c.need,c.load,c.sha
 for name,digest in CORE_BASELINES.values():need(sha(root/'docs/acceptance/evidence/issue307-d1'/name)==digest,'post-v28 current Core anchor changed')
 closure=load(c.PUBLIC/'terminal.json');read=EVIDENCE/'post-v28-seventeen01'
 need(closure['status']=='PASS_V28_MIXED_CLOSURE_APPEND_ONLY_SEALED' and closure['retained_unchanged'] and closure['native_delete']==closure['pins_released']==closure['budgets_refunded']==0,'v28 trusted closure incomplete')
 need(sha(c.PUBLIC/'terminal.json')==sha(c.CONTROL/'terminal.json')=='9ceecbfe9ef0b6e67b10a39895100770dd5a153535f63d952be496438eb66fd0','mixed closure terminal bytes changed')
 terminal=load(read/'terminal.json');need(terminal['status']=='PASS_POST_V28_SEVENTEEN_NATIVE_READONLY_SEALED' and terminal['present_versions']==17 and terminal['native_delete']==terminal['business_mutations']==0,'post-v28 native17 audit incomplete')
 plan=load(read/'plan.json');need(sha(read/'plan.json')==terminal['plan_sha256'],'new read audit plan changed')
 before=plan['preservation'];actual=c.snapshot()
 need({k:v for k,v in before.items() if k not in ('maintenance_files','maintenance_owners')}==actual,'v28 retained profile/budget/backup/failure bytes changed')
 for name,digest in before['maintenance_files'].items():need(sha(Path(name))==digest,'maintenance original bytes changed')
 known=set(c.known_ids())|set(before['maintenance_owners'])
 profiles={}
 for group in ('business-races','late-hold','mixed'):
  folder=private/'post-v28-seventeen01'/group;profile=folder/'six-readonly.json';cfg=load(profile)
  need(cfg['Enabled'] is False and cfg['MaxProviderDeletes']==0,'completed read profile enabled')
  cid=load(read/group/'allocated-owner.json')['cid'];known.add(cid);profiles[group]=dict(path=str(profile),sha256=sha(profile),cid=cid)
 need(len(known)==731 and ids.issubset(known),'post-v28 retained owner union differs')
 rows=c.inspect_many(sorted(known));need(all(closed(x) for x in rows),'retained v28 owner active')
 ids.update(known)
 return dict(status='PASS_RETAINED_V28_DENIED_AND_CLOSED_731',closed_cids=sorted(known),closure_sha256=sha(c.PUBLIC/'terminal.json'),read_sha256=sha(read/'terminal.json'),read_plan_sha256=sha(read/'plan.json'),retained=actual,completed_read_profiles=profiles,authorization_reusable=False,old_used_budget_refunded=False,pins_released=0,native_delete=0)
