"""Keep expired C22 and every observed budget; no source start or authority.

The earlier 996-owner gate still proves its original history. This append-only
anchor adds C22's 89 closed owners and an explicit complete 40-budget inventory,
including the seven real host-path rehearsal budgets omitted from old totals.
"""
import hashlib
import json
import stat
from pathlib import Path
import evidence_indexes as audit
import retained_v30 as prior

EXECUTION = '0e5cb8f3-46a9-4339-88e4-7154d6ebe13f'
OLD_ROOT = Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')
REL = 'docs/acceptance/evidence/issue307-d1'
ANCHOR = 'candidate23-planning01/retained-candidate22-anchor01.json'
ANCHOR_SHA256 = '6af1a5bf3ce4a4f599ca1de4cc7f81f8681e065123fb1e65cd9420fd3f167f2f'
need, sha, load = prior.need, prior.sha, prior.load
ISSUED_PINS = {
    'docs/acceptance/issue307-d1-candidate22-final-confirmation01.md': '40afd412b637597b367a9680dd476d4d9b24cfc993b80b5b04ca23fa5a1bdf29',
    REL+'/candidate22-publication01/branch-freeze01/execution-binding.json': 'a76132a5abc97b16209cf95317376ce0a9cfb8b3a3d7c9c845fd0509071bc437',
    REL+'/candidate22-planning01/approval-reference-manifest01.json': '7d75df3c48f8beca3c7bcedacad563a069112e0a2eae546afebaf1fcb321094e',
    REL+'/candidate22-planning01/final-static-evidence-audit-plan01.json': '9893a44b861cfcfd0d817e20e5e65b6e4cbcecca8bfc8edb881164f169206a5c',
}


def issued_packet(root):
    for name, digest in ISSUED_PINS.items():
        audit.checked_bytes(audit.bound_path(root, name), digest)
    plan = root/REL/'candidate22-planning01/final-static-evidence-audit-plan01.json'
    return audit.audit_plan(root, plan, ISSUED_PINS[plan.relative_to(root).as_posix()],
                            root/REL/'remaining-candidate22/candidate.json')


def absent(path):
    """Like the existing control-fact gate, only ENOENT means absence.

    lstat every original component first: dangling links/reparse points and
    unreadable parents must not be recorded as NOT_RECEIVED/NOT_CREATED.
    """
    path = Path(path).absolute()
    need('..' not in path.parts, 'traversing authority path')
    for component in reversed((path, *path.parents)):
        try:
            info = component.lstat()
        except FileNotFoundError:
            return True
        need(not stat.S_ISLNK(info.st_mode)
             and not (getattr(info, 'st_file_attributes', 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT),
             'authority path is linked or reparse-aliased')
        if component != path:
            need(stat.S_ISDIR(info.st_mode), 'authority parent is not a directory')
    return False


def closed_rows(docker, cids, closed):
    need(type(cids) is list and len(cids) == len(set(cids))
         and all(len(cid) == 64 and all(c in '0123456789abcdef' for c in cid) for cid in cids),
         'C22 full closed CID membership invalid')
    rows = []
    for at in range(0, len(cids), 48):
        rows += prior.json_docker(docker, 'inspect', *cids[at:at+48])
    need(len(rows) == len({x['Id'] for x in rows}) == len(cids)
         and {x['Id'] for x in rows} == set(cids) and all(closed(x) for x in rows),
         'C22 owner missing, active or lookup substituted')
    return {x['Id']: x for x in rows}


def cfg_digest(item):
    return hashlib.sha256(json.dumps({k:item[k] for k in
        ('Id','Name','Image','Config','HostConfig','Mounts')}, sort_keys=True).encode()).hexdigest()


def resource(docker, row):
    expected = row['metadata']
    actual = prior.json_docker(docker, row['kind'], 'inspect', row['identity'])
    fields = prior.VOLUME_FIELDS if row['kind'] == 'volume' else prior.NETWORK_FIELDS
    need(len(actual) == 1 and {k:actual[0].get(k) for k in fields} == expected,
         'C22 resource metadata/ownership changed')
    if row['kind'] == 'volume':
        prior.mounts(docker, row['identity'], row['mounted_cids'])
    else:
        need(expected['Internal'] is True and not expected['Containers'], 'C22 network not closed/internal')


def budget(docker, row):
    resource(docker, row)
    need(row['kind'] == 'volume' and row['wire'] in row['mounted_cids'], 'C22 budget reader binding differs')
    raw = prior.raw(docker, 'cp', row['wire']+':/run/budget', '-')
    need(hashlib.sha256(raw).hexdigest() == row['sha256'] and len(raw) == row['bytes'],
         'retained actual budget contents changed; no refund/reset allowed')


def verify(root, inspect, closed, project_owner, docker, sequence, base, private, hostpath):
    need(type(ANCHOR_SHA256) is str and len(ANCHOR_SHA256) == 64, 'C22 retention anchor not frozen')
    anchor = audit.decode(audit.checked_bytes(root/REL/ANCHOR, ANCHOR_SHA256))
    old = private/'remaining-candidate22'
    need(anchor['status'] == 'C22_RETAINED_EXPIRED_FROZEN_ANCHOR'
         and anchor['execution'] == EXECUTION and anchor['profile'] == 'candidate22/v32'
         and anchor['authority_reusable'] is False
         and anchor['source_start'] == anchor['native_delete'] == 0, 'C22 anchor scope differs')
    need(len(base['closed_cids']) == 996 and base['closed_cids'] == anchor['prior_closed_cids'],
         'pre-C22 original owner union differs')
    for name, meta in anchor['public_files'].items():
        audit.metadata(meta)
        for source in (root, OLD_ROOT):
            raw = audit.checked_bytes(audit.bound_path(source, name), meta['sha256'])
            need(len(raw) == meta['bytes'], 'original C22 public bytes differ')
    need(prior.file_set(old) == anchor['private_files'], 'C22 private bytes/membership differ')
    terminal = load(old/'preparation-terminal.json')
    need(terminal == load(root/REL/'candidate22-expiry01/terminal.json')
         and terminal['status'] == 'EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED'
         and terminal['execution'] == EXECUTION and terminal['retry_allowed'] is False
         and terminal['authorization'] == 'NOT_RECEIVED' and terminal['schedule'] == 'NOT_CREATED'
         and terminal['native_delete_this_observation'] == 0
         and terminal['fresh_native_IAM'] == 'NOT_OBSERVED_SOURCE_NOT_STARTED', 'C22 terminal relabelled')
    need(absent(old/'authorized-execution01')
         and absent(OLD_ROOT/REL/'delete-confirmed-v32-execution01')
         and absent(Path('C:/Users/Henry/.codex/automations/307-d1-candidate22-v32')),
         'expired C22 received authority or resumed')
    a = load(old/'allocation.json')
    need(a == load(root/REL/'remaining-candidate22-preparation01/allocation.json')
         and a['execution'] == EXECUTION, 'C22 original allocation drift')
    expected = anchor['owners']
    need(len(expected) == 89 and set(a['owners']) <= set(expected)
         and len(a['owners']) == 86 and not set(expected).intersection(base['closed_cids']),
         'C22 owner membership differs')
    rows = closed_rows(docker, sorted(expected), closed)
    for cid, item in rows.items():
        saved = expected[cid]
        need(cfg_digest(item) == saved['config_sha256']
             and {k:item['State'][k] for k in prior.STATE_FIELDS} == saved['state'],
             'C22 original owner restarted or configuration changed')
    permissions = list(old.glob('*/*/permission.json'))
    need(len(permissions) == 30 and all(load(p).get('Enabled') is False for p in permissions)
         and not list(old.rglob('human-confirmation.txt')), 'C22 permission or confirmation appeared')
    need(len(anchor['resources']) == 18, 'C22 resource membership differs')
    for row in anchor['resources']:
        need(row['metadata']['Labels'].get('lifecycle307.remaining') == EXECUTION
             and row['metadata']['Labels'].get('lifecycle307.allocation') == a['allocation_nonce'],
             'C22 resource not owned by original allocation')
        resource(docker, row)
    budgets = anchor['budgets']
    need(len(budgets) == 40 and len({row['identity'] for row in budgets}) == 40,
         'all 40 historical budgets, including 7 rehearsal budgets, must remain')
    for row in budgets:
        budget(docker, row)
    ids = sorted(set(base['closed_cids']) | set(expected))
    need(len(ids) == 1085, 'C22 retained closed union differs')
    return dict(status='PASS_RETAINED_V32_EXPIRED_FROZEN_CLOSED_1085', closed_cids=ids,
                anchor_sha256=ANCHOR_SHA256, prior=base, historical_budgets_preserved=40,
                backup_files_preserved=75, authorization_reusable=False, pins_released=0,
                budgets_refunded=0, native_delete=0)
