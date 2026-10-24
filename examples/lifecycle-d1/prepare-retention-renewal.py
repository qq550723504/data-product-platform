"""Prepare/check the finite two-version proposal. Deliberately has no apply mode.

Actual metadata writes require a separately authorized, independently reviewed
maintenance entry. This file cannot start Docker, enable IAM or contact a provider.
"""
import argparse
import datetime as dt
import json
from pathlib import Path
import native_protection as protection

MIN_RECOVERY = dt.timedelta(hours=2)
MAX_EXTENSION = dt.timedelta(hours=72)
KIND = 'RETENTION_EXTENSION_EXACT_TWO_V2'
TARGETS = {
    'business-races': ('2606d71f-207b-42a7-b671-4b7f4ca4d194', '2026-10-23T06:25:44Z',
                       'f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce'),
    'mixed': ('81a8402e-390f-4247-9d34-552df5eadeb3', '2026-10-23T06:33:33Z',
              '66c17878149ff1959339bc60e319f3b0494786e62da33b77017bfc8aab13613d'),
}


def bound_times(retain_until, maintenance_end):
    until,end=map(protection.instant,(retain_until,maintenance_end))
    protection.need(until.microsecond==0 and end.microsecond==0 and
                    end+MIN_RECOVERY<=until<=end+MAX_EXTENSION,'RENEWAL_BOUNDED_RETENTION_HORIZON')
    return until.isoformat().replace('+00:00','Z'),end.isoformat().replace('+00:00','Z')


def proposal(retain_until, maintenance_end):
    retain_until,maintenance_end=bound_times(retain_until,maintenance_end)
    objects = []
    for group,(version,old,source) in TARGETS.items():
        cp = protection.expected_copy(group)
        protection.need(cp['ProviderVersion'] == version and protection.instant(retain_until)>protection.instant(old), 'RENEWAL_FIXED_VERSION_OR_EXTENSION_DIFFERS')
        objects.append({'group':group, 'copy':cp, 'source_native_cid':source,
                        'expected_mode':'GOVERNANCE', 'expected_legal_hold':False,
                        'historical_retain_until':old, 'new_retain_until':retain_until,
                        'max_mutation_attempts':1,
                        'sdk':'minio-go/v7.PutObjectRetention',
                        'sdk_options':{'VersionID':version, 'Mode':'GOVERNANCE',
                                       'RetainUntilDate':retain_until, 'GovernanceBypass':False}})
    return {'schema':'EXACT_TWO_RETENTION_EXTENSION_PREPARATION_V2', 'kind':KIND,
            'retain_until':retain_until,'maintenance_window_end':maintenance_end,
            'retention_window_rules':{'min_recovery_seconds':7200,'max_extension_seconds':259200},
            'status':'PREPARED_NOT_AUTHORIZED', 'authority':'PREPARATION_DIRECTION_ONLY',
            'failed_execution':'4fad1007-6595-4037-a7a5-1d8d38cb8bcc',
            'failed_candidate':'candidate23/v33', 'objects':objects,
            'max_total_mutation_attempts':2, 'unknown_retry':False,
            'delete':0, 'content_put':0, 'hold_change':0, 'bypass':0, 'cleanup':0,
            'budget_reset':0, 'pin_release':0, 'old_authorization_reused':False,
            'sdk_max_retries':1, 'actual_source_write':False,
            'new_delete_candidate':'NOT_CREATED', 'new_delete_execution':'NOT_CREATED',
            'next_write_entry':'REQUIRES_NEW_AUTHORITY_AND_PACKET'}


def plan_bytes(retain_until,maintenance_end):
    return (json.dumps(proposal(retain_until,maintenance_end),ensure_ascii=False,indent=2)+'\n').encode()


def validate_plan(plan, at):
    # Whole typed equality is intentional for a fixed, finite preparation
    # contract: no unrecognized flags or unapproved version/privilege expansion.
    expected = proposal(plan['retain_until'],plan['maintenance_window_end'])
    protection.need(json.dumps(plan,sort_keys=True,separators=(',',':')) ==
                    json.dumps(expected,sort_keys=True,separators=(',',':')), 'RENEWAL_EXACT_PLAN_DIFFERS')
    protection.need(at < protection.instant(plan['retain_until']), 'RENEWAL_DEADLINE_EXPIRED')
    return {'status':'PASS_EXACT_TWO_RENEWAL_PREPARATION_ONLY', 'objects':2,
            'actual_source_write':False, 'delete':0, 'authority':'NOT_GRANTED_FOR_WRITE'}


def validate_fresh_before(group, receipt, at, bindings, binding_sha):
    """Future write precondition only; this never performs the proposed PUT.

    Expected old expiry is exact. A concurrent unrelated extension is a drift,
    not permission to overwrite it. Bytes/hash, hold and mode must be unchanged.
    """
    until,end=bound_times(bindings['RetainUntil'],bindings['NotAfter'])
    protection.need(group in TARGETS, 'RENEWAL_GROUP_DIFFERS')
    protection.need(protection.instant(bindings['NotBefore'])<=at<protection.instant(end),'RENEWAL_WRITE_WINDOW_EXPIRED')
    protection.need(receipt['maintenance_id']==bindings['MaintenanceID'] and receipt['bindings_sha256']==binding_sha and
                    receipt['phase']=='before' and receipt['retain_until']==until and receipt['maintenance_end']==end,'RENEWAL_ACTUAL_PHASE_BINDING_DIFFERS')
    started,finished = protection.instant(receipt['started_at']),protection.instant(receipt['finished_at'])
    protection.need(started <= finished <= at and at-started <= dt.timedelta(minutes=1), 'RENEWAL_OBSERVATION_NOT_FRESH')
    verification=receipt['verification']
    protection.need(verification['group']==group and verification['status']=='PASS_EXACT_NATIVE_READONLY', 'RENEWAL_READBACK_NOT_PASS')
    wanted=protection.expected_copy(group)
    rows=[r for r in verification['copies'] if r['Copy']['ID']==wanted['ID']]
    protection.need(len(rows)==1 and rows[0]['Copy']==wanted, 'RENEWAL_EXACT_COPY_DIFFERS')
    row=rows[0]
    protection.need(row['Retention']=='GOVERNANCE' and row['LegalHold'] is False
                    and row['RetainUntil']==TARGETS[group][1], 'RENEWAL_PROTECTION_DRIFT')
    protection.need(at < protection.instant(end) and protection.instant(row['RetainUntil']) < protection.instant(until), 'RENEWAL_NOT_EXTENSION')
    return {'status':'PASS_RENEWAL_FRESH_PRECONDITION_ONLY','group':group,'source_write':False}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check',type=Path)
    parser.add_argument('--output',type=Path)
    parser.add_argument('--retain-until')
    parser.add_argument('--maintenance-end')
    args=parser.parse_args()
    if args.check:
        protection.need(not args.retain_until and not args.maintenance_end,'CHECK_CANNOT_OVERRIDE_FROZEN_DATES')
        plan=json.loads(args.check.read_bytes())
    else:
        protection.need(args.retain_until and args.maintenance_end,'EXPLICIT_BOUND_DATES_REQUIRED')
        plan=proposal(args.retain_until,args.maintenance_end)
    result=validate_plan(plan,dt.datetime.now(dt.timezone.utc))
    if args.output:
        with args.output.open('x',encoding='utf-8',newline='\n') as stream:
            json.dump(plan,stream,ensure_ascii=False,indent=2);stream.write('\n')
    print(json.dumps(result))


if __name__=='__main__':main()
