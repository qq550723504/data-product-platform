"""Exclusive read-only successor anchor; no Docker start/exec/rm or provider.

Preserve the expired issued packet unchanged. Append its terminal, compare all
known live owners, and retain all real budgets, including rehearsal volumes.
"""
import datetime as dt
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import traceback
import fixture_artifacts
import retained_v32 as retained
import retained_v30 as prior
import evidence_indexes as audit

ROOT = Path(__file__).resolve().parents[2]
OLD = retained.OLD_ROOT
REL = Path(retained.REL)
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate22')
OUT = ROOT/REL/'candidate23-planning01'
need, sha, load = retained.need, retained.sha, retained.load


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('xb') as stream:
        stream.write((json.dumps(value, ensure_ascii=False, indent=2)+'\n').encode())


def preserve_public():
    originals = []
    for directory in (OLD/REL).iterdir():
        if directory.is_dir() and (directory.name.startswith('candidate22-') or directory.name.startswith('remaining-candidate22')):
            originals += [x for x in directory.rglob('*') if x.is_file() and '__pycache__' not in x.parts]
    originals += list((OLD/'docs/acceptance').glob('issue307-d1-candidate22*.md'))
    originals += list((OLD/'docs/acceptance').glob('issue307-d1-v32*.md'))
    public = {}
    for source in sorted(set(originals)):
        source = audit.unlinked_absolute(source)
        need(source.stat().st_nlink == 1, 'C22 public fact aliased')
        name = source.relative_to(OLD.resolve()).as_posix();destination = ROOT/name
        raw = source.read_bytes()
        if destination.exists():
            need(destination.read_bytes() == raw, 'new checkout differs from issued C22 bytes: '+name)
        else:
            destination.parent.mkdir(parents=True,exist_ok=True)
            with destination.open('xb') as stream:stream.write(raw)
        public[name] = dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
    return public


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--attempt', choices=('01','02','03'), required=True)
    args = parser.parse_args()
    JOURNAL = Path('C:/Users/Henry/.codex/fixture-private/candidate23-bootstrap01')/('retained-c22-capture'+args.attempt)
    need(not (OUT/'retained-candidate22-anchor01.json').exists() and not JOURNAL.exists(),
         'exclusive capture already attempted; retain failure, never overwrite')
    JOURNAL.mkdir(parents=True)
    calls = []
    def docker(*args):
        need(args[0] in ('inspect','ps','cp') or args[:2] in (('volume','inspect'),('network','inspect')),
             'capture permits only actual host reads')
        if args[0] == 'cp':
            need(len(args) == 3 and len(args[1].split(':')[0]) == 64
                 and args[1].endswith(':/run/budget') and args[2] == '-', 'budget copy must be read-only stdout')
        result = subprocess.run(['docker', *args], capture_output=True, timeout=60)
        need(type(result.stdout) is bytes and type(result.stderr) is bytes, 'Docker raw bytes required')
        row = dict(argv=list(args), observed_at=dt.datetime.now(dt.timezone.utc).isoformat(),
                   exit_code=result.returncode, stdout_sha256=hashlib.sha256(result.stdout).hexdigest(),
                   stderr_sha256=hashlib.sha256(result.stderr).hexdigest())
        calls.append(row)
        if result.returncode:
            prefix = JOURNAL/('%06d' % len(calls))
            prefix.with_suffix('.stdout').write_bytes(result.stdout)
            prefix.with_suffix('.stderr').write_bytes(result.stderr)
        need(result.returncode == 0, 'actual Docker read failed; raw channels retained privately')
        return result.stdout
    try:
        need(dt.datetime.now(dt.timezone.utc) >= dt.datetime(2026,10,23,10,40,tzinfo=dt.timezone.utc),
             'C22 window has not expired')
        need(retained.absent(PRIVATE/'authorized-execution01')
             and retained.absent(OLD/REL/'delete-confirmed-v32-execution01')
             and retained.absent(Path('C:/Users/Henry/.codex/automations/307-d1-candidate22-v32')),
             'C22 authorization/schedule appeared')
        issued = retained.issued_packet(OLD)
        need(sha(PRIVATE/'execution-binding.json') == retained.ISSUED_PINS[
             str(REL/'candidate22-publication01/branch-freeze01/execution-binding.json').replace('\\','/')],
             'C22 private post-lock execution binding drift')
        save(JOURNAL/'issued-static-audit.json', issued)
        # Reject checkout/receipt drift before repeating any Docker history I/O.
        preserve_public()
        spec = importlib.util.spec_from_file_location('c23_retained_old_constructor', ROOT/'examples/lifecycle-d1/prepare-six-candidate.py')
        p = importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
        # Existing original-history gate, with only the reviewed descriptor
        # comparison in this successor source. It is not a frozen C22 PASS.
        base = p.historical_gate_before_c22()
        need(len(base['closed_cids']) == 996, 'original retained union differs')
        save(JOURNAL/'legacy-comparison.json', dict(status='PASS_SUCCESSOR_OLD_HISTORY_COMPARISON',
            frozen_C22_entry_pass=False, native_delete=0, result=base))
        a = load(PRIVATE/'allocation.json')
        need(a == load(OLD/REL/'remaining-candidate22-preparation01/allocation.json')
             and a['execution'] == retained.EXECUTION, 'C22 allocation drift')
        fixture_artifacts.verify(OLD, PRIVATE, a, dict(execution=retained.EXECUTION, profile='candidate22/v32',
            start='2026-10-23T10:00:00Z', end='2026-10-23T10:40:00Z'))
        candidate = load(OLD/REL/'remaining-candidate22/candidate.json')
        need(sha(OLD/REL/'remaining-candidate22/candidate.json') == 'd7976ba7462ae30f53783b5111ed84d0656a58e62b4c5610bc93a73e538183db'
             and sha(OLD/REL/'remaining-candidate22/bindings.json') == 'e63b908ca5269853478f7d5a65621327bc5f52ff2ff389de96707e0a137296af',
             'C22 frozen packet drift')
        descriptors = load(OLD/REL/'remaining-candidate22-preparation01/owner-descriptors.json')
        readers = {load(OLD/REL/'remaining-candidate22-readonly01'/g/'allocated-owner.json')['cid'] for g in p.GROUPS}
        ids = sorted(set(base['closed_cids']) | set(a['owners']) | readers)
        need(len(ids) == 1085 and len(readers) == 3 and len(a['owners']) == 86, 'C22 complete membership differs')
        rows = retained.closed_rows(docker, ids, p.closed)
        for cid in a['owners']:
            need(p.project_owner(rows[cid]) == descriptors[cid], 'C22 original allocation descriptor drift')
        for group in candidate['Groups']:
            folder = PRIVATE/group['Group']
            for stage in group['Helpers']:
                need(load(folder/stage/'permission.json')['Enabled'] is False, 'C22 operator enabled')
            for action in ('check-disabled','enable-read','enable-send','disable-send','disable-all'):
                need(load(folder/('root-'+action)/'permission.json')['Enabled'] is False, 'C22 root enabled')
            p.sequence.budget_archive(docker('cp',group['Helpers']['wire']+':/run/budget','-'), candidate,group['Group'],initial=True)
            for name,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
                need(sha(folder/'checkpoint'/name) == group[key], 'C22 original backup bytes drift')
            # All actual private case/recovery/reader bytes are frozen below;
            # the issued complete read and closure remain separate facts.
        complete = load(OLD/REL/'remaining-candidate22-readonly01/complete.json')
        need(complete['status'] == 'PASS_COMPLETE_SIX_NATIVE_ENTRY', 'issued C22 native proof missing')
        resources = []
        for entry in a['resources']:
            kind, identity = ('volume',entry['volume']) if 'volume' in entry else ('network',entry['network'])
            value = prior.json_docker(docker,kind,'inspect',identity)[0]
            fields = prior.VOLUME_FIELDS if kind == 'volume' else prior.NETWORK_FIELDS
            row = dict(kind=kind,identity=identity,metadata={k:value.get(k) for k in fields})
            need(value['Labels'].get('lifecycle307.remaining') == retained.EXECUTION
                 and value['Labels'].get('lifecycle307.allocation') == a['allocation_nonce'], 'C22 original resource ownership drift')
            if kind == 'volume':
                row['mounted_cids'] = sorted(prior.raw(docker,'ps','-aq','--no-trunc','--filter','volume='+identity).decode('utf8','strict').split())
                expected = sorted(cid for cid,x in rows.items() if any(m['Type']=='volume' and m['Name']==identity for m in x['Mounts']))
                need(row['mounted_cids'] == expected, 'C22 complete mount graph expanded')
            else:
                need(value['Internal'] is True and not value['Containers'], 'C22 network not closed/internal')
            resources.append(row)
        mounts = {}
        wires = {}
        for cid,item in rows.items():
            for m in item['Mounts']:
                if m['Type'] != 'volume':continue
                mounts.setdefault(m['Name'],set()).add(cid)
                if m['Destination'] == '/run/budget' and item['Config']['Cmd'] == ['-stage','wire']:
                    need(m['Name'] not in wires, 'multiple wire readers for one retained budget')
                    wires[m['Name']] = cid
        need(len(wires) == 40, 'actual retained budget set must include all seven rehearsal budgets')
        budgets = []
        archived = Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01/capture04/budgets')
        for volume,wire in sorted(wires.items()):
            value = prior.json_docker(docker,'volume','inspect',volume)[0]
            users = sorted(mounts[volume])
            prior.mounts(docker,volume,users)
            raw = docker('cp',wire+':/run/budget','-')
            original = archived/(hashlib.sha256(volume.encode()).hexdigest()+'.tar')
            need(original.read_bytes() == raw, 'original observed used/empty budget bytes changed')
            budgets.append(dict(kind='volume',identity=volume,metadata={k:value.get(k) for k in prior.VOLUME_FIELDS},
                mounted_cids=users,wire=wire,sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw)))
        terminal = dict(status='EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED',
            observed_at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=retained.EXECUTION,
            profile='candidate22/v32',retry_allowed=False,authorization='NOT_RECEIVED',schedule='NOT_CREATED',
            owners_closed=1085,native_delete_this_observation=0,source_started=False,
            fresh_native_IAM='NOT_OBSERVED_SOURCE_NOT_STARTED',old_budgets_preserved=40,pins_released=0)
        private_terminal=PRIVATE/'preparation-terminal.json'
        public_terminal=OLD/REL/'candidate22-expiry01/terminal.json'
        if retained.absent(private_terminal) and retained.absent(public_terminal):
            save(private_terminal,terminal)
            save(public_terminal,terminal)
        else:
            existing=load(private_terminal)
            need(private_terminal.read_bytes()==public_terminal.read_bytes()
                 and set(existing)==set(terminal)
                 and {k:v for k,v in existing.items() if k!='observed_at'}=={k:v for k,v in terminal.items() if k!='observed_at'},
                 'existing C22 terminal differs; never overwrite or relabel')
        public = preserve_public()
        retained.issued_packet(OLD)
        owners = {cid:dict(config_sha256=retained.cfg_digest(rows[cid]),
                  state={k:rows[cid]['State'][k] for k in prior.STATE_FIELDS}) for cid in sorted(set(a['owners'])|readers)}
        result = dict(status='C22_RETAINED_EXPIRED_FROZEN_ANCHOR',execution=retained.EXECUTION,profile='candidate22/v32',
            observed_at=dt.datetime.now(dt.timezone.utc).isoformat(),authority_reusable=False,source_start=0,native_delete=0,
            prior_closed_cids=base['closed_cids'],owners=owners,resources=resources,budgets=budgets,
            private_files=prior.file_set(PRIVATE),public_files=public,
            original_retirement_failure_sha256=sha(archived.parent/'capture-failure.json'))
        save(ROOT/REL/retained.ANCHOR,result)
        save(JOURNAL/'success.json',dict(status='PASS_C22_SUCCESSOR_RETENTION_CAPTURE',anchor_sha256=sha(ROOT/REL/retained.ANCHOR),
            known_cids=1085,budget_volumes=40,public_files=len(public),native_delete=0,source_start=0))
        print(json.dumps(load(JOURNAL/'success.json')),flush=True)
    except BaseException as error:
        save(JOURNAL/'failure.json',dict(status='FAILED_RETAINED_C22_CAPTURE_NO_SOURCE',type=type(error).__name__,
            message=str(error),traceback=traceback.format_exc(),native_delete=0,source_start=0))
        raise
    finally:
        save(JOURNAL/'host-read-journal.json',dict(calls=calls,source_start=0,native_delete=0))


if __name__ == '__main__':main()
