"""Actual Docker read-only delta check, without starting any retained source."""
import json,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent
private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01')
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import retired_resources as r
import retirement_catalog as c
import successor_dependencies as d
previous=Path('C:/Users/Henry/.codex/worktrees/d1-reader-paths/data-product-platform/docs/acceptance/evidence/issue307-d1/container-cleanup02/dependency-gate-existing-failure01.json')
old=r.read(Path('C:/Users/Henry/.codex/fixture-private/d1-container-cleanup02/published-dependency-contract02.json'))
with (out/'original-rejection01.json').open('xb') as stream:stream.write(previous.read_bytes())
parent=r.plan_at(c.ARCHIVE/'plan.json',c.PLAN_SHA256)
import retired_volumes as v
volumes=v.checked(c.ARCHIVE/'volume-plan.json',c.VOLUME_PLAN_SHA256,parent,c.PLAN_SHA256)
contract=c.successor_contract(parent,volumes)
r.need(contract['schema']==d.SCHEMA and old['schema']=='d1-successor-cold-dependencies-v1','explicit comparison version transition absent')
r.need(set(contract['required'])==set(old['required']) and all(contract['required'][cid]['descriptor']==old['required'][cid]['descriptor'] for cid in contract['required']),'old raw identity/membership changed')
docker=r.Docker();result=d.verify(contract,docker)
rows=docker.inspect(sorted(contract['required']));aliases=[]
for cid,expected in contract['required'].items():
    if r.descriptor(rows[cid])!=expected['descriptor']:
        variants=d.empty_dns_descriptors(rows[cid]);r.need(expected['descriptor'] in variants,'not a one-field raw identity match')
        aliases.append(dict(cid=cid,name=rows[cid]['Name'],original_raw_sha256=expected['descriptor'],current_raw_sha256=r.descriptor(rows[cid]),
            canonical_empty_dns_sha256=variants[0],current_dns=rows[cid]['HostConfig']['Dns']))
r.need({x['cid'] for x in aliases}=={x['id'] for x in r.read(previous)['issues']} and len(aliases)==27,'original 27-owner case changed')
with (private/'actual-current-dependency-contract01.json').open('xb') as stream:stream.write(r.encoded(contract))
summary=dict(status='PASS_ACTUAL_184_COLD_DEPENDENCIES_27_EMPTY_DNS_ALIASES',observed_at=r.now(),
    result=result,raw_original_descriptors_and_membership_unchanged=True,exact_raw_matches=len(rows)-len(aliases),
    empty_dns_only_matches=len(aliases),aliases=aliases,source_started=0,provider_calls=0,protection_write=0,business_delete=0,
    actual_S3_17_versions='NOT_RUN_SOURCE_NOT_STARTED',old_rejection_preserved=True,not_a_replay_of_old_frozen_authorization=True)
r.save(out/'actual-dns-readonly01.json',summary)
print(json.dumps(dict(status=summary['status'],required=len(rows),exact_raw=len(rows)-len(aliases),empty_dns_matches=len(aliases),retained_volumes=result['retained_volumes'])))
