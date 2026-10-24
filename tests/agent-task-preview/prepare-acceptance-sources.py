"""Explicit synthetic slice-0 fixtures; no commands, runtime or original DB writes."""
import importlib.util,json,hashlib,subprocess
from pathlib import Path
spec=importlib.util.spec_from_file_location('s0_prepare',Path(__file__).with_name('prepare-readonly.py'))
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
def source_sql(query):
 result=subprocess.run(['docker','exec','-i',p.CONTAINER,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','postgres','-d',p.DATABASE],input=query,encoding='utf-8',capture_output=True)
 if result.returncode:raise RuntimeError('synthetic source preparation failed; credentials and payload withheld')
 return result.stdout.strip()
def main():
 meta=json.loads(subprocess.check_output(['docker','inspect',p.CONTAINER],text=True))[0]
 assert meta['State']['Running'] and meta['Config']['Labels'].get('codex.scope')=='agent-q2-test'
 assert p.DATABASE=='agent_q2_acceptance_20261024'
 original=p.fingerprint('agent_q2_test')
 assert p.sql(p.DATABASE,"BEGIN READ ONLY; SELECT count(*) FROM data_contract WHERE code='SYNTHETIC-ACCEPTANCE-S0'; ROLLBACK;")=='0','fixture exists; do not replay preparation'
 criterion={'id':'C1','clause':'3.1','text':'合成夹具：核对交付成果范围','required':True,'method':'MANUAL_CHECK','evidenceCategory':'RECEIPT'}
 doc={'metadata':{'syntheticAcceptance':True},'spec':{'acceptanceCriteria':{'version':'1','criteria':[criterion]}}}
 raw=json.dumps(doc,ensure_ascii=False);digest=hashlib.sha256(raw.encode()).hexdigest();quoted=raw.replace("'","''")
 source_sql("BEGIN; INSERT INTO data_contract(id,workspace_id,code,name) VALUES('81818181-8181-4181-8181-818181818181','11111111-1111-4111-8111-111111111111','SYNTHETIC-ACCEPTANCE-S0','合成验收标准 只读夹具'); INSERT INTO contract_version(id,contract_id,major_version,minor_version,patch_version,status,document,source_sha256,published_at) VALUES('82828282-8282-4282-8282-828282828282','81818181-8181-4181-8181-818181818181',1,0,0,'PUBLISHED','"+quoted+"','"+digest+"',now()),('83838383-8383-4383-8383-838383838383','81818181-8181-4181-8181-818181818181',1,0,1,'PUBLISHED','{\"metadata\":{\"syntheticAcceptance\":true},\"spec\":{}}',repeat('a',64),now()); GRANT SELECT ON data_contract,contract_version TO agent_q2_reader_20261024; COMMIT;")
 assert p.fingerprint('agent_q2_test')==original
 print('Synthetic standard versions and two SELECT grants prepared in independent acceptance DB; original unchanged.')
if __name__=='__main__':main()
