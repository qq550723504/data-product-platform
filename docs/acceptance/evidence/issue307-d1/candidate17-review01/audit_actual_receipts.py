import collections,datetime as dt,hashlib,json,re,subprocess,sys,uuid
from pathlib import Path
from urllib.parse import parse_qs,unquote
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent;BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
PACKET=BASE/'remaining-candidate17';PREP=BASE/'remaining-candidate17-preparation01';READ=BASE/'remaining-candidate17-readonly01';PLAN=BASE/'candidate17-planning01';PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate17')
load=lambda path:json.loads(path.read_bytes());sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
def save(name,obj):
    with (OUT/name).open('xb') as f:f.write((json.dumps(obj,indent=2,ensure_ascii=True)+'\n').encode())
def when(value):return dt.datetime.fromisoformat(value.replace('Z','+00:00'))
def journals(folder,prefix,full=False,expectation=None):
    starts={p.name[len(prefix+'-start-'):-5]:load(p) for p in folder.glob(prefix+'-start-*.json')}
    observations={p.name[len(prefix+'-observation-'):-5]:load(p) for p in folder.glob(prefix+'-observation-*.json')}
    assert starts and set(starts)==set(observations),str(folder)
    methods=collections.Counter();statuses=collections.Counter()
    for id,s in starts.items():
        assert str(uuid.UUID(id))==id and s['id']==id
        o=observations[id];assert o.get('id',id)==id and o['transport_error'] is False
        assert when(s['at'])<=when(o['at']);methods[s['method']]+=1;statuses[o['status']]+=1
        if full:
            assert s['method'] in ('GET','HEAD') and s['audit_id']==o['audit_id']==expectation['audit_id']
            assert when(expectation['not_before'])<=when(s['at'])<=when(o['at'])<=when(expectation['expires_at'])
    return starts,observations,{'pairs':len(starts),'methods':dict(methods),'statuses':dict(statuses)}
c=load(PACKET/'candidate.json');bindings=load(PACKET/'bindings.json');a=load(PREP/'allocation.json');complete=load(READ/'complete.json');terminal=load(PREP/'non-delete-terminal.json')
assert terminal['status']=='PASS_NONDELETE_SOURCE_RESTORE_PREPARATION' and terminal['groups_completed']==3 and terminal['failure'] is None and len(terminal['closure'])==92 and all(x['closed'] for x in terminal['closure'])
assert len(terminal['profile_revocations'])==19 and all(x['disabled'] for x in terminal['profile_revocations'])
assert sha(PREP/'non-delete-evidence-index.json')==terminal['evidence_index_sha256']
assert complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY' and complete['technical_requests']==125 and complete['native_delete']==complete['business_mutations']==0
expected_keys=['fixture/p2.csv','fixture/t1.csv','fixture/t2.csv','fixture/h2-a.csv','fixture/m1-a.csv','fixture/m1-b.csv'];targets=[];survivors=[];receipts=[];helper_counts={};old_preservation={};cost_growth={};all_read_calls=0
for group,b,result in zip(c['Groups'],bindings,complete['groups']):
    name=group['Group'];root=READ/name;expectation=load(root/'expectation.json');native=load(root/'files/six-native-verification.json');fresh=native['verification']
    assert fresh['status']=='PASS_NATIVE_MINIO_PROTECTION_SNAPSHOT' or fresh['status'].startswith('PASS_'),fresh['status']
    assert expectation['Execution']==c['Execution'] and expectation['Profile']==c['Profile']
    assert expectation['binary_sha256']==c['ReaderBinarySHA256'] and native['binary_sha256']==c['ReaderBinarySHA256'] and native['audit_id']==expectation['audit_id']
    assert native['candidate_sha256']==expectation['candidate_sha256']
    assert when(expectation['not_before'])<=when(native['started_at'])<=when(native['finished_at'])<=when(expectation['expires_at'])
    starts,observations,journal=journals(root/'files','readonly',True,expectation)
    assert journal['pairs']==result['technical_requests']=={'business-races':51,'late-hold':40,'mixed':34}[name]
    all_read_calls+=journal['pairs']
    assert len(fresh['copies'])==result['copies']=={'business-races':7,'late-hold':6,'mixed':4}[name]
    assert len(fresh['disabled_identities'])=={'business-races':13,'late-hold':8,'mixed':8}[name]
    frozen_iam=load(PACKET/name/'iam.json');assert fresh['disabled_identities']==frozen_iam['disabled_identities']
    for policy in result['policies']:
        assert policy['official_equals'] is True and sha(root/'files'/('native-policy-'+policy['policy']+'.json'))==policy['fresh_file_sha256']
    uncreated=load(root/'files/six-uncreated-users.json');assert uncreated['audit_id']==expectation['audit_id'] and len(uncreated['users'])=={'business-races':1,'late-hold':4,'mixed':4}[name]
    for user,fact in uncreated['users'].items():
        sid=fact['attempt_id'];s=starts[sid];o=observations[sid];assert fact['status']==o['status']==404 and fact['code']=='XMinioAdminNoSuchUser'
        assert s['method']=='GET' and s['path']=='/minio/admin/v3/user-info' and parse_qs(s['query'])=={'accessKey':[user]}
    if name=='business-races':
        absence=native['p1_absence'];sid=absence['attempt_id'];s=starts[sid];o=observations[sid]
        assert absence['status']==o['status']==404 and absence['code']==o['provider_error_code']=='NoSuchVersion' and o['provider_request_id']
        assert s['method']=='HEAD' and unquote(s['path'])=='/'+absence['copy']['Namespace']+'/'+absence['copy']['Key']
        assert parse_qs(s['query'])=={'versionId':[absence['copy']['ProviderVersion']]}
    assert load(root/'Core-before.json')==load(root/'Core-after.json')==load(PACKET/name/'core-readonly.json')==load(PREP/name/'source-Core-before.json')==load(PREP/name/'source-Core-after.json')
    closure=load(root/'closure.json');assert closure['failure'] is None and closure['all_closed'] is True and closure['unknown_temporary_owner'] is None and all(x['closed'] for x in closure['owners'])
    assert result==load(root/'result.json')
    for row in fresh['copies']:
        cp=row['Copy'];frozen=next(x for x in group['Copies'] if x['ID']==cp['ID']);assert frozen==cp
        assert row==next(x for x in b['copy_protection'] if x['Copy']['ID']==cp['ID'])
        if cp['Key'] in expected_keys:targets.append(cp)
        else:survivors.append(cp)
    # Independently rerun only the already built file-only Go aggregator.
    args=[str(PRIVATE/'readonly-check.exe'),'-six','-baseline',str(PACKET/name/'expectation'),'-fresh',str(root/'files'),'-group',name,'-expectation',str(root/'expectation.json'),'-inventory',str(PRIVATE/name/'present-inventory.json'),'-mapping',str(PREP/'six-map.json')]
    checked=subprocess.run(args,cwd=ROOT,capture_output=True,timeout=45)
    for suffix,raw in [('stdout.json',checked.stdout),('stderr.txt',checked.stderr)]:
        with (OUT/(name+'-file-checker-'+suffix)).open('xb') as f:f.write(raw)
    assert checked.returncode==0,checked.stderr.decode('utf-8','replace')
    assert json.loads(checked.stdout)==load(root/'checker-stdout.txt')
    assert checked.stderr==(root/'checker-stderr.txt').read_bytes()
    cp=load(PREP/name/'checkpoint.json');assert cp['core_full_restore']==cp['native_file_restore']==cp['native_exact_versions_restore']=='PASS' and len({cp['source_core'],cp['restored_core'],cp['rehearsal_core']})==3 and cp['source_native']!=cp['restored_native']
    assert load(PREP/name/'clean-Core-after-trial.json')==load(PREP/name/'clean-restored-Core.json')
    for filename,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:assert sha(PRIVATE/name/'checkpoint'/filename)==group[key]==cp['backups'][filename]
    rehearsal=PREP/name/'rehearsal';rr=load(rehearsal/'result.json');assert rr['status']=='PASS_ISOLATED_NONDELETE_COMMANDS' and rr['old_fact_multisets_preserved'] is True and rr['native_delete']==rr['native_object_put']==rr['source_business_mutations']==0
    assert load(rehearsal/'inspector-closure.json')['disabled_readback']=='FRESH_CONFIRMED_DISABLED'
    before=load(rehearsal/'immutable-before.json');after=load(rehearsal/'immutable-after.json');assert set(before)==set(after) and len(before)==20
    for table,rows in before.items():
        assert all(after[table].get(k,0)>=v for k,v in rows.items()),table
    old_preservation[name]={'tables':20,'all_old_hashed_row_multisets_preserved':True}
    cost_growth[name]=sum(after['cost_event'].values())-sum(before['cost_event'].values())
    ids=set();stage_counts={}
    for file in (PREP/name).rglob('*-start-*.json'):
        prefix=file.name.split('-start-')[0]
        if prefix not in ('readonly','prepare'):continue
        id=file.name.split('-start-')[1][:-5]
        other=file.with_name(prefix+'-observation-'+id+'.json');assert other.exists()
        s=load(file);o=load(other);assert s['id']==id and o.get('id',id)==id and str(uuid.UUID(id))==id and when(s['at'])<=when(o['at']) and o['transport_error'] is False
        assert s['method']!='DELETE'
        assert id not in ids;ids.add(id)
        stage_counts[file.parent.parent.name]=stage_counts.get(file.parent.parent.name,0)+1
    helper_counts[name]={'pairs':len(ids),'stages':stage_counts}
    receipts.append({'group':name,'copies':len(fresh['copies']),'known_users':len(fresh['disabled_identities']),'absent_users':len(uncreated['users']),'journal':journal,'Core_before_after_exact':True,'all_closed':True,'file_checker':'INDEPENDENT_REPLAY_EXACT_RESULT','amount':'NOT_ASSESSED'})
assert len(targets)==6 and len(survivors)==11 and [x['Key'] for x in targets]==expected_keys
fields=['ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize']
compact=json.dumps([{k:x[k] for k in fields} for x in targets],ensure_ascii=False,separators=(',',':')).encode();assert hashlib.sha256(compact).hexdigest()==c['TargetDigest']
# Concrete27 old backup hash members: original executable03 + c15 + c16.
retained=a['retained'];old_maps=[('remaining-executable03',retained['prior']['old_retained']['backup_sha256']),('remaining-candidate15',retained['c15']['backups']),('remaining-candidate16',retained['c16']['backups'])]
for generation,items in old_maps:
    assert len(items)==9
    for relative,digest in items.items():
        group,filename=relative.split('/');assert sha(PRIVATE.parent/generation/group/'checkpoint'/filename)==digest
physical=load(PLAN/'final-cold-inventory01.json');assert physical['known_union']==len(physical['rows'])==537 and not physical['failures'] and len({x['cid'] for x in physical['rows']})==537
assert all(x['running'] is False and x['pid']==0 and x['restart']=='no' and not x['port_bindings'] for x in physical['rows'])
expected_set=set(a['retained']['closed_cids'])|set(a['owners'])
extra=load(PLAN/'retained-v26-extra-readers01.json');expected_set.update(x['cid'] for x in extra['readers']);expected_set.update(load(READ/name/'allocated-owner.json')['cid'] for name in ('business-races','late-hold','mixed'))
assert expected_set=={x['cid'] for x in physical['rows']}
assert cost_growth=={'business-races':9,'late-hold':12,'mixed':6}
report={'status':'PASS_ACTUAL_NONDELETE_RECEIPTS_AND_FILE_AGGREGATION_REVIEW','complete_groups':receipts,'complete_actual_technical_pairs':all_read_calls,'preparation_helper_journal_pairs':helper_counts,'preparation_helper_pairs_total':sum(x['pairs'] for x in helper_counts.values()),'trial_old_facts':old_preservation,'trial_appended_cost_events':cost_growth,'cost_boundary':'Helper journal counts and27 persisted isolated rehearsal CostEvents are different accounting views, not a claimed aggregate provider HTTP total or amount. Amount NOT_ASSESSED.','targets':targets,'survivors':survivors,'exact_target_digest':c['TargetDigest'],'new_backups_hash_verified':9,'old_backups_hash_verified':27,'old_inventory_fixed_gate_plus_extra':448,'final_known_union':537,'final_physical_receipt_sha256':sha(PLAN/'final-cold-inventory01.json'),'physical_boundary':'Author actual fresh readback verified as file/identity set; reviewer did not operate Docker.','actual_native_executed_by':'parent fixed CLI; reviewer only original evidence and file-only checker replay','reviewer_Docker_Source_provider_enable_send_delete':0,'original_v26_IAM_UNCONFIRMED':'PRESERVED','authorization':'NOT_RECEIVED','schedule':'NOT_CREATED','full_D1':'NOT_ACCEPTED'}
save('actual-receipt-audit01.json',report);print(json.dumps({'status':report['status'],'complete_pairs':all_read_calls,'preparation_helper_pairs':report['preparation_helper_pairs_total'],'helper_by_group':helper_counts,'trial_cost_append':cost_growth,'physical':537,'targets':6,'survivors':11},indent=2))
