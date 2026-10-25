"""Observe the complete fixed history gate; only host reads, never preparation."""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('c21_history_only',ROOT/'examples/lifecycle-d1/prepare-six-candidate.py')
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
OUT=Path(__file__).with_name('retained-full-readonly01')
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/candidate21-bootstrap01/retained-full-readonly01')


def stamp():return dt.datetime.now(dt.timezone.utc).isoformat()
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as f:f.write(json.dumps(value,indent=2)+'\n')


def main():
    OUT.mkdir();PRIVATE.mkdir();calls=[]
    def call(args,data=None,output=None):
        args=list(map(str,args))
        if args[0]=='docker':
            if not(args[1] in ('inspect','ps','cp') or args[1:3] in (['volume','inspect'],['network','inspect'])):raise ValueError('history attempted non-read Docker verb')
        elif args[0]=='git':
            if not any(x in args for x in ('show','ls-files','rev-parse','cat-file')):raise ValueError('history attempted non-read Git verb')
        else:raise ValueError('history attempted unknown process')
        if output is not None:raise ValueError('history unexpectedly writes process output')
        start=stamp();result=subprocess.run(args,cwd=ROOT,input=data,capture_output=True)
        row={'argv':args,'started_at':start,'observed_at':stamp(),'exit':result.returncode,'stdout_sha256':hashlib.sha256(result.stdout).hexdigest(),'stderr_sha256':hashlib.sha256(result.stderr).hexdigest()}
        save(OUT/(f'{len(calls):04d}-process.json'),row);calls.append(row)
        if result.returncode:
            (PRIVATE/'failure.stdout.private').write_bytes(result.stdout);(PRIVATE/'failure.stderr.private').write_bytes(result.stderr)
            raise ValueError('history read failed; raw channels retained privately')
        return result.stdout
    p.call=call;p.p1.call=call;p.native.call=call
    save(OUT/'intent.json',{'scope':'HOST_ONLY_HISTORY_READ_NOT_CURRENT_CANDIDATE_RUNTIME','planned_execution':'9ba214bd-a481-499b-bef6-ca7a789ef501','window':'PENDING_HUMAN_SELECTION','compiled_existing_profile':p.PROFILE,'source_provider_actions':0,'allocated_resources':0})
    try:
        result=p.historical_gate();save(OUT/'retained.json',result)
        if result['status']!='PASS_RETAINED_907_CLOSED' or len(result['closed_cids'])!=907:raise ValueError('full retained set differs')
        terminal={'status':'PASS_C21_COMPLETE_HISTORY_ONLY_READ','at':stamp(),'closed_cids':907,'retained_c20':result['c20'],'host_read_processes':len(calls),'source_provider_actions':0,'new_resource_allocation':0,'native_delete':0,'window':'PENDING_HUMAN_SELECTION'}
    except BaseException as error:
        save(OUT/'terminal.json',{'status':'FAILED_HISTORY_READ_RETAINED','at':stamp(),'error_type':type(error).__name__,'error':str(error),'host_read_processes':len(calls),'source_provider_actions':0,'new_resource_allocation':0,'native_delete':0})
        raise
    save(OUT/'terminal.json',terminal);print(json.dumps(terminal))

if __name__=='__main__':main()
