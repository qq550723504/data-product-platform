"""Read-only C21 confirmation crosscheck; never enable, start, send or unlock."""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[5]
OUT = Path(__file__).parent
BASE = OUT.parent
PACKET = BASE / 'remaining-candidate21'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')
PUB = BASE / 'candidate21-publication01'
sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
import evidence_indexes as e
import execution_freeze as f
import fixture_artifacts as artifacts
import remaining_six_window as window
import remaining_sequence as sequence

EXECUTION = '9ba214bd-a481-499b-bef6-ca7a789ef501'
HEAD = 'f4f27c789f53b6c45910f55f83bee6cabfdee4f6'
BINDING_SHA = 'f27a617f278fcbaf64db29ddf536d5816d88acaba8a0d760280eb23bbe55d0de'
PROPOSAL_SHA = '50d88777d6ed5d9b54e33558c98b9866f5a07e630dc8c77e1c098404e3119989'
PLAN_SHA = 'dcc6c136e40950aa6c98a79e3673f820f6885993e82cdba65a5459d7ba83aedc'
MANIFEST_SHA = 'eb28982a4104906f81ce56740a981365f00bcdb41bc0c9c1dced0ae16def9708'
load = lambda p: e.decode(p.read_bytes())
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def save(path, value):
    f.save(path, value)


def need(ok, message):
    if not ok:
        raise ValueError(message)


def file_checks(folder):
    proposal = ROOT / 'docs/acceptance/issue307-d1-candidate21-final-confirmation02.md'
    text = e.checked_bytes(proposal, PROPOSAL_SHA).decode()
    c = load(PACKET / 'candidate.json')
    bindings = json.loads((PACKET / 'bindings.json').read_bytes())
    template = e.checked_bytes(PACKET/'confirmation-template.txt',
        '960d08299c6f06aa0b8528fd6aec28ea8c31c2b8ab7cd587fc74ca8e5d6b7691')
    need(c['Execution'] == EXECUTION and c['Profile'] == 'candidate21/v31', 'Execution/profile differs')
    need(len(template.splitlines()) == 137 and text.encode().count(template) == 1, 'Full exact template differs')
    for name, digest in [('candidate.json','813b941ff3d1be0d9b83f81bcd5dd532bd1f6da4df81181d2a1d1ecae7d34867'),
                         ('bindings.json','e47174bddd358850896e85d270b9a65bcf144cb783a862e14b772077d4314ca9')]:
        e.checked_bytes(PACKET/name,digest)
    source = load(PACKET/'source-freeze.json')
    allocation = load(BASE/'remaining-candidate21-preparation01/allocation.json')
    need(allocation == load(PRIVATE/'allocation.json'), 'Public/private allocation differs')
    contract = dict(execution=EXECUTION, profile=c['Profile'], start=c['NotBefore'], end=c['NotAfter'])
    spec = artifacts.verify(ROOT, PRIVATE, allocation, contract)
    artifacts.verify_frozen(PACKET,c,allocation,spec)
    need(len(source['initial_build_files']) == 12, 'Original build membership differs')
    for name, digest in source['initial_build_files'].items():
        artifacts.regular(PRIVATE/name)
        e.checked_bytes(PRIVATE/name,digest)
    plan = ROOT/'docs/acceptance/evidence/issue307-d1/candidate21-planning01/final-static-evidence-audit-plan01.json'
    audit = e.audit_plan(ROOT,plan,PLAN_SHA,PACKET/'candidate.json')
    need(len(audit['indexes']) == 8 and audit['members_verified'] == 6366, 'Static index/reference membership differs')
    save(folder/'static-audit.json',audit)
    manifest = load(BASE/'candidate21-planning01/approval-reference-manifest01.json')
    need(sha(BASE/'candidate21-planning01/approval-reference-manifest01.json') == MANIFEST_SHA
         and manifest['count'] == len(manifest['approved_reference_artifacts']) == 1102, 'Manifest differs')
    for row in manifest['approved_reference_artifacts']:
        raw = e.checked_bytes(e.bound_path(ROOT,row['path']),row['sha256'])
        need(len(raw) == row['bytes'], 'Manifest original byte count differs')
    for name, digest, count, member_root in [
        ('candidate21-publication01/publication-evidence-index01.json','8d3aa628835b139d62e86bf2b8f2d86ff862b36f1ed8eae6b390f62d18aaca4e',255,'.'),
        ('candidate21-publication01/independent-postlock-review01/evidence-index.json','593c090c88444501044cde906a71b4b1b2a534a8253a93d68269ad622ef104b4',70,'docs/acceptance/evidence/issue307-d1/candidate21-publication01/independent-postlock-review01'),
        ('candidate21-publication01/independent-postlock-review01/reviewed-artifacts01.json','885e2a378ff3d85bba11d0e112153c12287c7d8d85f8fdeb23c885178cb5abf5',1223,'.')]:
        path=BASE/name; doc=load(path)
        e.audit_index(ROOT,dict(id=name,index=path.relative_to(ROOT).as_posix(),sha256=digest,
            format='files-map-v1',member_root=member_root,expected_count=count,expected_status=doc['status']))
    w = window.validate_packet(PACKET,PRIVATE)
    need(w['candidate_use_until']=='2026-10-22T14:40:00Z'
         and w['recovery_until']=='2026-10-22T16:40:00Z', 'Fixed window/deadline differs')
    table = re.findall(r'^\| `([^`]+)` \| `([0-9a-f]{64})` \|$',text,re.M)
    for name,digest in table:
        path = ROOT/name
        if path.is_file(): e.checked_bytes(path,digest)
        else: need(name in source['initial_build_files'] and source['initial_build_files'][name]==digest,'Unknown original build binding')
    need(len(table)==94,'82 public +12 build binding rows differ')
    protection=json.loads(re.search(r'```json\n(.*?)\n```',text,re.S).group(1))
    binding=load(PUB/'branch-freeze01/execution-binding.json')
    need(sha(PUB/'branch-freeze01/execution-binding.json')==sha(PRIVATE/'execution-binding.json')==BINDING_SHA
         and (PUB/'branch-freeze01/execution-binding.json').read_bytes()==(PRIVATE/'execution-binding.json').read_bytes(), 'Public/private execution binding differs')
    need(binding['artifact_bindings']==f.artifact_bindings(PACKET,c)
         and binding['branch_lock']['review_head']==HEAD
         and protection==binding['branch_lock']['protection'], 'Artifact/HEAD/full protection differs')
    f.ready(binding['branch_lock']['after'],HEAD,require_frozen=True)
    indexed={x['Copy']['ID']:(b,x) for b in bindings for x in b['copy_protection']}
    candidates={cp['ID']:cp for g in c['Groups'] for cp in g['Copies']}
    ordered=sequence.targets(c); expected_targets={cp['ID'] for _,_,cp in ordered}
    rows=[]
    for line in template.decode().splitlines():
        if not line.startswith(('删除目标 ','保留版本 ')): continue
        parts=line.rstrip('。').split('；'); role,project,address=parts[0].split(' ',2)
        namespace_key,version=address.rsplit('@',1); namespace,key=namespace_key.split('/',1)
        fields={part.split(' ',1)[0]:part.split(' ',1)[1] for part in parts[1:6]}
        b,p=indexed[fields['copy']]; cp=p['Copy']
        need(cp==candidates[cp['ID']] and project==b['project'], 'Copy/project binding differs')
        need((namespace,key,version,fields['DV'],fields['SHA256'],int(fields['bytes']))==
             (cp['Namespace'],cp['Key'],cp['ProviderVersion'],cp['DatasetVersionID'],cp['ContentSHA256'],cp['ByteSize']), 'Exact namespace/key/version/copy/DV/bytes/SHA differs')
        need(parts[6:9]==[f'hold={p["LegalHold"]}',f'retention={p["Retention"]}',f'retain-until={p["RetainUntil"]}'], 'Hold/retention tuple differs')
        target=cp['ID'] in expected_targets
        need((role=='删除目标')==target and key!='fixture/p1.csv', 'Target/survivor or P1 exclusion differs')
        if parts[9].startswith('plan '):
            pid=parts[9][5:]; phash=parts[10][5:]
            need(any(x['id']==pid and x['scope_hash']==phash for x in b['plans']), 'Plan identity/hash differs')
        else: need(not target,'Target has no plan')
        rows.append(dict(role=role,group=b['group'],project=project,copy=cp,LegalHold=p['LegalHold'],Retention=p['Retention'],RetainUntil=p['RetainUntil']))
    need(len(rows)==len({x['copy']['ID'] for x in rows})==17 and sum(x['role']=='删除目标' for x in rows)==6, '17 exact version membership differs')
    need({x['copy']['ID'] for x in rows if x['role']=='删除目标'}==expected_targets,'Six target identity differs')
    save(folder/'exact-version-crosscheck.json',dict(targets=6,survivors=11,order=[s for _,s,_ in ordered],versions=rows))
    permissions=[]
    for g,b in zip(c['Groups'],bindings):
        for stage in [*g['Helpers'],*['root-'+x for x in b['root_owners']]]:
            owner=PRIVATE/g['Group']/stage
            need(load(owner/'permission.json')=={'Enabled':False} and not (owner/'human-confirmation.txt').exists(), 'Permission enabled or human text materialized')
            permissions.append(str((owner/'permission.json').relative_to(PRIVATE)))
    need(len(permissions)==30,'Permission membership differs')
    need(not (PRIVATE/'authorized-execution01').exists()
         and not (BASE/'delete-confirmed-v31-execution01').exists()
         and not Path('C:/Users/Henry/.codex/automations/307-d1-candidate21-v31').exists(), 'Unexpected new authorization/schedule')
    rollout=Path('C:/Users/Henry/.codex/sessions/2026/10/13/rollout-2026-10-13T23-50-22-01a13579-573d-7a32-b1e1-3297fe78380a_01a135f5-2d3a-7953-8149-c86a26446d07.jsonl')
    with rollout.open('rb') as stream:
        stream.seek(max(0,rollout.stat().st_size-15000000)); raw_lines=stream.read().splitlines()
    display=None
    for raw in reversed(raw_lines):
        try: item=json.loads(raw)
        except (ValueError,UnicodeDecodeError): continue
        message=item.get('payload',{})
        if item.get('type')=='response_item' and message.get('role')=='assistant' and message.get('phase')=='final_answer':
            value=''.join(x.get('text','') for x in message.get('content',[]) if isinstance(x,dict))
            if 'CONFIRM_DELETE_EXACT_SIX' in value and 'candidate21/v31' in value:
                display=value; break
    need(display is not None,'Actual preceding complete chat display not found')
    with (folder/'preceding-assistant-display.md').open('xb') as stream:stream.write(display.encode())
    blocks=re.findall(r'```text\n(.*?)```',display,re.S)
    display_table=re.findall(r'^\| `([^`]+)` \| `([0-9a-f]{64})` \|$',display,re.M)
    equal=len(blocks)==1 and blocks[0].encode()==template and display_table==table
    save(folder/'display-versus-frozen.json',dict(status='PASS_EXACT_CHAT_DISPLAY_MACHINE_AND_ALL_BINDING_ROWS' if equal else 'FAIL_DISPLAY_TRANSCRIPTION',machine_equal=len(blocks)==1 and blocks[0].encode()==template,all_94_rows_equal=display_table==table,display_sha256=hashlib.sha256(display.encode()).hexdigest(),proposal_sha256=PROPOSAL_SHA))
    need(equal,'Chat display differs from frozen machine text or binding table; preserve originals and do not authorize')
    result=dict(status='PASS_C21_V31_FULL_TEXT_AND_ALL_FROZEN_BINDINGS_CROSSCHECK',at=f.now(),
        execution=EXECUTION,final_head=HEAD,execution_binding_sha256=BINDING_SHA,proposal_sha256=PROPOSAL_SHA,
        static_indexes=8,static_references=6366,manifest_members=1102,publication_members=255,
        public_binding_rows=82,original_build_members=12,host_source_members=len(source['host']['files']),
        runtime_source_members=len(source['runtime']['files']),machine_lines=137,targets=6,survivors=11,
        windows=w,permissions_false=30,authorization='NOT_RECEIVED',schedule='NOT_CREATED',
        Docker_Core_provider_io=0,native_delete=0,execution_authority=False)
    save(folder/'result.json',result);return result


def live_checks(folder):
    client=f.GitHub(folder)
    c=load(PACKET/'candidate.json')
    result=f.verify_binding(ROOT,PRIVATE,PACKET,c,BINDING_SHA,client)
    query='''query { repository(owner:"qq550723504",name:"data-product-platform") {
      pullRequest(number:332) { headRefOid isDraft state merged
        reviewThreads(first:100) { pageInfo { hasNextPage } nodes { id isResolved } } }
      issue(number:307) { state } } }'''
    data=client.graph(query)['repository']; pr=data['pullRequest']; threads=pr['reviewThreads']
    need(pr['headRefOid']==HEAD and pr['isDraft'] and pr['state']=='OPEN' and not pr['merged']
         and data['issue']['state']=='OPEN' and not threads['pageInfo']['hasNextPage']
         and all(x['isResolved'] for x in threads['nodes']), 'Fresh review/PR/Issue state differs')
    save(folder/'review-threads.json',data)
    result.update(status='PASS_FRESH_C21_HEAD_NATIVE_LOCK_FULL_PROTECTION_CI_AND_THREADS',
        unresolved_threads=0,Draft=True,Issue307='OPEN',execution_authority=False,native_delete=0,
        rule_operation='READ_ONLY_NO_UPDATE_NO_UNLOCK')
    save(folder/'result.json',result);return result


def host_checks(folder):
    spec=importlib.util.spec_from_file_location('fixed_readonly_c21_closed_gate',ROOT/'examples/lifecycle-d1/verify-six-native.py')
    v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
    gate=v.sealed_gate(PACKET)
    save(folder/'gate.json',gate)
    result=dict(status='PASS_FRESH_FIXED_C21_CLOSED_HOST_GATE',at=f.now(),gate=gate,
        source_start=0,enable=0,sender_wire_signing=0,native_delete=0,execution_authority=False,
        limitation='This is a fresh closed host/budget/topology readback, not a fresh native/Core/IAM source preflight. Window-time full native verification remains required after new authorization.')
    save(folder/'result.json',result);return result


def main():
    parser=argparse.ArgumentParser();parser.add_argument('mode',choices=['files','live','host']);args=parser.parse_args()
    folder=OUT/args.mode;folder.mkdir(exist_ok=False)
    save(folder/'intent.json',dict(at=f.now(),mode=args.mode,execution=EXECUTION,execution_authority=False,native_delete=0))
    try:result={'files':file_checks,'live':live_checks,'host':host_checks}[args.mode](folder)
    except Exception as error:
        save(folder/'failure.json',dict(at=f.now(),error_type=type(error).__name__,message=str(error),no_gate_relaxation=True,native_delete=0));raise
    print(json.dumps({k:result[k] for k in ('status','at','execution','final_head','native_delete') if k in result}))


if __name__=='__main__': main()
