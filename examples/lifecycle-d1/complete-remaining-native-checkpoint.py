"""Finite continuation of 808a6960's already restored native volume.

No provision, PUT, restore replay or sender. Preserve the failed BusyBox tar -d
receipt. Compare existing original/restored files read-only, then perform fresh
GET/HEAD-only verification using the committed preparation helper.
"""
import importlib.util
from pathlib import Path

spec=importlib.util.spec_from_file_location('remaining_tools',Path(__file__).with_name('prepare-remaining-native.py'))
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

def main():
    out=m.PUBLIC/'business-races'
    binding=m.load(out/'native-continuation-binding.json')
    m.require(binding['run_id']=='808a6960-6c36-4200-ae98-f18a68472fe5','Foreign continuation refused')
    state=m.load(m.PRIVATE/'remaining-native01/business-races.json')
    private=Path(state['private']);r=state['resources'];stage=private/'checkpoint'
    m.require(r==binding['resources'] and not (out/'checkpoint.json').exists(),'One-shot continuation scope/replay refused')
    for name,digest in binding['private_file_sha256'].items():
        m.require(m.sha(private/name)==digest,'Frozen private file drift')
    for name,digest in binding['public_file_sha256'].items():
        m.require(m.sha(out/name)==digest,'Prior proof drift')
    for item in binding['containers']:
        current=m.inspect(item['id'])
        m.require(current['Id']==item['id'] and current['Image']==item['image'] and not current['State']['Running'] and current['State']['Pid']==0 and not any(current['NetworkSettings']['Ports'].values()),'Continuation topology drift')
    owned=[x['id'] for x in binding['containers']]
    pg,gw=(r['containers'][x] for x in ('postgres','gateway'))
    try:
        # Only Core/gateway startup, no original provider or failed helper replay.
        m.docker('start',pg);m.wait_pg(pg,'fixture');m.docker('start',gw)
        baseline=m.load(out/'source-before.json')
        m.require(m.fingerprints(pg)==baseline,'Source Core differs from retained checkpoint')
        core=m.inspect(r['project']+'-checkpoint')['Id']
        m.complete_native('business-races',private,r,out,stage,m.load(out/'manifest.json'),owned,baseline,core,r['project']+'-checkpoint-data')
        m.write(out/'preparation.json',{'group':'business-races','run_id':r['run_id'],'status':'PASS_PREPARATION_ONLY','checkpoint':'checkpoint.json','current_delete_budget':0,'original_failed_receipt':'native-restore-attempt01-failure.json','continuation':'explicit fixed read-only compare/verify; no provisioning/restore replay'})
    finally:
        owned.extend(m.docker('ps','-aq','--filter','label=lifecycle307.run='+r['run_id']).decode().split())
        seals=[]
        for ident in dict.fromkeys(owned):
            current=m.inspect(ident)
            if current['State']['Running']:m.docker('stop','--time','20',current['Id'])
            current=m.inspect(current['Id'])
            m.require(not current['State']['Running'] and current['State']['Pid']==0 and not any(current['NetworkSettings']['Ports'].values()),'Continuation failed to seal owned service')
            seals.append({'name':current['Name'],'id':current['Id'],'image':current['Image'],'status':current['State']['Status'],'pid':current['State']['Pid'],'ports':current['NetworkSettings']['Ports'],'mounts':current['Mounts']})
        m.write(out/'seal-continuation01.json',{'containers':seals,'delete':0,'cleanup':0,'retained':True})

if __name__=='__main__':main()
