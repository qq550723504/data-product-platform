"""Cold successor dependencies; maintenance State snapshots are never runtime gates."""
import retired_resources as r

SCHEMA = 'd1-successor-cold-dependencies-v1'
SOURCE_KEYS = {group + ':' + role for group in ('business-races', 'late-hold', 'mixed')
               for role in ('Core', 'Native')} | {'business-races:Redis'}


def namespace_parent(item, rows):
    mode = item['HostConfig']['NetworkMode']
    if not mode.startswith('container:'):
        return None
    target = mode.split(':', 1)[1]
    matches = [cid for cid, row in rows.items()
               if target and (cid.startswith(target) or row['Name'].lstrip('/') == target)]
    r.need(len(matches) == 1, 'required netns parent absent or ambiguous')
    return matches[0]


def build(plan, volume_plan, frozen_rows, sources):
    """Derive the finite custody/forward-parent closure from pinned facts only."""
    r.need(set(sources) == SOURCE_KEYS and len(set(sources.values())) == len(SOURCE_KEYS),
           'complete distinct source roles required')
    for cid in sources.values():
        r.full_cid(cid)
    retired = set(plan['retired_historical'])
    removed_volumes = set(volume_plan['remove'])
    volumes = {name: dict(metadata=row['metadata'],
                         mounted=sorted(set(row['mounted_before']) - retired))
               for name, row in plan['volumes'].items() if name not in removed_volumes}
    original = {**plan['live'], **plan['external'], **plan.get('additional_live', {})}
    r.need(set(sources.values()) <= set(original), 'source role not bound to the fixed plan')
    needed = set(sources.values())
    for row in volumes.values():
        needed.update(row['mounted'])
    pending = list(needed)
    while pending:
        cid = pending.pop()
        r.need(cid not in retired and cid in frozen_rows,
               'required dependency is not retained in the fixed plan')
        row = frozen_rows[cid]
        r.need(row['Id'] == cid and r.closed(row),
               'fixed dependency descriptor/closure differs')
        if cid in original:
            r.need(r.descriptor(row) == original[cid]['descriptor'], 'original dependency descriptor differs')
        parent = namespace_parent(row, frozen_rows)
        if parent is not None and parent not in needed:
            needed.add(parent)
            pending.append(parent)
    required = {cid: dict(descriptor=r.descriptor(frozen_rows[cid]),
                         netns_parent=namespace_parent(frozen_rows[cid], frozen_rows))
                for cid in sorted(needed)}
    # No State/PID/time fields or unrelated session membership enter this hash.
    return dict(schema=SCHEMA, phase='COLD_PREPARATION', sources=sources,
                required=required, volumes=volumes,
                retired_cids=sorted(retired), retired_volumes=sorted(removed_volumes))


def verify(contract, docker):
    r.need(contract['schema'] == SCHEMA and contract['phase'] == 'COLD_PREPARATION',
           'unsupported successor dependency phase')
    ids = docker.inventory()  # Failure is not absence; unrelated State is ignored.
    required = set(contract['required'])
    r.need(required <= ids and not ids.intersection(contract['retired_cids']),
           'required dependency missing or retired CID present')
    rows = docker.inspect(sorted(ids))
    for cid, expected in contract['required'].items():
        item = rows[cid]
        r.need(r.closed(item) and r.descriptor(item) == expected['descriptor']
               and namespace_parent(item, rows) == expected['netns_parent'],
               'required dependency running or descriptor/topology drift')
    raw = docker.raw('volume', 'ls', '-q')
    r.need(type(raw) is bytes, 'volume inventory must be bytes')
    names = raw.decode('utf8', 'strict').split()
    r.need(len(names) == len(set(names)) and set(contract['volumes']) <= set(names)
           and not set(contract['retired_volumes']).intersection(names),
           'retained volume missing or retired volume present')
    wanted = sorted(contract['volumes'])
    values = docker.json('volume', 'inspect', *wanted) if wanted else []
    r.need(type(values) is list and len(values) == len(wanted)
           and {v['Name'] for v in values} == set(wanted), 'volume metadata membership differs')
    current = {v['Name']: v for v in values}
    for name, expected in contract['volumes'].items():
        r.need({k: current[name].get(k) for k in r.VOLUME} == expected['metadata'],
               'retained volume identity/ownership drift')
        users = {cid for cid, item in rows.items()
                 if any(m['Type'] == 'volume' and m['Name'] == name for m in item['Mounts'])}
        r.need(users == set(expected['mounted']), 'retained volume complete mount graph drift')
    return dict(status='PASS_CURRENT_SUCCESSOR_COLD_DEPENDENCIES',
                contract_sha256=r.sha(r.encoded(contract)), required_owners=len(required),
                source_roles=len(contract['sources']), retained_volumes=len(wanted))
