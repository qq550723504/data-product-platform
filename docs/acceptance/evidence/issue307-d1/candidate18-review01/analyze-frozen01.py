"""File-only frozen c18/source/build/template validation; no native commands."""
import hashlib
import json
from pathlib import Path
import re
import sys
import zipfile

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[4]
PACKET = ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate18'
sys.path.insert(0, str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit
import remaining_six_window as window

def need(value, message):
    if not value: raise ValueError(message)

def load(path): return json.loads(audit.unlinked_absolute(path).read_bytes())
def sha(path): return hashlib.sha256(audit.unlinked_absolute(path).read_bytes()).hexdigest()

c = load(PACKET/'candidate.json')
b = load(PACKET/'bindings.json')
need(sha(PACKET/'candidate.json') == 'bb0924c1814b0bcdecd33d3b57a21a24beaafce2a3d64e10e5dac9aba1947f63', 'candidate differs')
need(sha(PACKET/'bindings.json') == '8d021af81d905cc6de255bf84e460fc0b416d5af5bacc8388cfbb2b423a60624', 'bindings differ')
need(sha(PACKET/'confirmation-template.txt') == '5cd148132b380b4a3361ae7210c8cc02fe6932fee3824ae77b4fec0cc2523a90', 'template differs')
need(c['Execution'] == '4fd5a7e7-779e-41d7-a35a-3bab7dd615b5' and c['Profile'] == 'candidate18/v28', 'generation differs')
need(c['NotBefore'] == '2026-10-21T14:00:00Z' and c['NotAfter'] == '2026-10-21T14:40:00Z', 'window differs')
sf = load(PACKET/'source-freeze.json')
need(sha(PACKET/'source-freeze.json') == c['SourceFreezeSHA256'] and sf['source'] == c['Source'] == '618f484061c3624ff2195c0b591e7d8e3d743ce1', 'source binding differs')
private = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate18')
archive = private/'approved-runtime-source.zip'
need(sha(archive) == sf['archive_sha256'] and archive.stat().st_size == sf['archive_bytes'], 'runtime ZIP differs')
with zipfile.ZipFile(archive) as z:
    names = z.namelist()
    need(len(names) == len(set(names)), 'duplicate runtime ZIP member')
    for name, meta in sf['files'].items():
        raw = audit.bound_path(ROOT, name).read_bytes()
        need(hashlib.sha256(raw).hexdigest() == meta['raw_sha256'] and hashlib.sha256(raw.replace(b'\r\n', b'\n')).hexdigest() == meta['lf_sha256'], 'raw/LF source differs')
        need(hashlib.sha256(z.read(name).replace(b'\r\n', b'\n')).hexdigest() == meta['lf_sha256'], 'ZIP source semantic bytes differ')
need(len(sf['files']) == 2129 and len(sf['initial_build_files']) == 12, 'source/build member count differs')
for name, digest in sf['initial_build_files'].items(): need(sha(audit.bound_path(private, name)) == digest, 'original build member differs')
need(sf['actual_build_source'] == sf['initial_allocation_build_source'] == c['Source'], 'build source differs')
for name, key in [('reader', 'ReaderBinarySHA256'), ('operator', 'SenderBinarySHA256'), ('owner', 'OwnerBinarySHA256')]:
    need(sha(private/name) == c[key], 'binary binding differs')
windows = window.validate_packet(PACKET, private)
plan = PACKET/'preparation-evidence-audit-plan.json'
audit_result = audit.audit_plan(ROOT, plan, sha(plan), PACKET/'candidate.json')
template = (PACKET/'confirmation-template.txt').read_text(encoding='utf-8')
target_lines = [x for x in template.splitlines() if x.startswith('删除目标 ')]
survivor_lines = [x for x in template.splitlines() if x.startswith('保留版本 ')]
need(len(target_lines) == 6 and len(survivor_lines) == 11, 'target/survivor membership differs')
ordered = [('business-races', 'fixture/p2.csv'), ('business-races', 'fixture/t1.csv'), ('business-races', 'fixture/t2.csv'),
           ('late-hold', 'fixture/h2-a.csv'), ('mixed', 'fixture/m1-a.csv'), ('mixed', 'fixture/m1-b.csv')]
fields = ('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')
targets = []
for group, key in ordered:
    current = next(g for g in c['Groups'] if g['Group'] == group)
    targets.append({k: next(x for x in current['Copies'] if x['Key'] == key)[k] for k in fields})
need(hashlib.sha256(json.dumps(targets, ensure_ascii=False, separators=(',', ':')).encode()).hexdigest() == c['TargetDigest'], 'ordered target digest differs')
present = []
for group, binding in zip(c['Groups'], b):
    need(group['Group'] == binding['group'], 'group order differs')
    iam = load(PACKET/group['Group']/'iam.json')
    need(sha(PACKET/group['Group']/'iam.json') == group['IAMSHA256'], 'IAM binding differs')
    for filename, key in [('gateway-active.Caddyfile', 'GatewaySHA256'), ('gateway-sealed.Caddyfile', 'SealedGatewaySHA256')]:
        need(sha(PACKET/group['Group']/filename) == sha(private/group['Group']/filename) == group[key], 'gateway binding differs')
    for cp in binding['copy_protection']:
        present.append((binding, cp))
    for stage in group['Helpers']:
        folder = private/group['Group']/stage
        op = load(folder/'operator.json')
        need(sha(folder/'operator.json') == group['ConfigSHA256'][stage] and not op.get('RootAccess') and not op.get('RootSecret'), 'worker root secret/config differs')
        need(bool(op.get('WirePrivateKey')) == (stage == 'wire'), 'wire key exposed to other worker')
        if stage == 'wire' or stage.startswith('recover-'):need(op['DeleteSecret'] == '', 'delete secret exposed to wire/recovery')
    for stage in group['CaseHelpers']:
        cfg = load(private/group['Group']/('case-'+stage)/'remaining.json')
        need(not cfg['Base'].get('RootAccess') and not cfg['Base'].get('RootSecret'), 'case root credential exposed')
for binding, row in present:
    cp = row['Copy']
    lines = target_lines if cp in targets else survivor_lines
    line = next((x for x in lines if 'copy '+cp['ID']+'；' in x), None)
    need(line is not None, 'present exact copy omitted from template')
    for text in (binding['project'], cp['Namespace']+'/'+cp['Key']+'@'+cp['ProviderVersion'],
                 'DV '+cp['DatasetVersionID'], 'SHA256 '+cp['ContentSHA256'], 'bytes '+str(cp['ByteSize']),
                 'hold='+str(row['LegalHold']), 'retention='+row['Retention'], 'retain-until='+str(row['RetainUntil'])):
        need(text in line, 'exact copy/template field differs')
need(len(present) == 17 and len({x['Copy']['ID'] for _,x in present}) == 17 and not any(x['Copy']['Key'] == 'fixture/p1.csv' for _,x in present), 'present membership wrong or P1 reintroduced')
permissions = list(private.glob('*/root-*/permission.json')) + [private/g['Group']/stage/'permission.json' for g in c['Groups'] for stage in g['Helpers']]
need(len(permissions) == 30 and len(set(permissions)) == 30, 'permission membership differs')
for path in permissions:
    permission = load(path)
    need(permission['Enabled'] is False and permission['Execution'] == c['Execution'] and permission['CandidateSHA256'] == sha(PACKET/'candidate.json')
         and permission['ConfirmationSHA256'] == sha(PACKET/'confirmation-template.txt') and permission['MaxTotal'] == 6 and permission['MaxPerTarget'] == 1
         and permission['NotBefore'] == c['NotBefore'] and permission['ExpiresAt'] == c['NotAfter'], 'permission expanded or enabled')
    need(not (path.parent/'human-confirmation.txt').exists(), 'authority materialized before new human authorization')
result = {'status': 'PASS_INDEPENDENT_C18_FROZEN_FILES', 'source_head': c['Source'], 'packet_commit': 'baa1555dab9b4fcaf058109b53a81e49ac13ccda',
          'candidate_sha256': sha(PACKET/'candidate.json'), 'bindings_sha256': sha(PACKET/'bindings.json'), 'template_sha256': sha(PACKET/'confirmation-template.txt'),
          'source_freeze_sha256': c['SourceFreezeSHA256'], 'source_files': 2129, 'original_build_members': 12, 'zip_sha256': sf['archive_sha256'],
          'targets': targets, 'survivor_count': 11, 'present_version_count': 17, 'P1_permanently_excluded': True,
          'permission_files_false': 30, 'worker_root_credential_absent': True, 'wire_key_only_wire': True, 'wire_recovery_delete_secret_absent': True,
          'windows': windows, 'preparation_audit_plan': audit_result, 'reviewer_native_calls': 0, 'real_DELETE': 0,
          'limits': 'Host descriptors and stopped state require the separate complete-entry and final physical closure receipts. No human authorization or schedule is created by this analysis.'}
with (OUT/'frozen-file-analysis01.json').open('x', encoding='utf-8', newline='\n') as target:
    json.dump(result, target, indent=2)
    target.write('\n')
raw=(OUT/'frozen-file-analysis01.json').read_bytes()
print(json.dumps({'status': result['status'], 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}))
