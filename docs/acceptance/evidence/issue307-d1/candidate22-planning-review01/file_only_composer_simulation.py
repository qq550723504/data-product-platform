from pathlib import Path
import ast,contextlib,hashlib,io,json,tempfile,sys
P=Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform')
E=P/'docs/acceptance/evidence/issue307-d1/candidate22-planning-review01'
sys.path.insert(0,str(P/'examples/lifecycle-d1'))
import evidence_indexes as audit
ORIGINAL=P/'docs/acceptance/evidence/issue307-d1/candidate22-planning01/finalize-static-bindings01.py'
EX='0e5cb8f3-46a9-4339-88e4-7154d6ebe13f';R='81a083751c01a281d42df594f6e91012255081f1';HOST='a50b5a6307e79896578669ed53a3e12f08a0a815'
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def write(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    raw=(json.dumps(value,indent=2)+'\n').encode()
    with path.open('xb') as stream:stream.write(raw)
def meta(path):return dict(sha256=sha(path),bytes=path.stat().st_size)
def fixture(root,fault=None):
    base=root/'docs/acceptance/evidence/issue307-d1';here=base/'candidate22-planning01';packet=base/'remaining-candidate22';review=base/'candidate22-artifacts-review01';source=base/'candidate22-source-review02';prep=base/'remaining-candidate22-preparation01';readonly=base/'remaining-candidate22-readonly01'
    for folder in (here,packet,review,source,prep,readonly):folder.mkdir(parents=True)
    leaf=root/'simulated-source.txt';leaf.write_bytes(b'FILE_ONLY_SIMULATION_NOT_NATIVE')
    c=dict(Execution=EX,Profile='candidate22/v32',Source=R,NotBefore='2026-10-23T10:00:00Z',NotAfter='2026-10-23T10:40:00Z')
    write(packet/'candidate.json',c);write(packet/'bindings.json',[]);(packet/'confirmation-template.txt').write_bytes(b'SIMULATED TEMPLATE NOT AUTHORITY\n')
    write(packet/'source-freeze.json',dict(host=dict(source=HOST)))
    write(review/'independent-final-review01.json',dict(status='PASS_BOUNDED_C22_FROZEN_NONDELETE_PREPARATION_REVIEW',remaining_blockers=[],execution=EX,runtime_source=R,host_source='b'*40 if fault else HOST,simulation=True))
    write(source/'independent-final-review01.json',dict(simulation=True,source_head=HOST))
    write(readonly/'complete.json',dict(simulation=True,native_preparation='NOT_RUN'))
    def index(path,member,member_root='.'):
        write(path,dict(status='SIMULATED_TYPED_INDEX',count=1,files={member.relative_to(member_root if isinstance(member_root,Path) else root).as_posix():meta(member)}))
    source_leaf=source/'source.txt';source_leaf.write_bytes(b'SIMULATED_SOURCE_REVIEW')
    prep_leaf=prep/'source.txt';prep_leaf.write_bytes(b'SIMULATED_PREPARATION')
    review_leaf=review/'source.txt';review_leaf.write_bytes(b'SIMULATED_ARTIFACT_REVIEW')
    index(source/'evidence-index.json',source_leaf,source);index(source/'reviewed-artifacts01.json',leaf)
    index(prep/'non-delete-evidence-index.json',prep_leaf,prep)
    index(review/'evidence-index.json',review_leaf,review);index(review/'reviewed-artifacts01.json',leaf)
    index(here/'candidate22-host-bindings01.json',leaf)
    binding=packet/'source-review-binding-index.json'
    write(binding,dict(status='SIMULATED_SOURCE_REVIEW_REPORT_BINDING',count=2,files={path.relative_to(root).as_posix():meta(path) for path in [source/'independent-final-review01.json',source/'evidence-index.json']}))
    specs=[]
    for identity,path,member_root in [('source-review',source/'evidence-index.json',source.relative_to(root).as_posix()),('nondelete-preparation',prep/'non-delete-evidence-index.json',prep.relative_to(root).as_posix()),('source-review-report-binding',binding,'.')]:
        d=json.loads(path.read_bytes());specs.append(dict(id=identity,index=path.relative_to(root).as_posix(),sha256=sha(path),format=audit.FILES_MAP,member_root=member_root,expected_count=d['count'],expected_status=d['status']))
    write(packet/'preparation-evidence-audit-plan.json',dict(version=1,execution=EX,candidate_sha256=sha(packet/'candidate.json'),indexes=specs))
    for name in ('final-cold-inventory01.json','retained-backups75-readback01.json','authority-schedule-absence01.json'):write(here/name,dict(simulation=True))
    docs=root/'docs/acceptance'
    for name in ('issue307-d1-candidate22-confirmation.md','issue307-d1-v32-window-runbook.md','issue307-d1-v32-preparation-contract03.md'):(docs/name).write_bytes(b'SIMULATION NOT ISSUED DOCUMENT\n')
    return dict(ROOT=root,BASE=base,HERE=here,PACKET=packet,REVIEW=review)
def composer(env,optimize):
    tree=ast.parse(ORIGINAL.read_bytes())
    tree.body=[x for x in tree.body if isinstance(x,(ast.Import,ast.ImportFrom,ast.FunctionDef))]
    scope=dict(env);exec(compile(tree,'AST_EXTRACTED_FILE_ONLY_COMPOSER','exec',optimize=optimize),scope)
    with contextlib.redirect_stdout(io.StringIO()):scope['main']()
results=[]
scratch=E/'file-only-scratch';scratch.mkdir(exist_ok=False)
for optimize in (0,2):
    with tempfile.TemporaryDirectory(dir=scratch) as tmp:
        env=fixture(Path(tmp));composer(env,optimize)
        plan=env['HERE']/'final-static-evidence-audit-plan01.json';result=audit.audit_plan(env['ROOT'],plan,sha(plan),env['PACKET']/'candidate.json')
        manifest=env['HERE']/'approval-reference-manifest01.json';document=json.loads(manifest.read_bytes())
        names={x['path'] for x in document['approved_reference_artifacts']}
        if manifest.relative_to(env['ROOT']).as_posix() in names:raise ValueError('Manifest self reference')
        if any('authorized-execution' in x or 'schedule' in x and 'absence' not in x for x in names):raise ValueError('Future authority/schedule reference')
        if document['execution_authority'] is not False or document['authorization']!='NOT_RECEIVED' or document['schedule']!='NOT_CREATED':raise ValueError('Simulated authority created')
        if not any('candidate22-source-review02/' in x for x in names) or any('candidate22-source-review01/' in x for x in names):raise ValueError('Wrong source review selected')
        results.append(dict(optimize=optimize,status='PASS_FILE_ONLY_TYPED_PLAN_AND_MANIFEST_SIMULATION',audit_indexes=len(result['indexes']),members_verified=result['members_verified'],manifest_members=document['count'],native='NOT_RUN',allocation='NOT_RUN'))
    with tempfile.TemporaryDirectory(dir=scratch) as tmp:
        env=fixture(Path(tmp),fault='foreign-host')
        try:composer(env,optimize)
        except ValueError as error:
            if str(error)!='independent packet review incomplete':raise
        else:raise ValueError('Foreign host accepted')
        if (env['HERE']/'final-static-reference-index01.json').exists():raise ValueError('Output before review gate')
        results.append(dict(optimize=optimize,status='PASS_FOREIGN_HOST_REJECTED_BEFORE_COMPOSER_OUTPUT',native='NOT_RUN'))
with (E/'file-only-composer-simulation.json').open('xb') as out:out.write((json.dumps(dict(status='PASS_SIMULATED_FILE_ONLY_NOT_NATIVE_NOT_ISSUED',runs=results,actual_phase_entry_executions=0,actual_Docker_provider_io=0),indent=2)+'\n').encode())
print(json.dumps(results))
