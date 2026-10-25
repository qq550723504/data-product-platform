"""Fixed maintenance-only CLI: check, exact container removal, verify.

No force/volume/network/image removal, service start, IAM or provider API.
"""
import argparse
from pathlib import Path
import retired_resources as r

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=('check', 'remove', 'verify', 'check-volumes', 'remove-volumes', 'verify-all'))
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--plan-sha256', required=True)
    parser.add_argument('--binding', type=Path)
    parser.add_argument('--binding-sha256')
    parser.add_argument('--volume-plan',type=Path)
    parser.add_argument('--volume-plan-sha256')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / 'journal').mkdir()
    docker = r.Docker(args.output / 'journal')
    root = r.audit.unlinked_absolute(args.archive)
    try:
        if args.action in ('check-volumes','remove-volumes','verify-all'):
            import retired_volumes as volumes
            r.need(args.volume_plan and args.volume_plan_sha256,'exact volume plan required')
            if args.action=='remove-volumes':
                r.need(args.binding and args.binding_sha256,'frozen maintenance source binding required')
                frozen=r.audit.decode(r.audit.checked_bytes(args.binding,args.binding_sha256))
                r.need(frozen['status']=='FROZEN_MAINTENANCE_ONLY' and frozen['plan_sha256']==args.plan_sha256 and frozen['volume_plan_sha256']==args.volume_plan_sha256
                       and frozen['volumes_authorized'] is True and frozen['provider_delete_authorized'] is False,'volume source binding differs')
                for name,expected in frozen['source_files'].items():r.audit.checked_bytes(r.audit.bound_path(ROOT,name),expected)
                result=volumes.remove(root/'plan.json',args.plan_sha256,root,args.volume_plan,args.volume_plan_sha256,root/'volume-removals',docker)
            else:
                result=volumes.verify(root/'plan.json',args.plan_sha256,root,args.volume_plan,args.volume_plan_sha256,root/'volume-removals',docker,complete=args.action=='verify-all')
        elif args.action == 'remove':
            r.need(args.binding and args.binding_sha256, 'frozen maintenance binding required')
            result = r.remove(root / 'plan.json', args.plan_sha256, root, root / 'removals',
                              docker, args.binding, args.binding_sha256, ROOT)
            files = {}
            for path in sorted((root / 'removals').rglob('*.json')):
                raw = path.read_bytes()
                files[path.relative_to(root).as_posix()] = dict(sha256=r.sha(raw), bytes=len(raw))
            r.save(root / 'removal-index.json', dict(status='EXACT_RETIREMENT_RECEIPTS',
                                                   count=len(files), files=files))
        else:
            result = r.verify(root / 'plan.json', args.plan_sha256, root, root / 'removals',
                              docker, complete=args.action == 'verify')
        r.save(args.output / 'result.json', result)
        print(r.encoded(result).decode())
    except Exception as error:
        r.save(args.output / 'failure.json', dict(status='FAILED_STOPPED_NO_MORE_REMOVALS',
               error_type=type(error).__name__, message=str(error), observed_at=r.now(),
               provider_delete=0, volume_delete=0))
        raise


if __name__ == '__main__':
    main()
