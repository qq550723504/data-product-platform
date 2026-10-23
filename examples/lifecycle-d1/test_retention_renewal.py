"""Finite preparation contract, never a source write or mutation authority."""
import copy
import datetime as dt
import importlib.util
from pathlib import Path
import unittest
from protection_test_fixtures import receipt

spec=importlib.util.spec_from_file_location('renewal',Path(__file__).with_name('prepare-retention-renewal.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)


class RenewalTests(unittest.TestCase):
    def setUp(self):self.at=dt.datetime(2026,10,23,14,tzinfo=dt.timezone.utc)

    def test_exact_two_proposal_is_not_apply_authority(self):
        plan=r.proposal();result=r.validate_plan(plan,self.at)
        self.assertEqual(result['objects'],2)
        self.assertFalse(result['actual_source_write'])
        self.assertEqual(plan['next_write_entry'],'NOT_IMPLEMENTED_NOT_AUTHORIZED')
        for g,(_,old,_) in r.TARGETS.items():
            self.assertEqual(r.validate_fresh_before(g,receipt(g,self.at,old),self.at)['status'],'PASS_RENEWAL_FRESH_PRECONDITION_ONLY')

    def test_expansion_shortening_flags_unknown_retry_or_native_write_claim_rejected(self):
        original=r.proposal()
        mutations=[lambda p:p['objects'].pop(), lambda p:p['objects'].append(copy.deepcopy(p['objects'][0])),
                   lambda p:p['objects'].reverse()]
        for key in ('unknown_retry','actual_source_write','old_authorization_reused'):
            mutations.append(lambda p,k=key:p.update({k:True}))
        for key in ('delete','content_put','hold_change','bypass','cleanup','pin_release','budget_reset','max_total_mutation_attempts','sdk_max_retries'):
            mutations.append(lambda p,k=key:p.update({k:99}))
        for key,val in [('ProviderVersion','wrong'),('Namespace','other'),('ContentSHA256','0'*64),('ByteSize',1)]:
            mutations.append(lambda p,k=key,v=val:p['objects'][0]['copy'].update({k:v}))
        for key,val in [('GovernanceBypass',True),('RetainUntilDate','2026-10-24T12:39:59Z'),('Mode','COMPLIANCE'),('VersionID','other')]:
            mutations.append(lambda p,k=key,v=val:p['objects'][0]['sdk_options'].update({k:v}))
        mutations.append(lambda p:p.update({'apply':True}))
        for mutate in mutations:
            bad=copy.deepcopy(original);mutate(bad)
            with self.subTest(plan=bad),self.assertRaises(ValueError):r.validate_plan(bad,self.at)

    def test_fresh_preconditions_drift_missing_and_deadline_reject(self):
        original=receipt('mixed',self.at,r.TARGETS['mixed'][1])
        for field,value in [('LegalHold',True),('Retention','NONE'),('RetainUntil',r.DEADLINE)]:
            bad=copy.deepcopy(original);bad['verification']['copies'][0][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):r.validate_fresh_before('mixed',bad,self.at)
        bad=copy.deepcopy(original);bad['started_at']=(self.at-dt.timedelta(minutes=1,seconds=1)).isoformat()
        with self.assertRaises(ValueError):r.validate_fresh_before('mixed',bad,self.at)
        with self.assertRaises(ValueError):r.validate_plan(r.proposal(),r.protection.instant(r.DEADLINE))


if __name__=='__main__':unittest.main()
