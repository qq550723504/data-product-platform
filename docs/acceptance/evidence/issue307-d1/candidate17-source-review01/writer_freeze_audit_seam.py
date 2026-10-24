from contextlib import ExitStack
import hashlib,importlib,json,subprocess,sys,tempfile
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
external=[]
def reject(*args,**kw):external.append(repr(args));raise AssertionError('real process forbidden')
with patch.object(subprocess,'Popen',reject):
    model=importlib.import_module('test_six_candidate_runtime')
    check=importlib.import_module('test_retained_v26')
    collected={};original_save=model.r.save
    def capture(path,value):
        original_save(path,value)
        if Path(path).name=='non-delete-evidence-index.json':
            collected['index']=Path(path).read_bytes()
            collected['members']={name:(Path(path).parent/name).read_bytes() for name in value['files']}
    with patch.object(model.r,'save',capture):model.SixHostTests('test_full_three_group_source_backup_restore_sequence').composition()
    f=check.f
    with tempfile.TemporaryDirectory() as temp,ExitStack() as stack:
        root=Path(temp).resolve();public=root/'preparation';packet=root/'packet';review=root/'docs/acceptance/evidence/issue307-d1/candidate17-source-review01'
        for directory in (public,packet,review):directory.mkdir(parents=True)
        for name,raw in collected['members'].items():
            path=public/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(raw)
        (public/'non-delete-evidence-index.json').write_bytes(collected['index'])
        (review/'recorded.txt').write_bytes(b'pure recorded source evidence')
        source='8cca345525af828a8c9461e75f2dc5bf5f1fb428'
        candidate={'Execution':f.p.EXECUTION,'Source':source}
        f.save(packet/'candidate.json',candidate)
        f.save(review/'evidence-index.json',{'status':'C17_SOURCE_REVIEW_EVIDENCE_INDEX','count':1,'files':{'recorded.txt':{'sha256':f.sha(review/'recorded.txt'),'bytes':29}}})
        f.save(review/'independent-final-review01.json',{'status':'PASS_BOUNDED_C17_SOURCE_PREPARATION_REVIEW','source_head':source,'remaining_blockers':[]})
        for obj,name,value in [(f.r,'ROOT',root),(f.r,'PUBLIC',public),(f,'PACKET',packet)]:stack.enter_context(patch.object(obj,name,value))
        built=f.preparation_audit_plan(candidate)
        plan=packet/'preparation-evidence-audit-plan.json'
        # This is the exact shared audit function called by real complete main.
        replay=f.r.evidence_indexes.audit_plan(root,plan,f.sha(plan),packet/'candidate.json')
        if built!=replay:raise AssertionError('writer/freeze/complete audit seam differs')
        actual_index=json.loads(collected['index'])
        if actual_index['status']!='SIX_NONDELETE_PREPARATION_EVIDENCE_INDEX' or actual_index['count']!=len(collected['members']):raise AssertionError('real writer metadata contract differs')
        record={'status':'PASS_RECORDED_WRITER_FREEZE_COMPLETE_AUDIT_SEAM','writer':'actual run_non_delete under author full-three-group process/provider doubles','preparation_members':actual_index['count'],'all_metadata_have_sha256_and_bytes':all(set(x)=={'sha256','bytes'} for x in actual_index['files'].values()),'freeze_builder':'actual preparation_audit_plan','complete_entry_auditor':'actual evidence_indexes.audit_plan also called by main','plan_result':replay,'external_process_attempts':external,'native_Source_Docker_provider_DELETE':0,'native_acceptance':'NOT_RUN'}
        raw=(json.dumps(record,indent=2)+'\n').encode()
        with (OUT/'writer-freeze-audit-seam01.json').open('xb') as stream:stream.write(raw)
        print(json.dumps({'status':record['status'],'preparation_members':actual_index['count'],'total_audited':replay['members_verified'],'sha256':hashlib.sha256(raw).hexdigest()}))
