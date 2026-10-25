"""Finite Docker acceptance-resource retirement, never provider deletion.

Historical inspect is an archive fact. Only successful current Docker reads
prove current presence/absence. No missing-resource exception is suppressed.
"""
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import subprocess
import evidence_indexes as audit

SCHEMA = 'd1-resource-retirement-v1'
STATE = ('Status', 'Running', 'Pid', 'StartedAt', 'FinishedAt', 'ExitCode',
         'OOMKilled', 'Dead', 'Paused', 'Restarting')
VOLUME = ('Name', 'Driver', 'Scope', 'Options', 'Labels', 'CreatedAt')


def need(ok, message):
    if not ok:
        raise ValueError('resource retirement: ' + message)


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def encoded(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode('utf-8')


def sha(raw):
    need(type(raw) is bytes, 'raw bytes required')
    return hashlib.sha256(raw).hexdigest()


def write(path, raw):
    need(type(raw) is bytes, 'exclusive writer requires bytes')
    with Path(path).open('xb') as stream:
        stream.write(raw)
        stream.flush()
        import os
        os.fsync(stream.fileno())


def save(path, value):
    write(path, encoded(value))


def read(path):
    return audit.decode(audit.unlinked_absolute(path).read_bytes())


def full_cid(value):
    need(type(value) is str and re.fullmatch('[0-9a-f]{64}', value), 'full CID required')
    return value


def unique(values):
    need(type(values) is list and len(values) == len(set(values)), 'duplicate or invalid CID list')
    return {full_cid(x) for x in values}


def closed(item):
    state, host = item['State'], item['HostConfig']
    return (state['Status'] in ('created', 'exited') and state['Running'] is False
            and state['Pid'] == 0 and not any(state[k] for k in ('Paused', 'Restarting', 'Dead'))
            and host['RestartPolicy']['Name'] == 'no' and not host.get('AutoRemove')
            and not host.get('PortBindings') and not any(item['NetworkSettings']['Ports'].values()))


def descriptor(item):
    # Full Config includes secrets: this digest is public, raw inspect private.
    return sha(encoded({k: item[k] for k in ('Id', 'Name', 'Image', 'Config', 'HostConfig', 'Mounts')}))


def historical_cfg(item):
    # The original v27 format, kept byte-for-byte rather than repacked as ours.
    return sha(json.dumps({k: item[k] for k in ('Image', 'Config', 'HostConfig', 'Mounts')},
                          sort_keys=True).encode())


def dns_representation_for_history(item, expected):
    """Only a proven null->[] representation delta, never a live-state cache.

    The full original config digest must match after this one zero-custom-DNS
    change. No arbitrary key omission, custom DNS or archived State is allowed.
    This adapter is only used by the new maintenance archive's historical
    comparison. The frozen C22 executable and actual archived inspect stay raw.
    """
    if historical_cfg(item) == expected:
        return item, False
    need(item['HostConfig']['Dns'] == [] and type(item['HostConfig']['Dns']) is list,
         'unproven historical descriptor or custom DNS')
    projected = {**item, 'HostConfig': {**item['HostConfig'], 'Dns': None}}
    need(historical_cfg(projected) == expected, 'historical config differs beyond empty DNS representation')
    return projected, True


def state(item):
    return {k: item['State'][k] for k in STATE}


def budget_membership(rows, volumes, origins):
    """Exact identity union, not a sum of per-generation budget counts.

    Origins are pinned allocation/packet/failed-rehearsal facts. The entire
    host mount graph is checked, including helpers outside the removal scope.
    Equal duplicate references coalesce; conflicting references fail closed.
    """
    by_name = {}
    for origin in origins:
        name = origin['volume']
        expected = dict(execution=origin['execution'], nonce=origin['nonce'],
                        wire=full_cid(origin['wire']), mounted=sorted(unique(origin['mounted'])))
        need(origin['references'], 'budget origin references missing')
        if name in by_name:
            need(by_name[name] == expected, 'conflicting duplicate budget origin')
        by_name[name] = expected
    observed = {}
    for cid, item in rows.items():
        for mount in item['Mounts']:
            if mount['Type'] == 'volume' and mount['Destination'] == '/run/budget':
                observed.setdefault(mount['Name'], set()).add(cid)
    need(set(observed) == set(by_name), 'budget identity union differs from pinned origins')
    need(len(volumes) == len({v['Name'] for v in volumes})
         and {v['Name'] for v in volumes} == set(by_name), 'budget volume metadata membership differs')
    metadata = {v['Name']: v for v in volumes}
    for name, origin in by_name.items():
        labels = metadata[name].get('Labels') or {}
        need(labels.get('lifecycle307.remaining') == origin['execution']
             and labels.get('lifecycle307.allocation') == origin['nonce'], 'budget execution/allocation ownership differs')
        users = {cid for cid, item in rows.items() if any(m['Type'] == 'volume' and m['Name'] == name for m in item['Mounts'])}
        need(users == set(origin['mounted']) == observed[name], 'budget complete mounted CID membership differs')
        need(origin['wire'] in users and rows[origin['wire']]['Config']['Cmd'] == ['-stage', 'wire']
             and all(closed(rows[cid]) for cid in users), 'budget wire identity or closure differs')
    return by_name


def private_original_roots(private, rows, scope, references):
    """Only original D1 bind/provenance owners, never every private session."""
    private=Path(private).resolve();roots=set()
    paths=[Path(entry['path']).resolve() for entry in references]
    paths += [Path(mount['Source']).resolve() for cid in scope for mount in rows[cid]['Mounts'] if mount['Type']=='bind']
    for path in paths:
        if not path.is_relative_to(private):continue
        parts=path.relative_to(private).parts
        need(parts and parts[0]!='resource-retirement01','broad or recursive maintenance original root')
        roots.add(private/parts[0])
    need(roots,'no pinned D1 private originals')
    return roots


class Docker:
    def __init__(self, journal=None):
        self.journal = journal
        self.number = 0

    def raw(self, *args):
        self.number += 1
        # The only mutating verb is exact rm without options, exercised by remove.
        need(args and (args[0] in ('inspect', 'ps', 'logs', 'cp', 'volume', 'network')
                       or (args[0] == 'rm' and len(args) == 2 and full_cid(args[1]))),
             'unapproved Docker verb')
        if args[0] == 'cp':
            need(len(args) == 3 and re.fullmatch('[0-9a-f]{64}:/run/budget', args[1])
                 and args[2] == '-', 'budget cp must be readonly stdout archive')
        if args[0] in ('volume', 'network'):
            need(args[1] == 'inspect' or (args[0] == 'volume' and args[1:] == ('ls','-q')), 'only metadata inspect/inventory allowed')
        start = now()
        result = subprocess.run(['docker', *map(str, args)], capture_output=True, timeout=60)
        need(type(result.stdout) is bytes and type(result.stderr) is bytes, 'Docker bytes contract')
        if self.journal:
            prefix = self.journal / ('%06d' % self.number)
            # Full inspect is archived once per owner. Repeated fresh inventory
            # hashes avoid multiplying hundreds of MB of identical private Env.
            if args[0] in ('rm', 'cp', 'logs') or result.returncode:
                write(prefix.with_suffix('.stdout'), result.stdout)
                write(prefix.with_suffix('.stderr'), result.stderr)
            save(prefix.with_suffix('.json'), dict(argv=list(args), started_at=start,
                 observed_at=now(), exit_code=result.returncode,
                 stdout_sha256=sha(result.stdout), stderr_sha256=sha(result.stderr)))
        need(result.returncode == 0, 'Docker command failed; raw channels retained')
        return result.stdout

    def json(self, *args):
        return json.loads(self.raw(*args).decode('utf-8', 'strict'))

    def inventory(self):
        ids = self.raw('ps', '-aq', '--no-trunc').decode('utf-8', 'strict').split()
        need(len(ids) == len(set(ids)), 'duplicate inventory identity')
        return {full_cid(cid) for cid in ids}

    def inspect(self, ids):
        values = []
        for at in range(0, len(ids), 48):
            values += self.json('inspect', *ids[at:at + 48])
        need(len(values) == len({x['Id'] for x in values}) == len(ids)
             and {x['Id'] for x in values} == set(ids), 'inspect response membership differs')
        return {x['Id']: x for x in values}


def netns_edges(rows):
    result = {}
    for cid, item in rows.items():
        mode = item['HostConfig']['NetworkMode']
        if not mode.startswith('container:'):
            continue
        target = mode.split(':', 1)[1]
        matches = [ident for ident, row in rows.items()
                   if ident.startswith(target) or row['Name'].lstrip('/') == target]
        need(len(matches) == 1, 'netns target absent or ambiguous')
        result.setdefault(matches[0], set()).add(cid)
    return result


def removal_order(candidates, rows):
    edges = netns_edges(rows)
    need(all(edges.get(cid, set()) <= candidates for cid in candidates), 'external netns dependent')
    pending, result = set(candidates), []
    while pending:
        leaves = sorted(cid for cid in pending if not edges.get(cid, set()).intersection(pending))
        need(leaves, 'cyclic netns dependency')
        result.extend(leaves)
        pending.difference_update(leaves)
    return result


def archive_file(root, spec):
    audit.metadata({'sha256': spec['sha256'], 'bytes': spec['bytes']})
    path = audit.bound_path(root, spec['path'])
    raw = audit.checked_bytes(path, spec['sha256'])
    need(len(raw) == spec['bytes'], 'archive byte count differs')
    return raw


def plan_at(path, digest):
    raw = audit.checked_bytes(audit.unlinked_absolute(path), digest)
    plan = audit.decode(raw)
    need(plan['schema'] == SCHEMA and plan['provider_delete'] == 0 and plan['volume_delete'] == 0,
         'maintenance scope differs')
    candidates, kept = unique(plan['retired_historical']), unique(plan['live_required'])
    external = unique(plan['external_held'])
    need(candidates and kept and not candidates.intersection(kept | external)
         and not kept.intersection(external), 'retirement/live scopes overlap')
    need(set(plan['archives']) == candidates and set(plan['live']) == kept
         and set(plan['external']) == external, 'descriptor or archive membership differs')
    need(set(plan['order']) == candidates and len(plan['order']) == len(candidates), 'order scope differs')
    need(plan['authority']['source_thread'] and plan['authority']['human_message_id']
         and plan['authority']['human_text'], 'direct human maintenance authority missing')
    return plan


def immutable(plan, root):
    if 'budget_origins' in plan:
        origins = audit.decode(archive_file(root, plan['budget_origins']))
        for reference in origins['references'].values():
            raw = audit.checked_bytes(audit.unlinked_absolute(reference['path']), reference['sha256'])
            need(len(raw) == reference['bytes'], 'original budget reference bytes differ')
    for cid, row in plan['archives'].items():
        original = audit.decode(archive_file(root, row['inspect']))
        need(original['Id'] == cid and closed(original) and descriptor(original) == row['descriptor'],
             'historical archive identity/closure differs')
        archive_file(root, row['stdout'])
        archive_file(root, row['stderr'])
    originals = audit.decode(archive_file(root, plan['originals']))
    for name, metadata in originals['files'].items():
        path = audit.unlinked_absolute(name)
        need(path.is_file() and path.stat().st_nlink == 1, 'original evidence aliased')
        raw = audit.checked_bytes(path, metadata['sha256'])
        need(len(raw) == metadata['bytes'], 'original evidence byte count differs')
    for row in plan['budgets']:
        # This is the actual old tar, including used/UNKNOWN attempts, never an
        # invented empty projection. No extraction or content rewrite occurs.
        archive_file(root, row['archive'])
    archive_file(root, plan['legacy_gate'])
    return len(originals['files'])


def present_contract(plan, docker, *, removed):
    candidates, kept = unique(plan['retired_historical']), unique(plan['live_required'])
    external = unique(plan['external_held'])
    ids = docker.inventory()  # Docker failure is never interpreted as absence.
    need((kept | external) <= ids and removed <= candidates and not removed.intersection(ids),
         'required live missing or retired CID still present')
    pending = candidates - removed
    need(pending <= ids, 'missing candidate has no fixed removal receipt')
    rows = docker.inspect(sorted(ids))
    for key, members in (('live', kept), ('external', external)):
        for cid in members:
            expected = plan[key][cid]
            need(closed(rows[cid]) and descriptor(rows[cid]) == expected['descriptor']
                 and state(rows[cid]) == expected['state'], 'kept resource restarted or changed')
    for cid, expected in plan.get('additional_live', {}).items():
        need(cid in rows and closed(rows[cid]) and descriptor(rows[cid]) == expected['descriptor']
             and state(rows[cid]) == expected['state'], 'successor or budget owner changed')
    for cid, expected in plan['running_protected'].items():
        need(cid in rows and descriptor(rows[cid]) == expected['descriptor']
             and state(rows[cid]) == expected['state'], 'other session runtime changed')
    for cid in pending:
        expected = plan['archives'][cid]
        need(closed(rows[cid]) and descriptor(rows[cid]) == expected['descriptor']
             and state(rows[cid]) == expected['state'], 'candidate restarted or changed')
    removal_order(pending, rows)  # Also rejects any newly-created external caller.
    names = sorted(plan['volumes'])
    values = docker.json('volume', 'inspect', *names) if names else []
    need(len(values) == len({x['Name'] for x in values}) == len(names)
         and {x['Name'] for x in values} == set(names), 'volume inspect membership differs')
    volumes = {x['Name']: x for x in values}
    for name, expected in plan['volumes'].items():
        need({k: volumes[name].get(k) for k in VOLUME} == expected['metadata'],
             'preserved volume identity/ownership drift')
        mounted = {cid for cid, item in rows.items()
                   if any(m['Type'] == 'volume' and m['Name'] == name for m in item['Mounts'])}
        need(mounted == set(expected['mounted_before']) - removed, 'preserved volume mount graph drift')
    return rows


def receipts(plan, directory):
    directory = audit.unlinked_absolute(directory)
    removed = set()
    allowed = set(plan['retired_historical'])
    for path in directory.iterdir():
        need(path.is_dir() and not path.is_symlink() and not path.is_junction()
             and path.name in allowed, 'unbound removal receipt')
        intent = read(path / 'intent.json')
        result = read(path / 'result.json')  # Missing/unknown outcome is blocking.
        need(intent == dict(cid=path.name, plan_sha256=plan['_sha256'], operation='docker rm')
             and result['cid'] == path.name and result['plan_sha256'] == plan['_sha256']
             and result['status'] == 'REMOVED_EXACT_CID' and result['exit_code'] == 0
             and result['stdout'].split() == [path.name] and result['confirmed_absent'] is True,
             'removal receipt scope or outcome differs')
        removed.add(path.name)
    return removed


def verify(path, digest, root, receipt_root, docker, *, complete):
    plan = plan_at(path, digest)
    plan['_sha256'] = digest
    count = immutable(plan, root)
    removed = receipts(plan, receipt_root)
    if complete:
        need(removed == set(plan['retired_historical']), 'retirement incomplete')
    present_contract(plan, docker, removed=removed)
    return dict(status='PASS_RETIRED_HISTORY' if complete else 'PASS_PREPARED_RETIREMENT',
                plan_sha256=digest, archived=len(plan['archives']), removed=len(removed),
                live_required=plan['live_required'], original_files=count,
                provider_delete=0, volume_delete=0, pins_released=0, budgets_refunded=0)


def remove(path, digest, root, receipt_root, docker, binding, binding_sha256, source_root):
    plan = plan_at(path, digest)
    plan['_sha256'] = digest
    frozen = audit.decode(audit.checked_bytes(audit.unlinked_absolute(binding), binding_sha256))
    need(frozen['status'] == 'FROZEN_MAINTENANCE_ONLY' and frozen['plan_sha256'] == digest
         and frozen['provider_delete_authorized'] is False and frozen['volumes_authorized'] is False,
         'frozen maintenance binding differs')
    for name, expected in frozen['source_files'].items():
        audit.checked_bytes(audit.bound_path(source_root, name), expected)
    for name, expected in frozen.get('precondition_files', {}).items():
        audit.checked_bytes(audit.bound_path(source_root, name), expected)
    # Evidence and originals are all checked before the first mutation.
    immutable(plan, root)
    removed = receipts(plan, receipt_root)
    order = [cid for cid in plan['order'] if cid not in removed]
    for at in range(0, len(order), 24):
        batch = order[at:at + 24]
        rows = present_contract(plan, docker, removed=removed)
        allowed_order = removal_order(set(order[at:]), rows)
        need(batch == [cid for cid in plan['order'] if cid not in removed][:len(batch)]
             and set(batch) <= set(allowed_order), 'fixed removal order drift')
        # Batch leaves may share netns roots in later batches, never external.
        for cid in batch:
            need(not netns_edges(rows).get(cid, set()) - removed,
                 'netns root must follow every dependent')
            folder = Path(receipt_root) / cid
            folder.mkdir(exist_ok=False)
            save(folder / 'intent.json', dict(cid=cid, plan_sha256=digest, operation='docker rm'))
            raw = docker.raw('rm', cid)  # No --force, --volumes, name, or prefix.
            output = raw.decode('utf-8', 'strict')
            need(output.split() == [cid], 'Docker rm returned a different identity')
            absent = cid not in docker.inventory()
            save(folder / 'result.json', dict(cid=cid, plan_sha256=digest,
                 status='REMOVED_EXACT_CID' if absent else 'UNCONFIRMED',
                 exit_code=0, stdout=output, confirmed_absent=absent, observed_at=now()))
            need(absent, 'removed CID remains present')
            removed.add(cid)
        print(encoded(dict(removed=len(removed), total=len(plan['order']), provider_delete=0)).decode(), flush=True)
    return verify(path, digest, root, receipt_root, docker, complete=True)
