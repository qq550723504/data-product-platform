"""Append finite local evidence indexes; never modify old snapshots."""
import datetime,hashlib,json,re,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
sha=lambda raw:hashlib.sha256(raw).hexdigest()

def save(name,value):
 with (out/name).open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,ensure_ascii=False,indent=2);stream.write('\n')

expected={'retention_composition.py':'b15ac3a0987f4eb121f00244ddef6cda865b068013a6b20f6e9ba790b7b96081','verify-six-native.py':'ce2a47ffd079526a1be38a6a63be378117134fa9f76bbd56272218f8b685ede8',
 'test_retention_composition.py':'867656ec58a486d0eec2af59b6d315a15fa703bde29e487d7b270317c66e7634','test_verify_six_native.py':'31efe0d2a7cfb89870dd479202027a1c073ca690a7dd6786c20a784ae4c70cbe'}
if any(sha((root/'examples/lifecycle-d1'/name).read_bytes())!=digest for name,digest in expected.items()):raise ValueError('independent final source drift')
commands=json.loads((out/'retest03/acceptance-commands01.json').read_bytes())
if len(commands['commands'])!=20 or not commands['all_commands_success']:raise ValueError('final command set incomplete')
tests={}
for path in sorted((out/'retest03').glob('*.stderr')):
 if path.stem.startswith('go-') or path.stem.startswith('actual-'):continue
 match=re.search(rb'Ran ([0-9]+) tests?',path.read_bytes())
 if match:tests[path.stem]=int(match.group(1))
save('final-nondelete-checkpoint01.json',dict(status='PASS_LOCAL_RETENTION_V2_SOURCE_AND_NONDELETE_ACCEPTANCE',at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
 published_base='f4e25a88f62da6bb14ed24da8b71b634be43f17a',runtime_source='0446fde5985b51de7da5863b45df3f1b47b6167c',python_tests=tests,
 source_final_raw_sha256=expected,actual_retained_source_start=0,actual_retained_provider_io=0,actual_protection_write=0,actual_business_delete=0,remote_mutations=0,
 isolated_probe=json.loads((out/'linux-probe01.json').read_bytes())['cid'],isolated_probe_volumes_retained=True,
 source_state_comparison='two exact target natives unchanged; no retained source start by this round',old_data_budgets_pins='NO_MUTATIONS',
 new_operational_maintenance_id='NOT_CREATED',new_operational_budget='NOT_CREATED',new_human_authorization='NOT_GRANTED',new_p_pr_ci='NOT_RUN',new_p_main_ci='NOT_APPLICABLE',
 historical_failures_preserved=['original acceptance01 verify-six test-double Path/Args failure','original Go strict plan binding failure before provider IO','original checker fixture used pre-extension date','independent missing dynamic reader P1','independent corrupt intent masking valid registration P1']))
# This index has one explicit files mapping; it contains no self-reference or
# following publication proposal, nor any secret/private raw inspect bytes.
files={}
for path in sorted(out.rglob('*')):
 if path.is_file():
  raw=path.read_bytes();files[path.relative_to(root).as_posix()]=dict(sha256=sha(raw),bytes=len(raw))
save('evidence-index01.json',dict(status='PASS_LOCAL_NONDELETE_EVIDENCE_BYTES',scope='NOT_EXECUTION_OR_PUBLICATION_AUTHORITY',count=len(files),files=files))
print(json.dumps(dict(files=len(files),python_methods=tests,retained_provider_io=0,protection_write=0,delete=0)))
