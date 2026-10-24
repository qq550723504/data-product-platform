import hashlib,importlib.util,json,subprocess,sys
from pathlib import Path
from unittest.mock import patch
import _winapi
ROOT=Path(__file__).resolve().parents[6]
OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('terminal_alias_counterexample',ROOT/'examples/lifecycle-d1/verify-six-native.py')
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
fixture=OUT/'terminal-parent-junction-counterexample01'
fixture.mkdir(exist_ok=False)
control=fixture/'control';control.mkdir()
original=fixture/'original-authorization';original.mkdir()
(original/'terminal.json').write_bytes(b'{"status":"FAILED"}\n')
empty=fixture/'empty-target';empty.mkdir()
alias=control/'authorized-execution01'
_winapi.CreateJunction(str(empty.resolve()),str(alias.absolute()))
external=[]
def forbidden(*a,**kw):
    external.append(repr(a));raise AssertionError('real external process forbidden')
with patch.object(v.r,'CONTROL',control),patch.object(subprocess,'Popen',forbidden):
    error=None
    try:v.require_unfinished_execution()
    except Exception as exc:error=type(exc).__name__+': '+str(exc)
record={'case':'actual Windows junction in authorization parent','original_terminal_sha256':hashlib.sha256((original/'terminal.json').read_bytes()).hexdigest(),'parent_is_junction':alias.is_junction(),'terminal_exists_through_alias':(alias/'terminal.json').exists(),'gate_error':error,'gate_returned_successfully':error is None,'expected':'reject aliased authorization parent before terminal absence','source_provider_io':0,'native_delete':0,'external_process_attempts':external}
with (OUT/'terminal-parent-junction-counterexample01.json').open('xb') as f:f.write((json.dumps(record,indent=2)+'\n').encode())
print(json.dumps(record,indent=2))
