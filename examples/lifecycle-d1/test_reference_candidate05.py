import copy
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('candidate05', Path(__file__).with_name('prepare-reference-candidate05.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class Candidate05PreparationBoundary(unittest.TestCase):
    def test_checkpoint_dsn_cannot_fall_back_to_source(self):
        source = 'postgres://fixture:fake@' + (m.PROJECT + '-postgres').upper() + ':5432/lifecycle307?sslmode=disable'
        # This representation made the previous case-sensitive replace a no-op.
        self.assertEqual(source.replace(m.PROJECT + '-postgres:5432', m.AUDIT + ':5432'), source)
        target = m.checkpoint_dsn(source)
        self.assertEqual(m.urlsplit(target).hostname, m.AUDIT)
        self.assertNotEqual(source, target)
        for changed in (source.replace('-POSTGRES', '-OTHER'), source.replace('5432', '5433'), source.replace('sslmode=disable', 'sslmode=require'), source.replace('fixture:', 'admin:')):
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                m.checkpoint_dsn(changed)

    def test_cli_rejects_execution_and_override_options_before_io(self):
        for args in (['delete'], ['--confirm'], ['--source', 'HEAD'], ['--execution', 'old'], ['--resume'], ['--cleanup']):
            with self.subTest(args=args), patch.object(sys, 'argv', ['prepare', *args]), patch.object(m, 'prepare') as prepare:
                with self.assertRaises(SystemExit):
                    m.main()
                prepare.assert_not_called()

    def test_prior_denial_closure_and_no_new_authority_are_required(self):
        prior = json.loads((m.PRIOR / 'final-core-readback.json').read_text(encoding='utf-8'))
        prior.update(new_basis=0, new_command_keys=0)
        m.validate_closure(prior)
        for field in ('new_basis', 'new_command_keys', 'absence', 'completion', 'unaccounted_storage', 'storage_costs'):
            with self.subTest(field=field):
                changed = copy.deepcopy(prior)
                changed[field] += 1
                with self.assertRaises(ValueError):
                    m.validate_closure(changed)
        for field in ('observations', 'releases', 'results', 'provider_costs', 'claims'):
            with self.subTest(field=field):
                changed = copy.deepcopy(prior)
                changed[field].pop()
                with self.assertRaises(ValueError):
                    m.validate_closure(changed)

    def test_seal_attempts_every_resource_when_first_stop_fails(self):
        class Host:
            def __init__(self):
                self.stopped = []
            def inspect(self, name):
                return {'Id': name, 'Name': name, 'State': {'Running': name not in self.stopped, 'Status': 'exited' if name in self.stopped else 'running', 'Pid': 0 if name in self.stopped else 1}, 'NetworkSettings': {'Ports': {}}}
            def docker(self, command, name):
                if name == 'failed':
                    raise RuntimeError('stop failure')
                self.stopped.append(name)
        host = Host()
        result = m.stop_all(host, ['failed', 'source-minio', 'source-pg', 'source-pg'])
        self.assertEqual(host.stopped, ['source-minio', 'source-pg'])
        self.assertEqual(result['failures'], ['failed'])
        self.assertEqual(len(result['states']), 2)


if __name__ == '__main__':
    unittest.main()
