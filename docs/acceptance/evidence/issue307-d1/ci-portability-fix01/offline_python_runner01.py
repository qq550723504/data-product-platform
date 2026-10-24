"""Test-only Python open audit: retained worktrees/archives are not fixtures.

This is an assertion/receipt, not a replacement OS sandbox or runtime gate.
Linux execution additionally has no retained host paths mounted or network.
"""
import argparse,json,os,runpy,sys
from pathlib import Path
parser=argparse.ArgumentParser();parser.add_argument('--root',type=Path,required=True);parser.add_argument('--test',required=True);parser.add_argument('--audit',type=Path,required=True);args=parser.parse_args()
root=args.root.resolve();allowed=str(root).replace('\\','/').casefold().rstrip('/')+'/'
reads=set();denied=[]

def audit(event,values):
 if event!='open' or not values or isinstance(values[0],int):return
 try:original=os.path.abspath(os.fsdecode(values[0])).replace('\\','/');path=original.casefold()
 except (TypeError,ValueError):return
 mode=values[1] if len(values)>1 else None
 if path==str(args.audit.resolve()).replace('\\','/').casefold() and mode=='x':return
 if any(path.startswith(prefix) for prefix in ('c:/users/henry/.codex/worktrees/','c:/users/henry/.codex/fixture-private/')) and not path.startswith(allowed):
  denied.append(path);raise PermissionError('retained host path forbidden by offline test assertion')
 if path.startswith(allowed) and isinstance(mode,str) and 'r' in mode:reads.add(original[len(allowed):])

sys.addaudithook(audit)
sys.argv=[str((root/'examples/lifecycle-d1'/args.test).resolve())];sys.path.insert(0,str(Path(sys.argv[0]).parent))
code=0
try:runpy.run_path(sys.argv[0],run_name='__main__')
except SystemExit as error:code=error.code or 0
except BaseException:
 code=1;raise
finally:
 with args.audit.open('x',encoding='utf-8',newline='\n') as stream:json.dump(dict(test=args.test,root=str(root),reads=sorted(reads),forbidden_host_reads=denied,code=code),stream,indent=2)
raise SystemExit(code)
