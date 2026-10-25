"""Fixed post-v22 complete read-only audit. No enable/delete/business command.

prepare allocates stopped, separately reviewed GET/HEAD-only maintenance owners.
run activates only their bounded read profile, preserves all raw channels, and
closes every attempted source/helper. Old authorizations/windows are not loaded.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
CONTROL = Path('C:/Users/Henry/.codex/fixture-private/post-v22-seventeen01')
PUBLIC = ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-seventeen01'
GROUPS = ('business-races', 'late-hold', 'mixed')
COUNTS = (7, 6, 4)
P1 = '5a4c9789-3e67-4e54-ab58-128d111cbd65'
ACTIVATION = {'Enabled', 'NotBefore', 'ExpiresAt'}


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, ROOT/'examples/lifecycle-d1'/file)
    value = importlib.util.module_from_spec(spec); spec.loader.exec_module(value)
    return value


closure = module('seventeen_old_closure', 'close-old-business-dispatches.py')
p1 = closure.p1
native = p1.native_module()
old_verifier = module('seventeen_old_verifier', 'verify-remaining-native.py')
need, load, sha, save = p1.need, p1.load, p1.sha, p1.write
docker, inspect, closed, hostpath = p1.docker, p1.inspect, p1.closed, p1.hostpath


def now(): return dt.datetime.now(dt.timezone.utc)


def source_files():
    # All tracked Go build inputs plus all host composition modules, not a
    # hand-maintained subset of runtime imports. Evidence is independently bound.
    paths = p1.call(['git', 'ls-files', 'apps/platform', 'examples/lifecycle-d1']).decode('utf-8', 'strict').splitlines()
    return {name: sha(ROOT/name) for name in paths
            if name.endswith(('.go', '.py', 'go.mod', 'go.sum', '.json', '.sql'))}


def budget_contract(volume, users, group, binding, execution):
    need(volume['Name'] == binding['budget_volume'] and volume['Labels'].get('lifecycle307.remaining') == execution
         and len(users) == len(set(users)) and set(users) == set(group['Helpers'].values()), 'old budget owner/mount membership expanded')
    return {'volume':volume['Name'],'owner_execution':execution,'mounted_cids':sorted(users)}


def retained():
    old = closure.audit.retained(p1)
    prior = closure.prior_preparation_closed()
    c, bindings = closure.packet()
    owners = closure.execution_owners(c, bindings)
    public = ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03'
    private = p1.PRIVATE/'post-v22-business-closure03'
    need(sha(public/'terminal.json') == '3ce05b24b298718b63dcd2c553c993b6f40a387c3a0eb26e13fda695ed5692cc', 'trusted closure terminal drift')
    need(sha(public/'preparation.json') == 'b6b92f1e87714799293f1cf68486e48802650741f985f11278aa60ed867c2684', 'trusted closure preparation drift')
    manifest = load(public/'preparation.json')
    projection = load(public/'disabled-observer-profile-projection.json')
    profile = private/'worker-private/closure.json'
    need(load(profile)['Enabled'] is False and sha(profile) == projection['private_full_profile_sha256'], 'trusted closure observer enabled or changed')
    current = {}
    for kind, owner in manifest['owners'].items():
        need(closed(closure.own_contract(owner, manifest)), 'trusted closure owner active')
        current[kind] = owner['cid']
    ids = set(old['owner_ids']) | set(owners) | set(current.values())
    for value in prior.values():
        ids.update(value.get('closed_cids', []))
        if 'closed_cid' in value: ids.add(value['closed_cid'])
    need(len(ids) == 87 and all(closed(inspect(cid)) for cid in ids), 'retained 87 owner set differs')
    networks, budgets = [], []
    for g,b in zip(c['Groups'],bindings):
        old_verifier.baseline(p1.PACKET,g)
        closure.source_contract({'source':g})
        need(g['CheckpointCore'] != g['Core'] and g['CheckpointNative'] != g['Native']
             and closed(inspect(g['CheckpointCore'])) and closed(inspect(g['CheckpointNative'])), 'restore aliases source or active')
        network = json.loads(docker('network','inspect',g['Network']))[0]
        need(network['Internal'] is True and not network.get('Containers'), 'source network not closed/internal')
        networks.append(g['Network'])
        volume = json.loads(docker('volume','inspect',b['budget_volume']))[0]
        users = p1.text('ps','-aq','--no-trunc','--filter','volume='+b['budget_volume']).split()
        budgets.append(budget_contract(volume,users,g,b,c['Execution']))
    return {'old_retained':old,'prior_failed_preparation':prior,'old_execution_owners':owners,
            'trusted_closure_owners':current,'trusted_closure_profile_sha256':sha(profile),
            'all_87_closed_cids':sorted(ids),'internal_networks':networks,'old_budget_mounts':budgets}


def owner_contract(owner, allow_running=False):
    x = inspect(owner['cid']); h = x['HostConfig']
    mounts = {m['Destination']:m for m in x['Mounts']}
    need(x['Id'] == owner['cid'] and x['Image'] == p1.PG and x['Config']['User'] == '10001:10001'
         and h['ReadonlyRootfs'] and not h['Privileged'] and not h.get('AutoRemove')
         and h['CapDrop'] == ['ALL'] and 'no-new-privileges' in h['SecurityOpt']
         and h['RestartPolicy']['Name'] == 'no' and not h.get('PortBindings')
         and not any(x['NetworkSettings']['Ports'].values())
         and h.get('Tmpfs') == {'/tmp':'','/var/lib/postgresql/data':'size=1m'}
         and h['NetworkMode'] == 'container:'+owner['Native']
         and x['Config']['Entrypoint'] == ['/run/reader'] and x['Config']['Cmd'] == ['-mode','post-v22-verify-native']
         and len(x['Mounts']) == 3 and set(mounts) == {'/run/private','/run/reader','/run/output'}, 'new readonly owner contract expanded')
    for target,path,rw in [('/run/private',owner['private'],False),('/run/reader',owner['binary'],False),('/run/output',owner['output'],True)]:
        m = mounts[target]
        need(m['Type'] == 'bind' and hostpath(m['Source']) == Path(path).resolve() and m['RW'] is rw, 'new readonly mount differs')
    folder = Path(owner['private']); entries = list(folder.iterdir())
    need({f.name for f in entries} == {'post-v22-readonly.json','frozen-inventory.json','present-inventory.json'}
         and all(f.is_file() and not f.is_symlink() and not f.is_junction() for f in entries), 'readonly private directory expanded or aliased')
    need(sha(Path(owner['binary'])) == owner['binary_sha256'], 'readonly binary drift')
    if not allow_running: need(closed(x), 'readonly owner not stopped')
    return x


def prepare_inner():
    need(not p1.call(['git','diff','--name-only']).strip() and not p1.call(['git','diff','--cached','--name-only']).strip(), 'commit reviewed sources first')
    CONTROL.mkdir(exist_ok=False); PUBLIC.mkdir(exist_ok=True)
    need(not (PUBLIC/'preparation.json').exists() and not (PUBLIC/'terminal.json').exists(),'public audit already allocated')
    p1.CONTROL = CONTROL
    preservation = retained()
    candidate, bindings = closure.packet()
    env = os.environ.copy(); env.update(GOARCH='amd64',CGO_ENABLED='0')
    for system,name,command in [('linux','reader','lifecycle-fixture-reader'),('windows','checker.exe','lifecycle-fixture-readonly-check')]:
        env['GOOS'] = system
        built = subprocess.run(['go','build','-o',str(CONTROL/name),'./cmd/'+command],cwd=ROOT/'apps/platform',env=env,capture_output=True)
        with (CONTROL/(name+'-build.private')).open('xb') as f: f.write(built.stdout+built.stderr)
        need(built.returncode == 0, 'readonly build failed; private diagnostics retained')
    audit_id = str(uuid.uuid4()); owners = {}
    for g,b,count in zip(candidate['Groups'],bindings,COUNTS):
        group = g['Group']; need(group == GROUPS[len(owners)], 'group order drift')
        private, output = CONTROL/group, PUBLIC/group/'readonly'
        private.mkdir(); output.mkdir(parents=True)
        old = p1.PRIVATE/'remaining-executable03'/group/'source-verifier'
        frozen = (old/'inventory.json').read_bytes()
        need(hashlib.sha256(frozen).hexdigest() == load(p1.PACKET/group/'readonly/native-verification.json')['inventory_sha256'], 'frozen inventory differs')
        present = [row for row in json.loads(frozen) if row['Copy']['ID'] != P1]
        need(len(present) == count and [row['Copy'] for row in json.loads(frozen)] == g['Copies'], 'present copies differ')
        with (private/'frozen-inventory.json').open('xb') as f: f.write(frozen)
        save(private/'present-inventory.json',present)
        base = load(old/'remaining.json')['Base']
        profile = dict(Enabled=False,AuditID=audit_id,Purpose='POST_V22_SEVENTEEN_NATIVE_READONLY',Group=group,
            CandidateSHA256=sha(p1.PACKET/'candidate.json'),BinarySHA256=sha(CONTROL/'reader'),
            Core=g['Core'],Native=g['Native'],Network=g['Network'],Endpoint='127.0.0.1:9000',RootAccess=base['RootAccess'],RootSecret=base['RootSecret'],
            FrozenInventorySHA256=sha(private/'frozen-inventory.json'),PresentInventorySHA256=sha(private/'present-inventory.json'),
            NotBefore='0001-01-01T00:00:00Z',ExpiresAt='0001-01-01T00:00:00Z',MaxProviderDeletes=0)
        save(private/'post-v22-readonly.json',profile)
        save(CONTROL/(group+'-disabled-original.json'),profile)
        cid = p1.text('create','--pull','never','--name','dpp307-seventeen-'+audit_id[:8]+'-'+group,'--label','lifecycle307.readonly17='+audit_id,
            '--network','container:'+g['Native'],'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
            '--restart','no','--pids-limit','64','--memory','256m','--cpus','1','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m',
            '--mount','type=bind,source='+str(private)+',target=/run/private,readonly',
            '--mount','type=bind,source='+str(CONTROL/'reader')+',target=/run/reader,readonly',
            '--mount','type=bind,source='+str(output)+',target=/run/output','--entrypoint','/run/reader',p1.PG,'-mode','post-v22-verify-native')
        owner = dict(cid=cid,Core=g['Core'],Native=g['Native'],Network=g['Network'],private=str(private),output=str(output),binary=str(CONTROL/'reader'),binary_sha256=sha(CONTROL/'reader'))
        # Preserve a CID immediately, before any inspection can fail.
        save(CONTROL/(group+'-allocated-owner.json'),owner)
        owner_contract(owner)
        owners[group] = owner
    manifest = dict(status='PREPARED_DISABLED_GET_HEAD_ONLY',audit_id=audit_id,source_head=p1.call(['git','rev-parse','HEAD']).decode().strip(),
        source_files=source_files(),preservation=preservation,owners=owners,checker_sha256=sha(CONTROL/'checker.exe'),activation_fields=sorted(ACTIVATION),
        inputs={f.relative_to(CONTROL).as_posix():sha(f) for f in CONTROL.rglob('*') if f.is_file() and f.suffix!='.private'},
        native_delete=0,business_mutations=0,send_authority='NONE',old_authorization_reused=False)
    save(CONTROL/'manifest.json',manifest);save(PUBLIC/'preparation.json',manifest)
    print(json.dumps({'status':manifest['status'],'audit_id':audit_id,'manifest_sha256':sha(CONTROL/'manifest.json'),'source_started':False}),flush=True)


def prepare():
    need(not CONTROL.exists(),'one-shot private audit already allocated')
    try: prepare_inner()
    except Exception as error:
        owners=[load(p)['cid'] for p in CONTROL.glob('*-allocated-owner.json')]
        failure={'status':'FAILED_PREPARATION_RETAINED','error':type(error).__name__+': '+str(error),
                 'allocated_owners':owners,'physical_close':close_all(owners),'source_started':False,'native_delete':0}
        if CONTROL.exists(): save(CONTROL/'preparation-failure.json',failure)
        if PUBLIC.exists(): save(PUBLIC/'preparation-failure.json',failure)
        raise


def fresh(manifest):
    need(p1.call(['git','rev-parse','HEAD']).decode().strip() == manifest['source_head']
         and not p1.call(['git','diff','--name-only']).strip() and not p1.call(['git','diff','--cached','--name-only']).strip(), 'readonly source HEAD/tracked drift')
    need(source_files() == manifest['source_files'], 'readonly source bytes drift')
    for name,digest in manifest['inputs'].items(): need(sha(CONTROL/name) == digest, 'readonly immutable preparation input drift')
    need(retained() == manifest['preservation'], 'retained owners/controls/backups/budgets drift')
    for owner in manifest['owners'].values(): owner_contract(owner)


def replace_profile(path, value):
    # Intentional lifecycle projection only; original bytes remain immutable.
    temporary = path.with_suffix('.next')
    with temporary.open('x',encoding='utf-8',newline='\n') as f: json.dump(value,f,indent=2);f.write('\n')
    os.replace(temporary,path)


def close_all(ids):
    # Reuse the accepted containment contract: unreadable inspect still tries
    # stop for this exact known CID, then reads back and continues all others.
    return [p1.close_known(cid) for cid in reversed(list(dict.fromkeys(ids)))]


def collect(manifest, group, not_before, expires_at):
    owner=manifest['owners'][group];folder=PUBLIC/group;private=Path(owner['private']);output=Path(owner['output'])
    profile_path=private/'post-v22-readonly.json';original=load(profile_path)
    need(original == load(CONTROL/(group+'-disabled-original.json')) and original['Enabled'] is False and original['MaxProviderDeletes']==0, 'readonly profile modified')
    started=now()
    need(not_before<=started<expires_at and dt.timedelta(0)<expires_at-not_before<=dt.timedelta(minutes=55),'shared readonly audit deadline expired or expanded')
    active=dict(original,Enabled=True,NotBefore=not_before.isoformat(),ExpiresAt=expires_at.isoformat())
    owned=[];failure=None;result=None;before=None;channels={};phase='wait'
    try:
        source=next(g for g in load(p1.PACKET/'candidate.json')['Groups'] if g['Group']==group)
        closure.source_contract({'source':source})
        for field in ('Core','Native'):
            owned.append(source[field]);docker('start',source[field])
        native.wait_pg(source['Core'],'fixture');time.sleep(3)
        before=native.fingerprints(source['Core']);save(folder/'Core-before.json',before)
        baseline = (ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json' if group=='business-races' else p1.PACKET/group/'core-readonly.json')
        if group=='business-races': need(sha(baseline)=='74e45ae37d0dd4e998e461624dcddfffa80116a08cf3234a7e00dd0ad1fce6f8','trusted Core baseline drift')
        need(before==load(baseline),'current Core differs from authoritative post-closure checkpoint')
        network=json.loads(docker('network','inspect',owner['Network']))[0]
        need(network['Internal'] and set(network['Containers'])=={owner['Core'],owner['Native']},'active source network expanded')
        owner_contract(owner)
        need(now()<expires_at,'shared readonly audit deadline expired before helper start')
        replace_profile(profile_path,active)
        expectation = dict(audit_id=manifest['audit_id'],profile_sha256=sha(profile_path),binary_sha256=owner['binary_sha256'],candidate_sha256=active['CandidateSHA256'],
            frozen_inventory_sha256=active['FrozenInventorySHA256'],present_inventory_sha256=active['PresentInventorySHA256'],core=owner['Core'],native=owner['Native'],network=owner['Network'],not_before=active['NotBefore'],expires_at=active['ExpiresAt'])
        save(folder/'expectation.json',expectation)
        owned.append(owner['cid']);docker('start',owner['cid'])
        waited=subprocess.run(['docker','wait',owner['cid']],capture_output=True,timeout=780)
        channels['wait-stdout'],channels['wait-stderr']=waited.stdout,waited.stderr
        need(waited.returncode==0 and waited.stdout.strip()==b'0','readonly helper failed')
        x=owner_contract(owner)
        begin=dt.datetime.fromisoformat(x['State']['StartedAt'].replace('Z','+00:00'));end=dt.datetime.fromisoformat(x['State']['FinishedAt'].replace('Z','+00:00'))
        need(started<=begin<=end<=now() and now()-end<=dt.timedelta(minutes=1),'helper not fresh')
        save(folder/'owner.json',dict(cid=owner['cid'],binary_sha256=owner['binary_sha256'],started_at=x['State']['StartedAt'],finished_at=x['State']['FinishedAt'],exit=x['State']['ExitCode'],closed=closed(x)))
        phase='checker'
        checked=subprocess.run([str(CONTROL/'checker.exe'),'-post-v22','-baseline',str(p1.PACKET/group/'readonly'),'-fresh',str(output),'-group',group,
            '-expectation',str(folder/'expectation.json'),'-inventory',str(private/'present-inventory.json')],capture_output=True,timeout=45)
        channels['checker-stdout'],channels['checker-stderr']=checked.stdout,checked.stderr
        need(checked.returncode==0,'post-v22 complete file aggregation rejected')
        result=json.loads(checked.stdout)
        need(result['status']=='PASS_COMPLETE_POST_V22_NATIVE_AGGREGATION' and result['copies']==COUNTS[GROUPS.index(group)],'offline result membership differs')
        result.update(technical_requests=old_verifier.journals(output),amount='NOT_ASSESSED',native_delete=0,business_mutations=0)
    except subprocess.TimeoutExpired as error:
        channels[phase+'-stdout'],channels[phase+'-stderr']=error.stdout or b'',error.stderr or b''
        failure='TimeoutExpired: '+phase+' result unconfirmed'
    except Exception as error:
        failure=type(error).__name__+': '+str(error)
    finally:
        # Capture raw logs regardless of wait/inspection outcome, then attempt
        # every physical close. A failed profile revoke cannot skip shutdown.
        try:
            logs=subprocess.run(['docker','logs',owner['cid']],capture_output=True,timeout=30)
            channels['stdout'],channels['stderr']=logs.stdout,logs.stderr
            if logs.returncode: failure=failure or 'owner logs unconfirmed'
        except subprocess.TimeoutExpired as error:
            channels['stdout'],channels['stderr']=error.stdout or b'',error.stderr or b''
            failure=failure or 'owner logs timeout; partial channels retained'
        except Exception: failure=failure or 'owner logs unavailable'
        try: replace_profile(profile_path,original)
        except Exception: failure=failure or 'profile revocation unconfirmed'
        # Stop the helper before the final Core observation; never let failure
        # skip source readback or the other physical containment attempts.
        helper_close=close_all([owner['cid']])
        after=None
        if before is not None:
            try:
                after=native.fingerprints(owner['Core']);save(folder/'Core-after.json',after)
                if after!=before: failure=failure or 'Core changed during readonly audit'
            except Exception: failure=failure or 'final Core observation unavailable'
        physical=helper_close+close_all([cid for cid in owned if cid!=owner['cid']])
        if not all(x['closed'] for x in physical): failure=failure or 'physical closure unconfirmed'
        for name,raw in channels.items():
            with (folder/(name+'.txt')).open('xb') as f:f.write(raw)
        save(folder/'physical-close.json',{'at':now().isoformat(),'owners':physical,'all_closed':all(x['closed'] for x in physical),'profiles_restored':load(profile_path)==original if profile_path.exists() else False,'resources_retained':True,'cleanup':0})
        if failure: save(folder/'failure.json',{'error':failure,'at':now().isoformat(),'native_delete':0,'business_mutations':0})
    need(failure is None,failure or 'readonly failure')
    need(result is not None and before is not None and after==before,'complete Core/native result missing')
    save(folder/'aggregation.json',result)
    return result


def run(review_path, review_sha, manifest_sha):
    p1.CONTROL=CONTROL
    need(not (PUBLIC/'terminal.json').exists() and not (CONTROL/'terminal.json').exists(),'one-shot audit already terminal')
    need(sha(CONTROL/'manifest.json')==manifest_sha==sha(PUBLIC/'preparation.json'),'readonly preparation binding differs')
    manifest=load(CONTROL/'manifest.json')
    need(sha(review_path)==review_sha,'independent review bytes differ')
    review=load(review_path)
    need(review['status']=='PASS_BOUNDED_INDEPENDENT_REVIEW' and review['remaining_blockers']==[]
         and review['manifest_sha256']==manifest_sha and review['source_head']==manifest['source_head'],'readonly review not closed for this exact preparation')
    results=[];attempted=[];failure=None;terminal=None
    try:
        not_before=now();expires_at=not_before+dt.timedelta(minutes=55)
        save(PUBLIC/'shared-readonly-window.json',{'audit_id':manifest['audit_id'],'not_before':not_before.isoformat(),'expires_at':expires_at.isoformat(),'max_provider_deletes':0,'send_authority':'NONE'})
        fresh(manifest)
        for group in GROUPS:
            attempted.append(group)
            results.append(collect(manifest,group,not_before,expires_at))
        need(retained()==manifest['preservation'],'post-read retained facts drift')
        terminal={'status':'PASS_COMPLETE_POST_V22_SEVENTEEN_READONLY','audit_id':manifest['audit_id'],'versions':17,'remaining_targets':6,'survivors':11,'groups':results,
            'technical_requests':sum(r['technical_requests'] for r in results),'amount':'NOT_ASSESSED','native_delete':0,'business_mutations':0,'old_authorization_reused':False,'D1':'NOT_ACCEPTED','six_candidate':'NOT_FROZEN'}
    except Exception as error:
        failure=type(error).__name__+': '+str(error)
        terminal={'status':'FAILED_READONLY_RETAINED','audit_id':manifest['audit_id'],'error':failure,'completed_groups':[r['group'] for r in results],
            'failed_groups':[g for g in attempted if g not in [r['group'] for r in results]],
            'not_run_groups':[g for g in GROUPS if g not in attempted],'native_delete':0,'business_mutations':0,'D1':'NOT_ACCEPTED','six_candidate':'NOT_FROZEN'}
    finally:
        profiles=[]
        for group,owner in manifest['owners'].items():
            try:
                original=load(CONTROL/(group+'-disabled-original.json'))
                need(original['Enabled'] is False,'disabled profile original is active')
                replace_profile(Path(owner['private'])/'post-v22-readonly.json',original)
                need(load(Path(owner['private'])/'post-v22-readonly.json')==original,'profile revoke readback differs')
                profiles.append({'group':group,'disabled_readback':True})
            except Exception as error:
                profiles.append({'group':group,'disabled_readback':False,'error':type(error).__name__+': '+str(error)})
        physical=close_all([cid for owner in manifest['owners'].values() for cid in (owner['Core'],owner['Native'],owner['cid'])])
        if not all(x['closed'] for x in physical) or not all(p['disabled_readback'] for p in profiles):
            failure=failure or 'final containment unconfirmed';terminal['status']='UNCONFIRMED_READONLY_RETAINED'
        journals={}
        for group,owner in manifest['owners'].items():
            directory=Path(owner['output'])
            starts=sorted(p.name for p in directory.glob('readonly-start-*.json'))
            observations=sorted(p.name for p in directory.glob('readonly-observation-*.json'))
            journals[group]={'start_facts':len(starts),'observation_facts':len(observations),'amount':'NOT_ASSESSED',
                             'unobserved_attempt_ids':sorted(set(p[len('readonly-start-'):-5] for p in starts)-set(p[len('readonly-observation-'):-5] for p in observations))}
        terminal.update(at=now().isoformat(),attempted_groups=attempted,physical_close=physical,disabled_profiles=profiles,request_journals=journals,resources_retained=True,manifest_sha256=manifest_sha,independent_review_sha256=review_sha)
        save(PUBLIC/'terminal.json',terminal);save(CONTROL/'terminal.json',terminal)
        index={p.relative_to(PUBLIC).as_posix():sha(p) for p in PUBLIC.rglob('*') if p.is_file()}
        save(PUBLIC/'execution-evidence-index01.json',index)
    print(json.dumps(terminal),flush=True)
    need(failure is None,failure or 'readonly audit failed')


def main():
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','run'])
    parser.add_argument('--review',type=Path);parser.add_argument('--review-sha256');parser.add_argument('--manifest-sha256')
    args=parser.parse_args()
    if args.action=='prepare': prepare()
    else:
        need(args.review and args.review_sha256 and args.manifest_sha256,'exact review and manifest bindings required')
        run(args.review.resolve(),args.review_sha256,args.manifest_sha256)


if __name__=='__main__':main()
