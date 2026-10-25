"""Fixed non-delete CLI. No candidate, execution, startup or mutation modes."""
import argparse
from pathlib import Path
import retirement_catalog as catalog
import retired_resources as r

ROOT = Path(__file__).resolve().parents[2]


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    # The leaf must not exist; validate its existing parent without resolving
    # or following a pre-existing output alias.
    r.need(args.output.is_absolute() and args.output.name not in ('', '.', '..'), 'absolute new output required')
    output = r.audit.unlinked_absolute(args.output.parent)/args.output.name
    r.need(output.is_relative_to(ROOT) and output != ROOT, 'output must be a new repository evidence directory')
    output.mkdir(parents=True, exist_ok=False)
    start = r.now()
    try:
        result = catalog.verify_history()
        r.save(output/'result.json', dict(started_at=start, observed_at=r.now(), result=result,
               source_started=0, provider_calls=0, business_delete=0, protection_write=0))
        print(r.encoded(result).decode('utf8'))
        return result
    except Exception as error:
        r.save(output/'failure.json', dict(started_at=start, observed_at=r.now(),
               status='FAILED_SUCCESSOR_READONLY_CHECK_NO_MUTATION',
               error_type=type(error).__name__, message=str(error),
               source_started=0, business_delete=0, protection_write=0))
        raise


if __name__ == '__main__':
    main()
