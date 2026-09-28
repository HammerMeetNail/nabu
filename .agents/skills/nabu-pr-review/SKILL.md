---
name: nabu-pr-review
description: Review Nabu pull requests with focused source traces and an independent evidence audit for correctness, authorization, state, persistence, and test validity. Use for PR or branch reviews. Produce findings and validation limits; fixes, publishing, and deployment are separate tasks.
---

# Nabu PR review

A review needs evidence for its decisions, including decisions to dismiss a
candidate. Follow the checkout's AGENTS.md and applicable ios/AGENTS.md. Keep
source unchanged unless the user also requested implementation. Review-owned
probes are appropriate when permitted by the task; honor explicit read-only
limits. Logs belong outside the repository. Use isolated worktrees for probes.

## If this is an assigned pass

A prompt marked `NABU_REVIEW_PASS` or `NABU_REVIEW_AUDIT` is a bounded part of an
ongoing review. Use `references/discovery.md`, its named reference, and
`references/record-format.md`; when their text is attached to the task, use that
text without additional file reads.
Do that pass and return its JSON record. Do not start the overall workflow, launch
another agent, read sibling reports, or infer an expected finding count.

## Run the review

1. Pin base/head, intended behavior, and changed files. For a PR, use its merge
   base for introduced-change comparisons; the target branch's current tip may
   include unrelated later changes. Inspect revisions with `git show` and
   `git diff`; do not switch, reset, restore, or stash the review checkout to
   inspect another revision. Follow changed behavior
   through directly coupled callers, defaults, consumers, and both clients.
   Descriptions, comments, and test names are claims to check.
2. Map the changed operations using [discovery.md](references/discovery.md).
   Before sharing any findings, run a fresh source-only
   [omission pass](references/omissions.md) against the diff and intended behavior.
   It must not receive earlier findings, coverage conclusions, or review artifacts.
   Keep this bounded to changed operations and directly coupled paths.
3. Select the relevant focused passes below. Complete a concrete source/evidence
   record for each before moving on. Prefer fresh contexts so one early verdict
   does not bias the remaining paths. Use host-supported roles from AGENTS.md;
   otherwise run passes sequentially and state that they shared context. If no
   fresh context is available for the audit, label any self-check non-independent
   and report the independent audit as unavailable. Apply the same disclosure to
   an omission pass that cannot run in a fresh context; do not label an ordinary
   reread of known findings an independent search.
4. Give all omission and focused-pass records and source access to an independent
   evidence audit. Read [audit.md](references/audit.md). Every candidate needs an explicit disposition;
   dismissal needs evidence just as confirmation does. Do not discard findings
   merely because a helper is old, a test is green, or the consequence is bounded.
5. Integrate the audited findings, retaining unresolved questions. The verdict
   must agree with those records. Schema checks establish completeness of records,
   not the truth of the model's source interpretation.

| Changed behavior | Focused pass |
| --- | --- |
| Resource reads/writes, joins, sessions, ownership or visibility | [access.md](references/access.md) |
| API fields, defaults, validation, time, persistence, compatibility | [behavior.md](references/behavior.md) |
| UI edits/autosave/rendering, callbacks, shared mutable state or lifetime | [state.md](references/state.md) |
| Tests, background work, async assertions or a claimed test guarantee | [tests.md](references/tests.md) |

Omit irrelevant passes; do not turn a small change into a repository-wide audit.
There is no finding quota. A no-finding outcome is legitimate. A new user path
can expose an old unsafe mechanism; determine this from base/head behavior.
An explicit product choice is not a bug simply because another design is possible.
If base evidence is unavailable, record introduction as unresolved rather than
inventing a comparison. The helper below requires reachable local commits.

## Local Qwen workflow

When using the local OpenCode2/default-model workflow, the bundled helper runs
a source-only omission pass first, selected focused passes in fresh sessions,
then a fresh audit of all candidates, and renders the findings
from the audit record. It does not override model settings or edit source:

```sh
python3 <skill-dir>/scripts/run_review.py \
  --repo <clean-review-worktree> --base <commit> --head <commit> \
  --intent '<intended behavior>' --out <new-directory-outside-repo>
```

Use an absolute skill-directory path resolved by the host. The default executable
is `opencode2`; `--cli` accepts an OpenCode2/v2-compatible executable or alias.
Older OpenCode uses the native-host workflow above; its export protocol differs.
The runner checks compatibility before inference. Its automatic pass selection
is conservative; use `--passes access,behavior,state,tests` with only the relevant
names to narrow smaller changes. The omission pass remains mandatory and scales
its depth to that scope. Its independence comes from fresh context and withholding
prior findings from its prompt; source-reading instructions are not filesystem
isolation. Do not use the helper recursively from an assigned pass. Other hosts can use their native
fresh-context tools with the same pass references and record format. If those
capabilities are unavailable, report that limitation instead of simulating an
independent pass in the same context.

Inspect `review.md`, `audit.json`, `manifest.json`, and any unresolved records.
A failed process, missing pass, or invalid audit must not become a clean review.
Stop on failure and preserve the evidence; do not silently select a better rerun.

## Validation and delivery

Use decisive source traces or focused execution appropriate to the claim. When
running tests, follow AGENTS.md and capture the actual producer exit status and
terminal summary. Do not overlap Go builds/vet/tests. Distinguish executed,
skipped, and unrun coverage; mocked SQL does not prove PostgreSQL behavior, and
a render assertion does not exercise an event handler. Check dependencies before
claiming they are missing. A timeout is not a measured runtime.

Report confirmed defects with trigger, consequence, changed source location,
and evidence. Keep test gaps, product questions, and existing issues distinct.
Silent loss of a new edit is a product defect even when its test is missing.
Do not attach speculative fixes to a finding. If implementation is requested,
verify the repair against the trigger and deliberate base policies separately.
Never describe an unresolved path as verified safe. Finish with validation limits
and a verdict proportionate to the evidence, not an assurance of complete safety.
