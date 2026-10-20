"""Read-only, explicitly typed evidence-index checks for the six-target entry.

The two existing writers have different contracts. Never guess a format from
the payload, coerce one into the other, or follow indexes recursively. The
caller pins one audit plan and each index's original bytes independently.
"""
import hashlib
import json
from pathlib import Path
import re


FILES_MAP = 'files-map-v1'
MEMBERS_LIST = 'members-list-v1'


def need(condition, message):
    if not condition:
        raise ValueError('evidence audit: ' + message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def pairs(items):
    result = {}
    for key, value in items:
        need(key not in result, 'duplicate JSON key')
        result[key] = value
    return result


def decode(raw):
    def constant(_):
        raise ValueError('evidence audit: non-JSON numeric constant')
    value = json.loads(raw.decode('utf-8', 'strict'), object_pairs_hook=pairs,
                       parse_constant=constant)
    need(type(value) is dict, 'JSON document must be an object')
    return value


def valid_digest(value):
    need(type(value) is str and re.fullmatch('[0-9a-f]{64}', value) is not None,
         'SHA256 must be lowercase hexadecimal')


def relative_parts(value):
    need(type(value) is str and value and '\\' not in value and ':' not in value,
         'member path must be a canonical relative POSIX path')
    parts = value.split('/')
    need(all(x and x not in ('.', '..') and x == x.rstrip(' .') for x in parts),
         'absolute, traversing or ambiguous member path')
    return parts


def bound_path(root, relative, *, directory=False):
    """Reject lexical escapes and links before reading the addressed file."""
    root = Path(root).resolve(strict=True)
    path = root
    for part in relative_parts(relative):
        path = path / part
        need(not path.is_symlink() and not path.is_junction(), 'linked evidence path')
    resolved = path.resolve(strict=True)
    need(resolved.is_relative_to(root), 'member escaped its declared root')
    need(resolved.is_dir() if directory else resolved.is_file(),
         'member is not the required regular file or directory')
    return resolved


def checked_bytes(path, expected):
    valid_digest(expected)
    raw = path.read_bytes()
    need(digest(raw) == expected, 'pinned file SHA256 differs: ' + path.name)
    return raw


def metadata(value):
    need(type(value) is dict and set(value) == {'sha256', 'bytes'},
         'member metadata must contain exactly sha256 and bytes')
    valid_digest(value['sha256'])
    need(type(value['bytes']) is int and value['bytes'] >= 0,
         'member byte count must be a nonnegative integer')
    return value


def audit_index(root, spec):
    need(type(spec) is dict and set(spec) == {
        'id', 'index', 'sha256', 'format', 'member_root', 'expected_count',
        'expected_status'}, 'index specification fields differ')
    need(type(spec['id']) is str and spec['id'], 'index identity missing')
    need(type(spec['expected_status']) is str and spec['expected_status'],
         'expected index status missing')
    count = spec['expected_count']
    need(type(count) is int and count > 0, 'expected count must be positive')
    kind = spec['format']
    need(kind in (FILES_MAP, MEMBERS_LIST), 'unsupported explicit index format')
    index_path = bound_path(root, spec['index'])
    # A repository-root member list uses '.', explicitly only for this root
    # declaration. Individual member names never accept '.' or dot segments.
    member_root = (Path(root).resolve(strict=True) if spec['member_root'] == '.'
                   else bound_path(root, spec['member_root'], directory=True))
    document = decode(checked_bytes(index_path, spec['sha256']))
    need(document.get('status') == spec['expected_status'], 'index status differs')
    if kind == FILES_MAP:
        need('members' not in document and type(document.get('files')) is dict,
             'files-map-v1 requires files mapping and no members field')
        need(type(document.get('count')) is int and document['count'] == count,
             'files-map count differs')
        entries = list(document['files'].items())
    else:
        need('files' not in document and type(document.get('members')) is list,
             'members-list-v1 requires members list and no files field')
        entries = []
        for item in document['members']:
            need(type(item) is dict and set(item) == {'path', 'sha256', 'bytes'},
                 'members-list row fields differ')
            entries.append((item['path'], {k: item[k] for k in ('sha256', 'bytes')}))
    need(len(entries) == count, 'indexed membership count differs or is empty')
    seen = set()
    for name, attributes in entries:
        attributes = metadata(attributes)
        path = bound_path(member_root, name)
        identity = str(path).casefold()
        need(identity not in seen and path != index_path,
             'duplicate, aliased or self-referential member')
        seen.add(identity)
        raw = checked_bytes(path, attributes['sha256'])
        need(len(raw) == attributes['bytes'], 'member byte count differs')
    return {'id': spec['id'], 'format': kind, 'index': spec['index'],
            'sha256': spec['sha256'], 'members_verified': count}


def audit_plan(root, plan_path, plan_sha256, candidate_path):
    root = Path(root).resolve(strict=True)
    plan_path = Path(plan_path).absolute()
    # Do not resolve the supplied path first: that would hide a symlink.
    try:
        relative = plan_path.relative_to(root).as_posix()
    except ValueError as error:
        raise ValueError('evidence audit: plan is outside the repository') from error
    plan = decode(checked_bytes(bound_path(root, relative), plan_sha256))
    need(set(plan) == {'version', 'execution', 'candidate_sha256', 'indexes'},
         'audit plan fields differ')
    need(type(plan['version']) is int and plan['version'] == 1,
         'unsupported audit plan version')
    candidate_raw = Path(candidate_path).read_bytes()
    candidate = decode(candidate_raw)
    valid_digest(plan['candidate_sha256'])
    need(plan['candidate_sha256'] == digest(candidate_raw) and
         type(plan['execution']) is str and plan['execution'] and
         plan['execution'] == candidate.get('Execution'),
         'audit plan candidate or execution differs')
    specs = plan['indexes']
    need(type(specs) is list and specs, 'audit plan must name its indexes')
    results, identities, paths = [], set(), set()
    for spec in specs:
        result = audit_index(root, spec)
        path = str(bound_path(root, spec['index'])).casefold()
        need(result['id'] not in identities and path not in paths,
             'duplicate index identity or path')
        identities.add(result['id'])
        paths.add(path)
        results.append(result)
    return {'status': 'PASS_EXPLICIT_EVIDENCE_INDEX_AUDIT',
            'execution': plan['execution'], 'plan_sha256': plan_sha256,
            'candidate_sha256': plan['candidate_sha256'], 'indexes': results,
            'members_verified': sum(x['members_verified'] for x in results),
            'source_provider_io': 0, 'native_delete': 0,
            'execution_authority': False}
