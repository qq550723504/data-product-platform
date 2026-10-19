#!/usr/bin/env python3
"""Fixed synthetic PostgreSQL trial: preflight/start/status/stop only."""
import argparse
import json
import sys
from runtime import STATE, Refused, exclusive, preflight, read, start, status, stop


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("preflight", "start", "status", "stop"))
    args = parser.parse_args()  # Reject reset/cleanup/fault actions before any IO.
    try:
        with exclusive():
            if args.command == "preflight": result = preflight()
            elif args.command == "start": result = start()
            else:
                if not (STATE / "state.json").exists(): raise Refused("No trial registry in this worktree; run preflight/start")
                s = read(STATE / "state.json")
                result = status(s) if args.command == "status" else stop(s)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except (Refused, OSError, ValueError, KeyError) as e:
        # No raw subprocess output, DSN, HTTP headers or credential values.
        print(json.dumps({"status": "REFUSED", "reason": str(e) if isinstance(e, Refused) else type(e).__name__, "registry": str(STATE / "state.json")}), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__": sys.exit(main())
