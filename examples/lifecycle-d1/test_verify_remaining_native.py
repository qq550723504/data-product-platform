"""Whole collect/aggregate/close entry regressions with recorded native bytes.

Docker is a recording double; policy checking invokes the actual compiled Go
CLI and maintained MinIO library. No provider or live environment is contacted.
"""
import copy
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('preflight', Path(__file__).with_name('verify-remaining-native.py'))
v = importlib.util.module_from_spec(spec); spec.loader.exec_module(v)
PACKET = v.ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-candidate07'
RECORDED = PACKET.parent / 'delete-confirmed-v17-window01/business-races/readonly'

class DockerDouble:
    def __init__(self, group, recorded, mutation):
        self.group, self.recorded, self.mutation = group, recorded, mutation
        self.states = {}; self.calls = []; self.output = None; self.helper = 'a' * 64
        for field, image in [('Core', v.runner.PG), ('Native', v.runner.MI)]:
            self.states[group[field]] = self.state(group[field], image)
    def state(self, ident, image):
        return {'Id': ident, 'Image': image, 'State': {'Running': False, 'Pid': 0},
                'HostConfig': {'RestartPolicy': {'Name': 'no'}, 'PortBindings': {}},
                'NetworkSettings': {'Ports': {}, 'Networks': {'private': {'NetworkID': self.group['Network']}}}}
    def inspect(self, ident): return copy.deepcopy(self.states[ident])
    def docker(self, *args):
        self.calls.append(args)
        action = args[0]
        if action == 'network': return b'[{"Internal":true}]'
        if action == 'create':
            need = lambda flag: args[args.index(flag) + 1]
            mounts = []
            for i, arg in enumerate(args):
                if arg == '--mount':
                    fields = dict(p.split('=', 1) for p in args[i+1].split(',') if '=' in p)
                    mounts.append({'Destination': fields['target'], 'Source': fields['source'], 'RW': 'readonly' not in args[i+1].split(',')})
                    if fields['target'] == '/run/output': self.output = Path(fields['source'])
            state = self.state(self.helper, v.runner.PG)
            state['HostConfig'].update(ReadonlyRootfs=True, Privileged=False, CapDrop=['ALL'],
                                      SecurityOpt=['no-new-privileges'], NetworkMode=need('--network'))
            state['Mounts'] = mounts; self.states[self.helper] = state
            return self.helper.encode()
        if action == 'start':
            state = self.states[args[1]]; state['State'].update(Running=True, Pid=123)
            if args[1] == self.helper:
                for file in self.recorded.iterdir():
                    if file.is_file(): shutil.copyfile(file, self.output / file.name)
                if self.mutation: self.mutation(self.output)
                state['State'].update(Running=False, Pid=0, ExitCode=0,
                                      StartedAt=v.now().isoformat(), FinishedAt=v.now().isoformat())
            return b''
        if action == 'wait': return b'0'
        if action == 'stop': self.states[args[-1]]['State'].update(Running=False, Pid=0); return b''
        raise AssertionError('unapproved Docker operation ' + str(args))

class CompleteEntryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory()
        cls.checker = Path(cls.build.name) / 'checker.exe'
        subprocess.run(['go', 'build', '-o', str(cls.checker), './cmd/lifecycle-fixture-readonly-check'],
                       cwd=v.ROOT / 'apps/platform', check=True, capture_output=True)
    @classmethod
    def tearDownClass(cls): cls.build.cleanup()
    def full_entry(self, mutation=None, denied=False):
        group = v.load(PACKET / 'candidate.json')['Groups'][0]
        double = DockerDouble(group, RECORDED, mutation)
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp); reader = root / 'reader'; reader.write_bytes(b'file-only test reader')
            fixture = root / 'fake-private/remaining-executable03/business-races/source-verifier'
            fixture.mkdir(parents=True)
            (fixture/'remaining.json').write_text('{"Base":{}}', encoding='utf-8')
            (fixture/'inventory.json').write_text('{}', encoding='utf-8')
            (fixture/'manifest.json').write_text('{}', encoding='utf-8')
            core = v.load(PACKET / 'business-races/core-readonly.json')
            with patch.object(v.runner, 'docker', double.docker), patch.object(v.runner, 'inspect', double.inspect), \
                 patch.object(v.runner.native, 'wait_pg'), patch.object(v.runner.native, 'fingerprints', return_value=core), \
                 patch.object(v.time, 'sleep'), patch.object(v, 'PRIVATE', root/'fake-private'):
                if denied:
                    with self.assertRaises(ValueError):
                        v.collect_group(PACKET, group, root/'private', root/'public', reader, self.checker)
                    self.assertTrue((root/'public/failure.json').is_file())
                    self.assertFalse((root/'public/aggregation.json').exists())
                else:
                    result = v.collect_group(PACKET, group, root/'private', root/'public', reader, self.checker)
                    self.assertEqual(result['status'], 'PASS_COMPLETE_NATIVE_AGGREGATION')
                    self.assertEqual(result['technical_requests'], 34)
                    self.assertEqual(sum(p['baseline_file_sha256'] != p['fresh_file_sha256'] for p in result['policies']), 3)
                self.assertTrue(v.load(root/'public/physical-close.json')['all_closed'])
                self.assertTrue(all(v.closed(state) for state in double.states.values()))
                self.assertTrue(all(call[0] in ('start','stop','create','wait','network') for call in double.calls))
                self.assertEqual([call[1] for call in double.calls if call[0] == 'start'],
                                 [group['Core'], group['Native'], double.helper])
    def test_actual_sort_changes_pass_complete_entry(self): self.full_entry()
    def test_expansion_fails_and_closes_every_started_source(self):
        def mutate(output):
            path = output/'native-policy-lifecycle307-remaining-business-remainread808a6960.json'
            doc=v.load(path); doc['Statement'][0]['Action']=['s3:GetObjectVersion','s3:DeleteObjectVersion']
            path.write_text(json.dumps(doc), encoding='utf-8')
        self.full_entry(mutate, denied=True)
    def test_malformed_policy_fails_and_retains_failure(self):
        def mutate(output): next(output.glob('native-policy-*.json')).write_bytes(b'invalid')
        self.full_entry(mutate, denied=True)
    def test_baseline_file_anchor_is_required(self):
        group = v.load(PACKET/'candidate.json')['Groups'][0]
        with tempfile.TemporaryDirectory() as temp:
            dest=Path(temp)/'packet'; shutil.copytree(PACKET,dest)
            file=next((dest/'business-races/readonly').glob('native-policy-*.json')); file.write_bytes(b'{}')
            with self.assertRaises(ValueError): v.aggregate(dest,group,RECORDED,self.checker)

if __name__ == '__main__': unittest.main(verbosity=2)
