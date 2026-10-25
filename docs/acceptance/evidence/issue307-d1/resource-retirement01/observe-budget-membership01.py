"""Read-only live resource and budget provenance observation; no starts/removals."""
import datetime as dt
import json
from pathlib import Path
import shutil
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
OUT=Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01/budget-forensics01')
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')

def main():
    OUT.mkdir(exist_ok=False);(OUT/'journal').mkdir()
    docker=r.Docker(OUT/'journal')
    ids=docker.inventory();rows=docker.inspect(sorted(ids))
    r.save(OUT/'all-containers.json',rows)
    mounted={}
    for cid,item in rows.items():
        for mount in item['Mounts']:
            if mount['Type']=='volume':mounted.setdefault(mount['Name'],[]).append(dict(cid=cid,destination=mount['Destination'],rw=mount['RW']))
    budget_names=sorted(name for name,users in mounted.items() if any(u['destination']=='/run/budget' for u in users))
    budgets=docker.json('volume','inspect',*budget_names)
    allocations=[]
    for path in sorted(PRIVATE.glob('remaining-candidate*/allocation.json')):
        value=r.read(path)
        allocations.append(dict(path=str(path),sha256=r.sha(path.read_bytes()),allocation=value))
    r.save(OUT/'allocations.json',allocations)
    entries=[]
    for volume in budgets:
        name=volume['Name'];refs=[]
        for allocation in allocations:
            for group in allocation['allocation'].get('groups',[]):
                if group.get('volumes',{}).get('budget')==name:
                    refs.append(dict(path=allocation['path'],sha256=allocation['sha256'],execution=allocation['allocation']['execution'],nonce=allocation['allocation'].get('allocation_nonce'),group=group['group'],expected=group['budget_expected_mounted_cids'],wire=group['helpers']['wire']))
        users=mounted[name]
        entries.append(dict(volume=volume,users=users,bindings=refs,owners=[dict(cid=u['cid'],name=rows[u['cid']]['Name'],cmd=rows[u['cid']]['Config']['Cmd'],labels=rows[u['cid']]['Config']['Labels']) for u in users]))
    r.save(OUT/'budgets.json',entries)
    audit=r.read('C:/Users/Henry/Documents/Codex/2026-10-23/new-chat/outputs/DPP307-dependency-audit.json')
    candidates=set(audit['phase1_candidate_ids_not_approved_for_current_frozen_execution'])
    exclusive=audit['phase1_exclusive_volume_names_not_approved_for_deletion']
    r.save(OUT/'candidate-volumes.json',dict(volumes=docker.json('volume','inspect',*exclusive),users={name:mounted.get(name,[]) for name in exclusive}))
    r.save(OUT/'before-disk.json',dict(at=r.now(),c_drive=shutil.disk_usage('C:/')._asdict(),containers=len(rows),running=[cid for cid,row in rows.items() if row['State']['Running']],candidate_count=len(candidates),present=len(candidates&ids)))
    observed=[]
    for entry in entries:
        scope=set(u['cid'] for u in entry['users'])
        observed.append(dict(name=entry['volume']['Name'],labels=entry['volume']['Labels'],bindings=len(entry['bindings']),mount_count=len(scope),candidate_mounts=len(scope&candidates),wire_count=sum(o['cmd']==['-stage','wire'] for o in entry['owners'])))
    r.save(OUT/'summary.json',dict(at=r.now(),budget_count=len(entries),budgets=observed))
    print(json.dumps(dict(output=str(OUT),containers=len(rows),budgets=len(entries),summary=observed)),flush=True)
if __name__=='__main__':main()
