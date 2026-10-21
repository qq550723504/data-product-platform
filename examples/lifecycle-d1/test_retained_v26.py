"""Retained fail-closure and current typed freeze audit, file-only doubles."""
from contextlib import ExitStack
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import retained_v26 as retained

spec=importlib.util.spec_from_file_location('c17_freeze_test',Path(__file__).with_name('freeze-six-candidate.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)


def raw(value):return json.dumps(value,separators=(',',':')).encode()
def digest(value):return hashlib.sha256(value).hexdigest()


class RetainedV26Tests(unittest.TestCase):
    def test_revoked_case_and_recovery_preserve_every_other_field(self):
        for field in ('Base.Enabled','Enabled'):
            before={'Base':{'Enabled':True,'Secret':'test-only','ExpiresAt':'old-window'},'Batch':retained.EXECUTION} if field=='Base.Enabled' else {'Enabled':True,'Token':'test-only','ValidUntil':'old-readonly'}
            after=copy.deepcopy(before);owner=after['Base'] if field=='Base.Enabled' else after;owner['Enabled']=False
            record={'before_sha256':digest(raw(before)),'disabled_sha256':digest(raw(after)),'field':field,'before':True,'after':False,'other_fields_equal':True}
            retained.revoked(raw(before),raw(after),record,digest(raw(before)),field)

    def test_rehashing_revocation_cannot_change_other_fields_or_reenable(self):
        before={'Base':{'Enabled':True,'Secret':'test-only','ExpiresAt':'old-window'},'Batch':retained.EXECUTION}
        for fault in ('reenable','expiry','secret','batch','proof-field','old-digest','boolean-type'):
            with self.subTest(fault=fault):
                after=copy.deepcopy(before);after['Base']['Enabled']=False
                if fault=='reenable':after['Base']['Enabled']=True
                if fault=='expiry':after['Base']['ExpiresAt']='new-window'
                if fault=='secret':after['Base']['Secret']='replacement'
                if fault=='batch':after['Batch']=f.p.EXECUTION
                record={'before_sha256':digest(raw(before)),'disabled_sha256':digest(raw(after)),'field':'Base.Enabled','before':True,'after':False,'other_fields_equal':True}
                if fault=='proof-field':record['field']='Enabled'
                if fault=='old-digest':record['before_sha256']='0'*64
                if fault=='boolean-type':record['other_fields_equal']=1
                with self.assertRaises(ValueError):retained.revoked(raw(before),raw(after),record,digest(raw(before)),'Base.Enabled')

    def test_only_new_execution_window_and_identities_are_selected(self):
        self.assertEqual(f.p.PROFILE,'candidate18/v28')
        self.assertEqual(f.p.EXECUTION,'4fd5a7e7-779e-41d7-a35a-3bab7dd615b5')
        self.assertEqual(f.p.W['NotBefore'],'2026-10-21T14:00:00Z')
        self.assertEqual(f.p.W['NotAfter'],'2026-10-21T14:40:00Z')
        self.assertEqual(f.p.W['RecoveryEnd'],'2026-10-21T16:40:00Z')
        self.assertNotEqual(f.p.EXECUTION,retained.EXECUTION)

    def fixture(self,root):
        public=root/'preparation';packet=root/'packet';review=root/'docs/acceptance/evidence/issue307-d1/candidate18-source-review01'
        for directory in (public,packet,review):directory.mkdir(parents=True)
        candidate={'Execution':f.p.EXECUTION,'Source':'f'*40}
        f.save(packet/'candidate.json',candidate)
        for directory,name,status in [(public,'non-delete-evidence-index.json','SIX_NONDELETE_PREPARATION_EVIDENCE_INDEX'),(review,'evidence-index.json','C18_SOURCE_REVIEW_EVIDENCE_INDEX')]:
            (directory/'source.txt').write_bytes(b'recorded-only')
            f.save(directory/name,{'status':status,'count':1,'files':{'source.txt':{'sha256':f.sha(directory/'source.txt'),'bytes':13}}})
        f.save(review/'independent-final-review01.json',{'status':'PASS_BOUNDED_C18_SOURCE_PREPARATION_REVIEW','source_head':'f'*40,'remaining_blockers':[]})
        return public,packet,review,candidate

    def test_current_source_report_and_preparation_form_exact_typed_plan(self):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);public,packet,review,candidate=self.fixture(root)
            for obj,name,value in [(f.r,'ROOT',root),(f.r,'PUBLIC',public),(f,'PACKET',packet)]:stack.enter_context(patch.object(obj,name,value))
            result=f.preparation_audit_plan(candidate)
            self.assertEqual(result['members_verified'],4)
            self.assertFalse(result['execution_authority'])
            plan=f.load(packet/'preparation-evidence-audit-plan.json')
            self.assertEqual(len(plan['indexes']),3)
            self.assertTrue(all(x['format']=='files-map-v1' for x in plan['indexes']))
            self.assertFalse((packet/'human-authorization.json').exists())

    def test_source_review_or_raw_index_drift_stops_freeze_audit(self):
        for fault in ('blocked','wrong-source','status','members-in-map','length','count','digest-map','traversal'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
                root=Path(tmp);public,packet,review,candidate=self.fixture(root)
                for obj,name,value in [(f.r,'ROOT',root),(f.r,'PUBLIC',public),(f,'PACKET',packet)]:stack.enter_context(patch.object(obj,name,value))
                report=f.load(review/'independent-final-review01.json');index=f.load(public/'non-delete-evidence-index.json')
                if fault=='blocked':report['remaining_blockers']=['B1']
                if fault=='wrong-source':report['source_head']='e'*40
                if fault=='status':report['status']='PENDING'
                if fault=='members-in-map':index['members']=[]
                if fault=='length':index['files']['source.txt']['bytes']=14
                if fault=='count':index['count']=False
                if fault=='digest-map':index['files']['source.txt']=index['files']['source.txt']['sha256']
                if fault=='traversal':index['files']['../source.txt']=index['files'].pop('source.txt')
                f.r.replace(review/'independent-final-review01.json',report);f.r.replace(public/'non-delete-evidence-index.json',index)
                with self.assertRaises(ValueError):f.preparation_audit_plan(candidate)
                self.assertFalse((packet/'preparation-evidence-audit-plan.json').exists())


if __name__=='__main__':unittest.main()
