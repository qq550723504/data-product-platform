"""Bind the observed union to original packets/allocations, not name heuristics."""
import json
from pathlib import Path
import sys
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
P=Path('C:/Users/Henry/.codex/fixture-private')
OLD=Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')
BASE=OLD/'docs/acceptance/evidence/issue307-d1'
HERE=P/'resource-retirement01/budget-forensics01'

def ref(path):
    raw=r.audit.unlinked_absolute(path).read_bytes()
    return dict(path=str(path.resolve()),sha256=r.sha(raw),bytes=len(raw))

def main():
    rows=r.read(HERE/'all-containers.json');observed=json.loads(r.audit.unlinked_absolute(HERE/'budgets.json').read_bytes().decode('utf-8','strict'))
    r.need(type(observed) is list,'explicit budget observation list required')
    by_name={entry['volume']['Name']:entry for entry in observed};origins=[]
    for number in range(7,24):
        generation='candidate%02d'%number
        folder=P/('remaining-'+generation)
        if number<13:
            packet_path=BASE/('remaining-'+generation)/'candidate.json'
            binding_path=packet_path.with_name('bindings.json')
            packet=r.read(packet_path);bindings=json.loads(r.audit.unlinked_absolute(binding_path).read_bytes().decode('utf-8','strict'))
            r.need(type(bindings) is list,'explicit original group binding list required')
            for group in packet['Groups']:
                binding=next(b for b in bindings if b['group']==group['Group'])
                origins.append(dict(volume=binding['budget_volume'],execution=packet['Execution'],nonce=None,
                    wire=group['Helpers']['wire'],mounted=sorted(group['Helpers'].values()),
                    kind='original-frozen-packet',references=[ref(packet_path),ref(binding_path)]))
        else:
            allocation_path=folder/'allocation.json';allocation=r.read(allocation_path)
            for group in allocation['groups']:
                origins.append(dict(volume=group['volumes']['budget'],execution=allocation['execution'],
                    nonce=allocation['allocation_nonce'],wire=group['helpers']['wire'],
                    mounted=group['budget_expected_mounted_cids'],kind='original-allocation',references=[ref(allocation_path)]))
    for number in (2,3,4):
        folder=P/('host-path-repair%02d'%number)
        terminal_path=folder/'terminal.json';allocation_path=folder/'allocations.json'
        terminal=r.read(terminal_path);allocation=r.read(allocation_path)
        closure={row['cid'] for row in terminal['closure']}
        r.need(closure==set(allocation['owners']), 'rehearsal original closure/allocation differ')
        for name in terminal['retained_volumes']:
            if name not in by_name:continue
            entry=by_name[name];users={u['cid'] for u in entry['users']}
            r.need(users<=closure and name in allocation['volumes'], 'rehearsal budget is outside original allocation')
            wires=[cid for cid in users if rows[cid]['Config']['Cmd']==['-stage','wire']]
            r.need(len(wires)==1,'rehearsal exact wire differs')
            origins.append(dict(volume=name,execution=terminal['execution'],nonce=None,wire=wires[0],mounted=sorted(users),
                kind='failed-isolated-rehearsal-preserved',references=[ref(terminal_path),ref(allocation_path)]))
    result=r.budget_membership(rows,[entry['volume'] for entry in observed],origins)
    originals={}
    for origin in origins:
        for member in origin['references']:
            old=originals.setdefault(member['path'],member)
            r.need(old==member,'conflicting original reference')
    value=dict(status='FROZEN_EXPLICIT_BUDGET_ORIGIN_UNION',at=r.now(),origins=origins,
        count=len(result),references=originals,summary={kind:sum(o['kind']==kind for o in origins) for kind in {o['kind'] for o in origins}},
        unused_or_used_budgets_preserved=True,budgets_refunded=0,budgets_reset=0)
    r.save(HERE/'budget-origins.json',value)
    print(json.dumps(dict(count=len(result),summary=value['summary'],sha256=r.sha((HERE/'budget-origins.json').read_bytes()))))
if __name__=='__main__':main()
