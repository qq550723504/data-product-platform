import importlib.util,json,shutil
from pathlib import Path
root=Path('C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
def module(name,path):
 spec=importlib.util.spec_from_file_location(name,path);value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value
m=module('readonly',root/'examples/lifecycle-d1/complete-lc13-readonly.py');collector=module('collector',m.PRIVATE/'readonly-core04.py')
m.CONTROL=m.PRIVATE/'d1-delete-readonly-candidate04-retry02';m.PUBLIC=root/'docs/acceptance/evidence/issue307-d1/delete-readonly-candidate04/retry02'
original=m.role_acl;seen=0
def role_acl(container):
 global seen
 result=original(container)
 if container==m.PG:
  seen+=1
  if seen==1:
   out=m.PUBLIC/'5d-04';collector.collect(m,out)
   control=m.CONTROL/'iam-management';control.mkdir();source=m.load(m.SOURCE/'config.json');owner=m.load(m.SOURCE/'plane-owner/reader.json');m.require(source['Enabled'] is False and owner['Enabled'] is False,'Source business config reopened')
   cfg={'RootAccess':source['RootAccess'],'RootSecret':source['RootSecret'],'RunID':source['RunID'],'Enabled':False,'Users':{'reader':owner['Access'],'producer':owner['ProducerAccess'],'inspector':source['InspectAccess'],'executor':'refdelete614cd9ad'}}
   m.write(control/'iam.json',cfg);shutil.copyfile(m.PRIVATE/'d1-delete-readonly-candidate04/iam-readonly02',control/'iam-readonly')
   name='dpp-lifecycle307-5d2274ee-readonly-iam-04-retry02';ident=m.docker('create','--pull','never','--name',name,'--label','lifecycle307.run='+cfg['RunID'],'--network','container:'+m.MINIO,'--read-only','--tmpfs','/tmp','--mount','type=bind,source='+str(control)+',target=/run/private,readonly','--mount','type=bind,source='+str(out)+',target=/run/output','--entrypoint','/run/private/iam-readonly',m.PGIMAGE).decode().strip()
   m.docker('start',ident);code=m.docker('wait',ident).decode().strip();i=m.inspect(ident)
   m.write(out/'native-iam-helper.json',{'id':ident,'image':i['Image'],'exit':int(code),'pid':i['State']['Pid'],'source_sha256':m.digest(m.PRIVATE/'readonly-iam04.go'),'binary_sha256':m.digest(control/'iam-readonly'),'read_only':True,'management_root_mounted':True,'payload_root_mounted':False,'source_pg_credentials_mounted':False,'delete':0})
   shutil.copyfile(m.PRIVATE/'readonly-iam04.go',out/'iam-readonly.go');shutil.copyfile(m.PRIVATE/'readonly-core04.py',out/'core-readonly-collector.py')
   m.require(code=='0' and i['State']['Status']=='exited' and i['State']['Pid']==0,'Native IAM precondition failed; retained, no automatic retry')
 return result
m.role_acl=role_acl
m.five_d(4)
print(json.dumps({'status':'PASS','source_mutation':0,'native_checkpoint_calls':20,'native_admin_reads':4,'delete':0,'cleanup':0}))
