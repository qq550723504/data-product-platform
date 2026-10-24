"""Select frozen once-only send and selected read-only recovery owners in order."""
import importlib.util
import json
from pathlib import Path
import sys
import time

spec=importlib.util.spec_from_file_location('c18_window_composition',Path(__file__).with_name('compose.py'))
x=importlib.util.module_from_spec(spec);spec.loader.exec_module(x)

def slot(group,stage,prefix,before_recovery=None):
    x.window();g=x.group(group);out=x.PRIVATE/group/'outputs'
    returned=out/stage/(stage+'-returned.json')
    x.need(not returned.exists(),'Original send return already exists; never resend')
    x.root(g,'enable-send',prefix+'-enable-send')
    x.permission(g,'wire',True);x.permission(g,stage,True)
    x.attest(g,'wire',prefix+'-wire-proof')
    x.start(g['Helpers']['wire']);time.sleep(0.5)
    wire=x.h.inspect(g['Helpers']['wire']);x.need(wire['State']['Running'] and wire['State']['Pid']>0,'Wire did not remain running')
    x.attest(g,'wire',prefix+'-wire-fresh-proof')
    x.attest(g,stage,prefix+'-sender-proof')
    x.owner(g['Helpers'][stage],prefix+'-sender',100)
    result=x.load(returned)
    x.need(result['execution']==x.C['Execution'] and result['stage']==stage and result['provider_invocation']=='CONFIRMED'
           and result['budget_consumed'] is True and result['claim_confirmed'] is True and result['provider_observation_present'] is True
           and result['certainty']=='CONFIRMED' and result['outcome']==('UNKNOWN' if stage in ('H2','M1B') else 'ACCEPTED')
           and not result['automatic_resend'] and not result['automatic_recovery'],'Frozen sender did not return its expected durable outcome')
    x.save(x.OUT/(prefix+'-actual-send-return.json'),result)
    x.permission(g,stage,False);x.permission(g,'wire',False)
    wire_close=x.h.stop(g['Helpers']['wire']);x.save(x.OUT/(prefix+'-wire-stop.json'),wire_close)
    x.root(g,'disable-send',prefix+'-disable-send')
    if before_recovery is not None:before_recovery(g,result)
    recover='recover-'+stage
    x.permission(g,recover,True)
    x.attest(g,recover,prefix+'-recovery-proof')
    x.owner(g['Helpers'][recover],prefix+'-recovery',110)
    x.permission(g,recover,False)
    recovery=x.load(out/recover/(recover+'.json'));queued=x.load(out/recover/(recover+'-queue.json'))
    x.need(recovery['read_only'] is True and recovery['resend']==0 and queued['actual_asynq_worker'] is True and queued['selected_only'] is True,'Selected Outbox/Asynq recovery not confirmed')
    import remaining_sequence
    seq=remaining_sequence.observe(x.C,x.PRIVATE,group,recover,x.h.inspect,x.w.r.p.native.sql,x.load)
    position=next(i for i,(name,s,_) in enumerate(remaining_sequence.ORDER) if (name,s)==(group,stage))
    x.need(seq['completed']==position+1,'Selected immutable Core absence has not committed')
    x.save(x.OUT/(prefix+'-completion.json'),{'status':'EXACT_TARGET_SENT_ONCE_RECOVERED_WITH_IMMUTABLE_ABSENCE','at':x.now().isoformat(),
           'execution':x.C['Execution'],'stage':stage,'send_return':result,'recovery':recovery,'queue':queued,'sequence':seq,'resend':0})
    return result

if __name__=='__main__':
    try:
        x.need(len(sys.argv)==2 and sys.argv[1] in ('P2','T1','T2'),'Only the chosen business slot is exposed')
        stage=sys.argv[1];x.need(x.load(x.OUT/'source-prepared.json')['status']=='SOURCE_BUSINESS_NEW_ADMISSION_READY','Source business preparation missing')
        slot('business-races',stage,'slot-'+stage)
        print(json.dumps({'status':'EXACT_BUSINESS_TARGET_SENT_ONCE_AND_ABSENCE_COMMITTED','stage':stage,'execution':x.C['Execution']}))
    except BaseException as error:
        try:sealed=x.containment(error);print(json.dumps({'status':'FAILED_SLOT_SEALED_STOP_NEW_SEND','reason':str(error),'all_steps_confirmed':sealed['all_steps_confirmed']}))
        except BaseException as close_error:print(json.dumps({'status':'FAILED_SLOT_CONTAINMENT_UNCONFIRMED','reason':str(error),'containment_error':str(close_error)}))
        raise
