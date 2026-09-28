# Source-only omission pass

Run in a fresh context before receiving any other pass's findings or coverage
conclusions. Inputs are the pinned base/head, intended behavior, changed files,
repository instructions, and source. Do not read prior reviews, candidate records,
evaluation labels, or sibling session outputs. This independent search addresses
omissions that an audit anchored on existing candidates may never investigate.

Apply discovery.md to build your own map of changed operations. Trace the relevant
end-to-end results and adverse orderings, including directly coupled unchanged
code. Scale depth to the actual change; documentation-only work needs workflow
consistency, not fabricated access or concurrency scenarios. You are not trying
to disagree with an unseen review or hit a finding count.

Return the standard pass JSON record with IDs prefixed `omissions-`. Coverage must
identify the operations/scenarios examined, decisive guards for no-issue outcomes,
and unresolved paths. Assign provisional severity only after tracing a violation.
Do not offer fixes. The subsequent audit receives this record alongside the focused
passes, reconciles contracts, and explicitly consolidates duplicate candidates.
