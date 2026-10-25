"""Freeze the final #332 HEAD before a new candidate's human authorization.

No unlock, provider, enable, delete, candidate regeneration or scheduling mode.
A protection owner performs any later unlock as a separate explicit operation.
"""
import argparse
import importlib.util
from pathlib import Path
import subprocess
import execution_freeze as f

ROOT = Path(__file__).resolve().parents[2]


def freeze(head, output):
    spec = importlib.util.spec_from_file_location('freeze_complete_entry', Path(__file__).with_name('verify-six-native.py'))
    v = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(v)
    private, packet = v.r.CONTROL, v.PACKET
    candidate = f.load(packet/'candidate.json')
    f.need(not (private/'execution-binding.json').exists()
           and not f.control_fact_present(private,'terminal.json')
           and not f.control_fact_present(private,'user-message-verbatim.txt'),
           'freeze must precede a new authorization; old/terminal grants cannot be adopted')
    f.need(subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD'],cwd=ROOT).decode().strip()==head
           and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD'],cwd=ROOT).strip(),
           'final local HEAD/tracked files drift')
    output = output.absolute()
    f.need(output.resolve()==output and output.is_relative_to(ROOT/'docs/acceptance/evidence/issue307-d1')
           and not any(p.is_symlink() or p.is_junction() for p in output.parents),
           'new freeze evidence must be an unaliased local evidence folder')
    # Existing fixed native/budget/topology gate, never another workflow runtime.
    gate = v.sealed_gate(packet)
    lock = f.acquire(f.GitHub(output),head,output)
    binding = dict(status='FROZEN_EXECUTION_BINDING_NOT_AUTHORIZED',version=1,at=f.now(),
                   execution=candidate['Execution'],profile=candidate['Profile'],
                   artifact_bindings=f.artifact_bindings(packet,candidate),
                   branch_lock=lock,
                   public_binding_path=(output/'execution-binding.json').relative_to(ROOT).as_posix(),
                   pre_freeze_closed_gate=gate,send_authorization=False)
    # Any failure here keeps the remote lock; do not automatically undo it.
    f.save(output/'execution-binding.json',binding)
    with (private/'execution-binding.json').open('xb') as stream:
        stream.write((output/'execution-binding.json').read_bytes())
    return dict(status=binding['status'],execution=candidate['Execution'],
                review_head=head,binding_sha256=f.sha(private/'execution-binding.json'),
                native_delete=0,authorization='NOT_RECEIVED')


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--head',required=True)
    parser.add_argument('--output',required=True,type=Path)
    args=parser.parse_args()
    import json
    print(json.dumps(freeze(args.head,args.output)))


if __name__=='__main__':
    main()

