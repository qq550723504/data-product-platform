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
        c['Groups']=[]; rows={'cases':[],'recoveries':[]}
        recovery={'business-races':['P2','T1','T2'],'late-hold':['H2'],'mixed':['M1A','M1B']}
        for group,modes in w.CASE_MODES.items():
            g={'Group':group,'Helpers':{},'CaseHelpers':{},'CaseConfigSHA256':{},'PolicyHeads':{str(n):str(n) for n in range(5)} if group=='business-races' else {}}
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
        for field,value in (('Batch','old-execution'),('CandidateUseUntil','2026-10-20T06:41:00Z'),('Base','2026-10-20T06:39:00Z')):
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


if __name__=='__main__': unittest.main()
