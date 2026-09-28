# Independent evidence audit

Read the omission and focused-pass records as claims, then verify against source.
Compare their operation/scenario coverage as well as their candidates. Missing
coverage stays visible even when every submitted candidate has a disposition.
Do not rely on the earlier reviewers' confidence. Inspect every candidate's trigger, contract,
head path, base comparison, and consequence. Return exactly one disposition per
candidate ID: confirmed, dismissed, or unresolved, with decisive references.
When passes disagree about expected behavior, resolve the contract from source
and the intended change. Do not settle it by confidence, majority, or copying the
current implementation as the specification. Correct a false counterexample's
premise even when its broader coverage-gap observation is valid.
If source and intended behavior conflict or remain insufficient, keep the
candidate unresolved and state the missing contract evidence.
For related old and new flows, first separate their contracts as flow -> expected
behavior -> source authority. An existing test for one flow does not prescribe
the contract for another. Before returning the record, compare the confirmed
findings' expected outcomes under the same inputs: one cannot call a behavior
correct while another calls that same behavior a regression. Correct or withdraw
the mistaken premise; do not preserve both just because they concern different
candidate categories.
Consolidate duplicate candidates that establish the same cause and consequence:
confirm one, and dismiss the duplicate with a reason naming the retained candidate
ID and explaining equivalence. Do not drop a distinct trigger or impact as a duplicate.

For dismissal, give the exact guard/contract/base sequence that defeats the
counterexample. A familiar helper, outer ownership filter, passing test, short
time interval, or older similar defect does not do that. New reachability of an
old mechanism counts as newly exposed when the new flow violates its contract.
Check product impact before reducing a finding to a missing-test observation.

For confirmation, re-walk the causal path. Check live versus historical fields,
actual defaults and fallback branches, lost draft ownership, and event ordering
when relevant. Verify arithmetic using actual predicates and a specified date,
or omit numeric examples from the finding. Preserve deliberate base policies.

The final finding should state the trigger, user/security consequence, changed
line, and evidence method. Do not propose a fix: this audit establishes the bug,
not the correctness of a patch. Rank impact separately from whether a reachable
violation exists. A bounded leak is still unauthorized access; a new lost edit
is still a correctness failure even if related old inputs were already affected.

Calibrate priority against the demonstrated impact. Silent loss of an intended
edit or persisted value ordinarily merits P2 as a correctness defect; P3 is for
minor polish or optional hardening. Do not lower priority merely because the
unsafe helper predates the new flow or a race's frequency is unmeasured. State
the required interleaving and uncertainty instead. P1 needs an explicit reason
for urgency or severe impact; do not invent how many production users are affected.

Separate the proven mechanism from possible downstream impact. Carry missing
prerequisites and validation limits into the finding's claim and priority, not
only a footer. Data shown in the wrong household context proves a state defect;
unauthorized disclosure additionally needs an actor/recipient who lacks access
to that data. Unsafe HTML interpolation, parsed DOM injection, and script
execution are different claims: trace the actual consumer and relevant policy
before asserting the latter. If the packet only proves the earlier step, report
that step and make downstream consequences conditional. A security label or a
plausible severe outcome is not evidence that its prerequisites hold.

For a claim that tests would miss faulty code, specify one fault at a time and
replay it through the existing happy-path and failure assertions. Removing a
guard and inverting it are not equivalent: a change that breaks the ordinary
success path may already be caught. An unexercised branch can establish a
coverage gap without proving that every proposed mutation survives. Conversely,
currently correct product code does not establish that its tests would catch a
future fault. Keep the supported coverage observation separate from any unproven
or defeated mutation example.

Inspect each pass's coverage and unresolved entries for omissions. If you find
a missed plausible issue, put it in audit_questions with source evidence; do not
silently certify the omitted path or invent a confirmed finding without a trace.
The lead can investigate that question. An unresolved candidate stays visible.

No finding quota applies. It is valid to dismiss unsupported candidates or
confirm none. Do not promote product preferences, hypothetical future stores,
or unchanged supported policies to defects. Final prose must agree with your
candidate dispositions. The helper will derive counts from this record.
