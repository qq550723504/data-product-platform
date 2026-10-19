# #325 stage 2 — explicit business artifact upgrade

Goal: continue the retained run through the existing entity review and frozen
native workflow, consuming its three already accepted PostgreSQL RAW versions.
The user explicitly authorized this upgrade and business commands on 2026-10-19.

Authority: #325, AGENTS.md, `deploy/trial/README.md`, the existing entity matching
and WorkflowVersion/Execution application commands, and
`examples/enterprise-activity/workflow/workflow-v1.yaml`. Historical B/C reports
are references, not acceptance of this run.

Write set: `deploy/trial/**`, `apps/web/.gitignore`, `apps/web/next.config.mjs`,
batch page/actions and this
acceptance record. No Core/internal, migration, routing, storage or queue changes;
no overlap with #307's retained Core/migration writer in worktree 9400. Existing
HTTP/application commands own all new business facts. No new commodity runtime,
queue, identity engine or processing engine is introduced.

Invariants:

- Keep run, root/worktree, workspace, actor, ports, exact containers, volumes,
  source rows, original batch/attempt/receipt and original RAW identities/bytes.
- Never call prepare/bootstrap/seed/migrate, re-extract or upload replacement RAW.
- Keep the original source/artifact binding and `resume-origin.json` unchanged.
  A separately hashed upgrade manifest identifies the new committed source,
  binaries, production Web, workflow and output datasets. Old files stay intact.
- Build to new paths before stopping hosts; never build into the running `.next`.
  Switch only through explicit upgrade, then use the same guarded stop/resume and
  the same finite, workspace-scoped Core STS policy. Failure is UNKNOWN with all
  journals and owners retained; no automatic rollback, reseed or duplicate work.
- Register only two output datasets and one frozen workflow through existing
  commands. A human decision uses the configured server principal for the same
  actor/workspace and appends reason/evidence; no direct SQL business writes.
- Processing freezes the original three RAW inputs plus the reviewed STANDARDIZED
  output. Execution creation uses the existing stable batch idempotency key.
  Audit/Evidence/Cost and failed attempts remain append-only.

Non-goals: negative-energy remediation/recheck, certification, delivery, metadata
provider enablement, production IAM, paid providers, cleanup or merge authority.
Full #325 stage 2 remains open; human explanation remains deferred/NOT_RUN.

Blockers: original fact/artifact loss, secret or workspace leakage, implicit
business upgrade during resume, uncommitted or out-of-scope source admission,
missing actual entity review/frozen processing path, failing required checks,
or duplicate activity caused by replay of the documented path. Future lifecycle
and crash recovery outside this finite path remain follow-ups.

Acceptance and current HEAD CI are recorded separately after execution. Local
activation does not imply a merged release or actual-merge main CI.
