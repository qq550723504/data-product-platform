"""Finite exact-two maintenance composition, outside Core.

The file-only check is the default. Live entry requires separately supplied,
signed human-reference grants. This module never creates those grants, unlocks
GitHub, changes IAM, releases pins, resets budgets or calls a DELETE API.
"""
import argparse
import base64
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

import evidence_indexes as indexes
import execution_freeze as freeze
import fixture_artifacts as artifacts
import retention_maintenance as host
import retirement_catalog as history
import release_binding as release

ROOT=Path(__file__).resolve().parents[2]
PRIVATE_BASE=Path.home()/'.codex/fixture-private/retention-maintenance-v2'
GROUPS=('business-races','late-hold','mixed')
SCHEMA='EXACT_TWO_RETENTION_COMPOSITION_V2'
RELEASE_SCHEMA='EXACT_TWO_RETENTION_COMPOSITION_MERGED_V3'
READ_KIND='EXACT_TWO_RETENTION_READONLY_AUTHORITY_V2'
need=host.need
now=lambda:dt.datetime.now(dt.timezone.utc)


def file_bytes(spec):
    need(type(spec) is dict and set(spec)=={'path','sha256','bytes'},'COMPOSITION_FILE_FIELDS')
    indexes.metadata({k:spec[k] for k in ('sha256','bytes')})
    path=indexes.unlinked_absolute(spec['path'])
    need(path.is_file() and path.stat().st_nlink==1,'COMPOSITION_FILE_CUSTODY')
    raw=path.read_bytes()
    need(host.sha(raw)==spec['sha256'] and len(raw)==spec['bytes'],'COMPOSITION_FILE_DRIFT')
    return raw


def save(path,value):
    path=Path(path)
    with path.open('xb') as stream:
        stream.write(host.canonical(value)+b'\n');stream.flush()
        os.fsync(stream.fileno())


def preserve(path,raw):
    """Archive original bytes once, including failed controls; never rewrite."""
    path=Path(path)
    if not path.exists():
        with path.open('xb') as stream:stream.write(raw);stream.flush();os.fsync(stream.fileno())
    need(indexes.unlinked_absolute(path).is_file() and path.stat().st_nlink==1 and path.read_bytes()==raw,'COMPOSITION_ARCHIVE_DRIFT')


def replace_control(path,raw,archive):
    path=indexes.unlinked_absolute(path)
    need(path.is_file() and path.stat().st_nlink==1,'COMPOSITION_CONTROL_CUSTODY')
    preserve(archive,path.read_bytes())
    # Existing Linux mount files keep their original uid/mode; replacing their
    # inode from Windows would silently change that custody contract.
    with path.open('r+b') as stream:stream.seek(0);stream.write(raw);stream.truncate();stream.flush();os.fsync(stream.fileno())
    need(path.read_bytes()==raw,'COMPOSITION_CONTROL_READBACK')


def checked_anchor(raw,expected_sha):
    need(host.sha(raw)==expected_sha,'COMPOSITION_ANCHOR_SHA256')
    anchor=indexes.decode(raw)
    need(set(anchor)=={'schema','MaintenanceID','bindings_sha256','public_key','controls','close_cids','readers'} and anchor['schema']=='EXACT_TWO_RETENTION_CONTAINMENT_V2' and
         str(uuid.UUID(anchor['MaintenanceID']))==anchor['MaintenanceID'] and uuid.UUID(anchor['MaintenanceID']).int!=0,'COMPOSITION_ANCHOR_IDENTITY')
    need(type(anchor['bindings_sha256']) is str and len(anchor['bindings_sha256'])==64 and all(c in '0123456789abcdef' for c in anchor['bindings_sha256']),'COMPOSITION_ANCHOR_BINDING')
    need(type(anchor['controls']) is list and len(anchor['controls'])==2 and len(set(anchor['controls']))==2 and
         all(type(p) is str and Path(p).is_absolute() and Path(p).name=='permission.json' for p in anchor['controls']),'COMPOSITION_ANCHOR_CONTROLS')
    ids=anchor['close_cids']
    need(type(ids) is list and len(ids)==9 and len(set(ids))==9 and
         all(type(cid) is str and len(cid)==64 and all(c in '0123456789abcdef' for c in cid) for cid in ids),'COMPOSITION_ANCHOR_CIDS')
    need(len(base64.b64decode(anchor['public_key'],validate=True))==32,'COMPOSITION_ANCHOR_PUBLIC_KEY')
    need(type(anchor['readers']) is list and [(r['phase'],r['group']) for r in anchor['readers']]==[(phase,group) for phase in ('before','after') for group in GROUPS],'COMPOSITION_ANCHOR_READERS')
    for reader in anchor['readers']:
        need(set(reader)=={'phase','group','audit_id','name','native','intent','registration','closure','profile'},'COMPOSITION_ANCHOR_READER_FIELDS')
        audit=str(uuid.uuid5(uuid.UUID(anchor['MaintenanceID']),'retention-readonly/'+reader['phase']+'/'+reader['group']))
        need(reader['audit_id']==audit and reader['name']=='dpp307-six-read-'+audit and reader['native'] in ids,'COMPOSITION_ANCHOR_READER_IDENTITY')
        for field in ('intent','registration','closure','profile'):need(type(reader[field]) is str and Path(reader[field]).is_absolute(),'COMPOSITION_ANCHOR_READER_PATH')
    return anchor


def signed_fact(raw,public):
    wrapper=indexes.decode(raw)
    need(set(wrapper)=={'Body','Signature'},'COMPOSITION_SIGNED_FACT_FIELDS')
    body=base64.b64decode(wrapper['Body'],validate=True)
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    Ed25519PublicKey.from_public_bytes(public).verify(base64.b64decode(wrapper['Signature'],validate=True),body)
    return indexes.decode(body)


def present(path):
    try:path=indexes.unlinked_absolute(path);info=path.stat()
    except FileNotFoundError:return False
    need(path.is_file() and info.st_nlink==1,'COMPOSITION_CONTROL_FACT_CUSTODY')
    return True


def contain(anchor,ops,archive):
    """Only a SHA-pinned closure plan is needed, even if runtime inputs drift.

    Closure never constructs a source/provider client. Failed archiving or an
    unreadable control does not veto an attempted false control and all stops.
    Such a round remains UNCONFIRMED even when physical stops succeed.
    """
    results=[]
    # The fixed six names/nonces/paths are approved before any create. A signed
    # CID registration is durable before reader start and is sufficient to try
    # that known CID even if later inspect or the main packet is unreadable.
    public=base64.b64decode(anchor['public_key'],validate=True)
    for reader in anchor['readers']:
        failures=[];cid=None;attempted=False
        expected=dict(maintenance_id=anchor['MaintenanceID'],bindings_sha256=anchor['bindings_sha256'],phase=reader['phase'],group=reader['group'],audit_id=reader['audit_id'],name=reader['name'],native=reader['native'])
        try:
            attempted=present(reader['intent'])
            if attempted:
                intent=signed_fact(Path(reader['intent']).read_bytes(),public)
                need(intent==dict(expected,kind='MAINTENANCE_READ_CREATE_INTENT_V2'),'COMPOSITION_READER_INTENT_DRIFT')
        except Exception as error:failures.append(type(error).__name__)
        try:
            # An independently valid CID fact must still drive stop when the
            # earlier intent is missing/corrupt. Keep that defect UNCONFIRMED.
            if present(reader['registration']):
                fact=signed_fact(Path(reader['registration']).read_bytes(),public)
                need(set(fact)==set(expected)|{'kind','cid','descriptor_sha256'} and all(fact[k]==v for k,v in expected.items()) and fact['kind']=='MAINTENANCE_READ_CREATED_V2','COMPOSITION_READER_REGISTRATION_DRIFT')
                candidate=fact['cid'];need(type(candidate) is str and len(candidate)==64 and all(c in '0123456789abcdef' for c in candidate),'COMPOSITION_READER_CID')
                need(type(fact['descriptor_sha256']) is str and len(fact['descriptor_sha256'])==64 and all(ch in '0123456789abcdef' for ch in fact['descriptor_sha256']),'COMPOSITION_READER_DESCRIPTOR')
                cid=candidate
                if not attempted:failures.append('MISSING_CREATE_INTENT')
            elif attempted:failures.append('CREATE_OUTCOME_WITHOUT_TRUSTED_CID')
        except Exception as error:failures.append(type(error).__name__)
        try:
            if present(reader['profile']):
                path=Path(reader['profile']);raw=path.read_bytes();profile=indexes.decode(raw)
                try:preserve(Path(archive)/('reader-profile-'+reader['audit_id']+'-'+host.sha(raw)+'.json'),raw)
                except Exception as error:failures.append(type(error).__name__)
                need(profile.get('AuditID')==reader['audit_id'] and profile.get('MaintenanceID')==anchor['MaintenanceID'],'COMPOSITION_READER_PROFILE_DRIFT')
                profile['Enabled']=False
                with path.open('r+b') as stream:stream.seek(0);stream.write(host.canonical(profile)+b'\n');stream.truncate();stream.flush();os.fsync(stream.fileno())
                need(host.load(path)['Enabled'] is False,'COMPOSITION_READER_REVOKE_FAILED')
        except Exception as error:failures.append(type(error).__name__)
        if cid is not None:
            try:need(ops.stop(cid),'COMPOSITION_READER_STOP_UNCONFIRMED')
            except Exception as error:failures.append(type(error).__name__)
        # A closure fact cannot erase an unknown create or missing registration.
        if attempted or cid is not None:
            try:
                closure=host.load(Path(reader['closure']))
                need(closure['all_closed'] is True and closure['unknown_temporary_owner'] is None,'COMPOSITION_READER_PRIOR_CLOSURE_UNCONFIRMED')
            except Exception as error:failures.append(type(error).__name__)
        results.append(dict(reader=reader['name'],cid=cid,closed=not failures,errors=failures))
    for number,entry in enumerate(anchor['controls']):
        failures=[];path=Path(entry)
        try:
            raw=indexes.unlinked_absolute(path).read_bytes()
            need(path.is_file() and path.stat().st_nlink==1,'COMPOSITION_CONTROL_CUSTODY')
            preserve(Path(archive)/('permission-before-seal-'+str(number)+'-'+host.sha(raw)+'.json'),raw)
        except Exception as error:failures.append(type(error).__name__)
        try:
            value=host.load(path)
            need(set(value)=={'Enabled','BindingsSHA256'} and value['BindingsSHA256']==anchor['bindings_sha256'] and type(value['Enabled']) is bool,'COMPOSITION_CONTROL_DRIFT')
        except Exception as error:failures.append(type(error).__name__)
        try:
            # The pinned anchor specifies only this disabled, exact binding.
            target=indexes.unlinked_absolute(path)
            need(target.is_file() and target.stat().st_nlink==1,'COMPOSITION_CONTROL_CUSTODY')
            raw=host.canonical(dict(Enabled=False,BindingsSHA256=anchor['bindings_sha256']))+b'\n'
            with target.open('r+b') as stream:stream.seek(0);stream.write(raw);stream.truncate();stream.flush();os.fsync(stream.fileno())
            need(host.load(target)==dict(Enabled=False,BindingsSHA256=anchor['bindings_sha256']),'COMPOSITION_REVOKE_FAILED')
        except Exception as error:failures.append(type(error).__name__)
        results.append(dict(permission=str(path),permission_closed=not failures,errors=failures))
    for cid in anchor['close_cids']:
        try:results.append(dict(cid=cid,closed=ops.stop(cid)))
        except Exception as error:results.append(dict(cid=cid,closed=False,error=type(error).__name__))
    return dict(status='PASS_MAINTENANCE_PHYSICAL_CLOSURE' if all(r.get('closed',r.get('permission_closed',False)) for r in results) else 'UNCONFIRMED_MAINTENANCE_PHYSICAL_CLOSURE',results=results,source_start=0,provider_delete=0,protection_write=0)


def known_sources():
    document=indexes.decode(indexes.checked_bytes(history.SOURCE_PACKET,history.SOURCE_PACKET_SHA256))
    return {g['Group']:g for g in document['Groups']}


class NativeOperations:
    """Reuse the published SDK/Core collector, Docker CLI and history audit."""
    def __init__(self,control):
        self.control=control
        spec=importlib.util.spec_from_file_location('maintenance_native_collect',Path(__file__).with_name('verify-six-native.py'))
        self.collector=importlib.util.module_from_spec(spec);spec.loader.exec_module(self.collector)

    def cold(self):return history.verify_history()
    def inspect(self,cid):return host.inspect(cid)
    def start(self,cid):return host.docker('start',cid)
    def stop(self,cid):
        # Inspect failure does not prevent an exact known-CID stop attempt.
        try:
            if host.closed(self.inspect(cid)):return True
        except Exception:pass
        try:host.docker('stop','--time','10',cid)
        except Exception:pass
        return host.closed(self.inspect(cid))

    def branch(self,packet):
        journal=self.control/('github-'+uuid.uuid4().hex);journal.mkdir()
        if packet['schema']==RELEASE_SCHEMA:
            result=release.check(ROOT,packet['release_source'],release.Client(journal))
            freeze.save(journal/'snapshot.json',result['snapshot'])
            return result
        client=freeze.GitHub(journal);snapshot=client.snapshot()
        freeze.ready(snapshot,packet['review_head'],require_frozen=True)
        need(snapshot['rule']['id']==packet['rule_id'] and
             host.sha(host.canonical(snapshot['protection']))==packet['protection_sha256'],'COMPOSITION_BRANCH_PROTECTION_DRIFT')
        freeze.save(journal/'snapshot.json',snapshot)
        source=artifacts.host_snapshot(ROOT)
        need(source['source']==packet['review_head'],'COMPOSITION_LOCAL_SOURCE_DRIFT')
        query='query { repository(owner:"qq550723504",name:"data-product-platform") { pullRequest(number:332) { reviewThreads(first:100) { pageInfo { hasNextPage } nodes { isResolved } } } } }'
        threads=client.graph(query)['repository']['pullRequest']['reviewThreads']
        need(not threads['pageInfo']['hasNextPage'] and all(row['isResolved'] for row in threads['nodes']),'COMPOSITION_UNRESOLVED_OR_INCOMPLETE_REVIEW_THREADS')
        freeze.save(journal/'review-threads.json',threads)
        return snapshot

    def read(self,group,context,destination,deadline):
        g=context['source']
        return self.collector.collect(g,None,ROOT,destination/'private',destination/'public',deadline,
                                      core_baseline=Path(context['core_checkpoint']),maintenance=context)

    def wait_ready(self,b,h,group,deadline):
        item=h['owners'][group];cid=item['cid'];path=Path(item['output'])/'wait-ready.json'
        while now()<deadline:
            row=self.inspect(cid)
            need(row['State']['Running'] and row['State']['Pid']>0,'COMPOSITION_WRITER_EXITED_BEFORE_READY')
            if path.exists():return host.load(path)
            time.sleep(.1)
        raise ValueError('COMPOSITION_WAIT_READY_TIMEOUT')

    def proof(self,b,h,group,key_path,at):return host.attest(b,h,group,key_path,at)
    def prepare_writer(self,b,h,group,source):
        self.collector.p.prior.closure.source_contract({'source':source})
        for cid in h['closed_cids']:need(host.closed(self.inspect(cid)),'COMPOSITION_OLD_OWNER_ACTIVE')
        for owner in b['Owners']:
            need(host.closed(self.inspect(owner['CID'])),'COMPOSITION_OTHER_WRITER_ACTIVE')
            item=h['owners'][owner['Group']];private=Path(item['private'])
            need((private/'bindings.json').read_bytes()==h['_bindings_raw'] and
                 (private/'approved-proposal.json').read_bytes()==host.proposal.plan_bytes(b['RetainUntil'],b['NotAfter']),'COMPOSITION_MOUNTED_BINDINGS_PLAN_DRIFT')
        host.budget_state(b,h,group)
    def wait(self,cid,output):
        failure=None
        try:
            completed=subprocess.run(['docker','wait',cid],capture_output=True,timeout=50)
            for suffix,raw in [('stdout',completed.stdout),('stderr',completed.stderr)]:
                with (output/('wait-'+suffix)).open('xb') as stream:stream.write(raw)
            need(completed.returncode==0 and completed.stdout.strip()==b'0','COMPOSITION_WRITER_FAILED_OR_UNKNOWN')
        except subprocess.TimeoutExpired as error:
            failure=error
            for suffix,raw in [('stdout',error.stdout or b''),('stderr',error.stderr or b'')]:
                with (output/('wait-'+suffix)).open('xb') as stream:stream.write(raw)
            raise
        finally:
            # Logs may contain only the fixed worker's generic rejection; keep
            # raw channels private. A logging failure is not successful closure.
            result=subprocess.run(['docker','logs',cid],capture_output=True,timeout=20)
            for suffix,raw in [('stdout',result.stdout),('stderr',result.stderr)]:
                with (output/('worker-'+suffix)).open('xb') as stream:stream.write(raw)
            need(result.returncode==0 and failure is None,'COMPOSITION_WRITER_LOGS_OR_WAIT_UNCONFIRMED')
        row=self.inspect(cid)
        need(host.closed(row) and row['State']['ExitCode']==0,'COMPOSITION_WRITER_NOT_TRUSTED_CLOSED')


class Composition:
    def __init__(self,packet,packet_sha,ops=None,clock=now):
        need(host.sha(host.canonical(packet))==packet_sha,'COMPOSITION_CANONICAL_MANIFEST_DRIFT')
        common={'schema','bindings','host','public_key','private_key','reader','checker','control','sources','anchor'}
        need(packet['schema'] in (SCHEMA,RELEASE_SCHEMA),'COMPOSITION_SCHEMA')
        required=common|({'release_source'} if packet['schema']==RELEASE_SCHEMA else {'review_head','rule_id','protection_sha256'})
        need(set(packet)==required,'COMPOSITION_PACKET_FIELDS')
        if packet['schema']==RELEASE_SCHEMA:release.load_source(ROOT,packet['release_source'])
        self.packet,self.packet_sha,self.clock=packet,packet_sha,clock
        self.b_raw=file_bytes(packet['bindings']);self.b=indexes.decode(self.b_raw)
        self.h=indexes.decode(file_bytes(packet['host']));self.h['_bindings_raw']=self.b_raw
        host.fixed_bindings(self.b,self.h)
        self.release_scope()
        self.control=indexes.unlinked_absolute(packet['control'])
        need(self.control.name==self.b['MaintenanceID'] and self.control.parent==indexes.unlinked_absolute(PRIVATE_BASE),'COMPOSITION_CONTROL_IDENTITY')
        self.public=file_bytes(packet['public_key'])
        need(len(self.public)==32 and host.sha(self.public)==self.b['HostPublicSHA256'],'COMPOSITION_PUBLIC_KEY_DRIFT')
        self.key=indexes.unlinked_absolute(packet['private_key'])
        need(self.key.is_file() and self.key.stat().st_nlink==1 and self.key.is_relative_to(self.control),'COMPOSITION_PRIVATE_KEY_CUSTODY')
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
        need(Ed25519PrivateKey.from_private_bytes(self.key.read_bytes()).public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)==self.public,'COMPOSITION_PRIVATE_KEY_DRIFT')
        for item in self.h['owners'].values():
            for role in ('private','output'):
                need(not self.key.is_relative_to(indexes.unlinked_absolute(item[role])),'COMPOSITION_PRIVATE_KEY_MOUNTED')
        self.reader=Path(packet['reader']['path']);self.checker=Path(packet['checker']['path'])
        file_bytes(packet['reader']);file_bytes(packet['checker'])
        self.sources=packet['sources'];known=known_sources()
        need([g['Group'] for g in self.sources]==list(GROUPS),'COMPOSITION_SOURCE_MEMBERSHIP')
        for g in self.sources:
            need(set(g)=={'Group','source','config','core_checkpoint','frozen_inventory','present_inventory','before_inventory','after_inventory','expectation','expectation_files'},'COMPOSITION_SOURCE_FIELDS')
            need(g['source']==known[g['Group']],'COMPOSITION_FIXED_SOURCE_DESCRIPTOR')
            for field in ('config','core_checkpoint','frozen_inventory','present_inventory','before_inventory','after_inventory'):file_bytes(g[field])
            expected=indexes.unlinked_absolute(g['expectation'])
            need({p.name for p in expected.iterdir()}==set(g['expectation_files']),'COMPOSITION_EXPECTATION_MEMBERSHIP')
            for name,attributes in g['expectation_files'].items():
                need('/' not in name and '\\' not in name,'COMPOSITION_EXPECTATION_PATH')
                file_bytes(dict(path=str(expected/name),**attributes))
        self.anchor=checked_anchor(file_bytes(packet['anchor']),packet['anchor']['sha256'])
        need(self.anchor['MaintenanceID']==self.b['MaintenanceID'] and self.anchor['bindings_sha256']==self.h['bindings_sha256'] and base64.b64decode(self.anchor['public_key'],validate=True)==self.public,'COMPOSITION_ANCHOR_BINDING')
        source_ids=[row['source'][role] for row in self.sources for role in ('Core','Native')]
        required={*[o['CID'] for o in self.b['Owners']],*source_ids,history.BUSINESS_REDIS}
        need(set(self.anchor['close_cids'])==required and len(self.anchor['close_cids'])==len(required),'COMPOSITION_ANCHOR_CLOSURE_SET')
        need(self.anchor['controls']==[str(Path(self.h['owners'][g]['private'])/'permission.json') for g in host.proposal.TARGETS],'COMPOSITION_ANCHOR_CONTROL_SET')
        for reader in self.anchor['readers']:
            destination=self.control/'run'/reader['phase']/reader['group']
            need(reader['native']==known[reader['group']]['Native'] and reader['intent']==str(destination/'reader-create-intent.json') and reader['registration']==str(destination/'reader-created.json') and
                 reader['closure']==str(destination/'public/closure.json') and reader['profile']==str(destination/'private/retention-readonly.json'),'COMPOSITION_ANCHOR_READER_PATH_BINDING')
        self.ops=ops or NativeOperations(self.control)
        self.activated=False

    def release_scope(self):
        if self.packet['schema']==RELEASE_SCHEMA:
            # No Go format change: both signed grants bind the existing
            # immutable BudgetID through BindingsSHA. No self-referencing hash.
            release.load_source(ROOT,self.packet['release_source'])
            need(self.b['BudgetID']==release.maintenance_budget_id(self.b['MaintenanceID'],self.packet['release_source']['sha256']),
                 'COMPOSITION_RELEASE_READ_WRITE_BINDING_DRIFT')

    def grant(self,path,phase,human_path):
        self.release_scope()
        wrapper=host.load(path)
        need(set(wrapper)=={'Body','Signature'},'COMPOSITION_SIGNED_GRANT_FIELDS')
        raw=base64.b64decode(wrapper['Body'],validate=True);signature=base64.b64decode(wrapper['Signature'],validate=True)
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
        Ed25519PublicKey.from_public_bytes(self.public).verify(signature,raw)
        human_file=indexes.unlinked_absolute(human_path)
        need(human_file.is_file() and human_file.stat().st_nlink==1,'COMPOSITION_HUMAN_CUSTODY')
        body=indexes.decode(raw);human=human_file.read_bytes();at=self.clock()
        if phase=='read':
            need(set(body)=={'Kind','MaintenanceID','BindingsSHA256','ManifestSHA256','HumanMessageSHA256','NotBefore','NotAfter','ConfirmedAt','SourceStart','Phases','MaxProtectionWrites','MaxProviderDeletes','OldAuthorizationReused'},'COMPOSITION_READ_AUTHORITY_FIELDS')
            need(body['Kind']==READ_KIND and body['ManifestSHA256']==self.packet_sha and body['SourceStart'] is True and body['Phases']==['before','after'] and
                 type(body['MaxProtectionWrites']) is int and body['MaxProtectionWrites']==0 and type(body['MaxProviderDeletes']) is int and body['MaxProviderDeletes']==0,'COMPOSITION_READ_AUTHORITY_SCOPE')
            start,end=map(host.proposal.protection.instant,(body['NotBefore'],body['NotAfter']))
            need(start>=host.proposal.protection.instant(self.b['NotBefore'])-dt.timedelta(minutes=15) and end<=host.proposal.protection.instant(self.b['NotAfter'])+dt.timedelta(hours=2),'COMPOSITION_READ_WINDOW_EXPANSION')
        else:
            need(phase=='write','COMPOSITION_AUTHORITY_PHASE')
            need(set(body)=={'Kind','MaintenanceID','BindingsSHA256','HumanMessageSHA256','NotBefore','NotAfter','ConfirmedAt','MaxPerTarget','MaxTotal','OldAuthorizationReused','RetainUntil'},'COMPOSITION_WRITE_AUTHORITY_FIELDS')
            need(body['Kind']==host.KIND and body['RetainUntil']==self.b['RetainUntil'] and body['NotBefore']==self.b['NotBefore'] and body['NotAfter']==self.b['NotAfter'] and
                 type(body['MaxPerTarget']) is int and body['MaxPerTarget']==1 and type(body['MaxTotal']) is int and body['MaxTotal']==2,'COMPOSITION_WRITE_AUTHORITY_SCOPE')
            start,end=map(host.proposal.protection.instant,(self.b['NotBefore'],self.b['NotAfter']))
        need(human and body['HumanMessageSHA256']==host.sha(human) and body['MaintenanceID']==self.b['MaintenanceID'] and body['BindingsSHA256']==self.h['bindings_sha256'] and body['OldAuthorizationReused'] is False and
             start<=at<end and host.proposal.protection.instant(body['ConfirmedAt'])<=at,'COMPOSITION_NEW_AUTHORITY_OR_WINDOW')
        return body,wrapper,human

    def read_phase(self,phase,read_authority,human_path,output):
        grant,_,_=self.grant(read_authority,'read',human_path)
        self.ops.branch(self.packet)
        results=[]
        deadline=min(self.clock()+dt.timedelta(minutes=50),host.proposal.protection.instant(grant['NotAfter']))
        need(self.clock()<deadline,'COMPOSITION_READ_DEADLINE')
        for g in self.sources:
            self.revalidate()
            self.grant(read_authority,'read',human_path)
            cfg=indexes.decode(file_bytes(g['config']))
            need(set(cfg)=={'Group','Endpoint','RootAccess','RootSecret'} and cfg['Group']==g['Group'] and cfg['Endpoint']=='127.0.0.1:9000' and cfg['RootAccess'] and cfg['RootSecret'],'COMPOSITION_READ_CONFIG')
            source=g['source']
            reader=next(r for r in self.anchor['readers'] if r['phase']==phase and r['group']==g['Group'])
            profile=dict(Enabled=False,AuditID=reader['audit_id'],Purpose='RETENTION_MAINTENANCE_NATIVE_READONLY',Group=g['Group'],CandidateSHA256=self.h['bindings_sha256'],
                BinarySHA256=self.packet['reader']['sha256'],Core=source['Core'],Native=source['Native'],Network=source['Network'],Endpoint=cfg['Endpoint'],RootAccess=cfg['RootAccess'],RootSecret=cfg['RootSecret'],
                FrozenInventorySHA256=g['frozen_inventory']['sha256'],PresentInventorySHA256=g['present_inventory']['sha256'],NotBefore=self.b['NotBefore'],ExpiresAt=self.b['NotAfter'],MaxProviderDeletes=0,
                MaintenanceID=self.b['MaintenanceID'],Phase=phase,BindingsSHA256=self.h['bindings_sha256'],EffectiveInventorySHA256=g[phase+'_inventory']['sha256'],RetainUntil=self.b['RetainUntil'],MaintenanceEnd=self.b['NotAfter'])
            context=dict(source=source,profile=profile,reader=str(self.reader),checker=str(self.checker),core_checkpoint=g['core_checkpoint']['path'],expectation=g['expectation'],
                inputs={'frozen-inventory.json':g['frozen_inventory']['path'],'present-inventory.json':g['present_inventory']['path'],'effective-inventory.json':g[phase+'_inventory']['path']})
            destination=output/g['Group'];destination.mkdir(exist_ok=False)
            fact=dict(maintenance_id=self.b['MaintenanceID'],bindings_sha256=self.h['bindings_sha256'],phase=phase,group=g['Group'],audit_id=reader['audit_id'],name=reader['name'],native=source['Native'])
            def signed_save(path,value):
                from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
                from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
                raw=host.canonical(value);key=Ed25519PrivateKey.from_private_bytes(self.key.read_bytes())
                need(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)==self.public,'COMPOSITION_REGISTRATION_KEY_DRIFT')
                save(path,dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()))
            context['record_creation']=lambda:signed_save(Path(reader['intent']),dict(fact,kind='MAINTENANCE_READ_CREATE_INTENT_V2'))
            context['register_owner']=lambda cid,descriptor:signed_save(Path(reader['registration']),dict(fact,kind='MAINTENANCE_READ_CREATED_V2',cid=cid,descriptor_sha256=descriptor))
            self.activated=True
            result=self.ops.read(g['Group'],context,destination,deadline)
            need(result['status']=='PASS_COMPLETE_RETENTION_MAINTENANCE_NATIVE_AGGREGATION' and result['copies']=={'business-races':7,'late-hold':6,'mixed':4}[g['Group']] and
                 result['maintenance_id']==self.b['MaintenanceID'] and result['bindings_sha256']==self.h['bindings_sha256'] and result['phase']==phase and
                 result['retain_until']==self.b['RetainUntil'] and result['maintenance_end']==self.b['NotAfter'],'COMPOSITION_FULL_NATIVE_PHASE_NOT_PASS')
            results.append(result)
        save(output/'complete.json',dict(status='PASS_MAINTENANCE_SEVENTEEN_READONLY',phase=phase,maintenance_id=self.b['MaintenanceID'],bindings_sha256=self.h['bindings_sha256'],groups=results,versions=17,provider_delete=0,protection_write=0))
        return results

    def permission(self,group,enabled):
        path=Path(self.h['owners'][group]['private'])/'permission.json';raw=path.read_bytes();value=indexes.decode(raw)
        need(set(value)=={'Enabled','BindingsSHA256'} and value['BindingsSHA256']==self.h['bindings_sha256'] and type(value['Enabled']) is bool,'COMPOSITION_PERMISSION_DRIFT')
        archive=self.control/('permission-'+group+'-'+host.sha(raw)+'.json')
        value['Enabled']=enabled;replace_control(path,host.canonical(value)+b'\n',archive)
        need(host.load(path)==value,'COMPOSITION_PERMISSION_READBACK')

    def writer(self,group,read_authority,read_human,write_authority,write_human,output):
        self.revalidate()
        self.grant(read_authority,'read',read_human);_,wrapper,human=self.grant(write_authority,'write',write_human)
        self.ops.branch(self.packet)
        item=self.h['owners'][group];owner=next(o for o in self.b['Owners'] if o['Group']==group)
        need(host.closed(self.ops.inspect(owner['CID'])) and host.closed(self.ops.inspect(owner['Native'])),'COMPOSITION_WRITER_OR_SOURCE_ALREADY_ACTIVE')
        self.ops.prepare_writer(self.b,self.h,group,next(g['source'] for g in self.sources if g['Group']==group))
        need(not list(Path(item['output']).iterdir()),'COMPOSITION_PREVIOUS_OUTPUT_NO_RESTART')
        for other in self.h['owners'].values():
            need(host.load(Path(other['private'])/'permission.json')==dict(Enabled=False,BindingsSHA256=self.h['bindings_sha256']),'COMPOSITION_PERMISSION_ALREADY_ENABLED')
        self.permission(group,False)
        private=Path(item['private'])
        # These are separately preserved actual human bytes and signed machine
        # authority. Neither this entry nor the worker signs a human grant.
        replace_control(private/'user-message-verbatim.txt',human,self.control/('original-human-'+group))
        replace_control(private/'authorization.json',host.canonical(wrapper)+b'\n',self.control/('original-authority-'+group))
        self.activated=True;self.ops.start(owner['Native'])
        self.grant(read_authority,'read',read_human);self.grant(write_authority,'write',write_human)
        self.ops.start(owner['CID'])
        ready_limit=min(self.clock()+dt.timedelta(seconds=20),host.proposal.protection.instant(self.b['NotAfter']))
        self.ops.wait_ready(self.b,self.h,group,ready_limit)
        self.grant(write_authority,'write',write_human)
        proof=self.ops.proof(self.b,self.h,group,self.key,self.clock())
        replace_control(private/'topology.json',host.canonical(proof)+b'\n',self.control/('original-topology-'+group))
        self.grant(read_authority,'read',read_human);self.grant(write_authority,'write',write_human)
        proof_at=host.proposal.protection.instant(indexes.decode(base64.b64decode(proof['Body'],validate=True))['At'])
        need(dt.timedelta(0)<=self.clock()-proof_at<=dt.timedelta(minutes=1),'COMPOSITION_PROOF_EXPIRED_BEFORE_ENABLE')
        self.permission(group,True)
        try:self.ops.wait(owner['CID'],output)
        finally:self.permission(group,False)
        terminal=host.load(Path(item['output'])/'terminal.json')
        need(terminal.get('Status')=='PASS_EXACT_RETENTION_READBACK' and terminal.get('readback_confirmed') is True and terminal.get('MaintenanceID')==self.b['MaintenanceID'] and terminal.get('BindingsSHA256')==self.h['bindings_sha256'],'COMPOSITION_WRITER_UNKNOWN_OR_UNCONFIRMED')
        need(self.ops.stop(owner['Native']),'COMPOSITION_NATIVE_CLOSE_UNCONFIRMED')
        return terminal

    def seal(self):
        return contain(self.anchor,self.ops,self.control)

    def revalidate(self):
        # Runtime mutable controls are excluded from the frozen file refs.
        # Re-read every actual source/config/checker/key byte before a new stage.
        Composition(self.packet,self.packet_sha,self.ops,self.clock)

    def run(self,read_authority,read_human,write_authority,write_human):
        # New authority is checked before any Docker/Core/source operation.
        self.grant(read_authority,'read',read_human);self.grant(write_authority,'write',write_human)
        output=self.control/'run';output.mkdir(exist_ok=False)
        save(output/'start.json',dict(maintenance_id=self.b['MaintenanceID'],manifest_sha256=self.packet_sha,at=self.clock().isoformat(),no_retry=True))
        failure=None;writers=[];closure=None
        try:
            self.ops.branch(self.packet);save(output/'cold-before.json',self.ops.cold())
            before=output/'before';before.mkdir();self.read_phase('before',read_authority,read_human,before)
            for group in host.proposal.TARGETS:
                target=output/group;target.mkdir();writers.append(self.writer(group,read_authority,read_human,write_authority,write_human,target))
            after=output/'after';after.mkdir();self.read_phase('after',read_authority,read_human,after)
        except BaseException as error:
            failure=dict(type=type(error).__name__,message=str(error));raise
        finally:
            if self.activated:closure=self.seal();save(output/'closure.json',closure)
            if closure is not None and closure['status']!='PASS_MAINTENANCE_PHYSICAL_CLOSURE':failure=failure or dict(type='ClosureUnconfirmed',message='continue all closure; preserve budget/UNKNOWN')
            try:save(output/'cold-after.json',self.ops.cold())
            except Exception as error:failure=failure or dict(type=type(error).__name__,message='cold/history/budget after: '+str(error))
            save(output/'terminal.json',dict(status='PASS_MAINTENANCE_BEFORE_WRITE_AFTER_CLOSED' if failure is None else 'FAILED_MAINTENANCE_PRESERVE_ALL_EVIDENCE',failure=failure,writers_completed=len(writers),writer_results=writers,closure=closure,old_authorization_reused=False,provider_delete=0,cleanup=0,pin_release=0,budget_reset=0))
            need(failure is None,'COMPOSITION_FAILED_OR_UNCONFIRMED')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action',choices=['check','run','seal'])
    parser.add_argument('--packet',type=Path);parser.add_argument('--packet-sha256')
    parser.add_argument('--anchor',type=Path);parser.add_argument('--anchor-sha256');parser.add_argument('--output',type=Path)
    parser.add_argument('--read-authority',type=Path);parser.add_argument('--read-human',type=Path)
    parser.add_argument('--write-authority',type=Path);parser.add_argument('--write-human',type=Path)
    args=parser.parse_args()
    if args.action=='seal':
        need(args.anchor and args.anchor_sha256 and args.output,'COMPOSITION_PINNED_ANCHOR_AND_EXCLUSIVE_OUTPUT_REQUIRED')
        anchor=checked_anchor(args.anchor.read_bytes(),args.anchor_sha256)
        args.output.mkdir(parents=True,exist_ok=False)
        result=contain(anchor,NativeOperations(args.output),args.output);save(args.output/'closure.json',result);print(json.dumps(result));return
    need(args.packet and args.packet_sha256,'COMPOSITION_PINNED_PACKET_REQUIRED')
    raw=args.packet.read_bytes();packet=indexes.decode(raw)
    need(host.sha(raw)==args.packet_sha256 and raw==host.canonical(packet),'COMPOSITION_RAW_CANONICAL_PACKET')
    entry=Composition(packet,args.packet_sha256)
    if args.action=='check':print(json.dumps(dict(status='PASS_MAINTENANCE_FILE_INPUTS_ONLY_NOT_AUTHORIZED',source_start=0,provider_io=0)));return
    need(all((args.read_authority,args.read_human,args.write_authority,args.write_human)),'COMPOSITION_SEPARATE_READ_AND_WRITE_AUTHORITY_REQUIRED')
    entry.run(args.read_authority,args.read_human,args.write_authority,args.write_human)


if __name__=='__main__':main()
