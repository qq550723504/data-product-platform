"""Explicit per-volume decisions. Budget and unproven recovery data stay live."""
import json
from pathlib import Path
import sys
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
import retired_volumes as v
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
ARCHIVE=PRIVATE/'resource-retirement01/capture06'
OBSERVED=PRIVATE/'resource-retirement01/capture05'
OLD=Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')

def ref(path):
    raw=r.audit.unlinked_absolute(path).read_bytes()
    return dict(path=str(path.resolve()),sha256=r.sha(raw),bytes=len(raw))

def main():
    parent=r.read(ARCHIVE/'plan.json');digest=r.sha((ARCHIVE/'plan.json').read_bytes())
    candidates=r.read(PRIVATE/'resource-retirement01/budget-forensics01/candidate-volumes.json')
    content={}
    for path in (OBSERVED/'volume-observations').glob('*/content.json'):
        value=r.read(path);r.need(value['volume'] not in content,'duplicate content proof')
        content[value['volume']]=(path,value)
    origins={}
    for number in range(13,22):
        folder=PRIVATE/('remaining-candidate%02d'%number)
        allocation=folder/'allocation.json'
        packet=OLD/('docs/acceptance/evidence/issue307-d1/remaining-candidate%02d/candidate.json'%number)
        if not packet.is_file():continue
        actual=r.read(allocation);binding=r.read(packet)
        for group in actual['groups']:
            frozen=next(g for g in binding['Groups'] if g['Group']==group['group'])
            paths={name:folder/group['group']/'checkpoint'/name for name in ('core.dump','globals.sql','native.tar')}
            keys={'core.dump':'CoreDumpSHA256','globals.sql':'GlobalsSHA256','native.tar':'NativeArchiveSHA256'}
            if not all(path.is_file() and r.sha(path.read_bytes())==frozen[keys[name]] for name,path in paths.items()):continue
            entry=dict(group=group['group'],allocation=ref(allocation),packet=ref(packet),backups={name:ref(path) for name,path in paths.items()})
            for role in ('checkpoint-core','rehearsal-core'):origins[group['volumes'][role]]=entry
    approved={};preserved={}
    budgets={b['volume'] for b in parent['budgets']}
    names=[x['Name'] for x in candidates['volumes']]
    for name in names:
        if name in budgets:
            preserved[name]=dict(reason='BUDGET_FACTS_RETAINED_NO_RESET_OR_REFUND');continue
        path,value=content[name]
        entry=dict(metadata=parent['volumes'][name]['metadata'],content=ref(path))
        if all(f['type'] in ('0','5') and f['size']==0 for f in value['files']):
            approved[name]=dict(entry,reason='EMPTY_NO_FILE_DATA')
        elif name in origins:
            approved[name]=dict(entry,reason='REPRODUCIBLE_OLD_PG_RESTORE',**origins[name])
        else:
            queue=any(f['name'].endswith('.rdb') or 'appendonly' in f['name'] for f in value['files'])
            preserved[name]=dict(reason='NONEMPTY_RECOVERY_QUEUE_RETAINED' if queue else 'UNIQUE_NONEMPTY_RECOVERY_COPY_FULL_RECONSTRUCTION_NOT_PROVEN',content=ref(path),regular_bytes=value['regular_bytes'])
    plan=dict(schema=v.SCHEMA,parent_plan_sha256=digest,authority=r.read(ROOT/'docs/acceptance/evidence/issue307-d1/resource-retirement01/maintenance-human-authority02.json'),
        candidate_volumes=sorted(names),remove=approved,preserve=preserved,provider_delete=0,
        backups_preserved=True,budgets_preserved=True,pins_released=0,other_sessions_unchanged=True)
    r.save(ARCHIVE/'volume-plan.json',plan)
    vd=r.sha((ARCHIVE/'volume-plan.json').read_bytes());v.checked(ARCHIVE/'volume-plan.json',vd,parent,digest)
    (ARCHIVE/'volume-removals').mkdir()
    r.save(ARCHIVE/'volume-decisions-summary.json',dict(status='PASS_BOUNDED_PER_VOLUME_PROOFS',plan_sha256=vd,
        approved=len(approved),preserved=len(preserved),reasons={reason:sum(x['reason']==reason for x in list(approved.values())+list(preserved.values())) for reason in {x['reason'] for x in list(approved.values())+list(preserved.values())}}))
    print(json.dumps(r.read(ARCHIVE/'volume-decisions-summary.json')),flush=True)
if __name__=='__main__':main()
