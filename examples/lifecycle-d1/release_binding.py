"""Merged immutable P/R provenance plus the existing independent write fence.

No binding/authority allocation, Docker/provider call or mutation is exposed.
The legacy Draft/open gate is deliberately not relaxed or given fake snapshots.
"""
from pathlib import Path
import re
import uuid
from urllib.parse import quote
import evidence_indexes as indexes
import execution_freeze as freeze
import fixture_artifacts as artifacts

KIND = 'D1_MERGED_RELEASE_SOURCE_V1'
EXECUTION_KIND = 'FROZEN_MERGED_RELEASE_EXECUTION_V1'
CI_KIND = 'MERGED_RELEASE_SOURCE_CI_V1'
WORKFLOWS = ('.github/workflows/ci.yml', '.github/workflows/fork-approved-source-admission.yml')
CI_JOBS = {'classify-changes', 'temporary-container-contracts (windows-latest)',
           'temporary-container-contracts (ubuntu-latest)', 'go-platform', 'web-console',
           'hop-smoke', 'gx-smoke', 'splink-smoke', 'label-studio-smoke',
           'demo / demo-lifecycle', 'browser-contracts / web-browser-contracts',
           'live-core / live-core-browser', 'required'}
need = freeze.need


def sha1(value):
    need(type(value) is str and re.fullmatch('[0-9a-f]{40}', value), 'release exact Git SHA required')


def reference(root, ref):
    need(type(ref) is dict and set(ref) == {'path', 'sha256', 'bytes'}, 'release reference fields')
    indexes.metadata({k:ref[k] for k in ('sha256', 'bytes')})
    need(type(ref['path']) is str, 'release reference path')
    path = (indexes.unlinked_absolute(ref['path']) if Path(ref['path']).is_absolute()
            else indexes.bound_path(root, ref['path']))
    artifacts.regular(path)
    raw = path.read_bytes()
    need(len(raw) == ref['bytes'] and artifacts.digest(raw) == ref['sha256'], 'release reference byte drift')
    return path, raw


def load_source(root, ref):
    _, raw = reference(root, ref)
    source = indexes.decode(raw)
    need(set(source) == {'kind', 'repository', 'pull_request', 'review_head', 'merge_sha',
                         'tree_sha', 'base_sha', 'host', 'runtime', 'protection_anchor'}, 'release source fields')
    need(source['kind'] == KIND and source['repository'] == freeze.REPOSITORY,
         'explicit same-repository merged release required')
    need(type(source['pull_request']) is int and source['pull_request'] > 0, 'release PR identity')
    for key in ('review_head', 'merge_sha', 'tree_sha', 'base_sha'): sha1(source[key])
    need(type(source['host']) is dict and set(source['host']) == {'source', 'files'}
         and source['host']['source'] == source['merge_sha'], 'published execution host must be the merge commit')
    need(type(source['runtime']) is dict and set(source['runtime']) == {'spec_sha256', 'archive'}, 'release R fields')
    indexes.valid_digest(source['runtime']['spec_sha256'])
    anchor = source['protection_anchor']
    need(type(anchor) is dict and set(anchor) == {'branch', 'head', 'rule', 'protection'}
         and anchor['branch'] == freeze.BRANCH, 'independent protection anchor fields')
    sha1(anchor['head']); protection_ready(anchor, anchor)
    return source


def protection_ready(current, anchor):
    need(current['branch'] == anchor['branch'] == freeze.BRANCH
         and current['head'] == anchor['head'] and current['rule'] == anchor['rule']
         and current['protection'] == anchor['protection'], 'release independent protection drift')
    freeze.frozen_rule(current['rule'], freeze.BRANCH)
    for name, value in (('lock_branch', True), ('enforce_admins', True),
                        ('allow_fork_syncing', False), ('allow_force_pushes', False), ('allow_deletions', False)):
        need(current['protection'].get(name, {}).get('enabled') is value, 'release native protection flags')


def local_ready(root, source):
    """Before API/history/build/Docker: unpublished bytes cannot borrow old CI."""
    need(artifacts.git(root, 'rev-parse', 'HEAD').decode().strip() == source['merge_sha'],
         'release checkout is not the approved published merge')
    artifacts.verify_host(root, source['host'])
    for commit in (source['merge_sha'], source['review_head']):
        need(artifacts.git(root, 'rev-parse', commit+'^{tree}').decode().strip() == source['tree_sha'],
             'release local P/merge tree drift')
    spec_path = Path(root)/artifacts.SPEC
    need(artifacts.digest(spec_path.read_bytes()) == source['runtime']['spec_sha256'], 'release R spec drift')
    spec = artifacts.load(spec_path)
    spec = artifacts.specification(root, execution=spec['execution'], profile=spec['profile'],
                                   start=spec['not_before'], end=spec['not_after'])
    archive, _ = reference(root, source['runtime']['archive'])
    need(archive.name == 'approved-runtime-source.zip', 'release exact R archive name')
    artifacts.verify_archive(root, archive.parent, spec)
    return spec


def origin_ready(snapshot, source):
    """Pure provenance check; its result is explicitly not execution authority."""
    need(source['kind']==KIND and source['host']['source']==source['merge_sha'], 'release origin kind/host differs')
    need(snapshot['repository'] == source['repository'] == freeze.REPOSITORY, 'release repository drift')
    pr = snapshot['pr']
    need(pr['number'] == source['pull_request'] and pr['state'] == 'closed' and pr['merged'] is True
         and pr['draft'] is False and pr['head_repository'] == freeze.REPOSITORY
         and pr['head'] == source['review_head'] and pr['merge'] == source['merge_sha']
         and pr['base_branch'] == 'main', 'release actual merged PR differs')
    need(snapshot['review_threads_complete'] is True and snapshot['unresolved_threads'] == 0,
         'release unresolved or incomplete review threads')
    need(snapshot['review_commit'] == {'sha':source['review_head'], 'tree':source['tree_sha']}
         and snapshot['merge_commit'] == {'sha':source['merge_sha'], 'tree':source['tree_sha'], 'parents':[source['base_sha']]},
         'release actual squash P/tree/parent differs')
    need(snapshot['issue_state'] == 'open', 'Issue307 must remain open')
    protection_ready(snapshot['protection_anchor'], source['protection_anchor'])
    need(len(snapshot['workflows']) == len(WORKFLOWS)
         and [w['path'] for w in snapshot['workflows']] == list(WORKFLOWS), 'release required workflow set differs')
    for workflow in snapshot['workflows']:
        need(workflow['event'] == 'push' and workflow['branch'] == 'main'
             and workflow['repository'] == workflow['head_repository'] == freeze.REPOSITORY
             and workflow['head_sha'] == source['merge_sha'] and workflow['status'] == 'completed'
             and workflow['conclusion'] == 'success', 'release exact push/main CI is not successful')
        jobs, checks = workflow['jobs'], workflow['checks']
        names = CI_JOBS if workflow['path'] == WORKFLOWS[0] else {'synthetic-source-admission'}
        need(len(jobs) == len(names) and {j['name'] for j in jobs} == names
             and len(checks) == len(jobs) and len({j['check_run_id'] for j in jobs}) == len(jobs)
             and {j['check_run_id'] for j in jobs} == {c['id'] for c in checks}, 'release complete job/check membership')
        for job in jobs:
            need(job['head_sha'] == source['merge_sha'] and job['run_id'] == workflow['id']
                 and job['run_attempt'] == workflow['attempt'] and job['status'] == 'completed'
                 and job['conclusion'] == 'success', 'release job attempt/status differs')
            check = next(c for c in checks if c['id'] == job['check_run_id'])
            need(check['head_sha'] == source['merge_sha'] and check['name'] == job['name']
                 and check['status'] == 'completed' and check['conclusion'] == 'success'
                 and check['suite_id'] == workflow['suite_id'] and check['app_id'] == freeze.ACTIONS_APP_ID
                 and check['app_slug'] == 'github-actions', 'release check provenance differs')
    return {'status':'PASS_MERGED_RELEASE_ORIGIN_NOT_EXECUTION_AUTHORITY', 'merge_sha':source['merge_sha']}


class Client:
    """Reuse the existing host-only gh transport; only GET and GraphQL reads."""
    def __init__(self, journal): self.api = freeze.GitHub(journal)

    def snapshot(self, source):
        api = self.api; prefix = 'repos/'+freeze.REPOSITORY
        branch = api.request('GET', prefix+'/branches/'+quote(freeze.BRANCH, safe=''))
        policy = api.request('GET', prefix+'/branches/'+quote(freeze.BRANCH, safe='')+'/protection')
        query = ('query($ref:String!){repository(owner:"qq550723504",name:"data-product-platform")'
                 '{nameWithOwner ref(qualifiedName:$ref){target{oid} branchProtectionRule{'+freeze.RULE_FIELDS+'}}}}')
        repo = api.graph(query, {'ref':'refs/heads/'+freeze.BRANCH})['repository']
        need(repo['nameWithOwner'] == freeze.REPOSITORY and repo['ref']
             and repo['ref']['target']['oid'] == branch['commit']['sha'], 'release protection changed during read')
        pr = api.request('GET', prefix+'/pulls/'+str(source['pull_request']))
        issue = api.request('GET', prefix+'/issues/307')
        review = api.request('GET', prefix+'/git/commits/'+source['review_head'])
        merged = api.request('GET', prefix+'/git/commits/'+source['merge_sha'])
        query = ('query($number:Int!,$cursor:String){repository(owner:"qq550723504",name:"data-product-platform")'
                 '{pullRequest(number:$number){reviewThreads(first:100,after:$cursor)'
                 '{nodes{isResolved}pageInfo{hasNextPage endCursor}}}}}')
        rows=[]; cursor=None
        while True:
            page=api.graph(query, {'number':source['pull_request'], 'cursor':cursor})['repository']['pullRequest']['reviewThreads']
            rows += page['nodes']
            if not page['pageInfo']['hasNextPage']:break
            nxt=page['pageInfo']['endCursor'];need(nxt and nxt != cursor,'release review pagination');cursor=nxt
        result = api.request('GET', prefix+'/actions/runs?head_sha='+source['merge_sha']+'&event=push&branch=main&per_page=100')
        need(result['total_count'] == len(result['workflow_runs']), 'release CI run pagination')
        checks = api.request('GET', prefix+'/commits/'+source['merge_sha']+'/check-runs?per_page=100')
        need(checks['total_count'] == len(checks['check_runs']), 'release check pagination')
        workflows=[]
        for path in WORKFLOWS:
            matches=[v for v in result['workflow_runs'] if v['path'] == path]
            need(matches,'release required main workflow missing')
            run=max(matches,key=lambda v:v['id'])  # No fallback to an older green run/attempt.
            result_jobs=api.request('GET',prefix+'/actions/runs/'+str(run['id'])+'/attempts/'+str(run['run_attempt'])+'/jobs?per_page=100')
            need(result_jobs['total_count'] == len(result_jobs['jobs']), 'release job pagination')
            jobs=[]
            for job in result_jobs['jobs']:
                match=re.fullmatch('https://api.github.com/repos/'+re.escape(freeze.REPOSITORY)+r'/check-runs/([0-9]+)',job['check_run_url'])
                need(match is not None,'release foreign check URL')
                jobs.append(dict(name=job['name'],check_run_id=int(match[1]),head_sha=job['head_sha'],run_id=job['run_id'],
                                 run_attempt=job['run_attempt'],status=job['status'],conclusion=job['conclusion']))
            suite=[dict(id=v['id'],name=v['name'],head_sha=v['head_sha'],status=v['status'],conclusion=v['conclusion'],
                        suite_id=v['check_suite']['id'],app_id=v['app']['id'],app_slug=v['app']['slug'])
                   for v in checks['check_runs'] if v['check_suite']['id'] == run['check_suite_id']]
            workflows.append(dict(id=run['id'],attempt=run['run_attempt'],path=run['path'],event=run['event'],branch=run['head_branch'],
                repository=run['repository']['full_name'],head_repository=run['head_repository']['full_name'],head_sha=run['head_sha'],
                status=run['status'],conclusion=run['conclusion'],suite_id=run['check_suite_id'],jobs=jobs,checks=suite))
        return dict(repository=repo['nameWithOwner'],pr=dict(number=pr['number'],state=pr['state'],merged=pr['merged'],draft=pr['draft'],
            head_repository=pr['head']['repo']['full_name'],head=pr['head']['sha'],merge=pr['merge_commit_sha'],base_branch=pr['base']['ref']),
            review_commit=dict(sha=review['sha'],tree=review['tree']['sha']),merge_commit=dict(sha=merged['sha'],tree=merged['tree']['sha'],parents=[v['sha'] for v in merged['parents']]),
            issue_state=issue['state'],review_threads_complete=True,unresolved_threads=sum(not v['isResolved'] for v in rows),
            protection_anchor=dict(branch=freeze.BRANCH,head=branch['commit']['sha'],rule=repo['ref']['branchProtectionRule'],protection=policy),workflows=workflows)


def check(root, ref, client):
    source=load_source(root,ref);local_ready(root,source)
    snapshot=client.snapshot(source);origin_ready(snapshot,source)
    need(load_source(root,ref)==source,'release source changed during API observation')
    local_ready(root,source)
    return {'status':'PASS_FRESH_MERGED_RELEASE_SOURCE_GATE','release_source_sha256':ref['sha256'],
            'merge_sha':source['merge_sha'],'source':source,'snapshot':snapshot}


def maintenance_budget_id(maintenance_id, source_sha):
    need(type(maintenance_id) is str and str(uuid.UUID(maintenance_id))==maintenance_id
         and uuid.UUID(maintenance_id).int != 0,'release maintenance UUID')
    indexes.valid_digest(source_sha)
    return artifacts.digest(('D1_MERGED_RELEASE_MAINTENANCE_BUDGET_V1\0'+maintenance_id+'\0'+source_sha).encode())


def saved_build_ci(root, allocation, record, snapshot, snapshot_sha, execution):
    ci=allocation['source_ci']
    need(set(ci)=={'kind','head','run','receipt','sha256'},'release saved CI fields differ')
    need(ci['kind']==CI_KIND and record['status']=='PASS_MERGED_RELEASE_SOURCE_CI_BEFORE_ALLOCATION'
         and record['head']==ci['head']==allocation['host']['source'] and record['run']==ci['run']
         and record['execution']==execution and record['snapshot_sha256']==snapshot_sha,
         'release saved CI/build binding differs')
    source=load_source(root,record['release_source'])
    need(source['host']==allocation['host'] and source['merge_sha']==record['head']
         and source['runtime']['spec_sha256']==allocation['runtime_artifact_spec_sha256'],
         'release saved P/R artifact chain differs')
    need(artifacts.git(root,'rev-parse','HEAD').decode().strip()==source['merge_sha'], 'release saved host checkout differs')
    origin_ready(snapshot,source)
    need(snapshot['workflows'][0]['id']==ci['run'],'release saved main CI run differs')
    spec=artifacts.load(Path(root)/artifacts.SPEC)
    need(allocation['source']==spec['source'],'release saved R differs')


def verify_execution(root, private, packet, candidate, binding, auth, client):
    need(not freeze.control_fact_present(private,'terminal.json'), 'execution is terminal')
    need(set(binding)=={'kind','status','execution','profile','artifact_bindings','release_source','public_binding_path'}
         and binding['kind']==EXECUTION_KIND and binding['status']=='FROZEN_MERGED_RELEASE_EXECUTION_BINDING_NOT_AUTHORIZED'
         and binding['execution']==candidate['Execution'] and binding['profile']==candidate['Profile']
         and binding['artifact_bindings']==freeze.artifact_bindings(packet,candidate), 'release execution scope/artifacts differ')
    need(auth is not None and auth.get('old_authorization_reused') is False and auth['execution']==candidate['Execution']
         and auth.get('release_source_sha256')==binding['release_source']['sha256'], 'new release execution authority missing')
    source=load_source(root,binding['release_source'])
    need(auth.get('release_merge_sha')==source['merge_sha'] and auth.get('final_approved_head')==source['merge_sha'], 'release authority source differs')
    for relative, digest in ((binding['public_binding_path'],auth['execution_binding_sha256']),
                             (binding['release_source']['path'],binding['release_source']['sha256'])):
        path=indexes.bound_path(root,relative)
        need(artifacts.digest(path.read_bytes())==digest and any(v['path']==relative and v['sha256']==digest for v in auth['approved_reference_artifacts']),
             'release binding/source absent from current approved references')
    need((Path(root)/binding['public_binding_path']).read_bytes()==(Path(private)/'execution-binding.json').read_bytes(), 'release public/private binding differs')
    frozen=freeze.load(Path(packet)/'source-freeze.json')
    need(freeze.sha(Path(packet)/'source-freeze.json')==candidate['SourceFreezeSHA256']
         and frozen['host']==source['host'] and frozen['runtime_artifact_spec_sha256']==source['runtime']['spec_sha256']
         and frozen['source_ci'].get('kind')==CI_KIND, 'release frozen P/R source chain differs')
    spec=artifacts.load(Path(root)/artifacts.SPEC)
    need(candidate['Source']==frozen['source']==spec['source']
         and candidate['Execution']==spec['execution'] and candidate['Profile']==spec['profile']
         and candidate['NotBefore']==spec['not_before'] and candidate['NotAfter']==spec['not_after'],
         'release retained R scope/window differs')
    receipt=indexes.bound_path(root,frozen['source_ci']['receipt'])
    artifacts.regular(receipt)
    need(freeze.sha(receipt)==frozen['source_ci']['sha256']
         and freeze.load(receipt)['release_source']==binding['release_source'],'release allocation/source reference differs')
    captured=receipt.with_name('snapshot.json');artifacts.regular(captured)
    saved_build_ci(root,frozen,freeze.load(receipt),freeze.load(captured),freeze.sha(captured),candidate['Execution'])
    result=check(root,binding['release_source'],client)
    return {'status':'PASS_FRESH_MERGED_RELEASE_EXECUTION_BINDING','execution':candidate['Execution'],
            'merge_sha':result['merge_sha'],'release_source_sha256':binding['release_source']['sha256']}
