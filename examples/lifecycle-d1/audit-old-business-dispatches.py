"""Fixed old-v22 Core audit only. No observation write, proof, Native or IAM start."""
import argparse
import datetime as dt
import importlib.util
import json
from pathlib import Path
import uuid

ROOT = Path(__file__).resolve().parents[2]
WORKSPACE = 'a4aa2689-6e8f-4180-a567-dfb875407eac'
EXECUTION = '8e41cfba-a41e-44dd-9ce8-d6365b3bb90e'
CASES = {
    'P2': ('bce1b1b1-d6e2-44fd-8b87-171142350ab9', '6fabf3b5-76b8-4245-8f5d-01e64f610146', '670f5351-103f-472f-aeea-1f29213eda7c', '73d9251e-a128-47b1-8f5a-eaa35603c233'),
    'T1': ('247a2ca9-d2f3-4384-bcee-b10add497175', 'f48755f1-976b-4f47-98da-70ead936a4c3', '134315c3-bad8-4fda-a927-3ef71e2cca5e', 'bb6ecab6-53f1-4a9c-a333-f0cf6f4edd4a'),
    'T2': ('531c7b96-173f-4028-be97-2de9ed6c72f2', 'e09fd433-54cf-4e82-b8c4-098472505fa2', 'b9ebd875-8942-4303-8e17-41612dd8100c', '933462ae-04d6-4da4-97de-067b62863e0a'),
}


def need(ok, message):
    if not ok:
        raise ValueError(message)


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'examples/lifecycle-d1' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def query():
    # Every identifier is compiled, not taken from CLI or approval text.
    values = ','.join("('%s','%s'::uuid,'%s'::uuid,'%s'::uuid,'%s'::uuid)" % (name, *ids) for name, ids in CASES.items())
    arrays = []
    for key, table, where in [
        ('attempts', 'lifecycle_provider_attempt', 't.copy_id=s.copy'),
        ('dispatches', 'lifecycle_disposition_dispatch', 't.copy_id=s.copy'),
        ('claims', 'lifecycle_provider_send_claim', 't.attempt_id IN (SELECT id FROM lifecycle_provider_attempt WHERE copy_id=s.copy)'),
        ('observations', 'lifecycle_provider_observation', 't.attempt_id IN (SELECT id FROM lifecycle_provider_attempt WHERE copy_id=s.copy)'),
        ('releases', 'lifecycle_dispatch_release', 't.dispatch_id IN (SELECT id FROM lifecycle_disposition_dispatch WHERE copy_id=s.copy)'),
        ('absences', 'lifecycle_copy_absence_receipt', 't.copy_id=s.copy'),
        ('approvals', 'lifecycle_disposition_approval', 't.plan_id=s.plan'),
        ('revocations', 'lifecycle_approval_revocation', 't.approval_id IN (SELECT id FROM lifecycle_disposition_approval WHERE plan_id=s.plan)'),
        ('pins', 'lifecycle_reader_pin', 't.copy_id=s.copy'),
    ]:
        arrays += ["'%s', (SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]') FROM %s t WHERE t.workspace_id='%s' AND %s)" % (key, table, WORKSPACE, where)]
    arrays += ["'unresolved', (SELECT count(*) FROM lifecycle_disposition_dispatch d WHERE d.workspace_id='%s' AND d.copy_id=s.copy AND NOT EXISTS (SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id))" % WORKSPACE]
    return 'WITH scope(name,plan,copy,dispatch,attempt) AS (VALUES ' + values + ") SELECT jsonb_build_object('case',s.name," + ','.join(arrays) + ')::text FROM scope s ORDER BY s.name;'


def classify(row):
    name = row['case']
    need(name in CASES, 'unknown audit case')
    plan, copy, dispatch, attempt = CASES[name]
    attempts, dispatches = row['attempts'], row['dispatches']
    need(len(attempts) == len(dispatches) == 1, 'unexpected attempt/dispatch membership')
    a, d = attempts[0], dispatches[0]
    need((a['id'], a['workspace_id'], a['plan_id'], a['copy_id'], a['dispatch_id']) ==
         (attempt, WORKSPACE, plan, copy, dispatch), 'old attempt identity differs')
    need((d['id'], d['workspace_id'], d['plan_id'], d['copy_id']) ==
         (dispatch, WORKSPACE, plan, copy), 'old dispatch identity differs')
    approvals = [p for p in row['approvals'] if p['id'] == d['approval_id']]
    need(len(approvals) == 1 and approvals[0]['basis_ref'] == 'REMAINING_NON_DELETE_REHEARSAL:' + EXECUTION
         and approvals[0]['principal_ref'] == 'remaining-fixture:' + EXECUTION, 'old approval source differs')
    conflicts = {k: len(row[k]) for k in ('claims', 'observations', 'releases', 'absences') if row[k]}
    return {'case': name, 'status': 'CONFLICT_REQUIRES_REVIEW' if conflicts or row['unresolved'] != 1 else 'CORE_NO_CONFLICT_NOT_YET_TRUSTED_NOT_SENT',
            'conflicts': conflicts, 'unresolved': row['unresolved'], 'proof_signed': False, 'business_mutations': 0}


def retained(p1):
    old = p1.old_closed()
    need(p1.sha(p1.PUBLIC / 'preparation.json') == '08575df192136760be928c199ab06df921bb3ef1bd13c81115d25886ba3eea4c', 'final P1 preservation anchor drift')
    anchor = p1.load(p1.PUBLIC / 'preparation.json')
    need(old == anchor['old_closed'], 'old controls differ from final P1 preparation anchor')
    last = p1.load(p1.PUBLIC / 'post-close-physical-readback.json')
    ids = set(old['owners'])
    for audit in last['audits']:
        ids.update(audit['owners'])
    need(len(ids) == 77, 'retained owner membership differs')
    for cid in ids:
        need(p1.closed(p1.inspect(cid)), 'retained audit owner not closed')
    profiles = {}
    for audit in ('01', '02', '03', '04'):
        base = p1.PRIVATE / ('post-v22-p1-readonly' + audit)
        public_manifest = p1.PUBLIC.parent / ('post-v22-p1-readonly' + audit) / 'preparation.json'
        need(p1.sha(base / 'manifest.json') == p1.sha(public_manifest), 'prior read-only manifest drift')
        manifest = p1.load(public_manifest)
        inputs = dict(p1.input_entries(manifest))
        for name in ('worker-private/recovery.json', 'root-private/p1-readonly-permission.json'):
            path = base / name
            need(p1.load(path)['Enabled'] is False, 'old read-only profile enabled')
            original = base / (path.stem + '-disabled-original.json')
            if original.exists():
                need(p1.sha(original) == inputs[name] and p1.load(path) == p1.load(original), 'prior read-only profile binding drift')
            else:
                need(p1.sha(path) == inputs[name], 'prior unstarted read-only profile drift')
            profiles[str(path.relative_to(p1.PRIVATE))] = p1.sha(path)
    backup, budget = p1.backups(), p1.budgets()
    need(backup == anchor['backup_sha256'] and budget == anchor['budgets'], 'old backup/budget anchor drift')
    return {'old': old, 'owner_ids': sorted(ids), 'readonly_profile_sha256': profiles,
            'backup_sha256': backup, 'old_budgets': budget}


def run(audit_id):
    p1 = module('old_dispatch_p1_tools', 'reconcile-p1-post-v22.py')
    audit_id = str(uuid.UUID(audit_id))
    need(audit_id not in (EXECUTION, str(uuid.UUID(int=0))), 'fresh read-only audit identity required')
    control = p1.PRIVATE / 'post-v22-dispatch-audit' / audit_id
    public = ROOT / 'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-audit' / audit_id
    control.mkdir(parents=True, exist_ok=False)
    public.mkdir(parents=True, exist_ok=False)
    # Reuse byte-only Docker wrapper and exact closed checks. Never invoke P1 run.
    p1.CONTROL = control
    source = None
    attempted_start = False
    failure = None
    result = {'audit_id': audit_id, 'old_execution': EXECUTION, 'started_at': dt.datetime.now(dt.timezone.utc).isoformat(),
              'native_delete': 0, 'business_mutations': 0, 'proof_signed': False, 'fresh_IAM_or_native_verified': False}
    try:
        need(p1.sha(p1.PACKET / 'candidate.json') == 'f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243'
             and p1.sha(p1.PACKET / 'bindings.json') == '8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5', 'historical packet drift')
        before = retained(p1)
        p1.write(public / 'retained-before.json', before)
        group = next(g for g in p1.load(p1.PACKET / 'candidate.json')['Groups'] if g['Group'] == 'business-races')
        source = group['Core']
        x = p1.inspect(source)
        need(x['Image'] == p1.PG and p1.closed(x) and not x['HostConfig']['Privileged']
             and {v['NetworkID'] for v in x['NetworkSettings']['Networks'].values()} == {group['Network']}, 'Core identity/isolation differs')
        network = json.loads(p1.docker('network', 'inspect', group['Network']))[0]
        need(network['Internal'] and not network.get('Containers'), 'Core network has an active owner')
        # Log only a hash of the unchanged full source inspect; credentials stay private.
        p1.write(control / 'core-inspect-before.private.json', x)
        attempted_start = True
        p1.docker('start', source)
        native = p1.native_module()
        native.wait_pg(source, 'fixture')
        core_before = native.fingerprints(source)
        p1.write(public / 'Core-before.json', core_before)
        raw = native.sql(source, query())
        rows = [json.loads(line) for line in raw.splitlines()]
        need(len(rows) == 3 and {r['case'] for r in rows} == set(CASES), 'incomplete Core audit')
        p1.write(public / 'actual-Core-rows.json', rows)
        result['cases'] = [classify(r) for r in rows]
        core_after = native.fingerprints(source)
        p1.write(public / 'Core-after.json', core_after)
        need(core_before == core_after, 'read-only Core audit changed relations/roles/ACL')
        result['Core_unchanged'] = True
    except Exception as error:
        failure = type(error).__name__ + ': ' + str(error)
    finally:
        if attempted_start:
            # A failed inspect/start never suppresses the stop attempt or readback.
            try:
                p1.docker('stop', '-t', '15', source)
            except Exception:
                result['stop_error'] = 'original private diagnostics retained'
            try:
                result['source_stopped_PID0'] = p1.closed(p1.inspect(source))
            except Exception:
                result['source_stopped_PID0'] = False
        else:
            result['source_stopped_PID0'] = 'NOT_STARTED'
        try:
            after = retained(p1)
            p1.write(public / 'retained-after.json', after)
            result['retained_unchanged'] = 'before' in locals() and before == after
        except Exception:
            result['retained_unchanged'] = False
        result['failure'] = failure
        result['status'] = 'PASS_CORE_READONLY_AUDIT_ONLY' if not failure and result.get('Core_unchanged') and result.get('source_stopped_PID0') is True and result['retained_unchanged'] else 'BLOCKED_OR_UNCONFIRMED'
        p1.write(public / 'terminal.json', result)
    print(json.dumps({'status': result['status'], 'receipt': str(public / 'terminal.json')}))
    return result['status'] == 'PASS_CORE_READONLY_AUDIT_ONLY'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--audit-id', required=True, help='fresh read-only audit UUID, never an old execution')
    args = parser.parse_args()
    need(str(uuid.UUID(args.audit_id)) != EXECUTION, 'cannot reuse old execution')
    raise SystemExit(0 if run(args.audit_id) else 1)
