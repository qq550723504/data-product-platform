"""Observe the unmodified production gate against real Windows path aliases."""
import ctypes,inspect,json,sys,tempfile
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import test_retention_composition as suite
c=suite.c;results=[];real_need=c.need
def observed_need(condition,reason):
 if not condition and reason=='COMPOSITION_ANCHOR_READER_PATH_BINDING':
  frame=inspect.currentframe().f_back;entry=frame.f_locals['self'];reader=frame.f_locals['reader'];destination=frame.f_locals['destination']
  row=dict(packet_control=entry.packet['control'],canonical_control=str(entry.control),native_equal=reader['native']==c.known_sources()[reader['group']]['Native'],reader_phase=reader['phase'],reader_group=reader['group'],paths={})
  for field,relative in [('intent','reader-create-intent.json'),('registration','reader-created.json'),('closure','public/closure.json'),('profile','private/retention-readonly.json')]:
   row['paths'][field]=dict(actual=reader[field],expected=str(destination/relative),equal=reader[field]==str(destination/relative))
  results[-1]['gate_observation']=row
 real_need(condition,reason)
for mode in ('canonical','upper-case','short-name'):
 with tempfile.TemporaryDirectory(prefix='dpp307 reader alias ') as d:
  root=Path(d);alias=root
  if mode=='upper-case':alias=Path(str(root).upper())
  elif mode=='short-name':
   query=ctypes.windll.kernel32.GetShortPathNameW;query.argtypes=[ctypes.c_wchar_p,ctypes.c_wchar_p,ctypes.c_uint];query.restype=ctypes.c_uint
   buffer=ctypes.create_unicode_buffer(32768);need=query(str(root),buffer,len(buffer));real_need(0<need<len(buffer),'real short path unavailable');alias=Path(buffer.value)
  real_need(alias.samefile(root),'alias is not the same directory')
  if mode!='canonical':real_need(str(alias)!=str(alias.resolve(strict=True)),'alias did not change spelling')
  record=dict(mode=mode,original=str(alias),canonical=str(alias.resolve(strict=True)),samefile=True);results.append(record)
  test=suite.CompositionTests();test.setUp()
  try:
   with patch.object(c,'need',observed_need):
    try:entry,ops,*_=test.fixture(alias);record['status']='PASS_CONSTRUCTOR';record['ops_calls']=ops.calls
    except ValueError as error:record['status']='REJECTED';record['error']=str(error)
  finally:test.doCleanups()
assert results[0]['status']=='PASS_CONSTRUCTOR'
assert all(x['status']=='REJECTED' and 'COMPOSITION_ANCHOR_READER_PATH_BINDING' in x['error'] and x['gate_observation']['native_equal'] for x in results[1:])
value=dict(status='REPRODUCED_RAW_VERSUS_CANONICAL_READER_BINDING_ON_PY313_WINDOWS',python=sys.version,results=results,production_gate_changed=False,retained_source_start=0,provider_io=0)
with (OUT/'windows313-real-alias-reproduction01.json').open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,indent=2)
print(json.dumps(value))
