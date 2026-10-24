"""One-shot read-only archive. No container/service start or removal."""
import argparse
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import retired_resources as r

OLD = Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private')
AUDIT = Path('C:/Users/Henry/Documents/Codex/2026-10-23/new-chat/outputs/DPP307-dependency-audit.json')
EXECUTION = '0e5cb8f3-46a9-4339-88e4-7154d6ebe13f'
HEAD = '8de3bc4c9f737c3c3e81fddb60491422ba6039f2'
OLD_V27_PLAN = 'docs/acceptance/evidence/issue307-d1/post-v27-seventeen01/plan.json'
OLD_V27_SHA = '26d27adacec543172a7c570ca3a8d174565348e9c424e5a17dea06b286cc1667'
OLD_V28_PLAN = 'docs/acceptance/evidence/issue307-d1/post-v28-seventeen01/plan.json'
OLD_V28_SHA = '5f2574f9bdca02cfce9782896b54c8d1bc3b21d49928617d25e01267aac4fb41'


def file_spec(root, path):
    raw = path.read_bytes()
    return dict(path=path.relative_to(root).as_posix(), sha256=r.sha(raw), bytes=len(raw))


def current_cold_contract():
    """Reuse the actual current allocation/role/window/budget contracts only.

    Does not call state()/historical_gate or assert that the old frozen full
    entry passed. No source/helper start or native IAM read is performed.
    """
    sys.path.insert(0, str(OLD / 'examples/lifecycle-d1'))
    spec = importlib.util.spec_from_file_location('retirement_current_cold', OLD / 'examples/lifecycle-d1/six_candidate_runtime.py')
    current = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(current)
    p = current.p
    packet = OLD / 'docs/acceptance/evidence/issue307-d1/remaining-candidate22'
    allocation = r.read(current.PUBLIC / 'allocation.json')
    r.need(allocation == r.read(current.CONTROL / 'allocation.json')
           and allocation['execution'] == EXECUTION, 'current allocation private/public differ')
    p.window.validate_packet(packet, current.CONTROL)
    current.build_contract(allocation)
    topology = current.cold_topology(allocation)
    candidate = r.read(packet / 'candidate.json')
    permissions = list(current.CONTROL.glob('*/*/permission.json'))
    r.need(len(permissions) == 30 and all(r.read(path)['Enabled'] is False for path in permissions),
           'current send/root permission enabled')
    for group in candidate['Groups']:
        current.p.sequence.budget_archive(current.docker('cp', group['Helpers']['wire'] + ':/run/budget', '-'),
                                          candidate, group['Group'], initial=True)
        folder = current.CONTROL / group['Group'] / 'checkpoint'
        for name, key in [('core.dump', 'CoreDumpSHA256'), ('globals.sql', 'GlobalsSHA256'), ('native.tar', 'NativeArchiveSHA256')]:
            r.audit.checked_bytes(folder / name, group[key])
    return dict(status='PASS_CURRENT_C22_COLD_CONTRACT_ONLY', topology=topology,
                permissions_false=30, budgets_empty=3, backups=9, source_started=0, provider_delete=0,
                not_a_frozen_full_entry_pass=True, observed_at=r.now())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--authority', type=Path, required=True)
    parser.add_argument('--budget-origins', type=Path, required=True)
    parser.add_argument('--budget-origins-sha256', required=True)
    args = parser.parse_args()
    root = args.archive.absolute()
    root.mkdir(exist_ok=False)
    (root / 'journal').mkdir()
    (root / 'owners').mkdir()
    (root / 'budgets').mkdir()
    (root / 'removals').mkdir()
    docker = r.Docker(root / 'journal')
    try:
        authority = r.read(args.authority)
        origins = r.audit.decode(r.audit.checked_bytes(args.budget_origins,args.budget_origins_sha256))
        r.need(origins['status']=='FROZEN_EXPLICIT_BUDGET_ORIGIN_UNION','unbound budget origins')
        for reference in origins['references'].values():
            raw=r.audit.checked_bytes(r.audit.unlinked_absolute(reference['path']),reference['sha256'])
            r.need(len(raw)==reference['bytes'],'budget origin byte count differs')
        proposal = r.read(AUDIT)
        r.need(proposal['execution'] == EXECUTION, 'proposal execution differs')
        for row in proposal['frozen_bindings']:
            r.audit.checked_bytes(r.audit.unlinked_absolute(row['path']), row['sha256'])
        head = subprocess.run(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'],
                              cwd=OLD, check=True, capture_output=True).stdout.decode().strip()
        r.need(head == HEAD and not (PRIVATE / 'remaining-candidate22/authorized-execution01').exists(),
               'C22 changed or received new authority')
        r.need(r.dt.datetime.now(r.dt.timezone.utc) >= r.dt.datetime(2026, 10, 23, 10, 40, tzinfo=r.dt.timezone.utc),
               'never retire dependencies of a still-open C22 window')
        # Run the existing owner gate, including all old disabled permissions,
        # budgets, failure outcomes, private/public indexes and complete mounts.
        # It is evidence of this capture, not the successor's live-state API.
        sys.path.insert(0, str(OLD / 'examples/lifecycle-d1'))
        spec = importlib.util.spec_from_file_location('retirement_original_c22', OLD / 'examples/lifecycle-d1/prepare-six-candidate.py')
        old = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(old)
        # The first unadapted archive failed on raw Dns null/[] drift. Preserve
        # it. This successor has an explicit semantic comparison, restricted to
        # the original 620 independently pinned descriptor identities. Actual
        # live State is never substituted and archived inspect remains raw.
        accepted = r.audit.decode(r.audit.checked_bytes(OLD / OLD_V27_PLAN, OLD_V27_SHA))['preservation']['descriptors']
        newer = r.audit.decode(r.audit.checked_bytes(OLD / OLD_V28_PLAN, OLD_V28_SHA))['preservation']['descriptors']
        r.need(all(accepted[cid] == newer[cid] for cid in set(accepted) & set(newer)),
               'historical authoritative descriptor digests conflict')
        accepted = {**accepted, **newer}
        deltas = {}
        original_run = subprocess.run
        def comparison_read(argv, *positional, **keywords):
            words = list(map(str, argv))
            if words[0] == 'docker':
                r.need(words[1] in ('inspect', 'ps', 'cp', 'volume', 'network'), 'historical comparison tried a mutation')
                if words[1] in ('volume', 'network'):
                    r.need(words[2] == 'inspect', 'historical resource mutation')
            result = original_run(argv, *positional, **keywords)
            if words[:2] != ['docker', 'inspect'] or result.returncode:
                return result
            values = json.loads(result.stdout.decode('utf-8', 'strict'))
            compared = []
            for item in values:
                cid = item['Id']
                if cid in accepted:
                    projected, changed = r.dns_representation_for_history(item, accepted[cid])
                    if changed:
                        deltas[cid] = dict(original_sha256=accepted[cid], actual_sha256=r.historical_cfg(item),
                                           delta='HostConfig.Dns null -> [] only')
                    item = projected
                compared.append(item)
            result.stdout = json.dumps(compared).encode()
            return result
        # Old modules import each other afresh and own multiple process wrappers.
        # Scope the read adapter to this one isolated comparison, then restore.
        subprocess.run = comparison_read
        try:
            legacy = old.historical_gate()
        finally:
            subprocess.run = original_run
        r.save(root / 'legacy-gate.json', dict(status='PASS_MAINTENANCE_PRESERVATION_COMPARISON',
               observed_at=r.now(), comparison=legacy, dns_representation=deltas,
               frozen_C22_entry_pass=False, native_provider_calls=0))
        r.save(root / 'current-cold.json', current_cold_contract())
        ids = docker.inventory()
        rows = docker.inspect(sorted(ids))
        budget_names=sorted({origin['volume'] for origin in origins['origins']})
        budget_metadata=docker.json('volume','inspect',*budget_names)
        budget_members=r.budget_membership(rows,budget_metadata,origins['origins'])
        r.save(root/'budget-origins.json',origins)
        original = {x['id'] for x in proposal['containers']}
        candidates = set(proposal['phase1_candidate_ids_not_approved_for_current_frozen_execution'])
        kept = original - candidates
        external = {x['dependent'] for x in proposal['external_netns_dependents']}
        r.need(len(candidates) == 986 and len(kept) == 99 and len(external) == 139
               and original <= ids and all(r.closed(rows[x]) for x in original | external),
               'fresh scope or closure differs')
        order = r.removal_order(candidates, rows)
        archives = {}
        for count, cid in enumerate(order):
            folder = root / 'owners' / cid
            folder.mkdir()
            r.save(folder / 'inspect.json', rows[cid])
            log = subprocess.run(['docker', 'logs', '--timestamps', cid], capture_output=True, timeout=60)
            r.write(folder / 'stdout.log', log.stdout)
            r.write(folder / 'stderr.log', log.stderr)
            r.save(folder / 'logs-observation.json', dict(cid=cid, observed_at=r.now(), exit_code=log.returncode))
            r.need(log.returncode == 0, 'raw log archive failed')
            archives[cid] = dict(inspect=file_spec(root, folder / 'inspect.json'),
                                 stdout=file_spec(root, folder / 'stdout.log'),
                                 stderr=file_spec(root, folder / 'stderr.log'),
                                 descriptor=r.descriptor(rows[cid]), state=r.state(rows[cid]))
            if count % 100 == 0:
                print(json.dumps(dict(archived=count + 1, total=len(order))), flush=True)
        volumes = {}
        mounts = {}
        for cid, row in rows.items():
            for mount in row['Mounts']:
                if mount['Type'] == 'volume':
                    mounts.setdefault(mount['Name'], set()).add(cid)
        names = {m['Name'] for cid in original for m in rows[cid]['Mounts'] if m['Type'] == 'volume'} | set(budget_members)
        values = docker.json('volume', 'inspect', *sorted(names))
        r.need(len(values) == len(names), 'volume membership differs')
        for volume in values:
            name = volume['Name']
            volumes[name] = dict(metadata={k: volume.get(k) for k in r.VOLUME}, mounted_before=sorted(mounts[name]))
        budgets = []
        for name in sorted(budget_members):
            users = mounts[name]
            wires = [cid for cid in users if any(m['Type'] == 'volume' and m['Name'] == name
                     and m['Destination'] == '/run/budget' for m in rows[cid]['Mounts'])
                     and rows[cid]['Config']['Cmd'] == ['-stage', 'wire']]
            if not wires:
                continue
            r.need(len(wires) == 1, 'budget must have one exact old wire reader')
            path = root / 'budgets' / (r.sha(name.encode()) + '.tar')
            r.write(path, docker.raw('cp', wires[0] + ':/run/budget', '-'))
            budgets.append(dict(volume=name, wire=wires[0], archive=file_spec(root, path),
                                mounted_before=sorted(users), refunded=False, reset=False))
        r.need({entry['volume'] for entry in budgets}==set(budget_members), 'budget archives differ from exact origin union')
        # Originals remain at their original private/public paths. Container rm
        # cannot remove them. Pin bytes; do not duplicate large backup datasets.
        originals = {}
        successor_owners={cid for origin in origins['origins'] for cid in origin['mounted']} - (candidates|kept|external)
        for path in PRIVATE.glob('remaining-candidate*/allocation.json'):
            allocation=r.read(path)
            successor_owners.update(set(allocation['owners'])-(candidates|kept|external))
        private_roots=r.private_original_roots(PRIVATE,rows,original|external|successor_owners,origins['references'].values())
        roots = [*private_roots, OLD / 'docs/acceptance/evidence/issue307-d1', OLD / 'examples/lifecycle-d1/results']
        for base in roots:
            for path in sorted(base.rglob('*')):
                if path.is_relative_to(root):
                    continue
                r.need(not path.is_symlink() and not path.is_junction(), 'linked original evidence')
                if path.is_file():
                    r.need(path.stat().st_nlink == 1, 'hardlinked original evidence')
                    raw = path.read_bytes()
                    originals[str(path.resolve())] = dict(sha256=r.sha(raw), bytes=len(raw))
        for row in proposal['frozen_bindings']:
            path = Path(row['path'])
            raw = path.read_bytes()
            originals[str(path.resolve())] = dict(sha256=r.sha(raw), bytes=len(raw))
        r.save(root / 'originals.json', dict(files=originals))
        protected = lambda scope: {cid: dict(descriptor=r.descriptor(rows[cid]), state=r.state(rows[cid])) for cid in sorted(scope)}
        successor_owners={cid for origin in origins['origins'] for cid in origin['mounted']} - (candidates|kept|external)
        for path in PRIVATE.glob('remaining-candidate*/allocation.json'):
            allocation=r.read(path)
            successor_owners.update(set(allocation['owners'])-(candidates|kept|external))
        r.need(successor_owners<=ids and all(r.closed(rows[cid]) for cid in successor_owners),'successor owner absent or active')
        plan = dict(schema=r.SCHEMA, source_execution=EXECUTION, source_head=HEAD,
                    observed_at=r.now(), authority=authority, retired_historical=sorted(candidates),
                    live_required=sorted(kept), external_held=sorted(external),
                    archives=archives, live=protected(kept), external=protected(external),
                    additional_live=protected(successor_owners),
                    running_protected=protected({cid for cid in ids if rows[cid]['State']['Running']}),
                    order=order, volumes=volumes, budgets=budgets,
                    budget_origins=file_spec(root,root/'budget-origins.json'),
                    legacy_gate=file_spec(root, root / 'legacy-gate.json'),
                    originals=file_spec(root, root / 'originals.json'), provider_delete=0, volume_delete=0)
        r.save(root / 'plan.json', plan)
        digest = r.sha((root / 'plan.json').read_bytes())
        result = r.verify(root / 'plan.json', digest, root, root / 'removals', docker, complete=False)
        r.save(root / 'prepared.json', result)
        print(json.dumps(dict(status=result['status'], plan_sha256=digest, archives=len(archives),
                              budgets=len(budgets), originals=len(originals), native_delete=0)))
    except Exception as error:
        r.save(root / 'capture-failure.json', dict(status='FAILED_NO_REMOVAL', error_type=type(error).__name__,
                                               message=str(error), at=r.now(), provider_delete=0, removed=0))
        raise


if __name__ == '__main__':
    main()
