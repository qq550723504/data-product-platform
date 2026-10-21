#!/usr/bin/env python3
"""Fixed synthetic PostgreSQL trial with explicit same-environment resume."""
import argparse
import json
import sys
from contextlib import nullcontext
from runtime import STATE, Refused, exclusive, preflight, read, start, status, stop
from resume import resume, restart
from upgrade import upgrade
from activation import plan, activate, encoded
from resume import write_once
from runtime import digest
import bootstrap as b
import recovery as recovery


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("preflight", "start", "status", "stop", "resume", "restart", "upgrade", "bootstrap-plan", "bootstrap", "bootstrap-recovery-plan", "bootstrap-recover", "custody-plan", "custody", "activation-plan", "activate"))
    parser.add_argument("--facts")
    parser.add_argument("--custody")
    parser.add_argument("--output")
    parser.add_argument("--plan")
    parser.add_argument("--plan-sha256")
    parser.add_argument("--admission")
    parser.add_argument("--profile")
    args = parser.parse_args()  # Reject reset/cleanup/fault actions before any IO.
    if args.command == "bootstrap-recovery-plan":
        if not args.admission or any((args.facts, args.custody, args.plan, args.plan_sha256, args.profile)):
            parser.error("bootstrap-recovery-plan requires only --admission and optional --output")
    elif args.command in ("bootstrap-plan", "custody-plan"):
        if not args.admission or args.custody or args.plan or args.plan_sha256 or (args.command == "bootstrap-plan" and (not args.facts or args.profile)) or (args.command == "custody-plan" and (not args.profile or args.facts)):
            parser.error("bootstrap-plan requires --facts/--admission; custody-plan requires --profile/--admission; optional --output")
    elif args.command in ("bootstrap", "custody", "bootstrap-recover"):
        if not args.plan or not args.plan_sha256 or any((args.facts, args.custody, args.output, args.admission, args.profile)):
            parser.error("stage execution requires only --plan and --plan-sha256")
    elif args.admission or args.profile:
        parser.error("ledger admission/profile options require a ledger plan")
    elif args.command == "activation-plan":
        if not args.facts or not args.custody or args.plan or args.plan_sha256:
            parser.error("activation-plan requires --facts and --custody; optional --output")
    elif args.command == "activate":
        if not args.plan or not args.plan_sha256 or args.facts or args.custody or args.output:
            parser.error("activate requires only --plan and --plan-sha256")
    elif any((args.facts, args.custody, args.output, args.plan, args.plan_sha256)):
        parser.error("activation options require an explicit activation command")
    try:
        # Advisory plan has no registry/lock writes. Execution re-derives it
        # inside the existing exclusive operator lock.
        with (nullcontext() if args.command.endswith("-plan") else exclusive()):
            if args.command == "preflight": result = preflight()
            elif args.command == "start": result = start()
            else:
                if not (STATE / "state.json").exists(): raise Refused("No trial registry in this worktree; run preflight/start")
                s = read(STATE / "state.json")
                if args.command == "bootstrap-recovery-plan":
                    result = recovery.plan(s, args.admission)
                    if args.output: write_once(args.output, result)
                    result = {"plan": result, "planSha256": digest(encoded(result))}
                elif args.command == "bootstrap-recover": result = recovery.run(s, args.plan, args.plan_sha256)
                elif args.command in ("bootstrap-plan", "custody-plan"):
                    result = b.plan(s, args.command.removesuffix("-plan"), facts=args.facts, admission=args.admission, profile=args.profile)
                    if args.output: write_once(args.output, result)
                    result = {"plan": result, "planSha256": digest(encoded(result))}
                elif args.command in ("bootstrap", "custody"): result = b.run(s, args.command, args.plan, args.plan_sha256)
                elif args.command == "activation-plan":
                    result = plan(s, args.facts, args.custody)
                    if args.output: write_once(args.output, result)
                    result = {"plan": result, "planSha256": digest(encoded(result))}
                elif args.command == "activate": result = activate(s, args.plan, args.plan_sha256)
                else: result = {"status": status, "stop": stop, "resume": resume, "restart": restart, "upgrade": upgrade}[args.command](s)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except (Refused, OSError, ValueError, KeyError) as e:
        # No raw subprocess output, DSN, HTTP headers or credential values.
        print(json.dumps({"status": "REFUSED", "reason": str(e) if isinstance(e, Refused) else type(e).__name__, "registry": str(STATE / "state.json")}), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__": sys.exit(main())
