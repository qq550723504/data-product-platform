"""Fixed retained v26 facts, including legitimate fail-closure revocations.

No service is started, identity enabled, budget initialized or fact rewritten.
This contract explains one real terminal execution, not a compatibility mode.
"""
import copy
import hashlib
import json
from pathlib import Path

import evidence_indexes as audit

OLD_ROOT = Path('C:/Users/Henry/.codex/worktrees/d1-cli-fix/data-product-platform')
RELATIVE = 'docs/acceptance/evidence/issue307-d1/'
EXECUTION = 'aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a'
PINS = {
    'remaining-candidate16/candidate.json': '6cfb8173b853f45775eac3fd0c55095fc90ed9df50b1313d817040f6edc75643',
    'remaining-candidate16/bindings.json': 'ba4be219a0a567579684afbb9ced23d7fdec556654d746b7b86c254d35e4e580',
    'remaining-candidate16-preparation01/allocation.json': '60855ee4859ba33dc7ebb12bc757aaece6d7abd6799679d0cb65873df8bbd8a4',
    'remaining-candidate16-preparation01/owner-descriptors.json': '6d8b358fc8106d5d57d8584b37414081b786e49af478ed895eb122b725f09ba7',
    'remaining-candidate16-preparation01/non-delete-terminal.json': 'f57db89dfbd1c231ed6ea8080dec726d0817de1996a5ab17c051018cea3e38e3',
    'remaining-candidate16-preparation01/non-delete-evidence-index.json': '037e1343958de0f547b1a892bbaeff7ee3d2044673df8e55d8bdc4acf1031273',
    'delete-confirmed-v26-execution01/human-authorization.json': 'b09d9a5024ccb46a1c8dd8fd19cefa96085aa4ea89f5a4666fe33f1afb933d14',
    'delete-confirmed-v26-execution01/window01/terminal.json': 'f94fcc9d2ccb8074ce9c1810c085394a2ef61144a4bbf118db5c82d385cda8c2',
}


def need(ok, message):
    if not ok:
        raise ValueError('retained v26: ' + message)


def sha(path):
    return hashlib.sha256(audit.unlinked_absolute(path).read_bytes()).hexdigest()


def load(path):
    return audit.decode(audit.unlinked_absolute(path).read_bytes())


def revoked(original, current, record, expected_sha, field):
    """Prove only the permitted Enabled field changed, preserving old bytes."""
    need(hashlib.sha256(original).hexdigest() == expected_sha == record['before_sha256'], 'original frozen profile differs')
    need(hashlib.sha256(current).hexdigest() == record['disabled_sha256'], 'revoked profile digest differs')
    need(record['field'] == field and record['before'] is True and record['after'] is False and record['other_fields_equal'] is True, 'revocation fact differs')
    before, after = audit.decode(original), audit.decode(current)
    expected = copy.deepcopy(before)
    owner = expected['Base'] if field == 'Base.Enabled' else expected
    need(owner['Enabled'] is True, 'original profile was not enabled')
    owner['Enabled'] = False
    need(after == expected, 'revocation changed another field')


def verify(inspect, closed, project_owner, docker, sequence, ids, private_base, hostpath):
    root = audit.unlinked_absolute(OLD_ROOT)
    evidence = audit.bound_path(root, RELATIVE.rstrip('/'), directory=True)
    private = audit.bound_path(private_base, 'remaining-candidate16', directory=True)
    authdir = audit.bound_path(private, 'authorized-execution01', directory=True)
    for name, digest in PINS.items():
        need(sha(audit.bound_path(evidence, name)) == digest, 'pinned historical bytes changed: ' + name)
    packet = evidence/'remaining-candidate16'
    prep = evidence/'remaining-candidate16-preparation01'
    candidate, bindings = load(packet/'candidate.json'), json.loads((packet/'bindings.json').read_bytes())
    auth = load(evidence/'delete-confirmed-v26-execution01/human-authorization.json')
    terminal = load(authdir/'terminal.json')
    need(sha(authdir/'terminal.json') == PINS['delete-confirmed-v26-execution01/window01/terminal.json'] and terminal['execution'] == EXECUTION
         and terminal['status'] == 'FAILED_OFFLINE_AUDIT_SEALED_NO_DELETE' and terminal['physical_delete_attempts'] == 0
         and terminal['fresh_identity_disable'] == 'UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START' and terminal['authorization_reusable'] is False,
         'terminal failed/UNKNOWN facts changed')
    need(len(auth['approved_reference_artifacts']) == 76, 'approved original membership differs')
    for item in auth['approved_reference_artifacts']:
        need(sha(audit.bound_path(root, item['path'])) == item['sha256'], 'original approved reference changed')
    for name, digest in [('user-message-verbatim.txt', auth['user_message_sha256']), ('authorization-envelope.txt', auth['authorization_envelope_sha256'])]:
        need(sha(audit.bound_path(authdir, name)) == digest, 'actual historical human authorization changed')
    private_index = authdir/'window01-seal-private-evidence-index.json'
    need(sha(private_index) == 'd5e94e522876d24b840face5f81347339642b5709236bf70520b431e4b0e0331', 'seal history index changed')
    entries = load(private_index)['files']
    need(len(entries) == 25 and len({x['path'] for x in entries}) == 25, 'seal history membership differs')
    for item in entries:
        path = audit.bound_path(authdir, item['path'])
        need(sha(path) == item['sha256'] and path.stat().st_size == item['bytes'], 'seal history member changed')
    # The original preparation writer used a pinned digest-map, not either
    # current typed index schema. This historical contract remains explicit.
    for name, digest in load(prep/'non-delete-evidence-index.json')['files'].items():
        need(sha(audit.bound_path(prep, name)) == digest, 'original preparation member changed')
    allocation = load(prep/'allocation.json'); descriptors = load(prep/'owner-descriptors.json')
    need(allocation['execution'] == candidate['Execution'] == EXECUTION and len(allocation['owners']) == 86
         and set(allocation['owners']) == set(descriptors) and not ids.intersection(allocation['owners']), 'old allocation overlaps or differs')
    for cid in allocation['owners']:
        actual = inspect(cid)
        need(closed(actual) and project_owner(actual) == descriptors[cid], 'old owner topology/closure changed')
        ids.add(cid)
    for name, digest in allocation['build_files'].items():
        need(sha(audit.bound_path(private, name)) == digest, 'old original build member changed')
    windows = load(packet/'profile-windows.json')
    seal = 'seal-history/718f79ee-1eeb-4ee9-9b9c-32f50c3e6b7c/'
    budgets, backups = {}, {}
    for a, g, b in zip(allocation['groups'], candidate['Groups'], bindings):
        group = g['Group']; folder = private/group
        need(group == a['group'] == b['group'], 'group order differs')
        common = dict(Enabled=False, Execution=EXECUTION, CandidateSHA256=PINS['remaining-candidate16/candidate.json'],
                      ConfirmationSHA256=sha(packet/'confirmation-template.txt'), NotBefore=candidate['NotBefore'], ExpiresAt=candidate['NotAfter'],
                      ConfirmedAt='0001-01-01T00:00:00Z', MaxPerTarget=1, MaxTotal=6)
        for stage in g['Helpers']:
            owner = folder/stage
            need(load(owner/'permission.json') == dict(common, Profile=candidate['Profile'], BinarySHA256=candidate['SenderBinarySHA256'])
                 and not (owner/'human-confirmation.txt').exists(), 'old operator permission/authority changed')
            need(sha(owner/'operator.json') == g['ConfigSHA256'][stage], 'old operator config changed')
            if stage.startswith('recover-'):
                archived = authdir/(seal+group+'/recovery-'+stage)
                expected = next(x['config_sha256'] for x in windows['recoveries'] if x['group'] == group and x['stage'] == stage)
                revoked((archived/'original.private').read_bytes(), (owner/'recovery.json').read_bytes(), load(archived/'revocation.json'), expected, 'Enabled')
        for action in b['root_owners']:
            owner = folder/('root-'+action)
            need(load(owner/'permission.json') == common and not (owner/'human-confirmation.txt').exists(), 'old root permission/authority changed')
        for stage, expected in g['CaseConfigSHA256'].items():
            archived = authdir/(seal+group+'/case-'+stage)
            revoked((archived/'original.private').read_bytes(), (folder/('case-'+stage)/'remaining.json').read_bytes(), load(archived/'revocation.json'), expected, 'Base.Enabled')
        for name, key in [('core.dump','CoreDumpSHA256'), ('globals.sql','GlobalsSHA256'), ('native.tar','NativeArchiveSHA256')]:
            need(sha(folder/'checkpoint'/name) == g[key], 'old backup changed'); backups[group+'/'+name] = g[key]
        volume = json.loads(docker('volume','inspect',a['volumes']['budget']))[0]
        mounted = docker('ps','-aq','--no-trunc','--filter','volume='+volume['Name']).decode('utf-8','strict').split()
        need(volume['Labels'].get('lifecycle307.remaining') == EXECUTION and volume['Labels'].get('lifecycle307.allocation') == allocation['allocation_nonce']
             and sorted(mounted) == a['budget_expected_mounted_cids'], 'old budget ownership/mount expansion')
        sequence.budget_archive(docker('cp',g['Helpers']['wire']+':/run/budget','-'),candidate,group,initial=True)
        budgets[group] = {'volume':volume['Name'], 'mounted_cids':sorted(mounted), 'uid':10001, 'files':[]}
        gateway = inspect(g['Gateway'])
        mount = next(x for x in gateway['Mounts'] if x['Destination'] == '/etc/caddy/Caddyfile')
        need(not mount['RW'] and sha(hostpath(mount['Source'])) == g['SealedGatewaySHA256'], 'old gateway no longer sealed')
    return {'execution':EXECUTION, 'terminal_sha256':PINS['delete-confirmed-v26-execution01/window01/terminal.json'],
            'status':'PASS_RETAINED_V26_PHYSICAL_FILES_BUDGET_ONLY', 'historical_identity_closure':'UNCONFIRMED',
            'approved_references':76, 'cases_revoked':6, 'recoveries_revoked':6, 'backups':backups, 'budgets':budgets, 'never_replayed':True}
