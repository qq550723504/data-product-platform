import copy
from pathlib import Path
import tempfile
import unittest
import retired_resources as r
import retired_volumes as v

A='a'*64

class VolumeProofTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name).resolve()
        self.content=self.root/'content.json'
        self.value=dict(volume='old-test',reader=A,exit_code=0,source_start=0,provider_calls=0,files=[dict(name='.',type='5',size=0)])
        self.parent=dict(volumes={'old-test':dict(metadata={'Name':'old-test'},mounted_before=[A])},budgets=[dict(volume='used-budget')],retired_historical=[A])
        self.plan=dict(schema=v.SCHEMA,parent_plan_sha256='fixed',authority=dict(human_message_id='human'),provider_delete=0,
            candidate_volumes=['old-test'],remove={'old-test':dict(reason='EMPTY_NO_FILE_DATA',metadata={'Name':'old-test'})},preserve={})
        self.repin()

    def repin(self):
        self.content.write_bytes(r.encoded(self.value))
        self.plan['remove']['old-test']['content']=dict(path=str(self.content),sha256=r.sha(self.content.read_bytes()),bytes=self.content.stat().st_size)
        self.path=self.root/'plan.json';self.path.write_bytes(r.encoded(self.plan));self.digest=r.sha(self.path.read_bytes())

    def check(self):return v.checked(self.path,self.digest,self.parent,'fixed')

    def test_empty_full_proof_accepted(self):self.assertEqual(list(self.check()['remove']),['old-test'])

    def test_rehashed_nonempty_links_and_special_nodes_reject(self):
        for kind,size in (('0',1),('2',0),('1',0),('3',0)):
            self.value['files']=[dict(name='actual',type=kind,size=size)]
            self.repin()
            with self.subTest(kind=kind),self.assertRaisesRegex(ValueError,'contains data/link/special'):self.check()

    def test_budget_and_current_user_never_deleted(self):
        for fault in ('budget','current-user'):
            parent=copy.deepcopy(self.parent)
            if fault=='budget':parent['budgets'].append(dict(volume='old-test'))
            else:parent['volumes']['old-test']['mounted_before'].append('b'*64)
            with self.subTest(fault=fault),self.assertRaises(ValueError):v.checked(self.path,self.digest,parent,'fixed')

    def test_wrong_identity_missing_proof_and_tamper_reject(self):
        for field,value in (('volume','wrong'),('reader','b'*64),('exit_code',1),('source_start',1)):
            original=self.value[field];self.value[field]=value;self.repin()
            with self.subTest(field=field),self.assertRaises(ValueError):self.check()
            self.value[field]=original
        self.repin();self.content.write_bytes(self.content.read_bytes()+b' ')
        with self.assertRaises(ValueError):self.check()

    def test_unbound_decision_and_missing_candidate_reject(self):
        for field,value in (('candidate_volumes',['different']),('parent_plan_sha256','wrong')):
            old=self.plan[field];self.plan[field]=value;self.repin()
            with self.subTest(field=field),self.assertRaises(ValueError):self.check()
            self.plan[field]=old

    def test_intent_without_outcome_is_not_resent_or_accepted(self):
        folder=self.root/'receipts';folder.mkdir();entry=folder/r.sha(b'old-test');entry.mkdir()
        r.save(entry/'intent.json',dict(volume='old-test',plan_sha256=self.digest,operation='docker volume rm'))
        class Docker:
            def raw(self,*args):return b''
        with self.assertRaises(FileNotFoundError):v.receipts(self.plan,self.digest,folder,Docker())

if __name__=='__main__':unittest.main()
