"""Current v20 offline freeze check. No Docker, DB, private files or network.

v17 evidence is audited at its own immutable snapshot; changing the current
compiled generation does not rewrite or loosen that historical checker.
"""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
PACKET = ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-candidate10'
EXECUTION = '67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5'
START = '2026-10-18T10:00:00Z'
END = '2026-10-18T10:40:00Z'

def need(ok, text):
    if not ok: raise ValueError(text)
def load(path): return json.loads(path.read_bytes())
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()

def check():
    c, s = load(PACKET/'candidate.json'), load(PACKET/'status.json')
    bindings = load(PACKET/'bindings.json')
    index = load(PACKET/'evidence-index.json')
    need(c['Execution'] == s['execution'] == EXECUTION and c['Profile'] == 'candidate10/v20', 'foreign generation')
    need(c['NotBefore'] == START and c['NotAfter'] == END, 'fixed proposal window differs')
    need(s['candidate_sha256'] == sha(PACKET/'candidate.json')
         and s['confirmation_sha256'] == sha(PACKET/'confirmation-template.txt'), 'frozen payload differs')
    need(s['human_confirmation'] == 'NOT_RECEIVED' and s['permission_enabled'] is False,
         'this preparation has no sending authority')
    need(all(s[k] == 0 for k in ('current_delete_budget','native_delete','hold_release','bypass','cleanup'))
         and s['PR323'] == 'DRAFT' and s['Issue307'] == 'OPEN' and s['full_D1'] == 'NOT_ACCEPTED', 'scope differs')
    for name, expected in {**index['files'], **index['references']}.items():
        p = ROOT/name
        need(p.resolve().is_relative_to(ROOT.resolve()) and '..' not in Path(name).parts
             and '\\' not in name and sha(p) == expected, 'frozen evidence differs: '+name)
    members = {p.relative_to(ROOT).as_posix() for p in PACKET.rglob('*') if p.is_file() and p.name != 'evidence-index.json'}
    need(set(index['files']) == members, 'unindexed candidate payload')
    source = load(PACKET/'source-freeze.json')
    need(sha(PACKET/'source-freeze.json') == c['SourceFreezeSHA256'] and source['source_commit'] == c['Source'], 'source binding differs')
    for name, hashes in source['files'].items():
        raw = (ROOT/name).read_bytes().replace(b'\r\n',b'\n')
        need(hashlib.sha256(raw).hexdigest() == hashes['lf_sha256'], 'current source drift: '+name)
        frozen = subprocess.check_output(['git','show',c['Source']+':'+name],cwd=ROOT)
        need(hashlib.sha256(frozen.replace(b'\r\n',b'\n')).hexdigest() == hashes['lf_sha256'], 'Git source differs: '+name)
    need(len(c['Groups']) == len(bindings) == 3 and sum(len(g['Copies']) for g in c['Groups']) == 18, 'finite inventory differs')
    requests = 0
    for g,b in zip(c['Groups'],bindings):
        folder = PACKET/g['Group']
        need(g['Group'] == b['group'] and sha(folder/'iam.json') == g['IAMSHA256'], 'IAM/group binding differs')
        need(sha(folder/'core-readonly.json') == b['source_core_sha256']
             and sha(folder/'readonly/native-verification.json') == b['native_readonly_sha256'], 'Core/native receipt differs')
        need(load(folder/'preflight/core-before.json') == load(folder/'preflight/core-after.json')
             == load(folder/'core-readonly.json'), 'Core changed')
        gate = load(folder/'preflight/aggregation.json')
        need(gate['status'] == 'PASS_COMPLETE_NATIVE_AGGREGATION' and all(p['official_equals'] for p in gate['policies'])
             and load(folder/'preflight/physical-close.json')['all_closed'], 'complete preflight did not close/pass')
        requests += gate['technical_requests']
    build = load(PACKET/'verification-build.json')
    need(build['entry_sha256'] == sha(ROOT/'examples/lifecycle-d1/verify-remaining-native.py'), 'complete entry changed')
    text = (PACKET/'confirmation-template.txt').read_text(encoding='utf-8')
    for value in [EXECUTION,START,END,c['Source'],s['candidate_sha256'],sha(PACKET/'bindings.json'),build['checker_sha256'],build['entry_sha256'],*[cp['ProviderVersion'] for g in c['Groups'] for cp in g['Copies']]]:
        need(value in text, 'precise confirmation omits binding')
    need(text.count('仅删除 ')==7 and text.count('必须保留 ')==11 and '旧授权全部不复用' in text, 'scope text differs')
    need('PASS_FROZEN_CANDIDATE_OFFLINE' in (PACKET/'offline-validator.txt').read_text(), 'actual Go validator missing')
    for file,count in [('sealed-owner-regression.json',17),('sealed-case-regression.json',7)]:
        rows = load(PACKET/file)
        need(len(rows)==count and all(r['exit_code']==1 and r['source_core_native_stopped'] and r['native_delete']==0 for r in rows), 'actual denial regression incomplete')
    return {'status':'PASS_CANDIDATE10_OFFLINE','execution':EXECUTION,'source':c['Source'],
            'targets':7,'survivors':11,'source_files':len(source['files']),
            'raw_evidence':len(index['files'])+len(index['references']), 'native_technical_requests':requests,
            'checker_provider_io':0,'native_delete':0,'human_confirmation':'NOT_RECEIVED'}

if __name__ == '__main__': print(json.dumps(check()))
