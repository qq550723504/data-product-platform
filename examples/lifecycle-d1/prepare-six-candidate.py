"""Finite candidate19 preparation. Allocate stopped owners; never send/enable DELETE.

Allocation, source/restore non-delete verification and final freeze are distinct
checkpoints. A stopped allocation is not a native PASS or deletion permission.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import uuid

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('six_prior_readonly',Path(__file__).with_name('verify-post-v22-seventeen.py'))
prior=importlib.util.module_from_spec(spec);spec.loader.exec_module(prior)
p1,native=prior.p1,prior.native
sys.path.insert(0,str(Path(__file__).parent))
import remaining_six_window as window
import remaining_sequence as sequence

W=window.compiled(); EXECUTION=W['Execution']; PROFILE=W['Profile']; GENERATION=PROFILE.split('/')[0]
CONTROL=p1.PRIVATE/('remaining-'+GENERATION)
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1'/('remaining-'+GENERATION+'-preparation01')
ZERO='0001-01-01T00:00:00Z'
GROUPS=('business-races','late-hold','mixed')
need,load,sha,save=prior.need,prior.load,prior.sha,prior.save
inspect,closed=p1.inspect,p1.closed


def call(args,data=None,output=None):
    """Preserve exact raw channels under this preparation, not an old audit."""
    if output is None:
        result=subprocess.run(list(map(str,args)),cwd=ROOT,input=data,capture_output=True)
    else:
        with output.open('xb') as stream:
            result=subprocess.run(list(map(str,args)),cwd=ROOT,input=data,stdout=stream,stderr=subprocess.PIPE)
    stdout=b'' if output is not None and result.stdout is None else result.stdout
    need(isinstance(stdout,bytes) and isinstance(result.stderr,bytes),'process wrapper must return bytes')
    if result.returncode:
        diagnostic=p1.PRIVATE/('remaining-'+GENERATION+'-diagnostics');diagnostic.mkdir(exist_ok=True)
        prefix=diagnostic/('operation-'+uuid.uuid4().hex)
        with prefix.with_suffix('.stdout.private').open('xb') as f:f.write(stdout)
        with prefix.with_suffix('.stderr.private').open('xb') as f:f.write(result.stderr)
        raise RuntimeError('command failed; original channels retained at '+str(prefix))
    return stdout


# All imported helpers keep their established bytes contract; only failure
# evidence destination belongs to the new preparation rather than old runs.
p1.call=call
native.call=call


def docker(*args,**kwargs):
    raw=call(['docker',*args],**kwargs)
    need(isinstance(raw,bytes),'Docker wrapper must return bytes')
    return raw


def compact(value):return json.dumps(value,ensure_ascii=False,separators=(',',':')).encode()


def identity(scope,kind):return {'inspect':'i','delete':'d'}[kind]+EXECUTION[:8]+scope['Run'][:8]


def project_owner(item):
    h=item['HostConfig']
    return {'id':item['Id'],'image':item['Image'],'user':item['Config']['User'],
            'entrypoint':item['Config']['Entrypoint'],'command':item['Config']['Cmd'],
            'host':{k:h.get(k) for k in ('NetworkMode','ReadonlyRootfs','Privileged','AutoRemove','RestartPolicy','PortBindings','CapDrop','CapAdd','SecurityOpt','Tmpfs')},
            'mounts':[{k:m.get(k) for k in ('Type','Name','Source','Destination','RW','Propagation')} for m in item['Mounts']]}


def historical_gate():
    base=prior.retained()
    packet=ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-seventeen01'
    need(sha(packet/'terminal.json')=='c6e12ffdb14b64c139d7352f67b6c54b1c3ff859fe0400f03c3343dcd4c92a33','accepted seventeen terminal differs')
    need(sha(packet/'source-independent/result-final-review01.json')=='c6508108d16f6095889b7dce66bb26d4940073faf1934a8b9b1fed9ec69ab616','accepted seventeen result review differs')
    manifest=load(packet/'preparation.json')
    ids=set(base['all_87_closed_cids'])
    for owner in manifest['owners'].values():
        prior.owner_contract(owner)
        ids.add(owner['cid'])
    need(len(ids)==90 and all(closed(inspect(cid)) for cid in ids),'accepted ninety owners not closed')
    old_public=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate13-preparation01'
    old_private=p1.PRIVATE/'remaining-candidate13'
    failures={'non-delete-terminal.json':'7db97768426c0616e0e28b36b03e3cccd602545f2e4491d0ac0f26b194f886e4',
              'allocation.json':'91198e014b72d289ddf9d071e5ba888c904f7638fbb70c9a01dfc6e111da80a7'}
    for name,digest in failures.items():need(sha(old_public/name)==digest,'retained c13 failure/allocation differs')
    bundle=old_public.parent/'remaining-candidate13-preparation-failure01/failure.json'
    need(sha(bundle)=='0ece4d5e2e5ced1acd0f60b3ca6f6ff8359bbf2c113c0bb10b1d364edc2537f9','retained partial-IAM failure differs')
    old=load(old_public/'allocation.json');descriptors=load(old_public/'owner-descriptors.json')
    need(len(old['owners'])==86 and len(set(old['owners']))==86,'retained c13 owner count differs')
    for cid in old['owners']:
        item=inspect(cid);need(closed(item) and project_owner(item)==descriptors[cid],'retained c13 owner topology/closure differs');ids.add(cid)
    for name,digest in old['binaries'].items():need(sha(old_private/name)==digest,'retained c13 binary changed')
    budgets={}
    old_packet=load(p1.PACKET/'candidate.json')
    packet={'Execution':old['execution'],'Groups':[{'Group':g['group'],'Helpers':g['helpers'],'Copies':next(x['Copies'] for x in old_packet['Groups'] if x['Group']==g['group'])} for g in old['groups']]}
    for g in old['groups']:
        folder=old_private/g['group']
        for stage in list(g['helpers'])+['root-'+x for x in g['root_owners']]:need(load(folder/stage/'permission.json')=={'Enabled':False},'retained c13 permission differs')
        volume=json.loads(docker('volume','inspect',g['volumes']['budget']))[0]
        need(volume['Labels'].get('lifecycle307.remaining')==old['execution'] and volume['Labels'].get('lifecycle307.allocation')==old['allocation_nonce'],'retained budget ownership differs')
        mounts=docker('ps','-aq','--no-trunc','--filter','volume='+g['volumes']['budget']).decode('utf8','strict').split()
        need(sorted(mounts)==g['budget_expected_mounted_cids'],'retained budget mounted CID set expanded')
        raw=docker('cp',g['helpers']['wire']+':/run/budget','-')
        sequence.budget_archive(raw,packet,g['group'],initial=True)
        budgets[g['group']]={'volume':g['volumes']['budget'],'uid':10001,'files':[],'mounted_cids':sorted(mounts)}
    scratch={}
    scratch_pins={'post-v22-disabled-bootstrap-contract01': '421e9b0d5ebaca6fd7a4cc11af372b5be29f5d74595a1ec1606149f26f2ad0eb', 'post-v22-disabled-bootstrap-native02': '070a1b356ed6fbe4108a159b89a95b2e7b7bff82053e61fa9d972adee4bfafc1', 'post-v22-disabled-bootstrap-native03': '1e5343bb904ff4760c34dd28ac087cc5201a16503f2406ed9982b908a83e16c0', 'post-v22-disabled-bootstrap-native04': '8f8db644f2b23213d7d0b0e90ef93a7462c232f8eb036f7939a51146790ba108'}
    for name,digest in scratch_pins.items():
        folder=old_public.parent/name;need(sha(folder/'terminal.json')==digest,'retained scratch terminal changed');terminal=load(folder/'terminal.json')
        need(terminal['native_delete']==terminal['source_calls']==0,'scratch contract exceeded source/delete boundary')
        if name=='post-v22-disabled-bootstrap-native02':need(terminal['status']=='FAILED_OR_UNCONFIRMED_RETAINED' and sha(folder/'terminal.json')=='070a1b356ed6fbe4108a159b89a95b2e7b7bff82053e61fa9d972adee4bfafc1','original SDK-backoff failure differs')
        else:need(terminal['failure'] is None,'scratch API/adapter contract not successful')
        rows=terminal.get('closure',terminal.get('closed'))
        need(len(rows)==2,'scratch closure scope differs')
        for row in rows:need(closed(inspect(row['cid'])),'retained scratch caller not closed');ids.add(row['cid'])
        scratch[name]=sha(folder/'terminal.json')
    need(len(ids)==184,'retained first failure-chain owner count differs')
    c14=retained_c14(ids)
    need(len(ids)==270,'retained c14 closed owner count differs')
    c15=retained_c15(ids)
    need(len(ids)==359,'retained c15 closed owner count differs')
    import retained_v26
    c16=retained_v26.verify(inspect,closed,project_owner,docker,sequence,ids,p1.PRIVATE,p1.hostpath)
    need(len(ids)==445,'retained v26 closed owner count differs')
    import retained_v27
    c17=retained_v27.verify(ROOT,inspect,closed,docker,ids,p1.PRIVATE,p1.hostpath)
    need(len(ids)==623,'retained post-v27 closed owner count differs')
    import retained_v28
    c18=retained_v28.verify(ROOT,inspect,closed,docker,ids,p1.PRIVATE,p1.hostpath)
    need(len(ids)==731,'retained post-v28 closed owner count differs')
    return {'c18':c18,'c17':c17,'c16':c16,'c15':c15,'c14':c14,'status':'PASS_RETAINED_731_CLOSED','closed_cids':sorted(ids),'prior':base,'c13_failure':failures,'c13_bundle_sha256':sha(bundle),'c13_budgets':budgets,'scratch':scratch,'native_delete':0}


def retained_c14(ids):
    public=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate14-preparation01'
    private=p1.PRIVATE/'remaining-candidate14'
    pins={'allocation.json':'ba37a45ccc83dc21584bbfb929fc560f5da89e6bc397f96ad8cb00daba4a76b2',
          'non-delete-terminal.json':'680dbaeeca4f6e8bfa7097860994d5fee7c7b96ac71617cc6a4542aab94194f7',
          'budget-initialization.json':'c4061f5c1e2212ef96eed3e6b4fa9726e0ada68749b3bddeaf77b37eb1159f1b'}
    for name,digest in pins.items():need(sha(public/name)==digest,'retained c14 failure/budget bytes changed')
    bundle=public.parent/'remaining-candidate14-preparation-failure01/failure.json'
    need(sha(bundle)=='2e2ebeb0946df534297bb306761cc14b992e50b559871098f2822e4312c3e704','retained c14 final failure bundle changed')
    for name,digest in load(bundle)['private_retained_files_sha256'].items():
        path=(private/name).resolve();need(path.is_relative_to(private.resolve()) and sha(path)==digest,'retained c14 build/backup/case/trial bytes changed')
    a=load(public/'allocation.json');terminal=load(public/'non-delete-terminal.json');descriptors=load(public/'owner-descriptors.json')
    need(terminal['groups_completed']==terminal['native_delete']==0 and all(x['closed'] for x in terminal['closure']),'retained c14 failed closure incomplete')
    need(sha(public/'non-delete-evidence-index.json')==terminal['evidence_index_sha256'],'retained c14 index changed')
    for name,digest in load(public/'non-delete-evidence-index.json')['files'].items():
        path=(public/name).resolve();need(path.is_relative_to(public.resolve()) and sha(path)==digest,'retained c14 raw member changed')
    need(len(set(a['owners']))==86,'retained c14 scope differs')
    for cid in a['owners']:
        value=inspect(cid);need(closed(value) and project_owner(value)==descriptors[cid],'retained c14 caller/topology changed');ids.add(cid)
    for name,digest in a['binaries'].items():need(sha(private/name)==digest,'retained c14 binary changed')
    original=load(p1.PACKET/'candidate.json')
    packet={'Execution':a['execution'],'Groups':[{'Group':g['group'],'Helpers':g['helpers'],'Copies':next(x['Copies'] for x in original['Groups'] if x['Group']==g['group'])} for g in a['groups']]}
    budgets={}
    for g in a['groups']:
        folder=private/g['group']
        for stage in list(g['helpers'])+['root-'+x for x in g['root_owners']]:need(load(folder/stage/'permission.json')=={'Enabled':False},'retained c14 permission changed')
        for role in g['maintenance']:
            stage='maintenance-'+role.lower() if role in ('PROVISION','SOURCE','RESTORE') else role
            filename='six-provision.json' if role=='PROVISION' else ('restore-inspector.json' if role.startswith('restore-inspect-') else 'six-readonly.json')
            need(load(folder/stage/filename)['Enabled'] is False,'retained c14 maintenance enabled')
        for stage in g['rehearsals']:need(load(folder/('rehearsal-'+stage)/'remaining.json')['Base']['Enabled'] is False,'retained c14 trial enabled')
        volume=json.loads(docker('volume','inspect',g['volumes']['budget']))[0]
        need(volume['Labels'].get('lifecycle307.remaining')==a['execution'] and volume['Labels'].get('lifecycle307.allocation')==a['allocation_nonce'],'retained c14 budget owner changed')
        mounted=docker('ps','-aq','--no-trunc','--filter','volume='+volume['Name']).decode('utf8','strict').split()
        need(sorted(mounted)==g['budget_expected_mounted_cids'],'retained c14 budget mount graph expanded')
        sequence.budget_archive(docker('cp',g['helpers']['wire']+':/run/budget','-'),packet,g['group'],initial=True)
        budgets[g['group']]={'volume':volume['Name'],'uid':10001,'files':[],'mounted_cids':sorted(mounted)}
    return {'pins':pins,'failure_bundle_sha256':sha(bundle),'budgets':budgets,'failed_execution':a['execution'],'never_replayed':True}


def retained_c15(ids):
    # The actual approved checkout is retained separately from this generation.
    # Its source/reference hashes must not be checked against edited new code.
    old_root=Path('C:/Users/Henry/.codex/worktrees/d1-six/data-product-platform')
    evidence=old_root/'docs/acceptance/evidence/issue307-d1'
    public=evidence/'remaining-candidate15-preparation01'
    private=p1.PRIVATE/'remaining-candidate15'
    pins={'allocation.json':'80cf4a96e73759116a8a62952888361a7db06547c4318404a6ce50e46f4f1682',
          'non-delete-terminal.json':'921d7ed8b98974841c03de58efb7a53ef58198701236ead19765ea63d30087c4',
          'budget-initialization.json':'3a91e3df11bbf4fa7151bd91e73a6bb2447738a4d9f0018ed942fd5ccb96c017'}
    for name,digest in pins.items():need(sha(public/name)==digest,'retained c15 preparation/budget changed')
    need(sha(public/'non-delete-evidence-index.json')==load(public/'non-delete-terminal.json')['evidence_index_sha256'],'retained c15 non-delete index changed')
    for name,digest in load(public/'non-delete-evidence-index.json')['files'].items():
        path=(public/name).resolve();need(path.is_relative_to(public.resolve()) and sha(path)==digest,'retained c15 preparation raw bytes changed')
    packet=evidence/'remaining-candidate15'
    need(sha(packet/'candidate.json')=='0ec2f2e03829905af97c9f71eafb108f7b3bf0899e7e741a0334d44520a5dc99'
         and sha(packet/'bindings.json')=='d8a7075be2e8614f1af4fbaa159b45dc53430d19f06ea2920070b03c4e917da0','retained c15 frozen packet changed')
    auth_path=evidence/'delete-confirmed-v25-execution01/human-authorization.json'
    terminal_path=evidence/'delete-confirmed-v25-execution01/review-blocked-terminal.json'
    need(sha(auth_path)=='55f0be454529612e1d81c47c68f10542929f96d661be675d50aea32fe4029b9e'
         and sha(terminal_path)=='938c36cc10beba27c608bc588ab3a24139d047d76b35a2d9390b92c4496e034f','retained c15 authorization/blocked terminal changed')
    auth=load(auth_path);terminal=load(terminal_path)
    need(terminal['status']=='BLOCKED_PREWINDOW_REVIEW' and terminal['window_lifecycle_started'] is False and terminal['actual_DELETE_this_review']==0
         and sha(private/'authorized-execution01/terminal.json')==sha(terminal_path),'retained c15 prewindow terminal differs')
    for item in auth['approved_reference_artifacts']:
        path=(old_root/item['path']).resolve()
        need(path.is_relative_to(old_root.resolve()) and sha(path)==item['sha256'],'retained c15 original approved reference changed')
    need(sha(private/'authorized-execution01/user-message-verbatim.txt')==auth['user_message_sha256']
         and sha(private/'authorized-execution01/authorization-envelope.txt')==auth['authorization_envelope_sha256'],'retained c15 actual human authorization changed')
    review=evidence/'delete-confirmed-v25-review01'
    need(sha(review/'independent-authorization-window-review01.json')=='b0a4ef12e4b8604c2b18228ef1ce242b53c618df3dff56d4771b305d02c74331'
         and sha(review/'independent-isolated-cli-repair-addendum02.json')=='966dd18c814bf6890ced56a3b81b4d7c807436089d15ae026f8c3853dd27acad','retained c15 failure/repair reviews changed')
    for name,digest in [('independent-evidence-index01.json','7cd7803fbaba8308d32422be155cde267b3e46f92a94a719e1bc3c5ae07c0baf'),('independent-repair-evidence-index02.json','eb949351ea6e1e90d85954159bcab8c882defa1d13443a7060e16449a97c7bb6')]:
        need(sha(review/name)==digest,'retained c15 review index changed')
        for member,entry in load(review/name)['members'].items():
            path=(review/member).resolve();need(path.is_relative_to(review.resolve()) and sha(path)==entry['sha256'],'retained c15 review raw bytes changed')
    a=load(public/'allocation.json');candidate=load(packet/'candidate.json');bindings=load(packet/'bindings.json')
    descriptors=load(public/'owner-descriptors.json')
    need(len(a['owners'])==86 and a['execution']=='9f9c67d7-8a09-45a8-897f-9e3077b60980','retained c15 allocation differs')
    for cid in a['owners']:
        value=inspect(cid);need(closed(value) and project_owner(value)==descriptors[cid],'retained c15 owner topology/closure changed');ids.add(cid)
    for name,digest in a['build_files'].items():need(sha(private/name)==digest,'retained c15 initial build member changed')
    budgets={};backups={}
    for g,b,c in zip(a['groups'],bindings,candidate['Groups']):
        folder=private/g['group'];need(g['group']==b['group']==c['Group'],'retained c15 group order changed')
        for stage in list(g['helpers'])+['root-'+x for x in g['root_owners']]:
            owner=folder/stage
            need(load(owner/'permission.json')['Enabled'] is False and not (owner/'human-confirmation.txt').exists(),'retained c15 permission or materialized authority changed')
        for stage,digest in c['CaseConfigSHA256'].items():need(sha(folder/('case-'+stage)/'remaining.json')==digest,'retained c15 frozen case changed')
        for name,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
            need(sha(folder/'checkpoint'/name)==c[key],'retained c15 backup changed');backups[g['group']+'/'+name]=c[key]
        volume=json.loads(docker('volume','inspect',g['volumes']['budget']))[0]
        need(volume['Labels'].get('lifecycle307.remaining')==a['execution'] and volume['Labels'].get('lifecycle307.allocation')==a['allocation_nonce'],'retained c15 budget ownership changed')
        mounted=docker('ps','-aq','--no-trunc','--filter','volume='+volume['Name']).decode('utf8','strict').split()
        need(sorted(mounted)==g['budget_expected_mounted_cids'],'retained c15 budget mount graph expanded')
        sequence.budget_archive(docker('cp',g['helpers']['wire']+':/run/budget','-'),candidate,g['group'],initial=True)
        budgets[g['group']]={'volume':volume['Name'],'uid':10001,'files':[],'mounted_cids':sorted(mounted)}
        complete=evidence/'remaining-candidate15-readonly01'/g['group']
        owner=load(complete/'allocated-owner.json')['cid']
        closure=load(complete/'closure.json')
        need(closure['all_closed'] is True and closure['failure'] is None and closed(inspect(owner)),'retained c15 complete-read owner not closed');ids.add(owner)
    return {'pins':pins,'blocked_terminal_sha256':sha(terminal_path),'blocked_execution':a['execution'],'budgets':budgets,'backups':backups,
            'original_approved_root':str(old_root),'approved_references':len(auth['approved_reference_artifacts']),'never_replayed':True}


class Allocation:
    def __init__(self):self.owners=[];self.resources=[];self.pending=[];self.nonce=str(uuid.uuid4())

    def create(self,role,group,args):
        name=str(args[args.index('--name')+1])
        self.pending.append(name)
        save(PUBLIC/('allocation-intent-%03d.json'%len(self.pending)),{'role':role,'group':group,'name':name,'execution':EXECUTION,'argv':list(map(str,args))})
        cid=docker('create','--pull','never','--label','lifecycle307.remaining='+EXECUTION,'--label','lifecycle307.allocation='+self.nonce,*args).decode('utf-8','strict').strip()
        need(len(cid)==64 and all(c in '0123456789abcdef' for c in cid),'Docker create did not return one full CID')
        self.owners.append(cid)
        descriptor={'role':role,'group':group,'cid':cid,'argv':list(map(str,args))}
        # A crash cannot lose knowledge of a previously returned owner CID.
        save(PUBLIC/('allocation-%03d.json'%len(self.owners)),descriptor)
        return cid

    def volume(self,name):
        existing=docker('volume','ls','--filter','name=^'+name+'$','--format','{{.Name}}').decode('utf-8','strict').split()
        need(not existing,'same-name retained volume exists; never adopt or start it')
        number=len(self.resources)+1
        save(PUBLIC/('resource-intent-%03d.json'%number),{'volume':name,'execution':EXECUTION,'allocation':self.nonce,'preexisting':False})
        value=docker('volume','create','--driver','local','--label','lifecycle307.remaining='+EXECUTION,'--label','lifecycle307.allocation='+self.nonce,name).decode('utf-8','strict').strip()
        self.resources.append({'volume':name,'allocation':self.nonce})
        volume=json.loads(docker('volume','inspect',name))[0]
        need(value==name==volume['Name'] and volume['Driver']=='local' and volume['Scope']=='local' and not volume.get('Options')
             and volume['Labels'].get('lifecycle307.remaining')==EXECUTION and volume['Labels'].get('lifecycle307.allocation')==self.nonce
             and not docker('ps','-aq','--no-trunc','--filter','volume='+name).strip(),'new volume owner/driver/options or empty mount graph differs')
        save(PUBLIC/('resource-%03d.json'%number),{'volume':name,'driver':volume['Driver'],'scope':volume['Scope'],'labels':volume['Labels'],'mounted_cids':[]})
        return name

    def network(self,name,subnet):
        need(not docker('network','ls','--filter','name=^'+name+'$','--format','{{.Name}}').strip(),'same-name retained network exists')
        number=len(self.resources)+1
        save(PUBLIC/('resource-intent-%03d.json'%number),{'network_name':name,'subnet':subnet,'execution':EXECUTION,'allocation':self.nonce,'preexisting':False})
        ident=docker('network','create','--driver','bridge','--internal','--subnet',subnet,'--label','lifecycle307.remaining='+EXECUTION,'--label','lifecycle307.allocation='+self.nonce,name).decode('utf-8','strict').strip()
        self.resources.append({'network':ident,'name':name,'subnet':subnet,'allocation':self.nonce})
        n=json.loads(docker('network','inspect',ident))[0]
        need(len(ident)==64 and n['Id']==ident and n['Name']==name and n['Driver']=='bridge' and n['Scope']=='local' and n['Internal'] and not n.get('Containers')
             and n['Labels'].get('lifecycle307.remaining')==EXECUTION and n['Labels'].get('lifecycle307.allocation')==self.nonce,'new isolated network ownership differs')
        save(PUBLIC/('resource-%03d.json'%number),self.resources[-1])
        return ident

    def helper(self,group,name,mode,folder,binary,netns,output,*,budget=None,operator=False):
        folder.mkdir(parents=True,exist_ok=True);output.mkdir(parents=True,exist_ok=True)
        entry='/run/operator' if operator else '/run/reader'
        args=['--name',name,'--user','10001:10001','--network','container:'+netns,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
              '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m',
              '--mount',f'type=bind,source={folder},target=/run/private,readonly',
              '--mount',f'type=bind,source={binary},target={entry},readonly',
              '--mount',f'type=bind,source={output},target=/run/output']
        if budget:args+=['--mount','type=volume,source='+budget+',target=/run/budget']
        args+=['--entrypoint',entry,p1.PG,'-stage' if operator else '-mode',mode]
        return self.create(mode,group,args)


def build(destination=CONTROL):
    need(subprocess.run(['git','diff','--quiet','HEAD','--','apps/platform','examples/lifecycle-d1','migrations'],cwd=ROOT).returncode==0 and not subprocess.check_output(['git','ls-files','--others','--exclude-standard','apps/platform','examples/lifecycle-d1','migrations'],cwd=ROOT).strip(),'commit reviewed runtime before allocation')
    source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip()
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    for name,pkg in [('reader','lifecycle-fixture-reader'),('operator','lifecycle-fixture-remaining'),('owner','lifecycle-fixture-remaining-iam')]:
        result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(destination/name),'./cmd/'+pkg],cwd=ROOT/'apps/platform',env=env,capture_output=True)
        (destination/(name+'-build.stdout')).write_bytes(result.stdout);(destination/(name+'-build.stderr')).write_bytes(result.stderr)
        need(result.returncode==0,'exact runtime build failed')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(destination/'readonly-check.exe'),'./cmd/lifecycle-fixture-readonly-check'],cwd=ROOT/'apps/platform',capture_output=True)
    (destination/'checker-build.stdout').write_bytes(result.stdout);(destination/'checker-build.stderr').write_bytes(result.stderr)
    need(result.returncode==0,'file-only checker build failed')
    return source


def allocate():
    window.policy_checkpoint()
    need(PROFILE=='candidate19/v29' and EXECUTION=='a5d1e569-f80e-4b38-8f32-d8139a30a077','only the new proposed execution is supported')
    need(not CONTROL.exists() and not PUBLIC.exists(),'one-shot preparation already allocated; retain and review')
    # Verify retained history before allocating any owner or volume.
    retained=historical_gate()
    CONTROL.mkdir();PUBLIC.mkdir();save(PUBLIC/'retained-before.json',retained)
    a=Allocation();failure=None
    try:
        source=build()
        old=load(p1.PACKET/'candidate.json');scopes=load(ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
        state={'status':'STOPPED_ALLOCATION_NOT_NATIVE_ACCEPTANCE','source':source,'profile':PROFILE,'execution':EXECUTION,'allocation_nonce':a.nonce,
               'window':W,'policy_checkpoint_sha256':window.POLICY_CHECKPOINT_SHA256,'retained':retained,'binaries':{n:sha(CONTROL/n) for n in ('reader','operator','owner','readonly-check.exe')},'build_files':{name:sha(CONTROL/name) for name in ('reader','operator','owner','readonly-check.exe',*[binary+'-build.'+channel for binary in ('reader','operator','owner','checker') for channel in ('stdout','stderr')])},'groups':[],'native_delete':0,'authorization':'NOT_RECEIVED','schedule':'NOT_CREATED'}
        mapping={'Execution':EXECUTION,'Profile':PROFILE,'Groups':[]}
        for n,scope in enumerate(scopes):
            group=scope['Group'];g=next(x for x in old['Groups'] if x['Group']==group)
            folder=CONTROL/group;folder.mkdir();outputs=folder/'outputs';outputs.mkdir();backup=folder/'checkpoint';backup.mkdir()
            project='dpp-lifecycle307-'+scope['Run'][:8];prefix=project+'-'+GENERATION
            config=load(p1.PRIVATE/project/'config.json')
            need(all(closed(inspect(g[role])) for role in ('Core','Native','Gateway')),'source not closed')
            pg_env=backup/'postgres.env';pg_env.write_text('POSTGRES_USER=checkpoint_admin\nPOSTGRES_DB=postgres\nPOSTGRES_PASSWORD='+secrets.token_hex(32)+'\n',encoding='utf-8')
            subnet='10.246.'+str(215+n)+'.0/28'
            # Reuse the existing subnet overlap check; never attach to another run.
            runner=prior.module('six_allocation_runner','prepare-remaining-runner.py');runner.check_subnet(subnet)
            network=a.network(prefix+'-restore',subnet)
            vols={kind:a.volume(prefix+'-'+kind) for kind in ('checkpoint-core','rehearsal-core','objects','recovery-redis','budget')}
            budget_script=folder/'budget-init.sh'
            budget_script.write_bytes(b'test -z "$(find /run/budget -mindepth 1 -print -quit)"\nchown 10001:10001 /run/budget\nstat -c "%u:%g %a" /run/budget\n')
            budget_initializer=a.create('new-empty-budget-initializer',group,['--name',prefix+'-budget-init','--user','0:0','--network','none','--read-only','--cap-drop','ALL','--cap-add','CHOWN','--security-opt','no-new-privileges',
                '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=volume,source='+vols['budget']+',target=/run/budget',
                '--mount',f'type=bind,source={budget_script},target=/run/budget-init.sh,readonly','--entrypoint','/bin/sh',p1.PG,'-e','/run/budget-init.sh'])
            cores={}
            for kind in ('checkpoint','rehearsal'):
                volume=vols[kind+'-core']
                cores[kind]=a.create(kind+'-core',group,['--name',prefix+'-'+kind,'--network',network,'--env-file',pg_env,
                    '--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data','--mount',f'type=bind,source={backup},target=/run/backup,readonly',p1.PG])
            credentials=folder/'restore-credentials';credentials.mkdir()
            (credentials/'root-access').write_text(config['RootAccess'],encoding='utf-8')
            (credentials/'root-secret').write_text(config['RootSecret'],encoding='utf-8')
            (credentials/'start.sh').write_bytes(b'export MINIO_ROOT_USER="$(cat /run/credentials/root-access)"\nexport MINIO_ROOT_PASSWORD="$(cat /run/credentials/root-secret)"\nexec minio server /data --address :9000 --console-address :9001\n')
            restored=a.create('checkpoint-native',group,['--name',prefix+'-native','--network',network,'--network-alias','minio',
                '--mount','type=volume,source='+vols['objects']+',target=/data','--mount',f'type=bind,source={credentials},target=/run/credentials,readonly',
                '--entrypoint','/bin/sh',p1.MI,'/run/credentials/start.sh'])
            mapping['Groups'].append({'Group':group,'Source':[g['Core'],g['Native'],g['Network']],
                                      'Restore':[cores['checkpoint'],restored,network]})
            # New gateway/profile paths; never overwrite the old sealed gateway.
            gateway=folder/'gateway.Caddyfile';gateway.write_bytes(b'{\n admin off\n auto_https off\n}\n:9000 { respond 403 }\n')
            gateway_cid=a.create('source-gateway',group,['--name',prefix+'-gateway','--network',g['Network'],'--tmpfs','/config','--tmpfs','/data','--mount',f'type=bind,source={gateway},target=/etc/caddy/Caddyfile,readonly',runner.GW,'caddy','run','--config','/etc/caddy/Caddyfile'])
            restore_gateway=folder/'restore-gateway.Caddyfile';restore_gateway.write_bytes(gateway.read_bytes())
            restore_gateway_cid=a.create('restore-gateway',group,['--name',prefix+'-restore-gateway','--network',network,'--tmpfs','/config','--tmpfs','/data','--mount',f'type=bind,source={restore_gateway},target=/etc/caddy/Caddyfile,readonly',runner.GW,'caddy','run','--config','/etc/caddy/Caddyfile'])
            # A distinct recovery queue avoids consuming old retained obligations.
            redis=a.create('recovery-redis',group,['--name',prefix+'-recovery-redis','--network',g['Network'],'--mount','type=volume,source='+vols['recovery-redis']+',target=/data',runner.REDIS,'redis-server','--appendonly','yes'])
            new_secrets={identity(scope,k):secrets.token_hex(32) for k in ('inspect','delete')}
            save(folder/'new-secrets.private.json',new_secrets)
            helpers={};cases={};roots={};maintenance={};rehearsals={};utilities={}
            stages=['wire']+[stage for name,stage,_ in sequence.ORDER if name==group for stage in (stage,'recover-'+stage)]
            for stage in stages:
                helpers[stage]=a.helper(group,prefix+'-'+stage,stage,folder/stage,CONTROL/'operator',gateway_cid,outputs/stage,budget=vols['budget'],operator=True)
            for stage,mode in window.CASE_MODES[group].items():
                cases[stage]=a.helper(group,prefix+'-case-'+stage,mode,folder/('case-'+stage),CONTROL/'reader',gateway_cid,outputs/('case-'+stage))
            for action in ('check-disabled','enable-read','enable-send','disable-send','disable-all'):
                private=folder/('root-'+action);private.mkdir();out=outputs/('root-'+action);out.mkdir()
                roots[action]=a.create('root-'+action,group,['--name',prefix+'-root-'+action,'--user','10001:10001','--network','container:'+g['Native'],'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
                    '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={private},target=/run/private,readonly',
                    '--mount',f'type=bind,source={CONTROL}/owner,target=/run/owner,readonly','--mount',f'type=bind,source={out},target=/run/output','--entrypoint','/run/owner',p1.PG,'-action',action])
            for role,netns,mode in [('SOURCE',g['Native'],'six-verify-native'),('RESTORE',restored,'six-verify-native'),('PROVISION',g['Native'],'six-provision-disabled')]:
                stage='maintenance-'+role.lower()
                maintenance[role]=a.helper(group,prefix+'-'+stage,mode,folder/stage,CONTROL/'reader',netns,outputs/stage)
            for action in ('enable','disable'):
                stage='restore-inspect-'+action
                maintenance[stage]=a.helper(group,prefix+'-'+stage,'six-'+stage,folder/stage,CONTROL/'reader',restored,outputs/stage)
            # These are the only business Commands run during preparation. Their
            # DSNs point to the independent rehearsal Core, never either source
            # or the clean restore checkpoint. They have no DELETE credential.
            modes={'business-races':{'business':'remaining-resume-business'},'late-hold':{'late':'remaining-late'},'mixed':{'mixed':'remaining-mixed','protected':'remaining-mixed-protected'}}[group]
            for stage,mode in modes.items():
                rehearsals[stage]=a.helper(group,prefix+'-rehearsal-'+stage,mode,folder/('rehearsal-'+stage),CONTROL/'reader',restore_gateway_cid,outputs/('rehearsal-'+stage))
            source_mounts=inspect(g['Native'])['Mounts']
            need(len([m for m in source_mounts if m['Destination']=='/data' and m['Type']=='volume'])==1,'source native data is not one known named volume')
            source_volume=next(m['Name'] for m in source_mounts if m['Destination']=='/data')
            commands={
                'archive':('test ! -e /backup/native.tar; exec tar -C /source -cpf /backup/native.tar .', ['--mount','type=volume,source='+source_volume+',target=/source,readonly','--mount',f'type=bind,source={backup},target=/backup']),
                'unpack':('test -z "$(find /restore -mindepth 1 -print -quit)"; exec tar -C /restore -xpf /backup/native.tar .', ['--mount','type=volume,source='+vols['objects']+',target=/restore','--mount',f'type=bind,source={backup},target=/backup,readonly']),
                'compare':('cd /source; find . -type f -exec sha256sum {} \\; | sort > /tmp/source; cd /restore; find . -type f -exec sha256sum {} \\; | sort > /tmp/restore; cmp /tmp/source /tmp/restore; cat /tmp/source', ['--mount','type=volume,source='+source_volume+',target=/source,readonly','--mount','type=volume,source='+vols['objects']+',target=/restore,readonly'])}
            for stage,(script,mounts) in commands.items():
                utilities[stage]=a.create('checkpoint-'+stage,group,['--name',prefix+'-'+stage,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m',*mounts,'--entrypoint','/bin/sh',p1.PG,'-ec',script])
            state['groups'].append({'group':group,'scope':scope,'source':{k:g[k] for k in ('Core','Native','Network')},'gateway':gateway_cid,'restore_core':cores['checkpoint'],'rehearsal_core':cores['rehearsal'],
                'restore_native':restored,'restore_gateway':restore_gateway_cid,'restore_network':network,'volumes':vols,'helpers':helpers,'cases':cases,'root_owners':roots,'maintenance':maintenance,'rehearsals':rehearsals,'utilities':utilities,'source_volume':source_volume,'recovery_redis':redis,
                'budget_initializer':budget_initializer,'budget_initializer_script_sha256':sha(budget_script),'budget_expected_mounted_cids':sorted([*helpers.values(),budget_initializer]),
                'known_old_p1_absence':'b2d74d75-ed07-4b0a-b530-6c5d308cf1b8','profiles':'NOT_MATERIALIZED','private':str(folder)})
        save(PUBLIC/'six-map.json',mapping);save(CONTROL/'six-map.json',mapping)
        offline=CONTROL/'offline';offline.mkdir()
        state['offline_validator']=a.create('offline-candidate-validator','ALL',['--name','dpp307-'+GENERATION+'-offline','--network','none','--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
            '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={offline},target=/run/private,readonly','--mount',f'type=bind,source={CONTROL}/operator,target=/run/operator,readonly','--entrypoint','/run/operator',p1.PG,'-stage','check-candidate'])
        save(PUBLIC/'owner-descriptors.json',{cid:project_owner(inspect(cid)) for cid in a.owners})
        state['owner_descriptors_sha256']=sha(PUBLIC/'owner-descriptors.json')
        state['map_sha256']=sha(PUBLIC/'six-map.json');state['owners']=a.owners;state['resources']=a.resources
        save(PUBLIC/'allocation.json',state);save(CONTROL/'allocation.json',state)
    except BaseException as error:
        failure={'type':type(error).__name__,'message':str(error)}
        raise
    finally:
        unresolved=[]
        # A CLI failure after daemon-side create must not lose the known owner.
        # Resolve only these exact prior approved names and this full execution.
        for name in a.pending:
            try:
                item=inspect(name)
                need(item['Name']=='/'+name and item['Config']['Labels'].get('lifecycle307.remaining')==EXECUTION,'pending allocation owner differs')
                if item['Id'] not in a.owners:a.owners.append(item['Id'])
            except Exception:
                unresolved.append(name)
        closure=[p1.close_known(cid) for cid in reversed(a.owners)]
        save(PUBLIC/'allocation-closed.json',{'owners':closure,'unconfirmed_names':unresolved,'failure':failure,'native_delete':0,'provider_io':0,'resources_retained':True,'cleanup':0})
    need(not unresolved and all(row['closed'] for row in closure),'allocation containment remains unconfirmed')
    print(json.dumps({'status':state['status'],'execution':EXECUTION,'owners':len(a.owners),'native_delete':0}))


def main():
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['allocate']);args=parser.parse_args()
    allocate()


if __name__=='__main__':main()
