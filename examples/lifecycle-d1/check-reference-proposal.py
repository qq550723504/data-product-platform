"""Offline consistency check for the closed D1 proposal; never grants authority.

Reads public repository artifacts only. No Docker, database, network, credential
or execution entry point. Snapshot consistency is distinct from a fresh gate.
"""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASE = 'docs/acceptance/evidence/issue307-d1/'
CANDIDATE = BASE + 'lc13-continuation01/delete-candidate04/'
READONLY = BASE + 'delete-readonly-candidate04/'
PROPOSAL = BASE + 'delete-proposal-v6/proposal.json'
RECEIPTS = {
    'checkpoint': READONLY + 'retry02/5d-04/readonly-checkpoint.json',
    'native': READONLY + 'retry02/5d-04/native-six-version.json',
    'core': READONLY + 'retry02/5d-04/current-core-readback.json',
    'references': READONLY + 'retry02/5d-04/reference-qualification-readback.json',
    'iam': READONLY + 'retry02/5d-04/native-iam-readonly.json',
    'costs': READONLY + 'retry02/5d-04/audit-costs.json',
    'conditions': READONLY + 'execution-preconditions.json',
}


def require(ok, message):
    if not ok:
        raise ValueError(message)


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'Duplicate JSON key: ' + key)
        result[key] = value
    return result


def read(root, path):
    target = (root / path).resolve()
    require(target.is_relative_to(root.resolve()), 'Repository path escaped')
    return target.read_bytes()


def decode(raw):
    return json.loads(raw.decode('utf-8'), object_pairs_hook=unique)


def reference(root, path):
    return {'path': path, 'sha256': hashlib.sha256(read(root, path)).hexdigest()}


def bound(root, ref, path):
    require(ref == reference(root, path), 'Artifact binding drift: ' + path)
    return decode(read(root, path))


def check(p, root=ROOT):
    require(p['schema'] == 'lifecycle307-exact-delete-proposal-v6', 'Wrong proposal version')
    require(not {'this_preparation', 'readonly_preflight', 'readonly_preflight_v5',
                 'continuation_readonly', 'hashes'} & p.keys(), 'Ambiguous inherited current fields')
    frozen = decode(read(root, CANDIDATE + 'source-binding.json'))
    profile = decode(read(root, CANDIDATE + 'bounded-profile.json'))
    expected = {key: frozen[key] for key in (
        'source_commit', 'binary_sha256', 'bounded_profile_sha256', 'target_digest',
        'iam_policy_sha256', 'active_gateway_sha256', 'sealed_gateway_sha256')}
    require(p['candidate'] == expected, 'Frozen candidate drift')
    runtime = bound(root, p['runtime_text_binding'], BASE + 'delete-proposal-v6/runtime-source-lf.json')
    require(runtime['source_commit'] == frozen['source_commit']
            and set(runtime['files']) == set(frozen['runtime_source_hashes']), 'Runtime binding scope drift')
    for path, digest in runtime['files'].items():
        require(hashlib.sha256(read(root, path).replace(b'\r\n', b'\n')).hexdigest() == digest,
                'Runtime source drift: ' + path)
    require(hashlib.sha256(read(root, CANDIDATE + 'bounded-profile.json')).hexdigest()
            == expected['bounded_profile_sha256'], 'Profile bytes drift')
    require(hashlib.sha256(read(root, CANDIDATE + 'delete-executor-policy.json')).hexdigest()
            == expected['iam_policy_sha256'], 'Raw IAM bytes drift')
    for key, profile_key in {
        'environment_run_id': 'environment', 'execution_run_id': 'execution',
        'workspace_id': 'workspace', 'provider_instance_id': 'provider',
        'incarnation_id': 'incarnation', 'plan_id': 'plan', 'plan_hash': 'plan_hash',
        'inventory_hash': 'inventory_hash',
    }.items():
        require(p[key] == profile[profile_key], 'Environment/plan drift: ' + key)
    require(set(p['current_readonly']) == set(RECEIPTS), 'Current receipt set drift')
    proofs = {key: bound(root, p['current_readonly'][key], path)
              for key, path in RECEIPTS.items()}
    checkpoint, native, core = (proofs[k] for k in ('checkpoint', 'native', 'core'))
    require(checkpoint['audit_core'] == p['audit_core']
            == p['project'] + '-readonly-audit-core-04', 'Audit generation drift')
    require(checkpoint['status'] == 'PASS' and checkpoint['approval'] == 'NOT_RECEIVED'
            and checkpoint['source_business_mutations'] == checkpoint['native_delete'] == 0,
            'Checkpoint not closed')
    require(core['counts'] == p['source_sql_counts'] and core['as_of'] == p['source_sql_as_of'],
            'Core snapshot drift')
    require(checkpoint['source_rows'] == p['source_rows'] == 1879
            and checkpoint['source_relations'] == p['source_relations'] == 194,
            'Source checkpoint drift')
    require(proofs['costs'] == {'calls': 112, 'observations': 112, 'costs': 112,
            'unaccounted': 0, 'send_claims': 0, 'absence': 0, 'completion': 0}, 'Audit ledger drift')
    require(native['provider_delete'] == native['business_admission'] == 0, 'Not non-delete evidence')
    registry = {c['ID']: c for c in profile['inventory']}
    require(len(registry) == 6 and len(native['versions']) == 6, 'Native inventory count drift')
    native_by_id = {v['copy']['ID']: v for v in native['versions']}
    require(set(native_by_id) == set(registry), 'Native membership drift')
    for namespace in {c['Namespace'] for c in registry.values()}:
        inventory = native['inventories'][namespace]
        expected_versions = {(c['Key'], c['ProviderVersion'], c['ByteSize'], False)
                             for c in registry.values() if c['Namespace'] == namespace}
        versions = inventory['Versions']
        require(len(versions) == len(expected_versions)
                and {(v['Key'], v['Version'], v['ByteSize'], v['DeleteMarker'])
                     for v in versions} == expected_versions, 'Native version listing drift')
    targets = {c['ID'] for c in profile['targets']}
    seen = set()
    fields = {'copy_id': 'ID', 'dataset_version_id': 'DatasetVersionID',
              'namespace': 'Namespace', 'key': 'Key', 'provider_version': 'ProviderVersion',
              'content_sha256': 'ContentSHA256', 'byte_size': 'ByteSize'}
    require(len(p['targets']) == 2 and len(p['survivors']) == 4, 'Scope count drift')
    for group, expected_ids in (('targets', targets), ('survivors', set(registry) - targets)):
        require({v['copy_id'] for v in p[group]} == expected_ids, 'Delete/survivor membership drift')
        for v in p[group]:
            ident = v['copy_id']
            require(ident not in seen, 'Duplicate object')
            seen.add(ident)
            obj, proof = registry[ident], native_by_id[ident]
            require(proof['copy'] == obj and proof['protection']['Copy'] == obj
                    and proof['bytes'] == obj['ByteSize'] and proof['sha256'] == obj['ContentSHA256'],
                    'Native content identity drift')
            require(all(v[k] == obj[n] for k, n in fields.items()), 'Proposed object identity drift')
            for key, native_key in {'legal_hold': 'LegalHold', 'retention_mode': 'RetentionMode',
                                    'retain_until': 'RetainUntil', 'protection_observed_at': 'ObservedAt'}.items():
                require(v[key] == proof['protection'][native_key], 'Protection snapshot drift')
            require(v['requires_fresh_protection_before_send'] is True, 'Fresh protection required')
    require(p['total_bytes'] == sum(v['byte_size'] for v in p['targets']) == 129560, 'Byte budget drift')
    budget = p['execution_budget']
    require(budget['max_provider_delete_requests'] == 2 and budget['max_per_target'] == 1,
            'Send budget expanded')
    for key in ('protected_or_output_delete_requests', 'sdk_and_proxy_send_retries',
                'hold_release', 'retention_release', 'retention_bypass', 'bare_key_delete', 'bucket_cleanup'):
        require(budget[key] == 0, 'Excluded mutation enabled')
    require(p['authority']['human_confirmation'] == 'NOT_RECEIVED', 'No execution approval in proposal')
    for key in ('provider_delete_authorized', 'permission_enabled', 'new_send_gateway_created',
                'native_delete_iam_provisioned'):
        require(p['authority'][key] is False, 'Authority enabled')
    require(p['authority']['business_windows'] == p['authority']['maintenance_permissions'] == 'SEALED',
            'Window opened')
    require(p['authority']['future_execution_run_id'] == p['execution_run_id']
            and p['authority']['max_window_minutes'] == 40, 'Authority scope drift')
    bound(root, p['previous_proposal'], READONLY + 'proposal-v5.json')
    require(p['snapshot_is_send_authorization'] is False and p['full_D1'] == 'NOT_ACCEPTED',
            'Acceptance overstated')
    return {'status': 'PASS_OFFLINE_CONSISTENCY_ONLY', 'candidate_runtime_changed': False,
            'runtime_source_files': len(frozen['runtime_source_hashes']),
            'targets': 2, 'survivors': 4, 'current_receipts': len(RECEIPTS),
            'audit_core': p['audit_core'], 'snapshot_is_send_authorization': False,
            'provider_io': 0, 'core_mutation': 0, 'delete': 0}


if __name__ == '__main__':
    print(json.dumps(check(decode(read(ROOT, PROPOSAL))), indent=2))
