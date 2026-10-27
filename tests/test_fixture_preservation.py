import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

from fixture_preservation import PreservationError, canonical, capture_retained, compare, container_metadata, verify_retained


def snapshot():
    return {'id': 'a' * 64, 'status': 'running', 'startedAt': '2026-10-26T00:00:00Z', 'mounts': [
        {'Type': 'bind', 'Source': '/PRIVATE_HOST_PATH', 'Destination': '/fixture', 'RW': False, 'Propagation': 'rprivate', 'Options': {'PRIVATE_OPTION': 'PRIVATE_VALUE'}},
        {'Type': 'volume', 'Name': 'PRIVATE_VOLUME', 'Source': '/PRIVATE_VOLUME_PATH', 'Destination': '/var/lib/postgresql/data', 'Driver': 'local', 'RW': True} ]}


class PreservationTests(unittest.TestCase):
    def test_only_mount_array_order_is_ignored_and_inputs_are_not_mutated(self):
        before = snapshot()
        original = copy.deepcopy(before)
        after = copy.deepcopy(before)
        after['mounts'].reverse()
        self.assertNotEqual(before, after)  # Reproduces the former assertion.
        result = compare(before, after)
        self.assertEqual(result['changedFields'], [])
        self.assertTrue(result['mountOrderOnly'])
        self.assertEqual(before, original)
        self.assertEqual(result['before']['sha256'], result['after']['sha256'])

    def test_every_real_change_still_blocks_with_safe_diagnostics(self):
        changes = [
            ('id', lambda s: s.update(id='b'*64)),
            ('status', lambda s: s.update(status='exited')),
            ('startedAt', lambda s: s.update(startedAt='2026-10-26T01:00:00Z')),
            ('mounts', lambda s: s['mounts'][0].update(Source='/DIFFERENT_PRIVATE_SOURCE')),
            ('mounts', lambda s: s['mounts'][0].update(Destination='/changed')),
            ('mounts', lambda s: s['mounts'][0].update(RW=True)),
            ('mounts', lambda s: s['mounts'][0].update(Type='volume')),
            ('mounts', lambda s: s['mounts'][0].update(Mode='ro')),
            ('mounts', lambda s: s['mounts'][0].update(Propagation='shared')),
            ('mounts', lambda s: s['mounts'][1].update(Name='DIFFERENT_PRIVATE_VOLUME')),
            ('mounts', lambda s: s['mounts'][1].update(Driver='other')),
            ('mounts', lambda s: s['mounts'][0]['Options'].update(PRIVATE_OPTION='DIFFERENT_PRIVATE_VALUE')),
            ('mounts', lambda s: s['mounts'][0].update(NewField='PRIVATE_NEW_VALUE')),
            ('mounts', lambda s: s['mounts'][0].pop('Options')),
            ('mounts', lambda s: s['mounts'].pop()),
            ('mounts', lambda s: s['mounts'].append(copy.deepcopy(s['mounts'][0]))),
        ]
        for field, mutate in changes:
            with self.subTest(field=field, change=changes.index((field, mutate))):
                before, after = snapshot(), snapshot()
                mutate(after)
                with tempfile.TemporaryDirectory() as temp:
                    path = Path(temp)/'safe.json'
                    with self.assertRaisesRegex(PreservationError, '^RETAINED_CONTAINER_CHANGED$'):
                        verify_retained({'management': before}, path, observer=lambda _: after)
                    report = json.loads(path.read_text())
                    self.assertEqual(report['verdict'], 'FAIL')
                    self.assertEqual(report['resources'][0]['changedFields'], [field])
                    self.assertNotIn('PRIVATE', path.read_text())

    def test_image_and_presence_changes_block(self):
        before = {**snapshot(), 'image': 'sha256:'+'1'*64}
        after = {**snapshot(), 'image': 'sha256:'+'2'*64}
        self.assertEqual(compare(before, after)['changedFields'], ['image'])
        self.assertEqual(compare(before, None)['changedFields'], ['presence'])
        self.assertEqual(compare(None, before)['changedFields'], ['presence'])
        self.assertEqual(compare(None, None)['changedFields'], [])

    def test_duplicate_mounts_are_not_deduplicated(self):
        before = snapshot()
        before['mounts'].append(copy.deepcopy(before['mounts'][0]))
        after = copy.deepcopy(before)
        after['mounts'].reverse()
        self.assertEqual(compare(before, after)['changedFields'], [])
        after['mounts'].pop()
        report = compare(before, after)
        self.assertEqual(report['changedFields'], ['mounts'])
        self.assertEqual(sum(m['count'] for m in report['removedMounts']), 1)

    def test_key_order_is_not_a_change_but_value_types_are(self):
        before = snapshot()
        after = json.loads(json.dumps(before, sort_keys=True))
        self.assertEqual(compare(before, after)['changedFields'], [])
        after['mounts'][0]['Options']['PRIVATE_OPTION'] = 1
        before['mounts'][0]['Options']['PRIVATE_OPTION'] = True
        self.assertEqual(compare(before, after)['changedFields'], ['mounts'])

    def test_malformed_snapshots_fail_closed(self):
        for mutate in [lambda s: s.pop('id'), lambda s: s.update(status=None), lambda s: s.update(mounts=None), lambda s: s['mounts'].append(None), lambda s: s['mounts'][0].update(RW=1), lambda s: s.update(Config={'Env':['PRIVATE_VALUE']}), lambda s: s['mounts'][0].update(number=float('nan'))]:
            s=snapshot();mutate(s)
            with self.assertRaises(PreservationError):canonical(s)

    def test_observer_errors_before_and_after_are_unknown_and_block(self):
        for phase in ['before', 'after']:
            for error in [PreservationError('INSPECT_UNAVAILABLE'), PreservationError('PRIVATE_ERROR'), RuntimeError('PRIVATE_ERROR')]:
                with self.subTest(phase=phase), tempfile.TemporaryDirectory() as temp:
                    path=Path(temp)/'safe.json'
                    observer=Mock(side_effect=error)
                    with self.assertRaisesRegex(PreservationError, '^RETAINED_OBSERVATION_UNAVAILABLE$'):
                        if phase=='before':capture_retained(('original',),path,observer=observer)
                        else:verify_retained({'original':snapshot()},path,observer=observer)
                    report=json.loads(path.read_text())
                    self.assertEqual(report['verdict'],'UNKNOWN')
                    self.assertEqual(report['phase'],phase)
                    self.assertNotIn('PRIVATE',path.read_text())

    def test_success_report_handles_absent_original_and_order_only_management(self):
        before=snapshot();after=snapshot();after['mounts'].reverse()
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'safe.json'
            report=verify_retained({'original':None,'management':before},path,observer=lambda name:after if 'management' in name else None)
            self.assertEqual(report['verdict'],'PASS')
            self.assertFalse(report['resources'][0]['before']['present'])
            self.assertTrue(report['resources'][1]['mountOrderOnly'])
            self.assertNotIn('PRIVATE',path.read_text())

    def test_required_diagnostic_write_failure_blocks(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(PreservationError,'^DIAGNOSTIC_UNAVAILABLE$'):
                verify_retained({'original':None},Path(temp)/'missing'/'safe.json',observer=lambda _:None)

    def test_absence_requires_successful_listing_and_errors_do_not_become_absence(self):
        run=Mock(return_value=subprocess.CompletedProcess([],0,'',''))
        self.assertIsNone(container_metadata('test',run=run))
        run.assert_called_once()
        for effect in [subprocess.CompletedProcess([],1,'','PRIVATE_DAEMON_ERROR'), subprocess.TimeoutExpired('PRIVATE_COMMAND',10), OSError('PRIVATE_PATH')]:
            run=Mock(side_effect=effect) if isinstance(effect,Exception) else Mock(return_value=effect)
            with self.assertRaisesRegex(PreservationError,'^INSPECT_UNAVAILABLE$'):container_metadata('test',run=run)

    def test_inspect_is_pinned_to_listed_id_and_projects_only_existing_fields(self):
        s=snapshot();raw={'Id':s['id'],'Name':'/test','Image':'sha256:'+'1'*64,'State':{'Status':s['status'],'StartedAt':s['startedAt']},'Mounts':s['mounts'],'Config':{'Env':['PRIVATE_SECRET']}}
        run=Mock(side_effect=[subprocess.CompletedProcess([],0,s['id']+'\n',''),subprocess.CompletedProcess([],0,json.dumps([raw]),'')])
        self.assertEqual(container_metadata('test',run=run),s)
        self.assertEqual(run.call_args.args[0],['docker','container','inspect',s['id']])
        self.assertEqual(run.call_args.kwargs['timeout'],10)

    def test_list_ambiguity_inspect_failure_mismatch_and_invalid_json_block(self):
        s=snapshot()
        for result in ['x',s['id']+'\n'+'b'*64]:
            with self.assertRaises(PreservationError):container_metadata('test',run=Mock(return_value=subprocess.CompletedProcess([],0,result,'')))
        for code,raw in [(1,''),(0,'not-json'),(0,'[]'),(0,json.dumps([{'Id':'b'*64,'Name':'/test'}])),(0,json.dumps([{'Id':s['id'],'Name':'/wrong'}]))]:
            run=Mock(side_effect=[subprocess.CompletedProcess([],0,s['id'],''),subprocess.CompletedProcess([],code,raw,'PRIVATE_INSPECT_ERROR')])
            with self.assertRaises(PreservationError):container_metadata('test',run=run)

    def test_retained_set_is_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            for names in [(),('original','original'),('PRIVATE_ALIAS',)]:
                with self.assertRaises(PreservationError):capture_retained(names,Path(temp)/'safe.json')


if __name__=='__main__':unittest.main()
