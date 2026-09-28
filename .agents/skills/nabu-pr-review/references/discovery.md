# Discover changed failure paths

Use this method in each relevant pass, including the source-only omission pass.
Map the changed operations before deciding which findings to report: reads,
writes, asynchronous work, and access boundaries, with directly coupled callers,
consumers, and unchanged helpers newly reached by the change. Use the map to
choose concrete scenarios; do not expand a small change into a whole-repo audit.
For documentation/tooling changes, trace their workflow and consumers instead
of inventing application-state scenarios.

Trace an operation's complete observable result: input -> client state -> request
-> server processing -> persisted state -> reload/display, omitting layers it
does not traverse. Keep displayed values, in-flight values, and persisted values
separate. Correctness at one layer does not establish correctness at the others.
Include ordinary defaults, existing data, failure paths, and relevant alternate
clients/stores rather than assuming a happy-path fixture is representative.

For an operation sharing mutable state, identify its owner, competing operations,
and the mechanism preserving correctness. Select applicable adverse orderings:
read/write overlap, write/write overlap, reversed responses, retry, cancellation,
or identity/access changes. Walk through actual values at each step, including
reads before writes and what a later reload observes. Individual method locks do
not establish atomicity of a multi-call read-modify-write operation; a partial
request can still result in a whole-record write. Conversely, verify an actual
reachable caller/interleaving rather than declaring every shared operation racy.

For each selected scenario, record a candidate, the concrete guard/contract that
defeats it, or an unresolved gap. Use coverage rows in record-format.md; name the
operation and scenario and cite the decisive path. Explain excluded operations
when their apparent relevance could otherwise hide a gap. "Looks safe", passing
tests, or a checklist tick is not evidence. Missing evidence stays unresolved;
it is not proof of a defect or proof of safety.

When a defect is found, inspect directly coupled operations for distinct failures
through the same mechanism. Do not assume a display defect accounts for storage
correctness, or that one race accounts for all competing operations. Keep distinct
causes/consequences separate and let the audit consolidate actual duplicates.

Establish a reachable contract violation before assigning provisional priority.
Do not end discovery because the affected data seems unimportant, nor escalate
hypothetical consequences without evidence. A no-finding result is legitimate;
there is no minimum number of scenarios or findings and no exhaustive-safety claim.
