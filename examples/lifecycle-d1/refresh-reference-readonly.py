"""One fixed audit-core-06 refresh of reviewed v6; no send or IAM write switch."""
import argparse
import importlib.util
import json
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
PREVIOUS = BASE / 'delete-readonly-candidate04/retry02/5d-04'
PUBLIC = BASE / 'delete-readonly-v6-refresh02'


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def reviewed_inputs(m):
    checker = module('reviewed_v6', ROOT / 'examples/lifecycle-d1/check-reference-proposal.py')
    review = m.load(BASE / 'delete-proposal-v6-review01/independent-re-review.json')
    for path in ('examples/lifecycle-d1/check-reference-proposal.py',
                 'examples/lifecycle-d1/test_reference_proposal.py'):
        m.require(m.digest(ROOT / path) == review['reviewed_fix_sha256'][path], 'Reviewed v6 checker changed')
    checker.check(checker.decode(checker.read(ROOT, checker.PROPOSAL)))
    index = m.load(BASE / 'delete-readonly-candidate04/evidence-index.json')
    for entry in index['files']:
        m.require(m.digest(ROOT / entry['path']) == entry['sha256'], 'Prior indexed receipt changed')
    expected = m.load(PREVIOUS / 'native-iam-helper.json')
    binary = m.PRIVATE / 'd1-delete-readonly-candidate04/iam-readonly02'
    m.require(m.digest(binary) == expected['binary_sha256'], 'Read-only IAM binary changed')
    return binary


def refresh_paths(generation):
    if generation == 6:
        return PUBLIC, 'd1-delete-readonly-v6-refresh02'
    if generation == 7:
        return BASE / 'delete-readonly-v7-refresh01', 'd1-delete-readonly-v7-refresh01'
    if generation == 8:
        return BASE / 'delete-readonly-v8-refresh01', 'd1-delete-readonly-v8-refresh01'
    if generation == 9:
        return BASE / 'delete-readonly-v9-refresh01', 'd1-delete-readonly-v9-refresh01'
    if generation == 10:
        return BASE / 'delete-readonly-v10-refresh01', 'd1-delete-readonly-v10-refresh01'
    if generation == 11:
        return BASE / 'delete-readonly-v11-refresh01', 'd1-delete-readonly-v11-refresh01'
    raise ValueError('Refresh outside finite generation')


def run(generation=6):
    public, control_name = refresh_paths(generation)
    m = module('readonly', ROOT / 'examples/lifecycle-d1/complete-lc13-readonly.py')
    binary = reviewed_inputs(m)
    if generation in (7, 8, 9, 10, 11):
        reviewed = module('reviewed_v7', ROOT / 'examples/lifecycle-d1/check-reference-refresh.py')
        previous_generation = str(generation - 1).zfill(2)
        reviewed.check(reviewed.load(reviewed.refresh_paths(previous_generation)[2]), generation=previous_generation)
        review_path = BASE / ('delete-proposal-v' + str(generation) + '-review01/independent-review.json')
        review = m.load(review_path)
        expected_head = {7: 'e122a613322fba6009ec8175539e9eb9e3e7f05e',
                         8: 'a29bfe6019cd5b38415fd378394fcd537662525c',
                         9: 'd01c6cdc5918e04eb0981b4bff2a152bb72ee0a6',
                         10: 'cf65cbad0bf93601e2b8a2a24ea2ffcc09247774',
                         11: 'fb0ff60e2fc8e6fb808e96595e0ddae418bef1cd'}[generation]
        m.require(review['status'] == 'PASS_INDEPENDENT_OFFLINE_REVIEW'
                  and review['current_slice_blockers'] == 0
                  and review['reviewed_head'] == expected_head, 'Predecessor review not closed')
        for path, expected in review['reviewed_operator_sha256'].items():
            raw = m.call(['git', 'show', review['reviewed_head'] + ':' + path])
            m.require(m.hashlib.sha256(raw).hexdigest() == expected, 'Reviewed predecessor source history changed')
        index = m.load(review_path.parent / 'evidence-index.json')
        for entry in index['files']:
            m.require(m.digest(ROOT / entry['path']) == entry['sha256'], 'Predecessor review receipt changed')
    collector = module('collector', PREVIOUS / 'core-readonly-collector.py')
    m.CONTROL = m.PRIVATE / control_name
    m.PUBLIC = public
    original = m.role_acl
    collected = False

    def role_acl(container):
        nonlocal collected
        result = original(container)
        if container != m.PG or collected:
            return result
        collected = True
        out = public / ('5d-' + str(generation).zfill(2))
        collector.collect(m, out)
        control = m.CONTROL / 'iam-management'
        control.mkdir()
        source = m.load(m.SOURCE / 'config.json')
        owner = m.load(m.SOURCE / 'plane-owner/reader.json')
        m.require(source['Enabled'] is False and owner['Enabled'] is False, 'Business window reopened')
        cfg = {'RootAccess': source['RootAccess'], 'RootSecret': source['RootSecret'],
               'RunID': source['RunID'], 'Enabled': False,
               'Users': {'reader': owner['Access'], 'producer': owner['ProducerAccess'],
                         'inspector': source['InspectAccess'], 'executor': 'refdelete614cd9ad'}}
        m.write(control / 'iam.json', cfg)
        shutil.copyfile(binary, control / 'iam-readonly')
        ident = m.docker('create', '--pull', 'never', '--name',
                         'dpp-lifecycle307-5d2274ee-readonly-iam-' + str(generation).zfill(2), '--label',
                         'lifecycle307.run=' + cfg['RunID'], '--network', 'container:' + m.MINIO,
                         '--read-only', '--tmpfs', '/tmp', '--mount',
                         'type=bind,source=' + str(control) + ',target=/run/private,readonly',
                         '--mount', 'type=bind,source=' + str(out) + ',target=/run/output',
                         '--entrypoint', '/run/private/iam-readonly', m.PGIMAGE).decode().strip()
        try:
            m.docker('start', ident)
            code = m.docker('wait', ident).decode().strip()
            item = m.inspect(ident)
            m.write(out / 'native-iam-helper.json', {'id': ident, 'image': item['Image'],
                    'exit': int(code), 'pid': item['State']['Pid'],
                    'binary_sha256': m.digest(control / 'iam-readonly'),
                    'source_sha256': m.digest(PREVIOUS / 'iam-readonly.go'),
                    'read_only': True, 'management_root_mounted': True,
                    'payload_root_mounted': False, 'source_pg_credentials_mounted': False, 'delete': 0})
            m.require(code == '0' and item['State']['Status'] == 'exited'
                      and item['State']['Pid'] == 0, 'Native IAM precondition blocked; retained')
        finally:
            if m.inspect(ident)['State']['Running']:
                m.docker('stop', ident)
        return result

    m.role_acl = role_acl
    m.five_d(generation)
    print(json.dumps({'status': 'PASS', 'generation': str(generation).zfill(2), 'source_mutation': 0,
                      'native_checkpoint_calls': 20, 'native_admin_reads': 4,
                      'delete': 0, 'cleanup': 0}))


if __name__ == '__main__':
    argparse.ArgumentParser(description=__doc__).parse_args()
    run()
