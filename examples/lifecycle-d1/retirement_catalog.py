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
FINAL_INSPECT_SHA256 = '06e7d4f6790cb673842b8f61b9c9b48d2d11678f7a86d49e1d2ca03788d3431c'
SOURCE_PACKET = Path('C:/Users/Henry/.codex/worktrees/d1-c22/data-product-platform/docs/acceptance/evidence/issue307-d1/remaining-candidate22/candidate.json')
SOURCE_PACKET_SHA256 = 'd7976ba7462ae30f53783b5111ed84d0656a58e62b4c5610bc93a73e538183db'
BUSINESS_REDIS = '25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6'
EXECUTED_BINDINGS = {
    'containers': '8edaff8921bea8bad0e52de3b3ec8615ee7112cfab58b16a70cd4be847159a4d',
    'volumes': '8574c018b0d3481293fa30ab0891739287da1c7bbdd8a8868be4100a30b94a59'}


def fixed_index(name,digest,status):
    retirement.need(digest,'complete maintenance receipt index not frozen')
    index=audit.decode(audit.checked_bytes(ARCHIVE/name,digest))
    return audit.audit_index(ARCHIVE,dict(id=name,index=name,sha256=digest,format=audit.FILES_MAP,
        member_root='.',expected_count=index['count'],expected_status=status))


def pg_copy_facts(volume_plan):
    expected = {name for name, row in volume_plan['remove'].items()
                if row['reason'] == 'REPRODUCIBLE_OLD_PG_RESTORE'}
    actual = set()
    for path in (ARCHIVE/'current-pg-copy-archives').glob('*/result.json'):
        proof = retirement.read(path)
        retirement.need(proof['status'] == 'PASS_EXACT_CURRENT_PG_COPY_BYTES_ARCHIVED'
            and proof['volume'] not in actual
            and proof['archive_sha256'] == retirement.sha((path.parent/'data.tar.gz').read_bytes()),
            'current PG copy archive proof differs')
        actual.add(proof['volume'])
    retirement.need(actual == expected, 'current PG copy archive membership differs')


def verify_prepared():
    """Explicit maintenance precondition only; never a complete history PASS."""
    retirement.need(PLAN_SHA256, 'successor maintenance plan not frozen')
    return retirement.verify(ARCHIVE/'plan.json',PLAN_SHA256,ARCHIVE,
                             ARCHIVE/'removals',retirement.Docker(),complete=False)


def verify_maintenance_snapshot():
    """Original whole-host maintenance comparison, never a successor preflight."""
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
        pg_copy_facts(volume_plan)
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


def executed_sources():
    """Old execution bytes, not the successor's changed implementation."""
    root = Path(__file__).resolve().parents[2]
    for kind, digest in EXECUTED_BINDINGS.items():
        binding = audit.decode(audit.checked_bytes(ARCHIVE/('binding-'+kind+'.json'), digest))
        retirement.need(binding['status'] == 'FROZEN_MAINTENANCE_ONLY'
            and binding['plan_sha256'] == PLAN_SHA256
            and binding['volume_plan_sha256'] == VOLUME_PLAN_SHA256
            and binding['provider_delete_authorized'] is False
            and binding['volumes_authorized'] is (kind == 'volumes'), 'executed maintenance binding differs')
        folder = ARCHIVE/('source-'+kind)
        names = [Path(name).name for name in binding['source_files']]
        retirement.need(len(names) == len(set(names)) and {p.name for p in folder.iterdir()} == set(names),
                        'executed source snapshot membership differs')
        for name, expected in binding['source_files'].items():
            audit.checked_bytes(audit.bound_path(folder, Path(name).name), expected)
        for name, expected in binding['precondition_files'].items():
            audit.checked_bytes(audit.bound_path(root, name), expected)


def archive_history():
    """Exact history facts only. This cannot certify current dependencies."""
    import retired_volumes as volumes
    fixed_index('removal-index.json', RECEIPT_INDEX_SHA256, 'EXACT_RETIREMENT_RECEIPTS')
    fixed_index('volume-removal-index.json', VOLUME_RECEIPT_INDEX_SHA256, 'EXACT_TEST_VOLUME_RETIREMENT_RECEIPTS')
    fixed_index('pg-copy-archive-index.json', PG_COPY_INDEX_SHA256, 'EXACT_RETIRED_PG_COPY_ARCHIVES')
    executed_sources()
    parent = retirement.plan_at(ARCHIVE/'plan.json', PLAN_SHA256)
    parent['_sha256'] = PLAN_SHA256
    plan = volumes.checked(ARCHIVE/'volume-plan.json', VOLUME_PLAN_SHA256, parent, PLAN_SHA256)
    count = retirement.immutable(parent, ARCHIVE)
    retirement.need(retirement.receipts(parent, ARCHIVE/'removals') == set(parent['retired_historical']),
                    'container retirement facts incomplete')
    retirement.need(volumes.receipt_facts(plan, VOLUME_PLAN_SHA256, ARCHIVE/'volume-removals') == set(plan['remove']),
                    'volume retirement facts incomplete')
    pg_copy_facts(plan)
    return parent, plan, count


def successor_contract(parent, volume_plan):
    import successor_dependencies as dependencies
    frozen = audit.decode(audit.checked_bytes(ARCHIVE/'final-containers.json', FINAL_INSPECT_SHA256))
    retirement.need(type(frozen) is dict and all(cid == row['Id'] for cid, row in frozen.items()),
                    'fixed final inspect membership differs')
    packet = audit.decode(audit.checked_bytes(SOURCE_PACKET, SOURCE_PACKET_SHA256))
    sources = {g['Group']+':'+role: g[role] for g in packet['Groups'] for role in ('Core', 'Native')}
    sources['business-races:Redis'] = BUSINESS_REDIS
    return dependencies.build(parent, volume_plan, frozen, sources)


def verify_history():
    """Successor cold preparation: facts + scoped live dependencies + budgets."""
    import successor_dependencies as dependencies
    import retired_budget_bytes
    parent, volume_plan, count = archive_history()
    contract = successor_contract(parent, volume_plan)
    current = dependencies.verify(contract, retirement.Docker())
    budget_bytes = retired_budget_bytes.verify(parent, ARCHIVE)
    # Stable semantic identity only; transient observations never enter allocation equality.
    return dict(schema='retained-history-with-scoped-successor-dependencies-v2',
        status='PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES',
        retirement_plan_sha256=PLAN_SHA256, receipt_index_sha256=RECEIPT_INDEX_SHA256,
        volume_plan_sha256=VOLUME_PLAN_SHA256, dependency_contract_sha256=current['contract_sha256'],
        retired_historical=len(parent['retired_historical']), retired_test_volumes=len(volume_plan['remove']),
        required_owners=current['required_owners'], source_roles=current['source_roles'],
        retained_volumes=current['retained_volumes'], original_files=count,
        current_physical_budgets=budget_bytes['budgets'],
        provider_delete=0, pins_released=0, budgets_refunded=0)
