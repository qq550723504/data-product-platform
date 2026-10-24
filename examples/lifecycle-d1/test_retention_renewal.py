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
    def setUp(self):
        self.at=dt.datetime(2026,10,23,14,tzinfo=dt.timezone.utc)
        self.until='2026-10-24T12:40:00Z';self.end='2026-10-23T14:30:00Z'
        self.bindings={'MaintenanceID':'11111111-1111-4111-8111-111111111111','NotBefore':'2026-10-23T13:59:00Z','NotAfter':self.end,'RetainUntil':self.until}
        self.binding_sha='a'*64

    def actual(self,group):
        actual=receipt(group,self.at,r.TARGETS[group][1])
        actual.update(maintenance_id=self.bindings['MaintenanceID'],bindings_sha256=self.binding_sha,phase='before',retain_until=self.until,maintenance_end=self.end)
        return actual

    def test_exact_two_proposal_is_not_apply_authority(self):
        plan=r.proposal(self.until,self.end);result=r.validate_plan(plan,self.at)
        self.assertEqual(result['objects'],2)
        self.assertFalse(result['actual_source_write'])
        self.assertEqual(plan['next_write_entry'],'REQUIRES_NEW_AUTHORITY_AND_PACKET')
        for g,(_,old,_) in r.TARGETS.items():
            self.assertEqual(r.validate_fresh_before(g,self.actual(g),self.at,self.bindings,self.binding_sha)['status'],'PASS_RENEWAL_FRESH_PRECONDITION_ONLY')

    def test_expansion_shortening_flags_unknown_retry_or_native_write_claim_rejected(self):
        original=r.proposal(self.until,self.end)
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
        original=self.actual('mixed')
        for field,value in [('LegalHold',True),('Retention','NONE'),('RetainUntil',self.until)]:
            bad=copy.deepcopy(original);bad['verification']['copies'][0][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):r.validate_fresh_before('mixed',bad,self.at,self.bindings,self.binding_sha)
        bad=copy.deepcopy(original);bad['started_at']=(self.at-dt.timedelta(minutes=1,seconds=1)).isoformat()
        with self.assertRaises(ValueError):r.validate_fresh_before('mixed',bad,self.at,self.bindings,self.binding_sha)
        with self.assertRaises(ValueError):r.validate_plan(r.proposal(self.until,self.end),r.protection.instant(self.until))

    def test_frozen_dates_cover_recovery_are_finite_and_all_object_fields_agree(self):
        end=r.protection.instant(self.end)
        for hours in (2,24,72):
            until=(end+dt.timedelta(hours=hours)).isoformat().replace('+00:00','Z')
            plan=r.proposal(until,self.end)
            self.assertEqual(plan['retain_until'],until)
            for row in plan['objects']:
                self.assertEqual(row['new_retain_until'],until);self.assertEqual(row['sdk_options']['RetainUntilDate'],until)
        for seconds in (7199,259201):
            with self.subTest(seconds=seconds),self.assertRaises(ValueError):r.proposal((end+dt.timedelta(seconds=seconds)).isoformat(),self.end)
        with self.assertRaises(ValueError):r.proposal(self.until.replace('00Z','00.1Z'),self.end)
        for field,value in [('maintenance_id','other'),('bindings_sha256','0'*64),('phase','after'),('retain_until','2026-10-25T12:40:00Z'),('maintenance_end','2026-10-23T14:31:00Z')]:
            actual=self.actual('mixed');actual[field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):r.validate_fresh_before('mixed',actual,self.at,self.bindings,self.binding_sha)


if __name__=='__main__':unittest.main()
