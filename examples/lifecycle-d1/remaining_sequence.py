"""Finite six-slot host gate; Core receipts, never a host progress projection.

No send/start/enable or signing API lives here. The existing topology attester
calls this read-only gate before issuing a proof to an already authorized owner.
"""
import json
import io
import tarfile
from pathlib import PurePosixPath

ORDER = (('business-races', 'P2', 'fixture/p2.csv'),
         ('business-races', 'T1', 'fixture/t1.csv'),
         ('business-races', 'T2', 'fixture/t2.csv'),
         ('late-hold', 'H2', 'fixture/h2-a.csv'),
         ('mixed', 'M1A', 'fixture/m1-a.csv'),
         ('mixed', 'M1B', 'fixture/m1-b.csv'))


def need(ok, message):
    if not ok:
        raise ValueError(message)


def targets(candidate):
    groups = {g['Group']: g for g in candidate['Groups']}
    need(len(groups) == len(candidate['Groups']) == 3, 'six sequence group set differs')
    result = []
    for group, stage, key in ORDER:
        copies = [c for c in groups[group]['Copies'] if c['Key'] == key]
        need(len(copies) == 1 and stage in groups[group]['Helpers'], 'six sequence target binding differs')
        result.append((group, stage, copies[0]))
    need(len({c['ID'] for _, _, c in result}) == 6, 'six sequence target duplicates')
    return result


def budget_archive(raw,candidate,group,*,initial):
    """Inspect the actual mounted volume without starting a worker or extracting."""
    need(isinstance(raw,bytes),'budget readback must be bytes')
    copies={cp['ID']:cp for name,_,cp in targets(candidate) if name==group}
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:*') as archive:
        members=archive.getmembers();seen=set();files=[];root=False
        need(1<=len(members)<=1+2*len(copies),'budget member count differs')
        for member in members:
            path=PurePosixPath(member.name)
            need(not path.is_absolute() and '..' not in path.parts and member.name not in seen,'budget path escaped or duplicated')
            seen.add(member.name)
            need(member.uid==member.gid==10001,'budget filesystem owner differs')
            if member.isdir():
                need(len(path.parts)==1 and not root and bool(member.mode&0o200),'budget root is not writable by its worker')
                root=True;continue
            need(not initial and member.isfile() and len(path.parts)==2,'budget unexpected file/directory/link')
            filename=path.name
            name=filename.removeprefix('wire-').removesuffix('.json')
            need(filename in (name+'.json','wire-'+name+'.json') and name in copies,'budget escaped six target scope')
            value=json.load(archive.extractfile(member))
            need(value['execution']==candidate['Execution'] and value['copy']==copies[name], 'budget execution/exact copy differs')
            need(value['attempt']['CopyID']==name and value['attempt']['WorkspaceID']==copies[name]['WorkspaceID'],'budget attempt scope differs')
            files.append(filename)
        need(root,'budget root metadata missing')
    return {'status':'PASS_EXACT_SIX_BUDGET','group':group,'initial_empty':initial,
            'uid':10001,'gid':10001,'files':sorted(files),'native_delete':0}


def evaluate(candidate, group, stage, receipts, owners):
    """All inputs are fresh actual observations made by the trusted host.

    receipts: exact set of immutable absence copy IDs by workspace, queried in
    read-only transactions. owners: CID -> actual Docker inspect plus current
    permission; no caller-supplied closed boolean is accepted.
    """
    ordered = targets(candidate)
    groups = {g['Group']: g for g in candidate['Groups']}
    historical = [c for c in groups['business-races']['Copies'] if c['Key'] == 'fixture/p1.csv']
    need(len(historical) == 1 and historical[0]['ID'] in receipts.get(historical[0]['WorkspaceID'], set()),
         'historical P1 committed absence missing')
    absent = [c['ID'] in receipts.get(c['WorkspaceID'], set()) for _, _, c in ordered]
    completed = 0
    while completed < len(absent) and absent[completed]:
        completed += 1
    need(not any(absent[completed:]), 'out-of-order absence facts; no next slot')
    recovery = stage.startswith('recover-')
    selected = stage.removeprefix('recover-')
    if stage == 'wire':
        need(completed < 6 and ordered[completed][0] == group, 'wire is not the next global group')
        slot = completed
    else:
        slots = [n for n, (g, s, _) in enumerate(ordered) if (g, s) == (group, selected)]
        need(len(slots) == 1, 'not one of six current targets')
        slot = slots[0]
        need(completed in ({slot, slot + 1} if recovery else {slot}), 'next global slot not ready')
    for g in candidate['Groups']:
        for helper_stage, cid in g['Helpers'].items():
            if helper_stage.startswith('recover-'):
                continue
            need(helper_stage == 'wire' or any((name, helper_stage) == (g['Group'], s) for name, s, _ in ordered),
                 'historical/unknown sender in current helper set')
            actual, permission = owners[cid]
            need(actual['Id'] == cid, 'sender CID substituted')
            # Only the selected group's current sender/wire may be active while
            # refreshing their own <=1 minute proof. Every lookup requires all
            # sending owners physically stopped and their permissions false.
            current = (not recovery and g['Group'] == group and helper_stage in ('wire', ordered[slot][1]))
            if not current:
                need(actual['State']['Running'] is False and actual['State']['Pid'] == 0,
                     'other sender/wire still active')
                need(permission['Enabled'] is False, 'other sender/wire permission not revoked')
    return {'status': 'PASS_SIX_GLOBAL_SEQUENCE', 'slot': slot, 'group': group,
            'stage': stage, 'completed': completed,
            'prior_absence_copy_ids': [c['ID'] for _, _, c in ordered[:slot]],
            'all_senders_closed_for_recovery': recovery, 'native_delete': 0}


def observe(candidate, private, group, stage, inspect, sql, load):
    receipts, owners = {}, {}
    for g in candidate['Groups']:
        workspace = g['Copies'][0]['WorkspaceID']
        # UUIDs originate in the frozen candidate; never accept arbitrary SQL
        # values from the command line. Per-group whole receipt sets are exact.
        import uuid
        need(str(uuid.UUID(workspace)) == workspace, 'workspace is not canonical UUID')
        raw = sql(g['Core'], "SELECT coalesce(json_agg(copy_id::text),'[]') FROM lifecycle_copy_absence_receipt WHERE workspace_id='" + workspace + "'::uuid;")
        values = json.loads(raw)
        need(isinstance(values, list) and all(str(uuid.UUID(v)) == v for v in values), 'Core absence result invalid')
        receipts[workspace] = set(values)
        for name, cid in g['Helpers'].items():
            if not name.startswith('recover-'):
                owners[cid] = (inspect(cid), load(private/g['Group']/name/'permission.json'))
    return evaluate(candidate, group, stage, receipts, owners)
