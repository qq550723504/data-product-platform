import json,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parent
state=json.loads((root.parents[1]/'.data/independent299-runtime/runtime.json').read_text())
private=Path(state['private'])
facts=json.loads((private/'facts.json').read_text())
token=(private/'token').read_text().strip()
request=urllib.request.Request(state['api']+'/api/v1/authorizations/'+facts['Authorization']+'/revoke',data=b'{}',method='POST',headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
with urllib.request.urlopen(request,timeout=30) as response:
 result=json.load(response)
assert result['status']=='REVOKED'
(root/'authorization-revoked.json').write_text(json.dumps(result,indent=2))
print(json.dumps({'authorizationId':facts['Authorization'],'status':result['status']}))
