"""Six-case/six-recovery real file contract, no runtime/provider calls."""
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import remaining_six_window as w


def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes(json.dumps(value).encode())


def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()


class SixWindow(unittest.TestCase):
    def fixture(self,root):
        packet,owner=root/'packet',root/'owner'
        c={k:v for k,v in w.compiled().items() if k!='RecoveryEnd'}
        c['PolicyCheckpointSHA256']=w.POLICY_CHECKPOINT_SHA256
        c['Groups']=[]; rows={'cases':[],'recoveries':[]}
        recovery={'business-races':['P2','T1','T2'],'late-hold':['H2'],'mixed':['M1A','M1B']}
        for group,modes in w.CASE_MODES.items():
            g={'Group':group,'Helpers':{},'CaseHelpers':{},'CaseConfigSHA256':{},'PolicyHeads':w.policy_heads(group)}
            for stage in modes:
                cfg=w.case_profile({'Base':{},'Group':group},g['PolicyHeads'],'test-only','epoch')
                file=owner/group/('case-'+stage)/'remaining.json';save(file,cfg)
                g['CaseHelpers'][stage]='test-'+stage;g['CaseConfigSHA256'][stage]=sha(file)
                rows['cases'].append(w.project_case(cfg,group,stage,sha(file)))
            for name in recovery[group]:
                stage='recover-'+name;g['Helpers'][stage]='test-'+stage
                cfg={'ValidUntil':w.compiled()['RecoveryEnd']};file=owner/group/stage/'recovery.json';save(file,cfg)
                rows['recoveries'].append({'group':group,'stage':stage,'config_sha256':sha(file),**cfg})
            c['Groups'].append(g)
        save(packet/'profile-windows.json',rows);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c)
        return packet,owner,c,rows

    def test_whole_six_success_uses_one_end_without_renew_or_p1(self):
        with tempfile.TemporaryDirectory() as folder:
            packet,owner,_,_=self.fixture(Path(folder))
            result=w.validate_packet(packet,owner)
            self.assertEqual((result['cases'],result['recoveries']),(6,6))
            self.assertEqual(w.CASE_MODES['business-races'],{'business':'remaining-resume-business'})

    def test_cross_fields_rehash_does_not_allow_stale_or_extended_window(self):
        for field,value in (('Batch','old-execution'),('CandidateUseUntil','2026-10-21T06:41:00Z'),('Base','2026-10-21T06:39:00Z')):
            with self.subTest(field=field),tempfile.TemporaryDirectory() as folder:
                packet,owner,c,rows=self.fixture(Path(folder));row=rows['cases'][0]
                file=owner/row['group']/('case-'+row['stage'])/'remaining.json';cfg=json.loads(file.read_bytes())
                if field=='Base':cfg['Base']['ExpiresAt']=value
                else:cfg[field]=value
                save(file,cfg);row.update(w.project_case(cfg,row['group'],row['stage'],sha(file)))
                c['Groups'][0]['CaseConfigSHA256'][row['stage']]=sha(file)
                save(packet/'profile-windows.json',rows);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c)
                with self.assertRaises(ValueError):w.validate_packet(packet,owner)

    def test_foreign_p1_recovery_or_renew_owner_not_accepted(self):
        for kind in ('recover-P1','renew'):
            with self.subTest(kind=kind),tempfile.TemporaryDirectory() as folder:
                packet,owner,c,rows=self.fixture(Path(folder))
                if kind=='renew':c['Groups'][0]['CaseHelpers'][kind]='not-allowed'
                else:c['Groups'][0]['Helpers'][kind]='not-allowed'
                save(packet/'candidate.json',c)
                with self.assertRaises(ValueError):w.validate_packet(packet,owner)

    def test_actual_successor_checkpoint_and_generator_reject_old_snapshot(self):
        old=json.loads((w.base.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate12/candidate.json').read_bytes())['Groups'][0]
        current=w.policy_heads('business-races')
        stale={version:old['PolicyHeads'][version] for version in current}
        self.assertEqual(len(current),5)
        self.assertTrue(all(current[k]!=stale[k] for k in current))
        with self.assertRaises(ValueError):w.case_profile({'Base':{},'Group':'business-races'},stale,'test','epoch')
        generated=w.case_profile({'Base':{},'Group':'business-races'},dict(reversed(list(current.items()))),'test','epoch')
        self.assertEqual(generated['ExpectedPolicyHeads'],current)
        self.assertEqual(w.policy_checkpoint()['CoreSHA256'],'74e45ae37d0dd4e998e461624dcddfffa80116a08cf3234a7e00dd0ad1fce6f8')

    def test_rehashing_case_candidate_and_projection_cannot_accept_predecessors(self):
        for fault in ('predecessor','foreign','missing','extra','checkpoint'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as folder:
                packet,owner,c,rows=self.fixture(Path(folder))
                group=c['Groups'][0];heads=copy.deepcopy(group['PolicyHeads']);version=next(iter(heads))
                if fault=='predecessor':heads[version]=next(v['supersedes_policy_id'] for v in w.policy_checkpoint()['policies'] if v['dataset_version_id']==version)
                elif fault=='foreign':heads[version]='11111111-1111-4111-8111-111111111111'
                elif fault=='missing':del heads[version]
                elif fault=='extra':heads['76f7f44e-4a2a-46c6-a6dc-a529f9ade834']='310853bb-939b-4ba1-900c-33a4ae0e9a58'
                else:c['PolicyCheckpointSHA256']='f'*64
                group['PolicyHeads']=heads
                row=rows['cases'][0];file=owner/row['group']/('case-'+row['stage'])/'remaining.json'
                cfg=json.loads(file.read_bytes());cfg['ExpectedPolicyHeads']=heads;save(file,cfg)
                row['config_sha256']=sha(file);group['CaseConfigSHA256'][row['stage']]=sha(file)
                save(packet/'profile-windows.json',rows);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c)
                with self.assertRaises(ValueError):w.validate_packet(packet,owner)


if __name__=='__main__': unittest.main()
