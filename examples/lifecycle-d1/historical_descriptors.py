"""Compare two already issued descriptor sets without changing inspect facts.

Only their 725 exact CIDs may explain Docker's empty Dns null/[] representation.
The complete original Image/Config/HostConfig/Mounts digest must still match.
This is a successor preservation contract, never a frozen C22 entry PASS.
"""
import hashlib
import json
import re
import evidence_indexes as audit

PLANS = {
    'post-v27-seventeen01/plan.json': '26d27adacec543172a7c570ca3a8d174565348e9c424e5a17dea06b286cc1667',
    'post-v28-seventeen01/plan.json': '5f2574f9bdca02cfce9782896b54c8d1bc3b21d49928617d25e01267aac4fb41',
}


def need(ok, message):
    if not ok:
        raise ValueError('historical descriptors: ' + message)


def original_digest(item):
    return hashlib.sha256(json.dumps({k: item[k] for k in
        ('Image', 'Config', 'HostConfig', 'Mounts')}, sort_keys=True).encode()).hexdigest()


def expected(root):
    result = {}
    for name, digest in PLANS.items():
        document = audit.decode(audit.checked_bytes(root/'docs/acceptance/evidence/issue307-d1'/name, digest))
        values = document['preservation']['descriptors']
        need(type(values) is dict and values, 'fixed descriptor mapping missing')
        for cid, value in values.items():
            need(re.fullmatch('[0-9a-f]{64}', cid) is not None
                 and re.fullmatch('[0-9a-f]{64}', value) is not None, 'invalid full identity/digest')
            need(cid not in result or result[cid] == value, 'authoritative historical descriptors conflict')
            result[cid] = value
    need(len(result) == 725, 'fixed historical membership differs')
    return result


def comparison_digest(item, accepted):
    actual = original_digest(item)
    cid = item['Id']
    if cid not in accepted or actual == accepted[cid]:
        return actual
    # A copy, never a change to actual inspect or its live State/NetworkSettings.
    need(type(item['HostConfig'].get('Dns')) is list and item['HostConfig']['Dns'] == [],
         'descriptor drift or custom DNS')
    projected = {**item, 'HostConfig': {**item['HostConfig'], 'Dns': None}}
    need(original_digest(projected) == accepted[cid], 'descriptor drift beyond empty DNS representation')
    return accepted[cid]
