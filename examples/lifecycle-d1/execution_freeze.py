"""GitHub-native execution branch freeze; host-only, no provider/worker secrets.

GitHub supplies the write fence. This module compares before/after facts; it
does not claim that GraphQL offers atomic expected-HEAD/policy CAS.
"""
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import subprocess
import stat
from urllib.parse import quote

REPOSITORY = 'qq550723504/data-product-platform'
BRANCH = 'codex/issue307-post-v22-dispatch-closure'
PULL_REQUEST = 332
ACTIONS_APP_ID = 15368  # github.com GitHub Actions; also verify its canonical slug.
FLAGS = dict(lockBranch=True, isAdminEnforced=True,
             lockAllowsFetchAndMerge=False, allowsForcePushes=False,
             allowsDeletions=False)
REST_FLAGS = {'lock_branch', 'enforce_admins', 'allow_fork_syncing',
              'allow_force_pushes', 'allow_deletions'}
RULE_FIELDS = 'id pattern lockBranch isAdminEnforced lockAllowsFetchAndMerge allowsForcePushes allowsDeletions'


def need(ok, message):
    if not ok:
        raise ValueError(message)


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def sha(path):
    return digest(Path(path).read_bytes())


def load(path):
    path = Path(path)
    need(not path.is_symlink() and not path.is_junction(), 'freeze receipt is aliased')
    return json.loads(path.read_bytes())


def control_fact_present(private, name):
    """Unreadable or aliased control facts never mean absent authority/terminal."""
    private = Path(private)
    target = private/'authorized-execution01'/name
    for path in (private, target.parent, target):
        need(not path.is_symlink() and not path.is_junction(), 'control fact is aliased')
        try:
            info = path.stat()
        except FileNotFoundError:
            continue
        if path == target:
            need(stat.S_ISREG(info.st_mode), 'control fact is not a regular file')
            return True
    return False


def save(path, value):
    with Path(path).open('xb') as stream:
        stream.write((json.dumps(value, ensure_ascii=False, indent=2)+'\n').encode())


def policy_without_freeze(policy):
    return {k: copy.deepcopy(v) for k, v in policy.items() if k not in REST_FLAGS}


def frozen_rule(rule, branch):
    need(isinstance(rule, dict) and rule.get('id') and rule.get('pattern') == branch,
         'missing, wildcard or foreign branch protection rule')
    need(all(rule.get(k) is v for k, v in FLAGS.items()), 'branch freeze flags are not enforced')


def ready(snapshot, head, *, require_frozen=False):
    need(re.fullmatch('[0-9a-f]{40}', head) is not None, 'exact commit SHA required')
    need(snapshot['repository'] == REPOSITORY and snapshot['branch'] == snapshot['expected_branch']
         and snapshot['head'] == head, 'repository/branch/HEAD drift')
    need(snapshot['pr'] == PULL_REQUEST and snapshot['draft'] is True
         and snapshot['state'] == 'open' and snapshot['merged'] is False
         and snapshot['issue_state'] == 'open', 'PR must stay Draft/open and Issue307 open')
    ci=snapshot['ci']
    need(ci['path']=='.github/workflows/ci.yml' and ci['event']=='pull_request'
         and ci['repository']==ci['head_repository']==REPOSITORY
         and ci['head_sha']==head and PULL_REQUEST in ci['pull_requests']
         and ci['status']=='completed' and ci['conclusion']=='success',
         'required CI must be the exact repository/HEAD/PR pull_request workflow')
    checks = snapshot['checks']; jobs=ci['jobs']
    need(jobs and len({x['check_run_id'] for x in jobs})==len(jobs)
         and {x['check_run_id'] for x in jobs}=={x['id'] for x in checks}
         and all(x['run_id']==ci['id'] and x['run_attempt']==ci['attempt']
                 and x['head_sha']==head and x['status']=='completed'
                 and x['conclusion']=='success' for x in jobs),
         'necessary workflow jobs/check-run membership is incomplete or failed')
    need(checks and len({(x['name'], x['app_id']) for x in checks}) == len(checks)
         and any(x['name'] == 'required' for x in checks), 'required CI missing or ambiguous')
    need(all(x['head_sha'] == head and x['status'] == 'completed'
             and x['conclusion'] == 'success' and x['app_id']==ACTIONS_APP_ID
             and x['app_slug']=='github-actions' and x['suite_id']==ci['suite_id']
             and any(j['check_run_id']==x['id'] and j['name']==x['name'] for j in jobs)
             for x in checks), 'exact HEAD GitHub Actions CI provenance differs')
    if require_frozen:
        frozen_rule(snapshot['rule'], snapshot['branch'])
        policy = snapshot['protection']
        need(isinstance(policy, dict), 'complete branch protection unavailable')
        for key, value in (('lock_branch', True), ('enforce_admins', True),
                           ('allow_fork_syncing', False), ('allow_force_pushes', False),
                           ('allow_deletions', False)):
            need(policy.get(key, {}).get('enabled') is value, 'REST freeze enforcement differs')


class GitHub:
    """Existing gh CLI authentication stays on the host, never in a worker."""
    def __init__(self, journal, branch=BRANCH, runner=subprocess.run):
        need(branch == BRANCH or re.fullmatch(r'codex/issue307-freeze-rehearsal-[a-f0-9]{12}', branch),
             'only the fixed PR branch or isolated rehearsal is supported')
        self.journal = Path(journal)
        self.branch = branch
        self.runner = runner
        self.sequence = 0

    def request(self, method, endpoint, body=None, *, absent_protection=False):
        self.sequence += 1
        number = f'{self.sequence:03d}'
        args = ['gh', 'api', '--hostname', 'github.com', '--method', method, endpoint]
        if body is not None:
            args += ['--input', '-']
        save(self.journal/(number+'-intent.json'),
             dict(started_at=now(), method=method, endpoint=endpoint, body=body))
        try:
            result = self.runner(args, input=None if body is None else json.dumps(body).encode(),
                                 capture_output=True, timeout=40)
        except Exception as error:
            save(self.journal/(number+'-transport-failure.json'),
                 dict(observed_at=now(), error_type=type(error).__name__,
                      outcome='UNKNOWN_NO_AUTOMATIC_RETRY_OR_UNLOCK'))
            raise
        for suffix, raw in (('stdout.json', result.stdout), ('stderr.txt', result.stderr)):
            with (self.journal/(number+'-'+suffix)).open('xb') as stream:
                stream.write(raw)
        save(self.journal/(number+'-process.json'),
             dict(observed_at=now(), returncode=result.returncode))
        value = json.loads(result.stdout)
        if result.returncode != 0:
            need(absent_protection and value.get('status') == '404'
                 and value.get('message') == 'Branch not protected',
                 'GitHub request failed; unknown protection is not absence')
            return None
        need(not value.get('errors'), 'GraphQL errors; no automatic retry or unlock')
        return value

    def graph(self, query, variables=None):
        return self.request('POST', 'graphql',
                            {'query': query, 'variables': variables or {}})['data']

    def snapshot(self):
        prefix = 'repos/'+REPOSITORY
        branch = self.request('GET', prefix+'/branches/'+quote(self.branch, safe=''))
        protection = self.request('GET', prefix+'/branches/'+quote(self.branch, safe='')+'/protection',
                                  absent_protection=True)
        q = ('query($ref:String!){repository(owner:"qq550723504",name:"data-product-platform"){'
             'id nameWithOwner ref(qualifiedName:$ref){target{oid} branchProtectionRule{'+RULE_FIELDS+'}}}}')
        repo = self.graph(q, {'ref': 'refs/heads/'+self.branch})['repository']
        need(repo['nameWithOwner'] == REPOSITORY and repo['ref']
             and repo['ref']['target']['oid'] == branch['commit']['sha'], 'branch changed during observation')
        rule = repo['ref']['branchProtectionRule']
        need((protection is None) == (rule is None)
             and (protection is not None or branch['protected'] is False),
             'effective rule/protection absence is ambiguous')
        pr = self.request('GET', prefix+'/pulls/332')
        issue = self.request('GET', prefix+'/issues/307')
        need(pr['head']['repo']['full_name'] == REPOSITORY
             and pr['head']['ref'] == BRANCH and pr['base']['repo']['full_name'] == REPOSITORY,
             'foreign PR repository or branch')
        head = branch['commit']['sha']
        if self.branch == BRANCH:
            need(pr['head']['sha'] == head, 'PR/ref HEAD differs')
        response = self.request('GET', prefix+'/commits/'+head+'/check-runs?per_page=100')
        need(response['total_count'] == len(response['check_runs']), 'CI check pagination incomplete')
        runs=self.request('GET',prefix+'/actions/runs?head_sha='+head+'&event=pull_request&per_page=100')
        need(runs['total_count']==len(runs['workflow_runs']), 'CI run pagination incomplete')
        matching=[x for x in runs['workflow_runs'] if x['path']=='.github/workflows/ci.yml']
        need(matching,'No exact CI workflow run; a check named required is insufficient')
        run=max(matching,key=lambda x:x['id'])  # Never fall back to an earlier green run.
        jobs=self.request('GET',prefix+'/actions/runs/'+str(run['id'])+'/attempts/'+str(run['run_attempt'])+'/jobs?per_page=100')
        need(jobs['total_count']==len(jobs['jobs']), 'CI job pagination incomplete')
        job_rows=[]
        for x in jobs['jobs']:
            match=re.fullmatch('https://api.github.com/repos/'+re.escape(REPOSITORY)+r'/check-runs/([0-9]+)',x['check_run_url'])
            need(match is not None,'Foreign job/check-run URL')
            job_rows.append(dict(check_run_id=int(match[1]),name=x['name'],run_id=x['run_id'],
                                 run_attempt=x['run_attempt'],head_sha=x['head_sha'],
                                 status=x['status'],conclusion=x['conclusion']))
        checks = [dict(id=x['id'], name=x['name'], app_id=x['app']['id'],
                       app_slug=x['app']['slug'],suite_id=x['check_suite']['id'],
                       head_sha=x['head_sha'], status=x['status'], conclusion=x['conclusion'])
                  for x in response['check_runs'] if x['check_suite']['id']==run['check_suite_id']]
        ci=dict(id=run['id'],attempt=run['run_attempt'],path=run['path'],event=run['event'],
                repository=run['repository']['full_name'],head_repository=run['head_repository']['full_name'],
                head_sha=run['head_sha'],pull_requests=[x['number'] for x in run['pull_requests']],
                status=run['status'],conclusion=run['conclusion'],suite_id=run['check_suite_id'],jobs=job_rows)
        return dict(repository=repo['nameWithOwner'], repository_id=repo['id'],
                    branch=self.branch, expected_branch=self.branch, head=head,
                    pr=PULL_REQUEST, draft=pr['draft'], state=pr['state'],
                    merged=pr['merged'], issue_state=issue['state'], rule=rule,
                    protection=protection, checks=checks,ci=ci)

    def lock(self, snapshot):
        rule = snapshot['rule']
        if rule is None:
            kind = 'createBranchProtectionRule'
            values = dict(repositoryId=snapshot['repository_id'], pattern=self.branch, **FLAGS)
            type_name = 'CreateBranchProtectionRuleInput!'
        else:
            need(rule['pattern'] == self.branch, 'shared/wildcard rule must not be changed or shadowed')
            need(rule['lockBranch'] is False, 'already locked; do not adopt another freeze')
            kind = 'updateBranchProtectionRule'
            values = dict(branchProtectionRuleId=rule['id'], **FLAGS)
            type_name = 'UpdateBranchProtectionRuleInput!'
        query = ('mutation($input:'+type_name+'){'+kind
                 +'(input:$input){branchProtectionRule{'+RULE_FIELDS+'}}}')
        return self.graph(query, {'input': values})[kind]['branchProtectionRule']


def acquire(client, head, output):
    """Lock then reread; failed/unknown outcomes deliberately keep any lock."""
    output = Path(output)
    need(not output.exists(), 'one-shot freeze output already exists')
    output.mkdir(parents=True)
    client.journal = output
    before = client.snapshot()
    need(before['branch'] == client.branch, 'client branch scope differs')
    ready(before, head)
    rule = before['rule']
    need(rule is None or (rule['pattern'] == before['branch'] and rule['lockBranch'] is False),
         'existing lock or shared rule cannot be adopted')
    save(output/'before.json', before)
    save(output/'lock-intent.json', dict(at=now(), expected_head=head,
                                      no_atomic_CAS_claim=True, automatic_unlock=False))
    changed = client.lock(before)
    after = client.snapshot()
    ready(after, head, require_frozen=True)
    need(after['rule']['id'] == changed['id'], 'effective lock rule differs')
    if rule is not None:
        need(after['rule']['id'] == rule['id']
             and policy_without_freeze(after['protection']) == policy_without_freeze(before['protection']),
             'existing non-freeze protection changed; leave lock in place')
    save(output/'after.json', after)
    receipt = dict(status='FROZEN_EXECUTION_BRANCH_NOT_AUTHORIZED', at=now(),
                   repository=REPOSITORY, pr=PULL_REQUEST, branch=client.branch,
                   review_head=head, rule_id=after['rule']['id'],
                   protection=after['protection'], before=before, after=after,
                   native_lock=True, atomic_CAS=False, automatic_unlock=False)
    save(output/'branch-lock.json', receipt)
    return receipt


def artifact_bindings(packet, candidate):
    return dict(candidate_sha256=sha(packet/'candidate.json'),
                bindings_sha256=sha(packet/'bindings.json'),
                template_sha256=sha(packet/'confirmation-template.txt'),
                source_freeze_sha256=sha(packet/'source-freeze.json'),
                runtime_source=candidate['Source'],
                reader=candidate['ReaderBinarySHA256'],
                sender=candidate['SenderBinarySHA256'], owner=candidate['OwnerBinarySHA256'])


def verify_binding(root, private, packet, candidate, expected_sha, client, *, auth=None):
    private, packet, root = Path(private), Path(packet), Path(root)
    need(not control_fact_present(private,'terminal.json'), 'execution is terminal')
    path = private/'execution-binding.json'
    need(re.fullmatch('[0-9a-f]{64}', expected_sha or '') is not None
         and sha(path) == expected_sha, 'exact new execution binding absent/different')
    binding = load(path)
    # An explicit new kind, never a fallback from a failed legacy PR gate.
    if binding.get('kind') == 'FROZEN_MERGED_RELEASE_EXECUTION_V1':
        import release_binding
        need(auth is not None and auth.get('execution_binding_sha256') == expected_sha,
             'actual new merged-release binding authority differs')
        return release_binding.verify_execution(root, private, packet, candidate, binding, auth, client)
    need('kind' not in binding, 'unknown execution binding kind')
    need(binding['status'] == 'FROZEN_EXECUTION_BINDING_NOT_AUTHORIZED'
         and binding['execution'] == candidate['Execution']
         and binding['profile'] == candidate['Profile']
         and binding['artifact_bindings'] == artifact_bindings(packet, candidate)
         and binding['branch_lock']['branch'] == BRANCH, 'foreign execution/artifact/branch binding')
    if auth is not None:
        need(auth.get('execution_binding_sha256') == expected_sha
             and auth['execution'] == candidate['Execution']
             and auth['final_approved_head'] == binding['branch_lock']['review_head'],
             'human authorization does not bind this branch freeze')
        relative = binding['public_binding_path']
        need((root/relative).resolve().is_relative_to(root.resolve())
             and (root/relative).read_bytes() == path.read_bytes()
             and any(x['path'] == relative and x['sha256'] == expected_sha
                     for x in auth['approved_reference_artifacts']), 'binding absent from approved references')
    head = subprocess.check_output(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'], cwd=root).decode().strip()
    need(head == binding['branch_lock']['review_head']
         and not subprocess.check_output(['git', '-c', 'core.longpaths=true', 'diff', '--name-only', 'HEAD'], cwd=root).strip(),
         'local execution checkout HEAD/tracked bytes drift')
    current = client.snapshot()
    need(current['branch'] == BRANCH, 'current branch scope differs')
    ready(current, head, require_frozen=True)
    locked = binding['branch_lock']
    need(current['rule']['id'] == locked['rule_id']
         and current['protection'] == locked['protection'], 'frozen rule/full protection changed')
    return dict(status='PASS_FRESH_EXECUTION_BRANCH_FREEZE', at=now(),
                execution=candidate['Execution'], review_head=head,
                rule_id=current['rule']['id'], binding_sha256=expected_sha)


def authorized_gate(root, private, packet, candidate, *, expected_sha=None, journal=None):
    """Shared fixed-entry/Host/attester guard, before Docker or signing."""
    root, private = Path(root), Path(private)
    match = re.fullmatch(r'candidate[0-9]+/v([0-9]+)', candidate['Profile'])
    need(match is not None, 'unsupported candidate profile')
    auth_path = root/'docs/acceptance/evidence/issue307-d1'/('delete-confirmed-v'+match[1]+'-execution01')/'human-authorization.json'
    auth = load(auth_path)
    expected_sha = expected_sha or auth.get('execution_binding_sha256')
    need(expected_sha == auth.get('execution_binding_sha256'), 'CLI and actual authorization freeze differ')
    journal = Path(journal or (private/'branch-checks'/dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%f')))
    journal.mkdir(parents=True, exist_ok=False)
    binding = load(private/'execution-binding.json')
    if binding.get('kind') == 'FROZEN_MERGED_RELEASE_EXECUTION_V1':
        import release_binding
        client = release_binding.Client(journal)
    else:
        client = GitHub(journal)
    return verify_binding(root, private, packet, candidate, expected_sha, client, auth=auth)

