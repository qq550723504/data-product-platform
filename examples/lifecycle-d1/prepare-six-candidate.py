"""Finite candidate13 preparation. Allocate stopped owners; never send/enable DELETE.

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
        diagnostic=p1.PRIVATE/'post-v22-six-preparation01';diagnostic.mkdir(exist_ok=True)
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
    return {'status':'PASS_RETAINED_NINETY_CLOSED','closed_cids':sorted(ids),'prior':base,'native_delete':0}


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
    need(PROFILE=='candidate13/v23' and EXECUTION=='4e524c04-2376-4bc9-9e85-452909d3dba4','only the new proposed execution is supported')
    need(not CONTROL.exists() and not PUBLIC.exists(),'one-shot preparation already allocated; retain and review')
    # Verify retained history before allocating any owner or volume.
    retained=historical_gate()
    CONTROL.mkdir();PUBLIC.mkdir();save(PUBLIC/'retained-before.json',retained)
    a=Allocation();failure=None
    try:
        source=build()
        old=load(p1.PACKET/'candidate.json');scopes=load(ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
        state={'status':'STOPPED_ALLOCATION_NOT_NATIVE_ACCEPTANCE','source':source,'profile':PROFILE,'execution':EXECUTION,'allocation_nonce':a.nonce,
               'window':W,'retained':retained,'binaries':{n:sha(CONTROL/n) for n in ('reader','operator','owner','readonly-check.exe')},'groups':[],'native_delete':0,'authorization':'NOT_RECEIVED','schedule':'NOT_CREATED'}
        mapping={'Execution':EXECUTION,'Profile':PROFILE,'Groups':[]}
        for n,scope in enumerate(scopes):
            group=scope['Group'];g=next(x for x in old['Groups'] if x['Group']==group)
            folder=CONTROL/group;folder.mkdir();outputs=folder/'outputs';outputs.mkdir();backup=folder/'checkpoint';backup.mkdir()
            project='dpp-lifecycle307-'+scope['Run'][:8];prefix=project+'-'+GENERATION
            config=load(p1.PRIVATE/project/'config.json')
            need(all(closed(inspect(g[role])) for role in ('Core','Native','Gateway')),'source not closed')
            pg_env=backup/'postgres.env';pg_env.write_text('POSTGRES_USER=checkpoint_admin\nPOSTGRES_DB=postgres\nPOSTGRES_PASSWORD='+secrets.token_hex(32)+'\n',encoding='utf-8')
            subnet='10.246.'+str(190+n)+'.0/28'
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
