"""Pure gates only; no Docker, source, provider, protection write or DELETE."""
import copy
import datetime as dt
import unittest
import native_protection as n
from protection_test_fixtures import receipt


class ProtectionTests(unittest.TestCase):
    def setUp(self):
        self.at = dt.datetime(2026,10,23,14,tzinfo=dt.timezone.utc)
        self.until = '2026-10-24T12:40:00Z'

    def test_success_all_three_groups_exact_deadline(self):
        rows = {g: receipt(g,self.at,self.until) for g in ('business-races','late-hold','mixed')}
        result = n.validate_all(rows,self.at,self.until)
        self.assertEqual(result['required_versions'],2)
        self.assertFalse(result['source_write'])

    def test_expired_equal_short_missing_wrong_identity_and_type_reject(self):
        for group in n.REQUIRED_KEYS:
            original = receipt(group,self.at,self.until)
            for field,value in [('RetainUntil','2026-10-23T13:59:59Z'),('RetainUntil',self.at.isoformat()),
                ('RetainUntil','2026-10-24T12:39:59Z'),('RetainUntil',None),('RetainUntil',42),
                ('Retention','NONE'),('Retention','COMPLIANCE'),('LegalHold',True),('LegalHold',0)]:
                bad=copy.deepcopy(original);bad['verification']['copies'][0][field]=value
                with self.subTest(group=group,field=field,value=value), self.assertRaises((ValueError,TypeError)):
                    n.validate_group(group,bad,self.at,self.until)
            for field in ('ProviderVersion','ID','IncarnationID','ContentSHA256','ByteSize'):
                bad=copy.deepcopy(original);bad['verification']['copies'][0]['Copy'][field]='different'
                with self.subTest(field=field), self.assertRaises(ValueError): n.validate_group(group,bad,self.at,self.until)

    def test_freshness_membership_and_horizon_reject(self):
        original=receipt('mixed',self.at,self.until)
        for field,value in [('started_at',(self.at-dt.timedelta(minutes=5,seconds=1)).isoformat()),
                            ('started_at',(self.at+dt.timedelta(seconds=1)).isoformat()),
                            ('finished_at',(self.at+dt.timedelta(seconds=1)).isoformat())]:
            bad=copy.deepcopy(original);bad[field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):n.validate_group('mixed',bad,self.at,self.until)
        for rows in ([],original['verification']['copies']*2):
            bad=copy.deepcopy(original);bad['verification']['copies']=rows
            with self.assertRaises(ValueError):n.validate_group('mixed',bad,self.at,self.until)
        with self.assertRaises(ValueError):n.validate_group('mixed',original,n.instant(self.until),self.until)
        with self.assertRaises(ValueError):n.validate_all({'mixed':original},self.at,self.until)

    def test_old_equal_snapshot_remains_failed(self):
        for group,expiry in [('business-races','2026-10-23T06:25:44Z'),('mixed','2026-10-23T06:33:33Z')]:
            old=receipt(group,self.at,expiry)
            self.assertEqual(old['verification']['copies'][0]['RetainUntil'],expiry)
            with self.assertRaisesRegex(ValueError,'GOVERNANCE_RETENTION_EXPIRED'):
                n.validate_group(group,old,self.at,self.until)


if __name__=='__main__':unittest.main()
