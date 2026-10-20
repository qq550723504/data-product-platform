"""The candidate16 complete read-only entry: cold gate, then actual SDK/PG.

No IAM enable, send permission, DELETE, migration or retry mode exists. A new
bounded read owner is allocated per group and closed on every outcome. Frozen
case/recovery semantics and actual topology/budgets are checked before sources.
"""
import argparse
import datetime as dt
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import time
import uuid

spec=importlib.util.spec_from_file_location('six_complete_runtime',Path(__file__).with_name('six_candidate_runtime.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
p=r.p
import evidence_indexes
need,load,sha,save=r.need,r.load,r.sha,r.save
PACKET=r.ROOT/'docs/acceptance/evidence/issue307-d1'/('remaining-'+p.GENERATION)


def private_files(folder,allowed,required):
    entries=list(folder.iterdir());names={x.name for x in entries}
    need(not folder.is_symlink() and not folder.is_junction() and names<=set(allowed) and set(required)<=names
         and all(x.is_file() and not x.is_symlink() and not x.is_junction() for x in entries),'private mount membership expanded or aliased')


def require_unfinished_execution():
    terminal=r.CONTROL/'authorized-execution01/terminal.json'
    for component in (r.CONTROL,terminal.parent,terminal):
        need(not component.is_symlink() and not component.is_junction(),'terminal record or authorization directory is aliased')
    try:
        terminal.stat()
    except FileNotFoundError:
        return
    # Permission/I/O failures propagate; an unreadable terminal is not absence.
    raise ValueError('execution already terminal; saved-evidence audit only')


def sealed_gate(packet=PACKET):
    # Actual case/recovery bytes are checked before permission or Docker I/O.
    p.window.validate_packet(packet,r.CONTROL)
    candidate=load(packet/'candidate.json');bindings=load(packet/'bindings.json')
    need(candidate['Execution']==p.EXECUTION and candidate['Profile']==p.PROFILE,'current candidate scope differs')
    a=r.state();r.cold_topology(a)
    need(all(b['allocation_sha256']==sha(r.PUBLIC/'allocation.json') and b['owner_descriptors_sha256']==a['owner_descriptors_sha256'] for b in bindings),'frozen allocation or descriptor binding differs')
    common=dict(Enabled=False,Execution=p.EXECUTION,CandidateSHA256=sha(packet/'candidate.json'),ConfirmationSHA256=sha(packet/'confirmation-template.txt'),
                NotBefore=candidate['NotBefore'],ExpiresAt=candidate['NotAfter'],ConfirmedAt=r.ZERO,MaxPerTarget=1,MaxTotal=6)
    count=0
    for g,b in zip(candidate['Groups'],bindings):
        need(g['Group']==b['group'],'group order differs');folder=r.CONTROL/g['Group']
        allocation=next(x for x in a['groups'] if x['group']==g['Group'])
        need(g['Helpers']==allocation['helpers'] and g['CaseHelpers']==allocation['cases'] and g['Gateway']==allocation['gateway'],'executable CID set differs from reviewed allocation')
        for name,key in [('reader','ReaderBinarySHA256'),('operator','SenderBinarySHA256'),('owner','OwnerBinarySHA256')]:need(sha(r.CONTROL/name)==candidate[key],'frozen binary differs')
        for stage in g['Helpers']:
            owner=folder/stage
            required={'operator.json','candidate.json','permission.json','confirmation-template.txt'}|({'recovery.json'} if stage.startswith('recover-') else set())
            private_files(owner,required,required)
            need(load(owner/'permission.json')==dict(common,Profile=p.PROFILE,BinarySHA256=candidate['SenderBinarySHA256']),'operator permission differs or enabled')
            need(sha(owner/'operator.json')==g['ConfigSHA256'][stage] and (owner/'candidate.json').read_bytes()==(packet/'candidate.json').read_bytes()
                 and (owner/'confirmation-template.txt').read_bytes()==(packet/'confirmation-template.txt').read_bytes(),'operator candidate/config/template drift')
            cfg=load(owner/'operator.json')
            need(not any(k in cfg for k in ('RootAccess','RootSecret')) and (not stage.startswith('recover-') or (not cfg['DeleteSecret'] and not cfg['WirePrivateKey'])),'worker credential scope expanded')
            count+=1
        for action in b['root_owners']:
            owner=folder/('root-'+action)
            need({x.name for x in owner.iterdir()}=={'root.json','iam.json','policies','candidate.json','permission.json','confirmation-template.txt','status.json'}
                 and all(not x.is_symlink() and not x.is_junction() for x in owner.iterdir()),'root private directory expanded or aliased')
            need(load(owner/'permission.json')==common,'root permission differs or enabled')
            need(sha(owner/'root.json')==b['root_owners'][action]['root_config_sha256'] and (owner/'candidate.json').read_bytes()==(packet/'candidate.json').read_bytes(),'root private config differs')
            iam=load(owner/'iam.json')
            need(sha(owner/'iam.json')==g['IAMSHA256'] and {x.name for x in (owner/'policies').iterdir()}=={n+'.json' for n in iam['policy_file_sha256']},'root policy member set differs')
            for n,digest in iam['policy_file_sha256'].items():need(sha(owner/'policies'/(n+'.json'))==digest,'root frozen policy file differs')
            count+=1
        for stage in g['CaseHelpers']:
            owner=folder/('case-'+stage);private_files(owner,{'remaining.json','manifest.json'},{'remaining.json','manifest.json'})
            cfg=load(owner/'remaining.json')
            need(sha(owner/'manifest.json')==b['manifest_sha256'] and cfg['Base']['ManifestSHA256']==b['manifest_sha256']
                 and not any(cfg['Base'].get(k) for k in ('RootAccess','RootSecret','ProducerAccess','ProducerSecret')),'case manifest/credential scope expanded')
        for name,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:need(sha(folder/'checkpoint'/name)==g[key],'new retained backup drift')
        need(sha(packet/g['Group']/'core-readonly.json')==b['source_core_sha256'],'Core checkpoint anchor drift')
        volume=b['budget_volume'];need(volume==allocation['volumes']['budget'],'new budget identity differs')
        p.sequence.budget_archive(r.docker('cp',g['Helpers']['wire']+':/run/budget','-'),candidate,g['Group'],initial=True)
        gateway=r.inspect(g['Gateway']);mount=next(m for m in gateway['Mounts'] if m['Destination']=='/etc/caddy/Caddyfile')
        need(not mount['RW'] and sha(p.p1.hostpath(mount['Source']))==g['SealedGatewaySHA256'],'gateway not actually sealed')
    need(count==30,'finite permission count differs')
    return {'status':'PASS_COMPLETE_SIX_SEALED_HOST_GATE','cases':6,'recoveries':6,'permissions_false':count,'owners_closed':len(a['owners']),
            'backup_hashes':9,'old_backups_retained':18,'budgets_empty':3,'native_delete':0,'source_provider_io':0}


def collect(g,b,packet,private,output,deadline):
    private.mkdir(parents=True,exist_ok=False);output.mkdir(parents=True,exist_ok=False)
    (output/'files').mkdir()
    a=load(r.CONTROL/'allocation.json');allocation=next(x for x in a['groups'] if x['group']==g['Group'])
    old_source=next(x for x in load(p.p1.PACKET/'candidate.json')['Groups'] if x['Group']==g['Group'])
    cfg=r.profile(allocation,'SOURCE','SIX_TARGET_PREPARATION_NATIVE_READONLY',str(uuid.uuid4()))
    start=r.now().replace(microsecond=0)
    need(start<deadline and deadline-start<=dt.timedelta(minutes=55),'complete readonly deadline expired')
    cfg.update(Enabled=True,NotBefore=start.isoformat().replace('+00:00','Z'),ExpiresAt=deadline.isoformat().replace('+00:00','Z'))
    for name in ('frozen-inventory.json','present-inventory.json'):r.write_raw(private/name,(r.CONTROL/g['Group']/name).read_bytes())
    r.write_raw(private/'six-map.json',(r.CONTROL/'six-map.json').read_bytes());save(private/'six-readonly.json',cfg)
    cid=None;known=[];failure=None;name='dpp307-six-read-'+cfg['AuditID'];before=None;creation_attempted=False;unknown_owner=None
    try:
        p.prior.closure.source_contract({'source':old_source})
        for ident in (g['Core'],g['Native']):known.append(ident);r.docker('start',ident)
        p.native.wait_pg(g['Core'],'fixture');time.sleep(3)
        before=p.native.fingerprints(g['Core']);save(output/'Core-before.json',before)
        need(before==load(packet/g['Group']/'core-readonly.json'),'Core differs from frozen source checkpoint')
        n=json.loads(r.docker('network','inspect',g['Network']))[0]
        need(n['Internal'] and set(n['Containers'])=={g['Core'],g['Native']},'active source network expanded')
        creation_attempted=True
        allocated=r.docker('create','--pull','never','--name',name,'--label','lifecycle307.six-readonly='+p.EXECUTION,
                     '--network','container:'+g['Native'],'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
                     '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m',
                     '--mount',f'type=bind,source={private},target=/run/private,readonly','--mount',f'type=bind,source={r.CONTROL}/reader,target=/run/reader,readonly',
                     '--mount',f'type=bind,source={output}/files,target=/run/output','--entrypoint','/run/reader',p.p1.PG,'-mode','six-verify-native').decode('utf-8','strict').strip()
        need(len(allocated)==64 and all(c in '0123456789abcdef' for c in allocated),'full read owner CID required')
        cid=allocated;known.append(cid);save(output/'allocated-owner.json',{'cid':cid,'name':name,'profile_sha256':sha(private/'six-readonly.json'),'native_delete':0})
        x=r.inspect(cid);r.unprivileged(x,'/run/reader',['-mode','six-verify-native'],g['Native'])
        r.mounts_exact(x,{'/run/private':('bind',private,False),'/run/reader':('bind',r.CONTROL/'reader',False),'/run/output':('bind',output/'files',True)})
        private_files(private,{'six-readonly.json','six-map.json','frozen-inventory.json','present-inventory.json'},{'six-readonly.json','six-map.json','frozen-inventory.json','present-inventory.json'})
        r.run_owner(cid,output/'operation')
        expectation=dict(audit_id=cfg['AuditID'],profile_sha256=sha(private/'six-readonly.json'),binary_sha256=cfg['BinarySHA256'],candidate_sha256=cfg['CandidateSHA256'],frozen_inventory_sha256=cfg['FrozenInventorySHA256'],
                         present_inventory_sha256=cfg['PresentInventorySHA256'],core=g['Core'],native=g['Native'],network=g['Network'],not_before=cfg['NotBefore'],expires_at=cfg['ExpiresAt'],Execution=p.EXECUTION,Profile=p.PROFILE,Role='SOURCE')
        save(output/'expectation.json',expectation)
        completed=subprocess.run([str(r.CONTROL/'readonly-check.exe'),'-six','-baseline',str(packet/g['Group']/'expectation'),'-fresh',str(output/'files'),'-group',g['Group'],
                                  '-expectation',str(output/'expectation.json'),'-inventory',str(private/'present-inventory.json'),'-mapping',str(private/'six-map.json')],capture_output=True,timeout=45)
        r.write_raw(output/'checker-stdout.txt',completed.stdout);r.write_raw(output/'checker-stderr.txt',completed.stderr)
        need(completed.returncode==0,'complete six readonly aggregation denied')
        result=json.loads(completed.stdout);need(result['status']=='PASS_COMPLETE_SIX_NATIVE_AGGREGATION','complete six result missing')
        result.update(technical_requests=p.prior.old_verifier.journals(output/'files'),amount='NOT_ASSESSED')
        save(output/'result.json',result)
        return result
    except BaseException as error:
        failure={'type':type(error).__name__,'message':str(error)}
        raise
    finally:
        # Unknown create outcome cannot lose knowledge of the exact allocated
        # name. Only this execution label may be adopted for containment.
        if cid is None and creation_attempted:
            try:
                x=r.inspect(name)
                need(x['Name']=='/'+name and x['Config']['Labels'].get('lifecycle307.six-readonly')==p.EXECUTION
                     and len(x['Id'])==64 and all(c in '0123456789abcdef' for c in x['Id']),'uncertain temporary owner name/label differs')
                known.append(x['Id'])
            except Exception as error:unknown_owner={'name':name,'state':'UNCONFIRMED','error_type':type(error).__name__}
        if cid is not None:p.p1.close_known(cid)
        try:
            if before is not None:
                after=p.native.fingerprints(g['Core']);save(output/'Core-after.json',after)
                need(after==before,'Core changed during complete readonly verification')
        except Exception as error:failure=failure or {'type':type(error).__name__,'message':str(error)}
        try:
            cfg['Enabled']=False;r.replace(private/'six-readonly.json',cfg)
        except Exception as error:failure=failure or {'type':type(error).__name__,'message':'read profile revoke: '+str(error)}
        closure=[p.p1.close_known(ident) for ident in reversed(list(dict.fromkeys(known)))]
        all_closed=unknown_owner is None and all(x['closed'] for x in closure)
        save(output/'closure.json',{'owners':closure,'failure':failure,'unknown_temporary_owner':unknown_owner,'native_delete':0,'all_closed':all_closed})
        need(failure is None and all_closed,'complete verification failed or closure unconfirmed')


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--candidate-sha256',required=True);parser.add_argument('--bindings-sha256',required=True);parser.add_argument('--checker-sha256',required=True);parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--evidence-audit-plan',type=Path,required=True)
    parser.add_argument('--evidence-audit-plan-sha256',required=True)
    parser.add_argument('--audit-only',action='store_true',help='Audit saved evidence only; never inspect Docker or start native preflight')
    args=parser.parse_args()
    need(sha(PACKET/'candidate.json')==args.candidate_sha256 and sha(PACKET/'bindings.json')==args.bindings_sha256 and sha(r.CONTROL/'readonly-check.exe')==args.checker_sha256,'exact frozen invocation bindings differ')
    output=args.output.resolve();need(not output.exists() and output.is_relative_to((r.ROOT/'docs/acceptance/evidence/issue307-d1').resolve()),'new complete output must be an evidence subdirectory')
    audit=evidence_indexes.audit_plan(r.ROOT,args.evidence_audit_plan,args.evidence_audit_plan_sha256,PACKET/'candidate.json')
    if args.audit_only:
        output.mkdir();save(output/'evidence-audit.json',audit)
        print(json.dumps({'status':'PASS_SAVED_EVIDENCE_AUDIT_ONLY','execution':audit['execution'],'output':str(output),'native_preflight':'NOT_RUN','execution_authority':False,'native_delete':0}))
        return
    require_unfinished_execution()
    gate=sealed_gate();output.mkdir();save(output/'evidence-audit.json',audit)
    private=r.CONTROL/('complete-'+uuid.uuid4().hex);private.mkdir()
    candidate=load(PACKET/'candidate.json');bindings=load(PACKET/'bindings.json');results=[];deadline=r.now().replace(microsecond=0)+dt.timedelta(minutes=50)
    for g,b in zip(candidate['Groups'],bindings):results.append(collect(g,b,PACKET,private/g['Group'],output/g['Group'],deadline))
    need([x['copies'] for x in results]==[7,6,4],'complete current present membership differs')
    save(output/'complete.json',{'status':'PASS_COMPLETE_SIX_NATIVE_ENTRY','execution':p.EXECUTION,'evidence_audit':audit,'host_gate':gate,'groups':results,'present_versions':17,'historical_copies':18,'targets':6,'survivors':11,
                                  'technical_requests':sum(x['technical_requests'] for x in results),'native_delete':0,'business_mutations':0,'amount':'NOT_ASSESSED'})
    print(json.dumps({'status':'PASS_COMPLETE_SIX_NATIVE_ENTRY','output':str(output),'native_delete':0}))


if __name__=='__main__':main()
