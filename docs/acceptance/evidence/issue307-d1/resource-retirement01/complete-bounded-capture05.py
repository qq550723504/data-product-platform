"""Finish existing read-only archives with bounded D1 originals and fresh state."""
import json
from pathlib import Path
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
SOURCE=PRIVATE/'resource-retirement01/capture05'
ARCHIVE=PRIVATE/'resource-retirement01/capture06'
OLD=Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')
AUDIT=Path('C:/Users/Henry/Documents/Codex/2026-10-23/new-chat/outputs/DPP307-dependency-audit.json')

def spec(path):return dict(path=path.relative_to(ARCHIVE).as_posix(),sha256=r.sha(path.read_bytes()),bytes=path.stat().st_size)

def main():
    r.need(not (ARCHIVE/'plan.json').exists(),'do not replace an existing capture plan')
    ARCHIVE.mkdir(exist_ok=True)
    r.need(ARCHIVE.resolve()==PRIVATE.resolve()/'resource-retirement01/capture06','exact new bounded archive path required')
    for component in ('owners','budgets'):
        for path in (SOURCE/component).rglob('*'):
            r.need(not path.is_symlink() and not path.is_junction(),'linked original archive')
            destination=ARCHIVE/path.relative_to(SOURCE)
            if path.is_dir():destination.mkdir(parents=True,exist_ok=True)
            else:
                destination.parent.mkdir(parents=True,exist_ok=True)
                if destination.exists():r.need(destination.read_bytes()==path.read_bytes(),'existing archive copy differs')
                else:r.write(destination,path.read_bytes())
    for name in ('legacy-gate.json','budget-origins.json','current-cold.json'):
        destination=ARCHIVE/name
        if destination.exists():r.need(destination.read_bytes()==(SOURCE/name).read_bytes(),'existing archive provenance differs')
        else:r.write(destination,(SOURCE/name).read_bytes())
    (ARCHIVE/'removals').mkdir(exist_ok=True)
    if not (ARCHIVE/'capture-phase-provenance.json').exists():
        r.save(ARCHIVE/'capture-phase-provenance.json',dict(status='REUSED_COMPLETE_OWNER_BUDGET_ARCHIVES_FRESHLY_REVALIDATED',at=r.now(),reason='Only D1 original scope accepted; prior broader plan retained without replacement. Prior readonly verifier was stopped before prepared receipt.',source=str(SOURCE),removed=0,provider_delete=0))
    journal=ARCHIVE/'bounded-completion-journal02';journal.mkdir();docker=r.Docker(journal)
    proposal=r.read(AUDIT)
    for row in proposal['frozen_bindings']:r.audit.checked_bytes(r.audit.unlinked_absolute(row['path']),row['sha256'])
    head=subprocess.run(['git','-c','core.longpaths=true','rev-parse','HEAD'],cwd=OLD,capture_output=True,check=True).stdout.decode().strip()
    r.need(head=='8de3bc4c9f737c3c3e81fddb60491422ba6039f2','old frozen C22 head changed')
    legacy=r.read(ARCHIVE/'legacy-gate.json')
    r.need(legacy['status']=='PASS_MAINTENANCE_PRESERVATION_COMPARISON' and legacy['frozen_C22_entry_pass'] is False,'original successful historical comparison missing')
    candidates=set(proposal['phase1_candidate_ids_not_approved_for_current_frozen_execution'])
    original={entry['id'] for entry in proposal['containers']};kept=original-candidates
    external={entry['dependent'] for entry in proposal['external_netns_dependents']}
    ids=docker.inventory();rows=docker.inspect(sorted(ids))
    r.need(original|external<=ids and all(r.closed(rows[cid]) for cid in original|external),'original scope no longer closed')
    archives={}
    for cid in sorted(candidates):
        folder=ARCHIVE/'owners'/cid
        archived=r.read(folder/'inspect.json');logs=r.read(folder/'logs-observation.json')
        r.need(r.descriptor(archived)==r.descriptor(rows[cid]) and r.state(archived)==r.state(rows[cid]) and logs['cid']==cid and logs['exit_code']==0,'archived owner/log provenance differs from fresh closure')
        archives[cid]=dict(inspect=spec(folder/'inspect.json'),stdout=spec(folder/'stdout.log'),stderr=spec(folder/'stderr.log'),descriptor=r.descriptor(rows[cid]),state=r.state(rows[cid]))
    origin_path=PRIVATE/'resource-retirement01/budget-forensics01/budget-origins.json'
    origins=r.audit.decode(r.audit.checked_bytes(origin_path,'44a69b688b2d6a08ccc6ab1b6da437303a67847566b3257392d05b31f0744a70'))
    for entry in origins['references'].values():r.audit.checked_bytes(r.audit.unlinked_absolute(entry['path']),entry['sha256'])
    names=sorted({entry['volume'] for entry in origins['origins']})
    members=r.budget_membership(rows,docker.json('volume','inspect',*names),origins['origins'])
    budgets=[]
    for name,origin in members.items():
        path=ARCHIVE/'budgets'/(r.sha(name.encode())+'.tar')
        fresh=docker.raw('cp',origin['wire']+':/run/budget','-')
        r.need(r.sha(fresh)==r.sha(path.read_bytes()),'budget bytes changed after original archive')
        budgets.append(dict(volume=name,wire=origin['wire'],archive=spec(path),mounted_before=origin['mounted'],refunded=False,reset=False))
    mounts={}
    for cid,item in rows.items():
        for mount in item['Mounts']:
            if mount['Type']=='volume':mounts.setdefault(mount['Name'],set()).add(cid)
    volume_names={m['Name'] for cid in original for m in rows[cid]['Mounts'] if m['Type']=='volume'}|set(names)
    volumes={entry['Name']:dict(metadata={k:entry.get(k) for k in r.VOLUME},mounted_before=sorted(mounts[entry['Name']])) for entry in docker.json('volume','inspect',*sorted(volume_names))}
    additional={cid for origin in origins['origins'] for cid in origin['mounted']}-(candidates|kept|external)
    for path in PRIVATE.glob('remaining-candidate*/allocation.json'):additional.update(set(r.read(path)['owners'])-(candidates|kept|external))
    r.need(additional<=ids and all(r.closed(rows[cid]) for cid in additional),'additional owner missing or active')
    # Only D1-owned private roots, selected from actual bind graph and explicit
    # origin references. Unrelated sessions are not immutable dependencies.
    roots=r.private_original_roots(PRIVATE,rows,original|external|additional,origins['references'].values())
    originals={}
    for base in sorted(roots|{OLD/'docs/acceptance/evidence/issue307-d1',OLD/'examples/lifecycle-d1/results'}):
        r.need(base.exists(),'bound D1 original root missing')
        for path in base.rglob('*'):
            r.need(not path.is_symlink() and not path.is_junction(),'linked D1 original evidence')
            if path.is_file():
                r.need(path.stat().st_nlink==1,'hardlinked original evidence')
                raw=path.read_bytes();originals[str(path.resolve())]=dict(sha256=r.sha(raw),bytes=len(raw))
    for entry in origins['references'].values():originals[entry['path']]=dict(sha256=entry['sha256'],bytes=entry['bytes'])
    for entry in proposal['frozen_bindings']:
        path=Path(entry['path']);originals[str(path.resolve())]=dict(sha256=entry['sha256'],bytes=path.stat().st_size)
    r.save(ARCHIVE/'originals.json',dict(files=originals,bounded_roots=sorted(str(path) for path in roots),other_sessions_excluded=True))
    protected=lambda scope:{cid:dict(descriptor=r.descriptor(rows[cid]),state=r.state(rows[cid])) for cid in sorted(scope)}
    plan=dict(schema=r.SCHEMA,source_execution=proposal['execution'],source_head=head,observed_at=r.now(),
        authority=r.read(ROOT/'docs/acceptance/evidence/issue307-d1/resource-retirement01/maintenance-human-authority02.json'),
        retired_historical=sorted(candidates),live_required=sorted(kept),external_held=sorted(external),archives=archives,
        live=protected(kept),external=protected(external),additional_live=protected(additional),running_protected=protected({cid for cid,item in rows.items() if item['State']['Running']}),
        order=r.removal_order(candidates,rows),volumes=volumes,budgets=budgets,budget_origins=spec(ARCHIVE/'budget-origins.json'),legacy_gate=spec(ARCHIVE/'legacy-gate.json'),originals=spec(ARCHIVE/'originals.json'),provider_delete=0,volume_delete=0)
    r.save(ARCHIVE/'plan.json',plan);digest=r.sha((ARCHIVE/'plan.json').read_bytes())
    result=r.verify(ARCHIVE/'plan.json',digest,ARCHIVE,ARCHIVE/'removals',docker,complete=False)
    r.save(ARCHIVE/'prepared.json',result)
    print(json.dumps(dict(status=result['status'],plan_sha256=digest,archives=len(archives),budgets=len(budgets),originals=len(originals),additional_live=len(additional))),flush=True)
if __name__=='__main__':main()
