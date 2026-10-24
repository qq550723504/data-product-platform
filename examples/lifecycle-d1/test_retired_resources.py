"""Behavioral maintenance/absence tests. No real Docker or provider calls."""
import copy
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import retired_resources as r

A, B, LIVE, EXTERNAL, RUNNING = (str(x) * 64 for x in range(1, 6))


def item(cid, target='none', running=False):
    return dict(Id=cid, Name='/test-' + cid[:8], Image='sha256:fixed',
                Config=dict(Env=[], Cmd=['worker'], User='10001:10001'),
                HostConfig=dict(NetworkMode=target, RestartPolicy=dict(Name='no'),
                                AutoRemove=False, PortBindings={}),
                NetworkSettings=dict(Ports={}), Mounts=[],
                State=dict(Status='running' if running else 'exited', Running=running,
                           Pid=123 if running else 0, StartedAt='fixed-start', FinishedAt='fixed-end',
                           ExitCode=0, OOMKilled=False, Dead=False, Paused=False, Restarting=False))


class RecordedDocker:
    def __init__(self):
        self.rows = {A: item(A, 'container:' + B), B: item(B), LIVE: item(LIVE),
                     EXTERNAL: item(EXTERNAL, 'container:' + LIVE), RUNNING: item(RUNNING, running=True)}
        self.volume = dict(Name='old-used-budget', Driver='local', Scope='local',
                           Options=None, Labels={'execution': 'old-never-reset'}, CreatedAt='old')
        self.rows[A]['Mounts'] = [dict(Type='volume', Name=self.volume['Name'])]
        self.calls = []
        self.fault = None

    def inventory(self):
        if self.fault:
            raise self.fault
        return set(self.rows)

    def inspect(self, ids):
        return {cid: copy.deepcopy(self.rows[cid]) for cid in ids}

    def json(self, *args):
        assert args == ('volume', 'inspect', self.volume['Name'])
        return [copy.deepcopy(self.volume)]

    def raw(self, *args):
        self.calls.append(args)
        assert args[0] == 'rm' and len(args) == 2
        del self.rows[args[1]]
        return (args[1] + '\n').encode()


class RetirementTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.receipts = self.root / 'removals'
        self.receipts.mkdir()
        self.docker = RecordedDocker()
        self.original = self.root / 'original-failure-and-pins.txt'
        self.original.write_bytes(b'UNKNOWN\nused budget retained\npins never released\n')
        self.source = self.root / 'entry.py'
        self.source.write_bytes(b'# exact reviewed source\n')
        def spec(path):
            raw = path.read_bytes()
            return dict(path=path.relative_to(self.root).as_posix(), sha256=r.sha(raw), bytes=len(raw))
        self.spec = spec
        archives = {}
        for cid in (A, B):
            folder = self.root / cid
            folder.mkdir()
            r.save(folder / 'inspect.json', self.docker.rows[cid])
            r.write(folder / 'stdout', b'original stdout\n')
            r.write(folder / 'stderr', b'original stderr\n')
            archives[cid] = dict(inspect=spec(folder / 'inspect.json'), stdout=spec(folder / 'stdout'),
                                 stderr=spec(folder / 'stderr'), descriptor=r.descriptor(self.docker.rows[cid]),
                                 state=r.state(self.docker.rows[cid]))
        r.write(self.root / 'budget.tar', b'actual used-budget original bytes')
        r.save(self.root / 'originals.json', dict(files={str(self.original): dict(
            sha256=r.sha(self.original.read_bytes()), bytes=self.original.stat().st_size)}))
        r.save(self.root / 'legacy.json', dict(status='original failed facts unchanged'))
        protected = lambda cid: dict(descriptor=r.descriptor(self.docker.rows[cid]), state=r.state(self.docker.rows[cid]))
        self.plan = dict(schema=r.SCHEMA, provider_delete=0, volume_delete=0,
            authority=dict(source_thread='trusted', human_message_id='human', human_text='maintain old Docker only'),
            retired_historical=[A, B], live_required=[LIVE], external_held=[EXTERNAL],
            archives=archives, live={LIVE: protected(LIVE)}, external={EXTERNAL: protected(EXTERNAL)},
            running_protected={RUNNING: protected(RUNNING)}, order=[A, B],
            originals=spec(self.root / 'originals.json'), legacy_gate=spec(self.root / 'legacy.json'),
            volumes={self.docker.volume['Name']: dict(metadata=self.docker.volume, mounted_before=[A])},
            budgets=[dict(archive=spec(self.root / 'budget.tar'))])
        self.repin()

    def repin(self):
        self.path = self.root / 'plan.json'
        self.path.write_bytes(r.encoded(self.plan))
        self.digest = r.sha(self.path.read_bytes())
        self.binding = self.root / 'binding.json'
        self.binding.write_bytes(r.encoded(dict(status='FROZEN_MAINTENANCE_ONLY',
            plan_sha256=self.digest, provider_delete_authorized=False, volumes_authorized=False,
            source_files={'entry.py': r.sha(self.source.read_bytes())})))
        self.binding_sha = r.sha(self.binding.read_bytes())

    def verify(self, complete=False):
        return r.verify(self.path, self.digest, self.root, self.receipts, self.docker, complete=complete)

    def remove(self):
        return r.remove(self.path, self.digest, self.root, self.receipts, self.docker,
                        self.binding, self.binding_sha, self.root)

    def test_full_success_preserves_live_other_runtime_used_budget_and_failure(self):
        before = self.original.read_bytes()
        self.assertEqual(self.verify()['status'], 'PASS_PREPARED_RETIREMENT')
        result = self.remove()
        self.assertEqual(result['status'], 'PASS_RETIRED_HISTORY')
        self.assertEqual(self.docker.calls, [('rm', A), ('rm', B)])
        self.assertEqual(self.docker.inventory(), {LIVE, EXTERNAL, RUNNING})
        self.assertEqual(self.original.read_bytes(), before)
        self.assertEqual((self.root / 'budget.tar').read_bytes(), b'actual used-budget original bytes')
        self.assertEqual(self.verify(True)['removed'], 2)

    def test_unknown_missing_cid_is_not_retirement(self):
        del self.docker.rows[A]
        with self.assertRaisesRegex(ValueError, 'no fixed removal receipt'):
            self.verify()
        self.assertFalse(self.docker.calls)

    def test_docker_error_is_not_absence(self):
        self.docker.fault = subprocess.TimeoutExpired('docker', 1)
        with self.assertRaises(subprocess.TimeoutExpired):
            self.verify()

    def test_archive_state_is_not_current_closure(self):
        self.docker.rows[A]['State']['Running'] = True
        self.docker.rows[A]['State']['Pid'] = 777
        with self.assertRaisesRegex(ValueError, 'restarted'):
            self.remove()
        self.assertFalse(self.docker.calls)

    def test_new_external_netns_dependent_blocks_before_rm(self):
        self.docker.rows['a' * 64] = item('a' * 64, 'container:' + A)
        with self.assertRaisesRegex(ValueError, 'external netns'):
            self.remove()
        self.assertFalse(self.docker.calls)

    def test_new_volume_user_or_owner_change_blocks(self):
        for change in ('mount', 'owner'):
            with self.subTest(change=change):
                docker = RecordedDocker()
                if change == 'mount':
                    foreign = 'c' * 64
                    docker.rows[foreign] = item(foreign)
                    docker.rows[foreign]['Mounts'] = [dict(Type='volume', Name=docker.volume['Name'])]
                else:
                    docker.volume['Labels']['execution'] = 'reset-identity'
                with self.assertRaisesRegex(ValueError, 'volume'):
                    r.present_contract(self.plan, docker, removed=set())

    def test_originals_logs_and_budget_bytes_are_immutable(self):
        for path in (self.original, self.root / A / 'stderr', self.root / 'budget.tar'):
            raw = path.read_bytes()
            path.write_bytes(raw + b'changed')
            with self.subTest(path=path.name), self.assertRaises(ValueError):
                self.verify()
            path.write_bytes(raw)

    def test_missing_or_forged_archive_identity_blocks(self):
        path = self.root / A / 'inspect.json'
        archived = r.read(path)
        archived['Id'] = B
        path.write_bytes(r.encoded(archived))
        self.plan['archives'][A]['inspect'] = self.spec(path)
        self.repin()
        with self.assertRaisesRegex(ValueError, 'archive identity'):
            self.remove()
        self.assertFalse(self.docker.calls)

    def test_required_live_missing_or_other_session_pid_change_blocks(self):
        for fault in ('missing-live', 'running-pid', 'external-topology'):
            docker = RecordedDocker()
            if fault == 'missing-live':
                del docker.rows[LIVE]
            elif fault == 'running-pid':
                docker.rows[RUNNING]['State']['Pid'] = 999
            else:
                docker.rows[EXTERNAL]['Config']['Cmd'] = ['different']
            with self.subTest(fault=fault), self.assertRaises(ValueError):
                r.present_contract(self.plan, docker, removed=set())

    def test_root_cannot_be_removed_before_leaf(self):
        self.plan['order'] = [B, A]
        self.repin()
        with self.assertRaisesRegex(ValueError, 'root must follow'):
            self.remove()
        self.assertFalse(self.docker.calls)

    def test_missing_outcome_after_intent_is_unconfirmed_and_not_resent(self):
        folder = self.receipts / A
        folder.mkdir()
        r.save(folder / 'intent.json', dict(cid=A, plan_sha256=self.digest, operation='docker rm'))
        del self.docker.rows[A]
        with self.assertRaises(FileNotFoundError):
            self.remove()
        self.assertFalse(self.docker.calls)

    def test_forged_absence_receipt_does_not_hide_present_cid(self):
        folder = self.receipts / A
        folder.mkdir()
        r.save(folder / 'intent.json', dict(cid=A, plan_sha256=self.digest, operation='docker rm'))
        r.save(folder / 'result.json', dict(cid=A, plan_sha256=self.digest, status='REMOVED_EXACT_CID',
                                          exit_code=0, stdout=A + '\n', confirmed_absent=True))
        with self.assertRaisesRegex(ValueError, 'still present'):
            self.verify()

    def test_source_and_plan_drift_reject_before_mutation(self):
        for path in (self.source, self.path):
            raw = path.read_bytes()
            path.write_bytes(raw + b' ')
            with self.subTest(path=path.name), self.assertRaises(ValueError):
                self.remove()
            path.write_bytes(raw)
        self.assertFalse(self.docker.calls)

    def test_partial_catalog_never_returns_completed_pass(self):
        with self.assertRaisesRegex(ValueError, 'incomplete'):
            self.verify(True)

    def test_cid_scope_overlap_shortening_or_duplicates_reject(self):
        for value in ([A, A], [A[:12], B], [A, LIVE]):
            original = self.plan['retired_historical']
            self.plan['retired_historical'] = value
            self.repin()
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.verify()
            self.plan['retired_historical'] = original

    def test_native_wrapper_rejects_str_and_unapproved_verbs(self):
        with patch.object(r.subprocess, 'run', return_value=SimpleNamespace(stdout='str', stderr=b'', returncode=0)):
            with self.assertRaisesRegex(ValueError, 'bytes contract'):
                r.Docker().raw('ps', '-aq')
        for args in (('rm', '--force', A), ('rm', A[:12]), ('start', A), ('volume', 'rm', 'old-used-budget')):
            with self.subTest(args=args), self.assertRaises(ValueError):
                r.Docker().raw(*args)

    def test_dns_comparison_accepts_only_exact_full_config_with_null_empty_delta(self):
        original = item(A)
        original['HostConfig']['Dns'] = None
        expected = r.historical_cfg(original)
        current = copy.deepcopy(original)
        current['HostConfig']['Dns'] = []
        projected, changed = r.dns_representation_for_history(current, expected)
        self.assertTrue(changed)
        self.assertEqual(projected, original)
        self.assertEqual(current['HostConfig']['Dns'], [])  # raw actual untouched
        for field, value in (('Dns', ['8.8.8.8']), ('Privileged', True), ('NetworkMode', 'host')):
            bad = copy.deepcopy(current)
            bad['HostConfig'][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                r.dns_representation_for_history(bad, expected)

    def budget_fixture(self):
        rows={A:item(A),B:item(B),LIVE:item(LIVE)}
        volumes=[];origins=[]
        for cid,name,kind in ((A,'old-budget','old'),(B,'rehearsal-budget','rehearsal'),(LIVE,'successor-budget','current')):
            rows[cid]['Mounts']=[dict(Type='volume',Name=name,Destination='/run/budget')]
            rows[cid]['Config']['Cmd']=['-stage','wire']
            volumes.append(dict(Name=name,Labels={'lifecycle307.remaining':kind,'lifecycle307.allocation':kind+'-nonce'}))
            origins.append(dict(volume=name,execution=kind,nonce=kind+'-nonce',wire=cid,mounted=[cid],references=['exact-original']))
        return rows,volumes,origins

    def test_budget_union_includes_rehearsals_successors_and_equal_duplicates(self):
        rows,volumes,origins=self.budget_fixture()
        result=r.budget_membership(rows,volumes,origins+[copy.deepcopy(origins[0])])
        self.assertEqual(set(result),{v['Name'] for v in volumes})
        self.assertEqual(len(result),3)

    def test_budget_union_missing_extra_conflicting_and_owner_mount_drift_reject(self):
        for fault in ('missing-origin','extra-origin','conflicting','owner','mount','volume-duplicate','wrong-wire','restarted'):
            rows,volumes,origins=self.budget_fixture()
            if fault=='missing-origin':origins.pop()
            elif fault=='extra-origin':origins.append(dict(origins[0],volume='unseen'))
            elif fault=='conflicting':origins.append(dict(origins[0],execution='conflict'))
            elif fault=='owner':volumes[0]['Labels']['lifecycle307.remaining']='wrong'
            elif fault=='mount':rows[EXTERNAL]=item(EXTERNAL);rows[EXTERNAL]['Mounts']=[dict(Type='volume',Name='old-budget',Destination='/elsewhere')]
            elif fault=='volume-duplicate':volumes.append(copy.deepcopy(volumes[0]))
            elif fault=='wrong-wire':origins[0]['wire']=B
            else:rows[A]['State']['Running']=True;rows[A]['State']['Pid']=99
            with self.subTest(fault=fault),self.assertRaises(ValueError):
                r.budget_membership(rows,volumes,origins)

    def test_private_original_scope_excludes_unrelated_sessions_and_broad_root(self):
        private=self.root/'private';owned=private/'remaining-own';other=private/'unrelated-session'
        owned.mkdir(parents=True);other.mkdir()
        rows={A:item(A)}
        rows[A]['Mounts']=[dict(Type='bind',Source=str(owned/'profile.json'))]
        self.assertEqual(r.private_original_roots(private,rows,{A},[]),{owned})
        rows[A]['Mounts']=[dict(Type='bind',Source=str(private))]
        with self.assertRaisesRegex(ValueError,'broad or recursive'):r.private_original_roots(private,rows,{A},[])


if __name__ == '__main__':
    unittest.main()
