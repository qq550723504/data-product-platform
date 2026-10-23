"""Successor-only compiled history boundary; never modifies the C22 packet.

Populated only after exact maintenance receipts exist. An unfinished catalog
cannot authorize preparation, verification, signing or a provider operation.
"""
from pathlib import Path
import evidence_indexes as audit
import retired_resources as retirement

ARCHIVE = Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01/capture06')
PLAN_SHA256 = '3174aa70f3e461a14eefcd40adcd851f0136deb01467df91232f7062995dd1d6'
RECEIPT_INDEX_SHA256 = '70d794e933718f9512d09baca9e31bbd0010ccbc9d567758e440a415200f1b1f'
VOLUME_PLAN_SHA256 = '2be7fabc413fc8b8e339475cf619a6263ede31d08ca33fc95054801bd057b61f'
VOLUME_RECEIPT_INDEX_SHA256 = 'f01250f046150c9aa5eb1591d0778d051dd4c45c210ff56184a349e7ce299717'
PG_COPY_INDEX_SHA256 = '1d1fc58d8818daea303e7a7d7db2e8ffe57e572a02d134a849f67dd673b777d6'


def fixed_index(name,digest,status):
    retirement.need(digest,'complete maintenance receipt index not frozen')
    index=audit.decode(audit.checked_bytes(ARCHIVE/name,digest))
    return audit.audit_index(ARCHIVE,dict(id=name,index=name,sha256=digest,format=audit.FILES_MAP,
        member_root='.',expected_count=index['count'],expected_status=status))


def verify_prepared():
    """Explicit maintenance precondition only; never a complete history PASS."""
    retirement.need(PLAN_SHA256, 'successor maintenance plan not frozen')
    return retirement.verify(ARCHIVE/'plan.json',PLAN_SHA256,ARCHIVE,
                             ARCHIVE/'removals',retirement.Docker(),complete=False)


def verify_history():
    retirement.need(PLAN_SHA256 and RECEIPT_INDEX_SHA256, 'successor history catalog not frozen')
    index = audit.decode(audit.checked_bytes(ARCHIVE / 'removal-index.json', RECEIPT_INDEX_SHA256))
    audit.audit_index(ARCHIVE, dict(id='exact-retirement-receipts', index='removal-index.json',
        sha256=RECEIPT_INDEX_SHA256, format=audit.FILES_MAP, member_root='.',
        expected_count=index['count'], expected_status='EXACT_RETIREMENT_RECEIPTS'))
    if VOLUME_PLAN_SHA256:
        import retired_volumes
        fixed_index('volume-removal-index.json',VOLUME_RECEIPT_INDEX_SHA256,'EXACT_TEST_VOLUME_RETIREMENT_RECEIPTS')
        fixed_index('pg-copy-archive-index.json',PG_COPY_INDEX_SHA256,'EXACT_RETIRED_PG_COPY_ARCHIVES')
        volume_plan=retirement.read(ARCHIVE/'volume-plan.json')
        expected={name for name,entry in volume_plan['remove'].items() if entry['reason']=='REPRODUCIBLE_OLD_PG_RESTORE'}
        actual=set()
        for path in (ARCHIVE/'current-pg-copy-archives').glob('*/result.json'):
            proof=retirement.read(path)
            retirement.need(proof['status']=='PASS_EXACT_CURRENT_PG_COPY_BYTES_ARCHIVED'
                and proof['volume'] not in actual and proof['archive_sha256']==retirement.sha((path.parent/'data.tar.gz').read_bytes()),
                'current PG copy archive proof differs')
            actual.add(proof['volume'])
        retirement.need(actual==expected,'current PG copy archive membership differs')
        result = retired_volumes.verify(ARCHIVE/'plan.json',PLAN_SHA256,ARCHIVE,
            ARCHIVE/'volume-plan.json',VOLUME_PLAN_SHA256,ARCHIVE/'volume-removals',retirement.Docker(),complete=True)
        removed=result['removed_containers']
    else:
        result = retirement.verify(ARCHIVE / 'plan.json', PLAN_SHA256, ARCHIVE,
                                   ARCHIVE / 'removals', retirement.Docker(), complete=True)
        removed=result['removed']
    import retired_budget_bytes
    budget_bytes=retired_budget_bytes.verify(retirement.plan_at(ARCHIVE/'plan.json',PLAN_SHA256),ARCHIVE)
    # Return stable identities/bindings, not a cached live observation or an
    # invented PASS_RETAINED_996_CLOSED. Fresh reads happened above this return.
    return dict(schema='retained-history-with-explicit-retirements-v1',
                status=result['status'], retirement_plan_sha256=PLAN_SHA256,
                receipt_index_sha256=RECEIPT_INDEX_SHA256,
                retired_historical=removed, live_required=result['live_required'],
                original_files=result['original_files'], provider_delete=0,
                retired_test_volumes=result.get('removed_volumes',0),
                current_physical_budgets=budget_bytes['budgets'],
                pins_released=0, budgets_refunded=0)
