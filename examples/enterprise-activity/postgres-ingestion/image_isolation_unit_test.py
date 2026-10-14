"""Controlled Docker responses test the regression runner; no native acceptance is fabricated."""
import contextlib
import io
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import uuid

import image_isolation_test as runner

SHARED = ('dpp-pgi-hop-jdbc:42.7.14', 'dpp-pgi-minio:2025-10-15')


def image_id(value):
    return 'sha256:' + f'{value:064x}'


class ControlledDocker:
    def __init__(self, binary, aliases, mutation=None, inspect_error=None, trial_failure=False):
        self.binary, self.images = binary, dict(aliases)
        self.mutation, self.inspect_error = mutation, inspect_error
        self.trial_failure = trial_failure
        self.builds, self.containers = 0, {}

    def command(self, args, check=True, **kwargs):
        code, data, error = 0, b'', b''
        if args[:4] == ['docker', 'network', 'ls', '-q']:
            data = b'bridge\n'
        elif args[:3] == ['docker', 'network', 'inspect']:
            data = b'[{"IPAM":{"Config":[{"Subnet":"172.17.0.0/16"}]}}]'
        elif args[:3] == ['docker', 'image', 'inspect']:
            ref = args[3]
            if ref in SHARED and self.inspect_error:
                code, error = 1, self.inspect_error.encode()
            elif ref in self.images or ref in self.images.values():
                data = json.dumps([{'Id':self.images.get(ref, ref)}]).encode()
            else:
                code, error = 1, ('Error response from daemon: No such image: ' + ref).encode()
        elif args[:2] == ['docker', 'create']:
            self.containers[args[args.index('--name') + 1]] = args[-2]
        elif args[:2] == ['docker', 'inspect']:
            identity = self.containers[args[2]]
            data = json.dumps([{'Image':identity, 'Config':{'Image':identity}}]).encode()
        elif args[:3] == ['docker', 'start', '--attach']:
            data = b'controlled version response'
        else:
            raise AssertionError('unexpected Docker operation: ' + str(args[:3]))
        if check and code:
            raise RuntimeError('docker command failed (exit 1); raw output withheld')
        return subprocess.CompletedProcess(args, code, data, error)

    def prepare_binary(self, cache, tag):
        self.builds += 1
        identity = image_id(self.builds)
        self.images[tag] = identity
        return self.binary, identity

    def trial(self, *args, **kwargs):
        trial = SimpleNamespace(run_id='00000000-0000-0000-0000-000000000999',
            engine_image=image_id(999), hop='controlled-hop', pg='controlled-source',
            private=self.binary.parent, proxy=None, report={'checks':{'controlled':True}})
        def setup():
            if self.trial_failure:
                raise RuntimeError('controlled trial failure')
            self.images['dpp-pgi-hop-jdbc:trial-' + trial.run_id.replace('-', '')] = trial.engine_image
            self.containers[trial.hop] = trial.engine_image
        def fidelity():
            if self.mutation == 'create' or self.mutation == 'repoint':
                self.images[SHARED[0]] = image_id(777)
            elif self.mutation == 'remove':
                self.images.pop(SHARED[0])
        trial.setup, trial.fidelity = setup, fidelity
        return trial


class ImageIsolationRunnerContract(unittest.TestCase):
    def setUp(self):
        self.directory = Path(tempfile.mkdtemp(prefix='dpp-image-isolation-unit-')).resolve()
        self.binary = self.directory / 'controlled-binary'
        self.binary.write_bytes(b'controlled unit-test binary')

    def tearDown(self):
        assert self.directory.is_relative_to(Path(tempfile.gettempdir()).resolve())
        assert self.directory.name.startswith('dpp-image-isolation-unit-')
        shutil.rmtree(self.directory)

    def invoke(self, docker):
        command = docker.command
        output = self.directory / ('output-' + uuid.uuid4().hex)
        with patch.object(runner.smoke, 'command', command), \
             patch.object(runner.storage_smoke, 'prepare_binary', docker.prepare_binary), \
             patch.object(runner, 'private_directory', return_value=self.directory), \
             patch.object(runner, 'DurableStorage', return_value=SimpleNamespace(start=None)), \
             patch.object(runner.smoke, 'Trial', docker.trial):
            try:
                with contextlib.redirect_stdout(io.StringIO()):
                    runner.run(output, self.directory / 'cache', '10.250.210.0/24')
            finally:
                self.assertIs(runner.smoke.command, command, 'runner leaked its global command override')
        return json.loads((output / 'image-isolation.json').read_text())

    def test_absent_mixed_and_present_aliases_reach_the_checks(self):
        for aliases in ({}, {SHARED[0]:image_id(10)}, {SHARED[0]:image_id(10), SHARED[1]:image_id(11)}):
            with self.subTest(aliases=aliases):
                docker = ControlledDocker(self.binary, aliases)
                report = self.invoke(docker)
                expected = {ref:aliases.get(ref) for ref in SHARED}
                self.assertEqual(report['sharedBefore'], expected)
                self.assertEqual(report['sharedAfter'], expected)
                self.assertEqual(docker.builds, 2)

    def test_daemon_error_is_not_missing_image(self):
        docker = ControlledDocker(self.binary, {}, inspect_error='Cannot connect to the Docker daemon')
        with self.assertRaisesRegex(AssertionError, 'cannot inspect shared image alias'):
            self.invoke(docker)
        self.assertEqual(docker.builds, 0)

    def test_missing_error_for_another_reference_is_not_swallowed(self):
        docker = ControlledDocker(self.binary, {}, inspect_error='Error response from daemon: No such image: unrelated:tag')
        with self.assertRaisesRegex(AssertionError, 'cannot inspect shared image alias'):
            self.invoke(docker)
        self.assertEqual(docker.builds, 0)

    def test_created_removed_or_repointed_alias_is_rejected(self):
        for mutation, aliases in [('create', {}), ('remove', {SHARED[0]:image_id(10)}), ('repoint', {SHARED[0]:image_id(10)})]:
            with self.subTest(mutation=mutation):
                with self.assertRaisesRegex(AssertionError, 'trial rewrote shared image tags'):
                    self.invoke(ControlledDocker(self.binary, aliases, mutation=mutation))

    def test_command_override_restored_after_trial_failure(self):
        docker = ControlledDocker(self.binary, {ref:image_id(10) for ref in SHARED}, trial_failure=True)
        with self.assertRaisesRegex(RuntimeError, 'controlled trial failure'):
            self.invoke(docker)


if __name__ == '__main__':
    unittest.main()
