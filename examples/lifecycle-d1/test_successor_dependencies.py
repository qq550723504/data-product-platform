"""Real temporary evidence + full successor CLI; Docker/WSL are recorded adapters."""
import contextlib
import copy
import importlib.util
import io
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import evidence_indexes as audit
import retired_resources as r
import retired_volumes as volumes
import retirement_catalog as catalog
import successor_dependencies as dependencies
from test_retired_resources import item
from test_retired_budget_bytes import archive

SOURCES = [format(n, '064x') for n in range(10, 17)]
RETIRED, CUSTODIAN, NETNS, OTHER = [format(n, '064x') for n in range(20, 24)]


class ReadOnlyDocker:
    def __init__(self, rows, metadata):
        self.rows, self.metadata = copy.deepcopy(rows), copy.deepcopy(metadata)
        self.calls = []
        self.failure = None
        self.inventory_bytes = None

    def inventory(self):
        if self.failure:
            raise self.failure
        return set(self.rows)

    def inspect(self, ids):
        return {cid: copy.deepcopy(self.rows[cid]) for cid in ids}

    def raw(self, *args):
        self.calls.append(args)
        if args != ('volume', 'ls', '-q'):
            raise AssertionError('unexpected Docker operation: ' + str(args))
        return self.inventory_bytes if self.inventory_bytes is not None else ('\n'.join(self.metadata)+'\n').encode()

    def json(self, *args):
        self.calls.append(args)
        if args[:2] != ('volume', 'inspect'):
            raise AssertionError('unexpected Docker operation: ' + str(args))
        return [copy.deepcopy(self.metadata[name]) for name in args[2:]]


class SuccessorCompositionTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.repo = self.root/'repo'
        self.repo.mkdir()
        self.archive = self.root/'history'
        self.archive.mkdir()
        self.files = {}

        def save(name, value):
            path = self.archive/name
            path.parent.mkdir(parents=True, exist_ok=True)
            r.save(path, value)
            return path
        self.save = save

        def spec(path):
            raw = path.read_bytes()
            return dict(path=path.relative_to(self.archive).as_posix(), sha256=r.sha(raw), bytes=len(raw))
        self.spec = spec
        rows = {cid: item(cid) for cid in SOURCES}
        rows.update({CUSTODIAN: item(CUSTODIAN, 'container:'+NETNS), NETNS: item(NETNS), OTHER: item(OTHER, running=True)})
        for row in rows.values():
            row['HostConfig']['Dns'] = None
        self.metadata = {}
        for name in ('used-budget', 'source-data', 'old-empty'):
            self.metadata[name] = dict(Name=name, Driver='local', Scope='local', Options=None,
                Labels={'lifecycle307.remaining': 'old-execution', 'lifecycle307.allocation': 'old-nonce'}, CreatedAt='old')
        rows[CUSTODIAN]['Mounts'] = [dict(Type='volume', Name='used-budget', Destination='/run/budget')]
        rows[SOURCES[0]]['Mounts'] = [dict(Type='volume', Name='source-data', Destination='/data')]
        self.frozen = copy.deepcopy(rows)
        old = item(RETIRED)
        old['Mounts'] = [dict(Type='volume', Name='used-budget', Destination='/run/budget'),
                         dict(Type='volume', Name='old-empty', Destination='/old')]
        self.save('old/inspect.json', old)
        (self.archive/'old/stdout').write_bytes(b'original failure')
        (self.archive/'old/stderr').write_bytes(b'UNKNOWN')
        self.original = self.root/'pins-and-failed-attempt.txt'
        self.original.write_bytes(b'used attempt; UNKNOWN; pins retained')
        self.save('originals.json', dict(files={str(self.original): dict(sha256=r.sha(self.original.read_bytes()), bytes=self.original.stat().st_size)}))
        self.save('legacy.json', dict(status='original failure unchanged'))
        (self.archive/'budget.tar').write_bytes(archive('budget'))
        protected = lambda cid: dict(descriptor=r.descriptor(rows[cid]), state=r.state(rows[cid]))
        self.plan = dict(schema=r.SCHEMA, provider_delete=0, volume_delete=0,
            authority=dict(source_thread='human', human_message_id='bounded-maintenance', human_text='original authority, not successor authority'),
            retired_historical=[RETIRED], live_required=[*SOURCES, NETNS], external_held=[],
            live={cid: protected(cid) for cid in [*SOURCES, NETNS]}, external={}, additional_live={},
            running_protected={OTHER: protected(OTHER)}, order=[RETIRED],
            archives={RETIRED: dict(inspect=spec(self.archive/'old/inspect.json'), stdout=spec(self.archive/'old/stdout'),
                stderr=spec(self.archive/'old/stderr'), descriptor=r.descriptor(old), state=r.state(old))},
            originals=spec(self.archive/'originals.json'), legacy_gate=spec(self.archive/'legacy.json'),
            budgets=[dict(volume='used-budget', wire=RETIRED, archive=spec(self.archive/'budget.tar'))],
            volumes={name: dict(metadata=value, mounted_before=users) for name, value, users in [
                ('used-budget', self.metadata['used-budget'], [RETIRED, CUSTODIAN]),
                ('source-data', self.metadata['source-data'], [SOURCES[0]]),
                ('old-empty', self.metadata['old-empty'], [RETIRED])]})
        self.save('plan.json', self.plan)
        self.plan_sha = r.sha((self.archive/'plan.json').read_bytes())
        self.save('content.json', dict(volume='old-empty', reader=RETIRED, exit_code=0, source_start=0, provider_calls=0,
                                      files=[dict(name='.', type='5', size=0)]))
        self.vplan = dict(schema=volumes.SCHEMA, parent_plan_sha256=self.plan_sha, provider_delete=0,
            authority=dict(human_message_id='human'), candidate_volumes=['old-empty', 'used-budget'],
            remove={'old-empty': dict(reason='EMPTY_NO_FILE_DATA', metadata=self.metadata['old-empty'],
                                     content={**spec(self.archive/'content.json'), 'path':str(self.archive/'content.json')})}, preserve={'used-budget': dict(reason='BUDGET')})
        self.save('volume-plan.json', self.vplan)
        self.vsha = r.sha((self.archive/'volume-plan.json').read_bytes())
        self.save('removals/'+RETIRED+'/intent.json', dict(cid=RETIRED, plan_sha256=self.plan_sha, operation='docker rm'))
        self.save('removals/'+RETIRED+'/result.json', dict(cid=RETIRED, plan_sha256=self.plan_sha,
            status='REMOVED_EXACT_CID', exit_code=0, stdout=RETIRED+'\n', confirmed_absent=True))
        vf='volume-removals/'+r.sha(b'old-empty')
        self.save(vf+'/intent.json', dict(volume='old-empty', plan_sha256=self.vsha, operation='docker volume rm'))
        self.save(vf+'/result.json', dict(volume='old-empty', plan_sha256=self.vsha,
            status='REMOVED_EXACT_TEST_VOLUME', exit_code=0, stdout='old-empty\n', confirmed_absent=True))

        def index(name, status, paths):
            files={p.relative_to(self.archive).as_posix(): {k:v for k,v in spec(p).items() if k!='path'} for p in paths}
            self.save(name, dict(status=status, count=len(files), files=files))
            return r.sha((self.archive/name).read_bytes())
        receipt_sha = index('removal-index.json', 'EXACT_RETIREMENT_RECEIPTS', (self.archive/'removals').rglob('*.json'))
        volume_receipt_sha = index('volume-removal-index.json', 'EXACT_TEST_VOLUME_RETIREMENT_RECEIPTS', (self.archive/'volume-removals').rglob('*.json'))
        placeholder = self.save('no-pg-in-this-small-fixture.json', dict(physical_pg_copies=0))
        pg_sha = index('pg-copy-archive-index.json', 'EXACT_RETIRED_PG_COPY_ARCHIVES', [placeholder])
        self.save('final-containers.json', rows)
        packet = self.save('source-packet.json', dict(Groups=[dict(Group=group, Core=SOURCES[n*2], Native=SOURCES[n*2+1])
                    for n, group in enumerate(('business-races', 'late-hold', 'mixed'))]))
        precondition = self.repo/'approved-precondition.json'
        r.save(precondition, dict(status='historical precondition'))
        binding_shas = {}
        for kind in ('containers', 'volumes'):
            source = self.archive/('source-'+kind)
            source.mkdir()
            (source/'executed.py').write_bytes(b'# actual old execution source, not successor source')
            self.save('binding-'+kind+'.json', dict(status='FROZEN_MAINTENANCE_ONLY', plan_sha256=self.plan_sha,
                volume_plan_sha256=self.vsha, provider_delete_authorized=False, volumes_authorized=kind=='volumes',
                source_files={'examples/executed.py': r.sha((source/'executed.py').read_bytes())},
                precondition_files={'approved-precondition.json': r.sha(precondition.read_bytes())}))
            binding_shas[kind] = r.sha((self.archive/('binding-'+kind+'.json')).read_bytes())
        constants = dict(ARCHIVE=self.archive, PLAN_SHA256=self.plan_sha, VOLUME_PLAN_SHA256=self.vsha,
            RECEIPT_INDEX_SHA256=receipt_sha, VOLUME_RECEIPT_INDEX_SHA256=volume_receipt_sha,
            PG_COPY_INDEX_SHA256=pg_sha, FINAL_INSPECT_SHA256=r.sha((self.archive/'final-containers.json').read_bytes()),
            SOURCE_PACKET=packet, SOURCE_PACKET_SHA256=r.sha(packet.read_bytes()), BUSINESS_REDIS=SOURCES[6],
            EXECUTED_BINDINGS=binding_shas, __file__=str(self.repo/'examples/lifecycle-d1/retirement_catalog.py'))
        patches = patch.multiple(catalog, **constants)
        patches.start(); self.addCleanup(patches.stop)
        self.docker = ReadOnlyDocker(rows, {k:v for k,v in self.metadata.items() if k!='old-empty'})
        self.wsl = SimpleNamespace(returncode=0, stdout=archive('.'), stderr=b'')
        p=patch.object(r, 'Docker', return_value=self.docker);p.start();self.addCleanup(p.stop)
        p=patch('retired_budget_bytes.subprocess.run', side_effect=self.wsl_run);p.start();self.addCleanup(p.stop)
        entry_path = Path(dependencies.__file__).with_name('verify-successor-history.py')
        spec_entry=importlib.util.spec_from_file_location('successor_history_cli_under_test', entry_path)
        self.entry=importlib.util.module_from_spec(spec_entry);spec_entry.loader.exec_module(self.entry)
        self.entry.ROOT=self.repo
        self.out_number=0

    def wsl_run(self, args, **kwargs):
        self.assertEqual(args[:6], ['wsl', '-d', 'docker-desktop', '--', 'tar', '-cpf'])
        self.assertIn('-C', args)
        return self.wsl

    def full(self):
        self.out_number += 1
        output=self.repo/('evidence'+str(self.out_number))
        with contextlib.redirect_stdout(io.StringIO()):
            return self.entry.main(['--output', str(output)])

    def test_complete_cli_success_preserves_used_budget_pins_and_original_failure(self):
        before=self.original.read_bytes()
        with patch.object(catalog, 'verify_maintenance_snapshot', side_effect=AssertionError('maintenance snapshot is not a successor dependency')):
            result=self.full()
        self.assertEqual(result['status'], 'PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES')
        self.assertEqual((result['source_roles'],result['required_owners'],result['current_physical_budgets']), (7,9,1))
        self.assertEqual(self.original.read_bytes(), before)
        self.assertTrue(all(call[0]=='volume' and call[1] in ('inspect','ls') for call in self.docker.calls))

    def direct_contract(self, dns=None):
        frozen=copy.deepcopy(self.frozen);plan=copy.deepcopy(self.plan)
        for cid,row in frozen.items():
            row['HostConfig']['Dns']=copy.deepcopy(dns)
            if cid in plan['live']:plan['live'][cid]['descriptor']=r.descriptor(row)
        roles={group+':'+role:SOURCES[n*2+offset] for n,group in enumerate(('business-races','late-hold','mixed'))
               for offset,role in enumerate(('Core','Native'))}
        roles['business-races:Redis']=SOURCES[6]
        return dependencies.build(plan,self.vplan,frozen,roles),frozen

    def test_empty_dns_representations_are_bidirectional_and_raw_history_is_unchanged(self):
        originals={path:path.read_bytes() for path in (self.archive/'plan.json',self.archive/'final-containers.json',self.original)}
        for frozen_dns in (None,[]):
            contract,frozen=self.direct_contract(frozen_dns)
            self.assertEqual(contract['schema'],'d1-successor-cold-dependencies-v2')
            self.assertEqual(contract['descriptor_comparison'],dependencies.DESCRIPTOR_COMPARISON)
            for current_dns in (None,[]):
                rows=copy.deepcopy(frozen)
                for cid in contract['required']:rows[cid]['HostConfig']['Dns']=copy.deepcopy(current_dns)
                before=copy.deepcopy(rows)
                docker=ReadOnlyDocker(rows,self.docker.metadata)
                with self.subTest(frozen=frozen_dns,current=current_dns):
                    result=dependencies.verify(contract,docker)
                    self.assertEqual(result['status'],'PASS_CURRENT_SUCCESSOR_COLD_DEPENDENCIES')
                    self.assertEqual(rows,before)
                    self.assertEqual(contract['required'][CUSTODIAN]['descriptor'],r.descriptor(frozen[CUSTODIAN]))
                    if frozen_dns!=current_dns:self.assertNotEqual(r.descriptor(rows[CUSTODIAN]),r.descriptor(frozen[CUSTODIAN]))
        for path,raw in originals.items():self.assertEqual(path.read_bytes(),raw)

    def test_complete_cli_dns_alias_success_is_stable_and_keeps_strict_old_maintenance(self):
        baseline=self.full();before=copy.deepcopy(self.docker.rows)
        for cid in (*SOURCES,CUSTODIAN,NETNS):self.docker.rows[cid]['HostConfig']['Dns']=[]
        self.assertEqual(self.full(),baseline)
        with self.assertRaisesRegex(ValueError,'changed'):catalog.verify_maintenance_snapshot()
        for cid,row in before.items():row['HostConfig']['Dns']=self.docker.rows[cid]['HostConfig']['Dns']
        self.assertEqual(self.docker.rows,before)

    def test_nonempty_dns_is_strict_and_never_participates_in_empty_equivalence(self):
        contract,frozen=self.direct_contract(['192.0.2.53'])
        self.assertIsNone(contract['required'][CUSTODIAN]['empty_dns_descriptor'])
        self.assertEqual(dependencies.verify(contract,ReadOnlyDocker(frozen,self.docker.metadata))['status'],'PASS_CURRENT_SUCCESSOR_COLD_DEPENDENCIES')
        for changed in (None,[],['192.0.2.54']):
            rows=copy.deepcopy(frozen);rows[CUSTODIAN]['HostConfig']['Dns']=changed
            with self.subTest(changed=changed),self.assertRaises(ValueError):dependencies.verify(contract,ReadOnlyDocker(rows,self.docker.metadata))

    def test_dns_missing_wrong_type_or_new_resolvers_reject_in_complete_cli(self):
        for dns in ('missing','',{},False,0,(),[None],[False],['192.0.2.53'],['']):
            old=copy.deepcopy(self.docker.rows[CUSTODIAN])
            if dns=='missing':self.docker.rows[CUSTODIAN]['HostConfig'].pop('Dns')
            else:self.docker.rows[CUSTODIAN]['HostConfig']['Dns']=dns
            with self.subTest(dns=dns),self.assertRaises(ValueError):self.full()
            self.docker.rows[CUSTODIAN]=old

    def test_empty_dns_does_not_hide_any_other_configuration_or_liveness_drift(self):
        changes=[('HostConfig','DnsOptions',['ndots:1']),('HostConfig','DnsSearch',['example.invalid']),
                 ('HostConfig','Privileged',True),('HostConfig','CapAdd',['SYS_ADMIN']),
                 ('HostConfig','NetworkMode','host'),('Config','Cmd',['other']),('Config','Env',['EXTRA=1']),
                 ('Config','User','0:0'),('State','Running',True),('State','Pid',99),
                 (None,'Image','sha256:other'),(None,'Id','f'*64),(None,'Name','/other'),
                 (None,'Mounts',[dict(Type='bind',Source='/extra',Destination='/root')])]
        for section,key,value in changes:
            old=copy.deepcopy(self.docker.rows[CUSTODIAN]);row=self.docker.rows[CUSTODIAN]
            row['HostConfig']['Dns']=[]
            (row if section is None else row[section])[key]=value
            with self.subTest(section=section,key=key),self.assertRaises(ValueError):self.full()
            self.docker.rows[CUSTODIAN]=old

    def test_algorithm_hash_and_original_raw_hash_are_both_authoritative(self):
        for fault,current_dns in ((fault,dns) for fault in ('old-schema','missing-tag','bad-tag','missing-hash','bad-hash','forged-hash')
                                  for dns in (None,[])):
            contract,frozen=self.direct_contract();rows=copy.deepcopy(frozen)
            rows[CUSTODIAN]['HostConfig']['Dns']=current_dns
            if fault=='old-schema':contract['schema']='d1-successor-cold-dependencies-v1'
            elif fault=='missing-tag':contract.pop('descriptor_comparison')
            elif fault=='bad-tag':contract['descriptor_comparison']='IGNORE_ALL_EMPTY_FIELDS'
            elif fault=='missing-hash':contract['required'][CUSTODIAN].pop('empty_dns_descriptor')
            elif fault=='bad-hash':contract['required'][CUSTODIAN]['empty_dns_descriptor']='0'*64
            else:
                rows[CUSTODIAN]['HostConfig']['Privileged']=True
                contract['required'][CUSTODIAN]['empty_dns_descriptor']=dependencies.empty_dns_descriptors(rows[CUSTODIAN])[0]
            with self.subTest(fault=fault,current_dns=current_dns),self.assertRaises((ValueError,KeyError)):
                dependencies.verify(contract,ReadOnlyDocker(rows,self.docker.metadata))

    def test_budget_and_volume_checks_still_reject_with_empty_dns_alias(self):
        for fault in ('bytes','owner','mount','missing'):
            old_rows=copy.deepcopy(self.docker.rows);old_meta=copy.deepcopy(self.docker.metadata)
            self.docker.rows[CUSTODIAN]['HostConfig']['Dns']=[]
            if fault=='bytes':self.wsl.stdout=archive('.',b'new attempt')
            elif fault=='owner':self.docker.metadata['used-budget']['Labels']['lifecycle307.remaining']='other'
            elif fault=='mount':self.docker.rows[OTHER]['Mounts']=[dict(Type='volume',Name='used-budget',Destination='/extra')]
            else:self.docker.metadata.pop('used-budget')
            with self.subTest(fault=fault),self.assertRaises(ValueError):self.full()
            self.docker.rows=old_rows;self.docker.metadata=old_meta;self.wsl.stdout=archive('.')

    def test_unrelated_stop_restart_new_session_and_broken_foreign_netns_do_not_change_contract(self):
        original=self.full()
        self.docker.rows[OTHER]['State'].update(Running=False,Pid=0,Status='exited',StartedAt='new-start',FinishedAt='new-end')
        self.assertEqual(self.full(), original)
        self.docker.rows.pop(OTHER)
        foreign='f'*64
        self.docker.rows[foreign]=item(foreign, 'container:foreign-unavailable', running=True)
        self.assertEqual(self.full(), original)

    def test_required_closed_epoch_changes_accepted_but_maintenance_comparison_stays_strict(self):
        baseline=self.full()
        self.docker.rows[SOURCES[0]]['State'].update(StartedAt='new-start',FinishedAt='new-end',ExitCode=137)
        self.assertEqual(self.full(), baseline)
        with self.assertRaisesRegex(ValueError, 'restarted or changed'):
            volumes.present(self.plan, {'old-empty'}, self.docker)

    def test_original_maintenance_entry_stays_strict_while_successor_ignores_foreign_state(self):
        self.assertEqual(catalog.verify_maintenance_snapshot()['status'], 'PASS_RETIRED_CONTAINER_AND_TEST_VOLUME_HISTORY')
        self.docker.rows[OTHER]['State'].update(Running=False,Pid=0,Status='exited',FinishedAt='new-end')
        with self.assertRaisesRegex(ValueError, 'other session runtime changed'):
            catalog.verify_maintenance_snapshot()
        self.assertEqual(self.full()['status'],'PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES')

    def test_missing_source_custodian_and_forward_parent_reject(self):
        for cid in (SOURCES[0],CUSTODIAN,NETNS):
            old=self.docker.rows.pop(cid)
            with self.subTest(cid=cid),self.assertRaisesRegex(ValueError,'dependency missing'):self.full()
            self.docker.rows[cid]=old

    def test_required_running_descriptor_privilege_and_netns_drift_reject(self):
        for fault in ('running','image','privilege','netns'):
            old=copy.deepcopy(self.docker.rows[CUSTODIAN])
            if fault=='running':self.docker.rows[CUSTODIAN]['State'].update(Running=True,Pid=400,Status='running')
            elif fault=='image':self.docker.rows[CUSTODIAN]['Image']='different'
            elif fault=='privilege':self.docker.rows[CUSTODIAN]['HostConfig']['Privileged']=True
            else:self.docker.rows[CUSTODIAN]['HostConfig']['NetworkMode']='none'
            with self.subTest(fault=fault),self.assertRaisesRegex(ValueError,'descriptor/topology'):self.full()
            self.docker.rows[CUSTODIAN]=old

    def test_retained_volume_missing_owner_and_full_mount_expansion_reject_even_with_unrelated_restart(self):
        for fault in ('missing','owner','mount'):
            old=copy.deepcopy(self.docker.metadata)
            if fault=='missing':self.docker.metadata.pop('used-budget')
            elif fault=='owner':self.docker.metadata['used-budget']['Labels']['lifecycle307.remaining']='other-execution'
            else:self.docker.rows[OTHER]['Mounts']=[dict(Type='volume',Name='used-budget',Destination='/elsewhere')]
            self.docker.rows[OTHER]['State']['StartedAt']='legitimate-new-start'
            with self.subTest(fault=fault),self.assertRaises(ValueError):self.full()
            self.docker.metadata=old;self.docker.rows[OTHER]['Mounts']=[]

    def test_budget_reset_changed_content_type_and_read_failure_reject(self):
        for raw,code in ((archive('.',b''),0),(archive('.',b'new'),0),('str',0),(archive('.'),1)):
            self.wsl.stdout=raw;self.wsl.returncode=code
            with self.subTest(code=code,type=type(raw).__name__),self.assertRaises(ValueError):self.full()

    def test_old_budget_tar_and_pins_cannot_be_rewritten(self):
        for path in (self.archive/'budget.tar',self.original):
            old=path.read_bytes();path.write_bytes(old+b'changed')
            with self.subTest(path=path.name),self.assertRaises(ValueError):self.full()
            path.write_bytes(old)

    def test_retired_cid_or_volume_reappearing_reject(self):
        self.docker.rows[RETIRED]=item(RETIRED)
        with self.assertRaisesRegex(ValueError,'retired CID'):self.full()
        self.docker.rows.pop(RETIRED)
        self.docker.metadata['old-empty']=self.metadata['old-empty']
        with self.assertRaisesRegex(ValueError,'retired volume'):self.full()

    def test_missing_exact_receipt_and_changed_executed_source_reject(self):
        path=self.archive/'removals'/RETIRED/'result.json';old=path.read_bytes();path.unlink()
        with self.assertRaises(FileNotFoundError):self.full()
        path.write_bytes(old)
        (self.archive/'source-containers/executed.py').write_bytes(b'new successor source cannot replace history')
        with self.assertRaises(ValueError):self.full()

    def test_docker_read_error_or_str_inventory_is_never_success(self):
        self.docker.failure=TimeoutError('read failed')
        with self.assertRaises(TimeoutError):self.full()
        self.docker.failure=None;self.docker.inventory_bytes='str'
        with self.assertRaisesRegex(ValueError,'must be bytes'):self.full()
        failure=r.read(self.repo/('evidence'+str(self.out_number))/'failure.json')
        self.assertEqual(failure['status'],'FAILED_SUCCESSOR_READONLY_CHECK_NO_MUTATION')

    def test_required_source_omission_and_bad_fixed_snapshot_reject(self):
        roles={key:cid for key,cid in zip(sorted(dependencies.SOURCE_KEYS),SOURCES)}
        roles.pop(next(iter(roles)))
        with self.assertRaises(ValueError):dependencies.build(self.plan,self.vplan,self.frozen,roles)
        path=self.archive/'final-containers.json';path.write_bytes(path.read_bytes()+b' ')
        with self.assertRaises(ValueError):self.full()

    def test_output_is_exclusive_and_no_old_allocation_schema_replay(self):
        result=self.full()
        self.assertEqual(result['schema'],'retained-history-with-scoped-successor-dependencies-v2')
        self.assertNotIn('live_required', result)
        with self.assertRaises(FileExistsError):self.entry.main(['--output',str(self.repo/'evidence1')])


class PreparationHistoryIntegrationTests(unittest.TestCase):
    """Exercise the actual prepare hook without importing allocation/startup code."""
    def hook(self):
        import ast
        source = Path(__file__).with_name('prepare-six-candidate.py')
        tree = ast.parse(source.read_text(encoding='utf-8'))
        hooks = [node for node in tree.body if isinstance(node, ast.FunctionDef)
                 and node.name == 'historical_gate']
        self.assertEqual(len(hooks), 1, 'a later legacy definition must not shadow the successor gate')
        namespace = {}
        module = ast.Module(body=hooks, type_ignores=[])
        exec(compile(module, str(source), 'exec'), namespace)
        return namespace['historical_gate']

    def test_preparation_uses_complete_successor_result(self):
        expected = dict(schema='retained-history-with-scoped-successor-dependencies-v2',
                        status='PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES')
        with patch.object(catalog, 'verify_history', return_value=expected) as current:
            self.assertIs(self.hook()(), expected)
        current.assert_called_once_with()

    def test_preparation_preserves_dependency_failure_without_legacy_fallback(self):
        failure = ValueError('current physical budget drift')
        with patch.object(catalog, 'verify_history', side_effect=failure) as current:
            with self.assertRaises(ValueError) as result:
                self.hook()()
        self.assertIs(result.exception, failure)
        current.assert_called_once_with()


if __name__=='__main__':unittest.main()
