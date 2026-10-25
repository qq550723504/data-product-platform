"""Real temporary Git and signed host combinations, never retained runtimes."""
import copy
import json
from pathlib import Path
import subprocess
import tempfile
import importlib.util
import base64
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch
import release_binding as r
import fixture_artifacts as artifacts
import execution_freeze as freeze
import test_fixture_artifacts as artifact_tests
import test_retention_composition as composition_tests
import test_six_candidate_runtime as runtime_tests
from test_execution_freeze import snapshot as legacy_snapshot


def recorded(source):
    rows=[]
    for n,path in enumerate(r.WORKFLOWS):
        names=sorted(r.CI_JOBS) if n==0 else ['synthetic-source-admission']
        run=100+n;suite=200+n;jobs=[];checks=[]
        for k,name in enumerate(names):
            cid=1000+n*100+k
            jobs.append(dict(name=name,check_run_id=cid,head_sha=source['merge_sha'],run_id=run,run_attempt=1,status='completed',conclusion='success'))
            checks.append(dict(id=cid,name=name,head_sha=source['merge_sha'],status='completed',conclusion='success',suite_id=suite,app_id=freeze.ACTIONS_APP_ID,app_slug='github-actions'))
        rows.append(dict(id=run,attempt=1,path=path,event='push',branch='main',repository=freeze.REPOSITORY,head_repository=freeze.REPOSITORY,
                         head_sha=source['merge_sha'],status='completed',conclusion='success',suite_id=suite,jobs=jobs,checks=checks))
    return dict(repository=freeze.REPOSITORY,pr=dict(number=source['pull_request'],state='closed',merged=True,draft=False,head_repository=freeze.REPOSITORY,
        head=source['review_head'],merge=source['merge_sha'],base_branch='main'),issue_state='open',review_threads_complete=True,unresolved_threads=0,
        review_commit=dict(sha=source['review_head'],tree=source['tree_sha']),merge_commit=dict(sha=source['merge_sha'],tree=source['tree_sha'],parents=[source['base_sha']]),
        protection_anchor=copy.deepcopy(source['protection_anchor']),workflows=rows)


def source_fixture(root):
    # Reuse the real Git/R packaging fixture; create a genuine squash shape.
    spec,_=artifact_tests.ArtifactTests.fixture(None,root)
    reviewed=artifacts.git(root,'rev-parse','HEAD').decode().strip()
    tree=artifacts.git(root,'rev-parse','HEAD^{tree}').decode().strip()
    base=spec['base_source']
    merged=artifacts.git(root,'-c','user.name=Release Test','-c','user.email=release@example.invalid',
                         'commit-tree',tree,'-p',base,'-m','squash release').decode().strip()
    artifacts.git(root,'checkout','-q','--detach',merged)
    control=root.parent/'runtime';control.mkdir()
    archive=control/'approved-runtime-source.zip';archive.write_bytes(artifacts.runtime_archive(root,spec['source']))
    protection=legacy_snapshot(True)
    source=dict(kind=r.KIND,repository=freeze.REPOSITORY,pull_request=332,review_head=reviewed,merge_sha=merged,tree_sha=tree,base_sha=base,
                host=artifacts.host_snapshot(root),runtime=dict(spec_sha256=freeze.sha(root/artifacts.SPEC),archive=dict(path=str(archive),sha256=freeze.sha(archive),bytes=archive.stat().st_size)),
                protection_anchor=dict(branch=freeze.BRANCH,head=protection['head'],rule=protection['rule'],protection=protection['protection']))
    folder=root/'docs';folder.mkdir();path=folder/'release-source.json';path.write_text(json.dumps(source),encoding='utf-8')
    ref=dict(path='docs/release-source.json',sha256=freeze.sha(path),bytes=path.stat().st_size)
    return source,ref


def execution_fixture(root):
    source,ref=source_fixture(root);spec=artifacts.load(root/artifacts.SPEC)
    private=root/'private';private.mkdir();packet=root/'packet';packet.mkdir()
    snapshot=recorded(source);freeze.save(root/'docs/snapshot.json',snapshot)
    record=dict(status='PASS_MERGED_RELEASE_SOURCE_CI_BEFORE_ALLOCATION',head=source['merge_sha'],run=100,execution=spec['execution'],
                release_source=ref,snapshot_sha256=freeze.sha(root/'docs/snapshot.json'))
    freeze.save(root/'docs/build-ci.json',record)
    ci=dict(kind=r.CI_KIND,head=source['merge_sha'],run=100,receipt='docs/build-ci.json',sha256=freeze.sha(root/'docs/build-ci.json'))
    frozen=dict(source=spec['source'],host=source['host'],source_ci=ci,runtime_artifact_spec_sha256=source['runtime']['spec_sha256'])
    freeze.save(packet/'source-freeze.json',frozen);freeze.save(packet/'bindings.json',[])
    (packet/'confirmation-template.txt').write_bytes(b'NEW EXACT SCOPE')
    candidate=dict(Execution=spec['execution'],Profile=spec['profile'],Source=spec['source'],NotBefore=spec['not_before'],NotAfter=spec['not_after'],
        SourceFreezeSHA256=freeze.sha(packet/'source-freeze.json'),ReaderBinarySHA256='1'*64,SenderBinarySHA256='2'*64,OwnerBinarySHA256='3'*64)
    freeze.save(packet/'candidate.json',candidate)
    binding=dict(kind=r.EXECUTION_KIND,status='FROZEN_MERGED_RELEASE_EXECUTION_BINDING_NOT_AUTHORIZED',execution=candidate['Execution'],profile=candidate['Profile'],
                 artifact_bindings=freeze.artifact_bindings(packet,candidate),release_source=ref,public_binding_path='docs/execution-binding.json')
    auth=dict(execution=candidate['Execution'],release_merge_sha=source['merge_sha'],final_approved_head=source['merge_sha'],
              release_source_sha256=ref['sha256'],old_authorization_reused=False)
    def seal():
        (private/'execution-binding.json').write_text(json.dumps(binding),encoding='utf-8')
        (root/'docs/execution-binding.json').write_bytes((private/'execution-binding.json').read_bytes())
        expected=freeze.sha(private/'execution-binding.json');auth['execution_binding_sha256']=expected
        auth['approved_reference_artifacts']=[dict(path='docs/execution-binding.json',sha256=expected),dict(path=ref['path'],sha256=ref['sha256'])]
        return expected
    return source,ref,private,packet,candidate,binding,auth,seal,ci,record


class RecordedAPI:
    def __init__(self,source,fault=None):self.source,self.fault,self.calls=source,fault,[];self.record=recorded(source)
    def graph(self,query,variables):
        self.calls.append(('graph',query,variables))
        if not query.startswith('query'):raise AssertionError('mutation in read-only source client')
        if 'reviewThreads' in query:
            return {'repository':{'pullRequest':{'reviewThreads':dict(nodes=[{'isResolved':True}],pageInfo=dict(hasNextPage=self.fault=='review-pagination',endCursor=None))}}}
        anchor=self.source['protection_anchor']
        return {'repository':dict(nameWithOwner=freeze.REPOSITORY,ref=dict(target={'oid':anchor['head']},branchProtectionRule=anchor['rule']))}
    def request(self,method,path):
        self.calls.append((method,path))
        if method!='GET':raise AssertionError('mutation in source client')
        if self.fault=='transport':raise TimeoutError('primary read UNKNOWN')
        s=self.source
        if '/protection' in path:return copy.deepcopy(s['protection_anchor']['protection'])
        if '/branches/' in path:return dict(commit={'sha':s['protection_anchor']['head']})
        if '/pulls/' in path:return dict(number=s['pull_request'],state='closed',merged=True,draft=False,merge_commit_sha=s['merge_sha'],head=dict(sha=s['review_head'],repo={'full_name':freeze.REPOSITORY}),base={'ref':'main'})
        if '/issues/' in path:return {'state':'open'}
        if '/git/commits/' in path:
            merge=path.endswith(s['merge_sha'])
            return dict(sha=s['merge_sha'] if merge else s['review_head'],tree={'sha':s['tree_sha']},parents=[{'sha':s['base_sha']}])
        if '/actions/runs?' in path:
            rows=[]
            for w in self.record['workflows']:
                row=dict(id=w['id'],run_attempt=w['attempt'],path=w['path'],event=w['event'],head_branch=w['branch'],head_sha=w['head_sha'],
                         status=w['status'],conclusion=w['conclusion'],repository={'full_name':w['repository']},head_repository={'full_name':w['head_repository']},check_suite_id=w['suite_id'])
                rows.append(row)
                if self.fault=='newer-failed' and w['path']==r.WORKFLOWS[0]:rows.append(dict(row,id=999,conclusion='failure'))
            return dict(total_count=len(rows)+(1 if self.fault=='run-pagination' else 0),workflow_runs=rows)
        if '/check-runs?' in path:
            rows=[dict(id=c['id'],name=c['name'],head_sha=c['head_sha'],status=c['status'],conclusion=c['conclusion'],check_suite={'id':c['suite_id']},app={'id':c['app_id'],'slug':c['app_slug']}) for w in self.record['workflows'] for c in w['checks']]
            return dict(total_count=len(rows)+(1 if self.fault=='check-pagination' else 0),check_runs=rows)
        if '/attempts/' in path:
            run=int(path.split('/runs/')[1].split('/')[0]);w=self.record['workflows'][0 if run in (100,999) else 1]
            jobs=[dict(name=j['name'],check_run_url='https://api.github.com/repos/'+freeze.REPOSITORY+'/check-runs/'+str(j['check_run_id']),
                head_sha=j['head_sha'],run_id=run,run_attempt=1,status=j['status'],conclusion=j['conclusion']) for j in w['jobs']]
            if self.fault=='foreign-check-url':jobs[0]['check_run_url']='https://foreign.invalid/check-runs/1'
            return dict(total_count=len(jobs)+(1 if self.fault=='job-pagination' else 0),jobs=jobs)
        raise AssertionError('unexpected request '+path)


class ReleaseTests(unittest.TestCase):
    def test_complete_released_build_and_frozen_source_preserve_original_twelve_members(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref,private,packet,c,b,auth,seal,ci,record=execution_fixture(root)
            runtime=runtime_tests.r;control=root.parent/'runtime';spec=artifacts.load(root/artifacts.SPEC)
            for name in runtime.BUILD_MEMBERS:(control/name).write_bytes(name.encode())
            a=dict(source=c['Source'],host=source['host'],source_ci=ci,runtime_artifact_spec_sha256=source['runtime']['spec_sha256'],
                   build_environment={'GOWORK':'off','CGO_ENABLED':'0','GOARCH':'amd64'},build_cwd_relative='build-runtime/apps/platform',
                   build_files={n:freeze.sha(control/n) for n in runtime.BUILD_MEMBERS},binaries={n:freeze.sha(control/n) for n in ('reader','operator','owner','readonly-check.exe')})
            with patch.object(runtime,'ROOT',root),patch.object(runtime,'CONTROL',control),patch.object(runtime.p,'EXECUTION',c['Execution']),\
                 patch.object(runtime.p,'artifact_contract',return_value=dict(execution=c['Execution'],profile=c['Profile'],start=c['NotBefore'],end=c['NotAfter'])):
                self.assertEqual(runtime.build_contract(a)['source_ci'],ci)
                frozen=dict(version=2,source=a['source'],host=a['host'],source_ci=ci,runtime_artifact_spec_sha256=a['runtime_artifact_spec_sha256'],
                    actual_build_source=a['source'],initial_allocation_build_source=a['source'],initial_build_files=a['build_files'],runtime=spec,
                    archive_sha256=spec['archive_sha256'],archive_bytes=spec['archive_bytes'])
                final_packet=root/'frozen-final';final_packet.mkdir()
                freeze.save(final_packet/'source-freeze.json',frozen);c['SourceFreezeSHA256']=freeze.sha(final_packet/'source-freeze.json')
                artifacts.verify_frozen(final_packet,c,a,spec)
                for fault in ('CI-kind','receipt','snapshot','build-member','frozen-source'):
                    bad=copy.deepcopy(a);original=(root/'docs/snapshot.json').read_bytes()
                    if fault=='CI-kind':bad['source_ci']['kind']='unknown'
                    elif fault=='receipt':bad['source_ci']['sha256']='f'*64
                    elif fault=='snapshot':(root/'docs/snapshot.json').write_bytes(b'{}')
                    elif fault=='build-member':bad['build_files'].pop('owner-build.stderr')
                    else:
                        bad['source_ci']['run']=999
                        with self.assertRaises(ValueError):artifacts.verify_frozen(final_packet,c,bad,spec)
                        continue
                    with self.subTest(fault=fault),self.assertRaises((ValueError,KeyError)):runtime.build_contract(bad)
                    (root/'docs/snapshot.json').write_bytes(original)

    def test_primary_API_composition_latest_failure_pagination_and_no_mutation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref=source_fixture(root)
            for fault in (None,'newer-failed','run-pagination','check-pagination','job-pagination','review-pagination','foreign-check-url','transport'):
                client=object.__new__(r.Client);api=RecordedAPI(source,fault);client.api=api
                if fault is None:self.assertEqual(r.check(root,ref,client)['merge_sha'],source['merge_sha'])
                else:
                    with self.subTest(fault=fault),self.assertRaises((ValueError,TimeoutError)):r.check(root,ref,client)
                self.assertTrue(all(v[0] in ('GET','graph') for v in api.calls))
                if fault=='newer-failed':
                    self.assertTrue(any('/runs/999/attempts/1/jobs' in v[1] for v in api.calls if v[0]=='GET'))
                    self.assertFalse(any('/runs/100/attempts/1/jobs' in v[1] for v in api.calls if v[0]=='GET'))
    def test_new_execution_binding_actual_source_and_current_reference_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref,private,packet,c,b,auth,seal,ci,record=execution_fixture(root)
            expected=seal();client=SimpleNamespace(snapshot=lambda _:recorded(source))
            result=freeze.verify_binding(root,private,packet,c,expected,client,auth=auth)
            self.assertEqual(result['status'],'PASS_FRESH_MERGED_RELEASE_EXECUTION_BINDING')
            a=dict(source=c['Source'],host=source['host'],source_ci=ci,runtime_artifact_spec_sha256=source['runtime']['spec_sha256'])
            r.saved_build_ci(root,a,record,recorded(source),freeze.sha(root/'docs/snapshot.json'),c['Execution'])
            for fault in ('receipt','snapshot','CI-kind','run','source','host','R-spec'):
                bad=copy.deepcopy(a);rec=copy.deepcopy(record);snap=recorded(source);digest=freeze.sha(root/'docs/snapshot.json')
                if fault=='receipt':rec['release_source']['sha256']='f'*64
                elif fault=='snapshot':digest='f'*64
                elif fault=='CI-kind':bad['source_ci']['kind']='legacy'
                elif fault=='run':snap['workflows'][0]['id']=999
                elif fault=='source':bad['source']='f'*40
                elif fault=='host':bad['host']['source']='f'*40
                else:bad['runtime_artifact_spec_sha256']='f'*64
                with self.subTest(fault=fault),self.assertRaises(ValueError):r.saved_build_ci(root,bad,rec,snap,digest,c['Execution'])

    def test_terminal_old_authority_kind_scope_and_rehashed_frozen_chain_reject(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref,private,packet,c,b,auth,seal,*_=execution_fixture(root)
            original=copy.deepcopy((c,b,auth));frozen=(packet/'source-freeze.json').read_bytes()
            for fault in ('terminal','old-authority','source-approval','merge-approval','binding-approval','missing-reference','unknown-kind','execution','profile','R','frozen-host','frozen-CI-kind','R-window'):
                c.clear();b.clear();auth.clear();c.update(copy.deepcopy(original[0]));b.update(copy.deepcopy(original[1]));auth.update(copy.deepcopy(original[2]))
                (packet/'source-freeze.json').write_bytes(frozen)
                if fault=='terminal':
                    (private/'authorized-execution01').mkdir();freeze.save(private/'authorized-execution01/terminal.json',dict(terminal=True))
                elif fault=='old-authority':auth['old_authorization_reused']=True
                elif fault=='source-approval':auth['release_source_sha256']='f'*64
                elif fault=='merge-approval':auth['release_merge_sha']='f'*40
                elif fault=='unknown-kind':b['kind']='legacy-claim'
                elif fault=='execution':b['execution']='old-execution'
                elif fault=='profile':b['profile']='old-profile'
                elif fault=='R':c['Source']=source['merge_sha']
                elif fault=='R-window':c['NotAfter']='expanded'
                elif fault in ('frozen-host','frozen-CI-kind'):
                    old=json.loads(frozen)
                    if fault=='frozen-host':old['host']['source']=source['review_head']
                    else:old['source_ci']['kind']='legacy'
                    (packet/'source-freeze.json').write_text(json.dumps(old));c['SourceFreezeSHA256']=freeze.sha(packet/'source-freeze.json')
                freeze_path=packet/'candidate.json';freeze_path.write_text(json.dumps(c));b['artifact_bindings']=freeze.artifact_bindings(packet,c)
                expected=seal()
                if fault=='binding-approval':auth['execution_binding_sha256']='f'*64
                elif fault=='missing-reference':auth['approved_reference_artifacts']=auth['approved_reference_artifacts'][:1]
                client=SimpleNamespace(snapshot=Mock(return_value=recorded(source)))
                with self.subTest(fault=fault),self.assertRaises(ValueError):freeze.verify_binding(root,private,packet,c,expected,client,auth=auth)
                client.snapshot.assert_not_called()
                terminal=private/'authorized-execution01/terminal.json'
                if terminal.exists():terminal.unlink();terminal.parent.rmdir()

    def test_preallocation_complete_entry_preserves_merged_receipt_before_resources(self):
        loader=importlib.util.spec_from_file_location('release_preallocation',Path(__file__).with_name('prepare-six-candidate.py'))
        p=importlib.util.module_from_spec(loader);loader.loader.exec_module(p)
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref=source_fixture(root);(root/'docs/acceptance/evidence/issue307-d1').mkdir(parents=True)
            client=SimpleNamespace(snapshot=Mock(return_value=recorded(source)))
            with patch.object(p,'ROOT',root),patch.object(r,'Client',return_value=client):
                ci=p.preallocation_ci(ref)
            self.assertEqual(ci['kind'],r.CI_KIND);self.assertEqual(ci['head'],source['merge_sha'])
            receipt=freeze.load(root/ci['receipt']);self.assertEqual(receipt['release_source'],ref)
            (root/'examples/lifecycle-d1/host.py').write_bytes(b'not published')
            with patch.object(p,'ROOT',root),patch.object(r,'Client',return_value=client),\
                 patch.object(p,'historical_gate',side_effect=AssertionError('history before source gate')) as history:
                with self.assertRaises(ValueError):p.preallocation_ci(ref)
                history.assert_not_called()

    def test_real_squash_R_zip_P_raw_LF_and_closed_PR_full_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref=source_fixture(root);current=recorded(source)
            client=SimpleNamespace(snapshot=Mock(return_value=current))
            result=r.check(root,ref,client)
            self.assertEqual(result['status'],'PASS_FRESH_MERGED_RELEASE_SOURCE_GATE')
            self.assertEqual(len(result['snapshot']['workflows'][0]['jobs']),13)
            self.assertNotEqual(source['protection_anchor']['head'],source['merge_sha'])
            self.assertEqual(artifacts.git(root,'merge-base','--is-ancestor',source['base_sha'],source['merge_sha']),b'')
            result=subprocess.run(['git','merge-base','--is-ancestor',source['review_head'],source['merge_sha']],cwd=root,capture_output=True)
            self.assertEqual(result.returncode,1) # P is not a squash parent/ancestor.
            old=legacy_snapshot(True);old.update(state='closed',merged=True,draft=False)
            with self.assertRaises(ValueError):freeze.ready(old,old['head'],require_frozen=True)

    def test_unpublished_dirty_untracked_wrong_HEAD_raw_LF_and_R_reject_before_API(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref=source_fixture(root);client=SimpleNamespace(snapshot=Mock(return_value=recorded(source)))
            path=root/'examples/lifecycle-d1/host.py';original=path.read_bytes();archive=Path(source['runtime']['archive']['path']);zipraw=archive.read_bytes()
            spec=root/artifacts.SPEC;specraw=spec.read_bytes()
            faults=('dirty','untracked','wrong-HEAD','raw-only','ZIP','R-spec','R-shared','HOST-members')
            for fault in faults:
                with self.subTest(fault=fault):
                    changed=copy.deepcopy(source)
                    if fault=='dirty':path.write_bytes(b'unpublished patch')
                    elif fault=='untracked':(root/'examples/lifecycle-d1/injected.py').write_bytes(b'injected')
                    elif fault=='wrong-HEAD':artifacts.git(root,'checkout','-q','--detach',source['review_head'])
                    elif fault=='raw-only':path.write_bytes(original.replace(b'\n',b'\r\n'))
                    elif fault=='ZIP':archive.write_bytes(zipraw+b'changed')
                    elif fault=='R-spec':spec.write_bytes(specraw+b' ')
                    elif fault=='R-shared':(root/'apps/platform/scope.json').write_bytes(b'foreign scope')
                    elif fault=='HOST-members':changed['host']['files'].pop(next(iter(changed['host']['files'])))
                    with self.assertRaises((ValueError,subprocess.CalledProcessError)):r.local_ready(root,changed)
                    client.snapshot.assert_not_called()
                    artifacts.git(root,'checkout','-q','--detach',source['merge_sha'])
                    artifacts.git(root,'restore','--source=HEAD','--worktree','--','apps','examples','migrations')
                    injected=root/'examples/lifecycle-d1/injected.py'
                    if injected.exists():injected.unlink()
                    archive.write_bytes(zipraw)
            r.check(root,ref,client)

    def test_current_main_CI_complete_identity_and_independent_protection_negatives(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,_=source_fixture(root);good=recorded(source)
            faults=('repository','PR','not-merged','draft','review-head','merge','tree','parent','reviews','review-page','issue',
                    'anchor-head','anchor-rule','anchor-policy','unlocked','missing-workflow','PR-CI','foreign-branch','foreign-repo',
                    'old-head','pending','failed','skipped','missing-job','duplicate-job','foreign-run','foreign-attempt','job-failed',
                    'missing-check','foreign-app','foreign-slug','foreign-suite','wrong-check-name','check-failed','source-admission')
            for fault in faults:
                current=copy.deepcopy(good);w=current['workflows'][0]
                if fault=='repository':current['repository']='foreign/repo'
                elif fault=='PR':current['pr']['number']=333
                elif fault=='not-merged':current['pr']['merged']=False
                elif fault=='draft':current['pr']['draft']=True
                elif fault=='review-head':current['pr']['head']='f'*40
                elif fault=='merge':current['pr']['merge']='f'*40
                elif fault=='tree':current['merge_commit']['tree']='f'*40
                elif fault=='parent':current['merge_commit']['parents']=[source['review_head']]
                elif fault=='reviews':current['unresolved_threads']=1
                elif fault=='review-page':current['review_threads_complete']=False
                elif fault=='issue':current['issue_state']='closed'
                elif fault=='anchor-head':current['protection_anchor']['head']='f'*40
                elif fault=='anchor-rule':current['protection_anchor']['rule']['id']='foreign'
                elif fault=='anchor-policy':current['protection_anchor']['protection']['required_signatures']['enabled']=False
                elif fault=='unlocked':current['protection_anchor']['rule']['lockBranch']=False
                elif fault=='missing-workflow':current['workflows'].pop()
                elif fault=='PR-CI':w['event']='pull_request'
                elif fault=='foreign-branch':w['branch']='topic'
                elif fault=='foreign-repo':w['head_repository']='foreign/repo'
                elif fault=='old-head':w['head_sha']='f'*40
                elif fault=='pending':w['status']='queued'
                elif fault=='failed':w['conclusion']='failure'
                elif fault=='skipped':w['conclusion']='skipped'
                elif fault=='missing-job':w['jobs'].pop()
                elif fault=='duplicate-job':w['jobs'][1]=copy.deepcopy(w['jobs'][0])
                elif fault=='foreign-run':w['jobs'][0]['run_id']=999
                elif fault=='foreign-attempt':w['jobs'][0]['run_attempt']=2
                elif fault=='job-failed':w['jobs'][0]['conclusion']='failure'
                elif fault=='missing-check':w['checks'].pop()
                elif fault=='foreign-app':w['checks'][0]['app_id']=1
                elif fault=='foreign-slug':w['checks'][0]['app_slug']='other-app'
                elif fault=='foreign-suite':w['checks'][0]['suite_id']=1
                elif fault=='wrong-check-name':w['checks'][0]['name']='optional'
                elif fault=='check-failed':w['checks'][0]['conclusion']='cancelled'
                else:current['workflows'][1]['conclusion']='failure'
                with self.subTest(fault=fault),self.assertRaises(ValueError):r.origin_ready(current,source)

    def test_link_shape_reference_kind_and_mid_API_mutation_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';source,ref=source_fixture(root);path=root/ref['path'];original=path.read_bytes()
            for fault in ('kind','extra','repository','host-source','anchor','sha','bytes','traversal','link','duplicate-json'):
                with self.subTest(fault=fault):
                    bad=copy.deepcopy(source);reference=copy.deepcopy(ref)
                    if fault=='kind':bad['kind']='legacy'
                    elif fault=='extra':bad['fallback']=True
                    elif fault=='repository':bad['repository']='other/repo'
                    elif fault=='host-source':bad['host']['source']=source['review_head']
                    elif fault=='anchor':bad['protection_anchor']['rule']['allowsForcePushes']=True
                    if fault in ('kind','extra','repository','host-source','anchor'):
                        path.write_text(json.dumps(bad));reference.update(sha256=freeze.sha(path),bytes=path.stat().st_size)
                    elif fault=='sha':reference['sha256']='f'*64
                    elif fault=='bytes':reference['bytes']+=1
                    elif fault=='traversal':reference['path']='../release-source.json'
                    elif fault=='duplicate-json':
                        path.write_bytes(original[:-1]+b',"kind":"'+r.KIND.encode()+b'"}');reference.update(sha256=freeze.sha(path),bytes=path.stat().st_size)
                    with patch.object(Path,'is_symlink',return_value=True) if fault=='link' else patch.object(r,'sha1',wraps=r.sha1):
                        with self.assertRaises((ValueError,FileNotFoundError)):r.load_source(root,reference)
                    path.write_bytes(original)
            def change(_):
                (root/'examples/lifecycle-d1/host.py').write_bytes(b'changed while observing API')
                return recorded(source)
            with self.assertRaises(ValueError):r.check(root,ref,SimpleNamespace(snapshot=change))

    def test_budget_source_bridge_is_domain_separated_and_never_resets(self):
        one=r.maintenance_budget_id('11111111-1111-4111-8111-111111111111','a'*64)
        self.assertNotEqual(one,r.maintenance_budget_id('11111111-1111-4111-8111-111111111111','b'*64))
        self.assertNotEqual(one,r.maintenance_budget_id('22222222-2222-4222-8222-222222222222','a'*64))
        for mid,source in [('not-uuid','a'*64),('00000000-0000-0000-0000-000000000000','a'*64),('11111111-1111-4111-8111-111111111111','A'*64)]:
            with self.subTest(mid=mid,source=source),self.assertRaises(ValueError):r.maintenance_budget_id(mid,source)


class MergedCompositionTests(unittest.TestCase):
    volume=composition_tests.CompositionTests.volume
    bind=composition_tests.CompositionTests.bind
    def setUp(self):composition_tests.CompositionTests.setUp(self)

    def fixture(self,folder,fault=None):
        c=composition_tests.c
        repo=folder/'released';source,ref=source_fixture(repo)
        operation=folder/'operation';operation.mkdir()
        entry,ops,old_grants,read,write,key=composition_tests.CompositionTests.fixture(self,operation,fault)
        b=copy.deepcopy(entry.b);h=copy.deepcopy(entry.h);packet=copy.deepcopy(entry.packet)
        b['BudgetID']=r.maintenance_budget_id(b['MaintenanceID'],ref['sha256']);self.bind(b,h)
        ops.b=b;ops.h=h
        def replace(spec,raw):
            path=Path(spec['path']);path.write_bytes(raw);spec.update(sha256=c.host.sha(raw),bytes=len(raw))
        replace(packet['bindings'],h['_bindings_raw']);replace(packet['host'],c.host.canonical({k:v for k,v in h.items() if k!='_bindings_raw'}))
        anchor=copy.deepcopy(entry.anchor);anchor['bindings_sha256']=h['bindings_sha256'];replace(packet['anchor'],c.host.canonical(anchor))
        for item in h['owners'].values():
            (Path(item['output'])/'wait-ready.json').unlink()
            (Path(item['private'])/'permission.json').write_bytes(c.host.canonical(dict(Enabled=False,BindingsSHA256=h['bindings_sha256'])))
        for name in ('review_head','rule_id','protection_sha256'):packet.pop(name)
        packet.update(schema=c.RELEASE_SCHEMA,release_source=ref)
        packet_sha=c.host.sha(c.host.canonical(packet));read.update(BindingsSHA256=h['bindings_sha256'],ManifestSHA256=packet_sha);write['BindingsSHA256']=h['bindings_sha256']
        self.enterContext(patch.object(c,'ROOT',repo))
        client=SimpleNamespace(snapshot=Mock(return_value=recorded(source)))
        self.enterContext(patch.object(r,'Client',return_value=client))
        native=object.__new__(c.NativeOperations);native.control=entry.control
        def branch(value):ops.calls.append(('merged-branch',));return native.branch(value)
        ops.branch=branch
        merged=c.Composition(packet,packet_sha,ops,lambda:self.current)
        def sign(name,body):
            raw=c.host.canonical(body);path=merged.control/name
            c.save(path,dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()));return path
        grants=(sign('new-release-read.json',read),old_grants[1],sign('new-release-write.json',write),old_grants[3])
        return merged,ops,grants,old_grants,source,ref

    def test_complete_V3_real_Git_source_signature_before_two_writers_after_and_seal(self):
        with tempfile.TemporaryDirectory() as tmp:
            entry,ops,grants,*_=self.fixture(Path(tmp));entry.run(*grants)
            self.assertEqual(ops.enabled,['business-races','mixed'])
            result=composition_tests.c.host.load(entry.control/'run/terminal.json')
            self.assertEqual(result['status'],'PASS_MAINTENANCE_BEFORE_WRITE_AFTER_CLOSED')
            self.assertEqual([v[1:] for v in ops.calls if v[0]=='read'],[(phase,group) for phase in ('before','after') for group in composition_tests.c.GROUPS])
            calls=list(ops.calls)
            with self.assertRaises((ValueError,FileExistsError)):entry.run(*grants)
            self.assertEqual(ops.calls,calls)

    def test_new_read_cannot_revive_old_write_and_changed_source_never_rewrites_budget(self):
        c=composition_tests.c
        for fault in ('old-write','changed-source','new-budget','old-budget'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                entry,ops,grants,old,source,ref=self.fixture(Path(tmp),fault if fault in ('new-budget','old-budget') else None)
                original=c.host.canonical(entry.b)
                if fault=='old-write':grants=(grants[0],grants[1],old[2],old[3])
                elif fault=='changed-source':
                    path=Path(c.ROOT)/ref['path'];value=json.loads(path.read_bytes());value['protection_anchor']['head']='f'*40
                    path.write_text(json.dumps(value));entry.packet['release_source']=dict(ref,sha256=freeze.sha(path),bytes=path.stat().st_size)
                with self.assertRaises(ValueError):entry.run(*grants)
                self.assertEqual(c.host.canonical(entry.b),original)
                self.assertFalse(ops.enabled)
                if fault in ('old-write','changed-source'):self.assertFalse(ops.calls)

    def test_rehashed_V3_missing_source_or_budget_binding_is_rejected_before_operations(self):
        c=composition_tests.c
        for fault in ('missing-source','mixed-fields','old-budget'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                entry,ops,*_=self.fixture(Path(tmp));packet=copy.deepcopy(entry.packet)
                if fault=='missing-source':packet.pop('release_source')
                elif fault=='mixed-fields':packet['review_head']='a'*40
                else:
                    b=copy.deepcopy(entry.b);b['BudgetID']='b'*64;h=copy.deepcopy(entry.h);self.bind(b,h)
                    for key,raw in [('bindings',h['_bindings_raw']),('host',c.host.canonical({k:v for k,v in h.items() if k!='_bindings_raw'}))]:
                        path=Path(packet[key]['path']);path.write_bytes(raw);packet[key].update(sha256=c.host.sha(raw),bytes=len(raw))
                with self.assertRaises(ValueError):c.Composition(packet,c.host.sha(c.host.canonical(packet)),ops,lambda:self.current)
                self.assertFalse(ops.calls)


if __name__=='__main__':unittest.main()
