import copy
import importlib.util
import contextlib
import io
import tempfile
import unittest
from pathlib import Path
from unittest import mock

spec = importlib.util.spec_from_file_location('reference_preflight', Path(__file__).with_name('prepare-reference-delete.py'))
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)


class ClosedReferencePreflight(unittest.TestCase):
    def setUp(self):
        self.proposal = preflight.read(preflight.PROPOSAL)
        self.core = preflight.read(preflight.PROPOSAL.parent / 'core-readback.json')
        self.private = Path('fixture-private') / preflight.PROJECT
        self.script = '#!/bin/sh\nset -eu\nexec /run/private/reader -mode plane-reference-races\n'
        self.config = {'RootAccess': '', 'RootSecret': '', 'Enabled': False, 'RunID': preflight.RUN, 'WorkspaceID': preflight.WORKSPACE}
        self.helper = {
            'Id': preflight.HELPER_ID, 'Image': 'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea', 'RestartCount': 0,
            'State': {'Status': 'exited', 'Running': False, 'ExitCode': 0, 'StartedAt': '2026-10-14T00:59:49.71037659Z', 'FinishedAt': '2026-10-14T01:09:52.775625295Z'},
            'Config': {'Entrypoint': ['/bin/sh'], 'Cmd': ['/run/private/' + preflight.HELPER + '.sh'], 'Labels': {'lifecycle307.run': preflight.RUN}, 'Env': ['PATH=/usr/bin', 'PG_MAJOR=18']},
            'HostConfig': {'NetworkMode': 'container:ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4'},
            'Mounts': [{'Type': 'bind', 'Destination': dest, 'Source': str(source.resolve()), 'RW': dest == '/run/output'} for dest, source in {
                '/run/private': self.private / 'plane-owner', '/run/output': preflight.ROOT / 'examples/lifecycle-d1/results', '/run/repo': preflight.ROOT, '/run/rules': self.private / 'plane-owner/rules'}.items()],
        }

    def validate(self):
        preflight.validate_scope(self.core['copies'], self.proposal)
        preflight.validate_trial(self.core, self.helper, self.config, self.script, self.private)

    def test_known_closed_trial_is_candidate_only(self):
        self.validate()
        self.assertFalse(self.proposal['authority']['provider_delete_authorized'])

    def test_exact_scope_cannot_expand_or_change(self):
        changes = [
            lambda: self.core['copies'].pop(),
            lambda: self.core['copies'].append(copy.deepcopy(self.core['copies'][0])),
            lambda: self.core['copies'][0].update(provider_version='foreign'),
            lambda: self.core['copies'][0].update(dataset_version_id='foreign'),
            lambda: self.core['copies'][0].update(content_sha256='0' * 64),
            lambda: self.core['copies'][0].update(byte_size=999),
            lambda: self.core['copies'][0].update(incarnation_id='foreign'),
            lambda: self.proposal['targets'].append(copy.deepcopy(self.proposal['targets'][0])),
            lambda: self.proposal['targets'][0].update(legal_hold=True),
            lambda: self.proposal['authority'].update(permission_enabled=True),
        ]
        for change in changes:
            with self.subTest(change=change):
                self.setUp()
                change()
                with self.assertRaises(RuntimeError):
                    self.validate()

    def test_no_claim_alone_never_proves_no_send(self):
        changes = [
            lambda: self.helper['State'].update(Running=True),
            lambda: self.helper['State'].update(ExitCode=93),
            lambda: self.helper.update(RestartCount=1),
            lambda: self.helper['State'].update(StartedAt='2026-10-14T02:00:00Z'),
            lambda: self.helper['Config'].update(Cmd=['unbounded-command']),
            lambda: self.helper['Config']['Env'].append('AWS_ACCESS_KEY_ID=ambient'),
            lambda: self.helper['Mounts'][0].update(Source='foreign-private'),
            lambda: self.helper['Mounts'][0].update(RW=True),
            lambda: self.helper['Mounts'].append({'Destination': '/root', 'Source': 'foreign', 'RW': False}),
            lambda: self.config.update(RootAccess='root'),
            lambda: self.config.update(Enabled=True),
            lambda: self.core['attempts'][0].update(id='foreign'),
            lambda: self.core['dispatches'][0].update(approval_id='foreign'),
            lambda: self.core['approvals'][0].update(basis_ref='human-confirmed'),
            lambda: self.core['counts'].update(provider_observations=1),
            lambda: self.core['counts'].update(send_claims=1),
            lambda: self.core['counts'].update(active_pins=1),
            lambda: self.core['plans'][-1].update(computed_hash='0' * 64),
            lambda: self.core['processing_output_ready'].pop(),
        ]
        for change in changes:
            with self.subTest(change=change):
                self.setUp()
                change()
                with self.assertRaises(RuntimeError):
                    self.validate()

    def test_changed_script_is_not_trusted_executor(self):
        self.script += 'unbounded-command\n'
        with self.assertRaises(RuntimeError):
            self.validate()

    def test_cli_cannot_enable_or_execute_deletion(self):
        for option in ('--execute', '--human-confirmed', '--release-reservations'):
            with self.subTest(option=option), mock.patch('sys.argv', ['preflight', '--private-directory', 'unused', '--output-directory', 'unused', option]), mock.patch.object(preflight, 'call', side_effect=AssertionError('No subprocess allowed')), contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    preflight.main()
                self.assertEqual(error.exception.code, 2)

    def test_working_input_cannot_replace_pinned_manifest_or_sql(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            path = root / 'input.json'
            canonical = b'{"safe":true}\n'
            path.write_bytes(canonical.replace(b'\n', b'\r\n'))
            with mock.patch.object(preflight, 'ROOT', root), mock.patch.object(preflight, 'call', return_value=canonical):
                self.assertEqual(preflight.pinned_file(path, 'pinned-commit'), canonical)
                path.write_bytes(b'{"entries":[]}\n')
                with self.assertRaises(RuntimeError):
                    preflight.pinned_file(path, 'pinned-commit')


if __name__ == '__main__':
    unittest.main()
