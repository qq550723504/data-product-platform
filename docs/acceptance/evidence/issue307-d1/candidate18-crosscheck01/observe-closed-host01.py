"""Read existing frozen gates and Docker metadata; never start an owner."""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import subprocess
import sys
import tomllib

ROOT = Path.cwd()
OUT = Path(__file__).resolve().parent
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
spec = importlib.util.spec_from_file_location('c18_fixed_complete_entry', ROOT / 'examples/lifecycle-d1/verify-six-native.py')
entry = importlib.util.module_from_spec(spec); spec.loader.exec_module(entry)

def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()

def save(name, value):
    with (OUT / name).open('xb') as stream:
        stream.write((json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode())

start = now()
save('closed-host-intent01.json', {'started_at': start, 'operation': 'Call frozen complete-entry sealed_gate only; inspect saved 712 CIDs',
     'source_start': 0, 'provider_io': 0, 'native_delete': 0, 'authorization': 'NOT_RECEIVED'})
try:
    entry.require_unfinished_execution()
    entry.need(not (entry.r.CONTROL/'authorized-execution01').exists(), 'Unexpected authorization directory; read actual human scope first')
    window = entry.p.window.validate_packet(entry.PACKET, entry.r.CONTROL)
    save('actual-window-contract01.json', {'status': 'PASS_CURRENT_ACTUAL_CASE_RECOVERY_CROSS_FIELD_WINDOW', 'observed_at': now(), 'result': window})
    gate = entry.sealed_gate()
    save('fixed-sealed-gate01.json', {'started_at': start, 'finished_at': now(), 'entry_sha256': hashlib.sha256((ROOT/'examples/lifecycle-d1/verify-six-native.py').read_bytes()).hexdigest(),
         'result': gate, 'frozen_gate_modified': False, 'human_authorization_gate': 'NOT_RUN_NOT_AUTHORIZED', 'native_preflight': 'NOT_RUN'})
    previous = entry.load(BASE/'candidate18-planning01/final-cold-inventory01.json')
    cids = [row['cid'] for row in previous['owners']]
    entry.need(len(cids) == len(set(cids)) == 712, 'Saved known CID set differs')
    observed = []
    for offset in range(0, len(cids), 120):
        result = subprocess.run(['docker', 'inspect', *cids[offset:offset+120]], capture_output=True)
        entry.need(result.returncode == 0, 'Known retained CID inspection failed')
        values = json.loads(result.stdout)
        entry.need([x['Id'] for x in values] == cids[offset:offset+120], 'Docker inspection membership differs')
        for item in values:
            state = item['State']; config = item['HostConfig']
            entry.need(state['Running'] is False and state['Pid'] == 0 and not config.get('PortBindings') and not item['NetworkSettings'].get('Ports')
                       and config['RestartPolicy']['Name'] == 'no', 'Known CID not closed/noports/norestart: '+item['Id'])
            observed.append({'cid': item['Id'], 'running': False, 'PID': 0, 'noports': True, 'restart': 'no'})
    save('known-cids712-readback01.json', {'status': 'PASS_ALL_712_KNOWN_CIDS_STOPPED_PID0_NOPORTS_NORESTART', 'started_at': start, 'finished_at': now(),
         'owners': observed, 'count': len(observed), 'old_cids': 623, 'new_owners': 86, 'new_readers': 3,
         'fixed_gate_permissions_false': 30, 'new_budgets_empty': 3, 'old_budgets': 18, 'native_delete': 0, 'source_start': 0, 'provider_io': 0})
    automation_root = Path(r'C:\Users\Henry\.codex\automations')
    prior = []
    for number, version in [(7,17),(10,20),(11,21),(12,22),(15,25),(16,26),(17,27)]:
        automation_id = f'307-d1-candidate{number:02d}-v{version}' if number == 7 else f'307-d1-candidate{number}-v{version}'
        path = automation_root/automation_id/'automation.toml'
        value = tomllib.loads(path.read_text(encoding='utf-8'))
        entry.need(value['status'] == 'PAUSED', 'Old heartbeat still active: '+automation_id)
        prior.append({'id': automation_id, 'status': value['status'], 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
    entry.need(not (automation_root/'307-d1-candidate18-v28/automation.toml').exists(), 'Unexpected current heartbeat')
    save('authorization-and-schedule-absence01.json', {'status': 'PASS_NOT_AUTHORIZED_NOT_SCHEDULED', 'observed_at': now(),
         'execution': entry.p.EXECUTION, 'authorization_directory_exists': False, 'current_heartbeat_exists': False,
         'old_heartbeats': prior, 'execution_authority': False, 'native_delete': 0, 'schedule_changed': False})
    print(json.dumps({'status': 'PASS_READONLY_FROZEN_HOST_GATE_AND_712_CLOSED_CIDS', 'permissions_false': 30, 'new_budgets_empty': 3,
                      'native_delete': 0, 'source_start': 0, 'provider_io': 0}))
except BaseException as exc:
    save('closed-host-failure01.json', {'status': 'FAILED_READONLY_HOST_CROSSCHECK', 'started_at': start, 'failed_at': now(),
         'error_type': type(exc).__name__, 'error': str(exc), 'native_delete': 0, 'source_start': 0, 'provider_io': 0})
    raise
