"""One exact retained Core, read-only policy checkpoint; no native/IAM calls."""
import hashlib
import importlib.util
import json
from pathlib import Path
import uuid

ROOT = Path(__file__).resolve().parents[6]
OUT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('policy_checkpoint_retained', ROOT / 'examples/lifecycle-d1/six_candidate_runtime.py')
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)
need, load, save, sha = r.need, r.load, r.save, r.sha


def main():
    need(sha(ROOT/'examples/lifecycle-d1/six_candidate_runtime.py') == '40050b546ac99e6d57981d2a4dce729700fdfa955b8b645d718eeebf10d7d999', 'checkpoint implementation drift')
    old = ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate14-preparation01'
    need(sha(old/'non-delete-terminal.json') == '680dbaeeca4f6e8bfa7097860994d5fee7c7b96ac71617cc6a4542aab94194f7', 'failed preparation changed')
    allocation = load(old/'allocation.json')
    known = [*allocation['owners'], *[g['source'][role] for g in allocation['groups'] for role in ('Core', 'Native')]]
    need(len(set(known)) == 92 and all(r.closed(r.inspect(cid)) for cid in known), 'known preparation owner active')
    group = allocation['groups'][0]
    need(group['group'] == 'business-races', 'unexpected source group')
    core = group['source']['Core']
    audit = str(uuid.uuid4())
    save(OUT/'start.json', {'audit': audit, 'core': core, 'at': r.now().isoformat(), 'transaction': 'REPEATABLE READ READ ONLY ROLLBACK', 'source': allocation['source'], 'native_delete': 0, 'provider_requests': 0})
    failure = None
    try:
        r.docker('start', core)
        r.p.native.wait_pg(core, 'fixture')
        before = r.p.native.fingerprints(core)
        save(OUT/'Core-before.json', before)
        need(before == load(old/'business-races/source-Core-after.json') == r.baseline_core('business-races'), 'accepted retained Core changed')
        workspace = str(uuid.UUID(group['scope']['Workspace']))
        query = "SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY dataset_version_id),'[]')::text FROM lifecycle_current_copy_policy p WHERE workspace_id='" + workspace + "'::uuid;"
        raw = r.p.native.sql(core, query)
        (OUT/'current-policies.raw.json').open('xb').write((raw+'\n').encode())
        rows = json.loads(raw)
        copies = next(g for g in load(r.p.p1.PACKET/'candidate.json')['Groups'] if g['Group'] == 'business-races')['Copies']
        selected = {c['DatasetVersionID']: c for c in copies if c['Key'] not in ('fixture/p1.csv', 'fixture/control-hold.csv', 'fixture/control-retention.csv')}
        need(len(selected) == 5, 'fixed remaining policy membership differs')
        heads = {v['dataset_version_id']: v['id'] for v in rows if v['dataset_version_id'] in selected}
        need(len(heads) == 5 and set(heads) == set(selected), 'current policy head membership missing')
        renewal = ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v22-execution01/window01/candidate07-copy-policies.json'
        need(sha(renewal) == '867aa6032eddb5a9b1cb1ed489d1e4801b9ab2330f073002bb429722e08b68f3', 'actual renewal receipt changed')
        renewed = {v['ID'] for v in load(renewal)['facts']}
        need(set(heads.values()) < renewed, 'current five are not actual retained renewal facts')
        after = r.p.native.fingerprints(core)
        save(OUT/'Core-after.json', after)
        need(before == after, 'Core changed during read-only checkpoint')
        save(OUT/'policy-heads.json', {'group': 'business-races', 'workspace': workspace, 'source_core': core, 'CoreSHA256': sha(OUT/'Core-after.json'), 'renewal_receipt_sha256': sha(renewal), 'heads': heads,
            'policies': [v for v in rows if v['dataset_version_id'] in selected], 'source_business_mutations': 0, 'native_delete': 0})
    except BaseException as error:
        failure = {'type': type(error).__name__, 'message': str(error)}
        raise
    finally:
        closure = r.p.p1.close_known(core)
        save(OUT/'terminal.json', {'status': 'PASS_READONLY_CURRENT_POLICY_HEAD_CHECKPOINT' if failure is None and closure['closed'] else 'FAILED_OR_UNCONFIRMED_RETAINED', 'audit': audit, 'failure': failure, 'closure': closure, 'native_delete': 0, 'provider_requests': 0, 'source_business_mutations': 0, 'amount': 'NOT_ASSESSED'})
    print(json.dumps({'status': 'PASS_READONLY_CURRENT_POLICY_HEAD_CHECKPOINT', 'heads': heads, 'receipt_sha256': sha(OUT/'policy-heads.json')}))


if __name__ == '__main__':
    main()
