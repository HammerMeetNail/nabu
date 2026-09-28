# Access pass

Inspect only access and lifecycle consequences of the changed surface. Read the
relevant callers and concrete memory/PostgreSQL implementations, not just the
HTTP guard. Map actor → current membership/role → resource visibility → returned
or mutated fields. Use Nabu's existing CanView/GetVisible and filtered list paths
as evidence of the contract, not as proof they run here.

For each returned resource, record the predicate that authorizes its fields.
An outer row's owner predicate proves ownership of that row only. A join or
lookup can introduce data with different access requirements.

Where a retained record refers to another resource, trace a supported lifecycle
change and actual cleanup. Then consider a later update of that resource followed
by the same read. State whether fields are saved historical values or live data.
Check variants that the implementation actually supports (membership, role,
visibility); do not invent an impossible lifecycle. Cite the revocation path and
the read path. If cleanup blocks the sequence, show where it does so.

A candidate needs an observable unauthorized read/write, with a supported actor
and resource sequence. Prior access or a small result limit is not authorization
for new values. Distinguish confidence in the causal path from impact/severity.
Check base/head reachability of the new path before labeling it pre-existing.
Do not prescribe a fix in this pass. Return the record format with any remaining
uncertainty. Zero candidates is a valid result if supported by the trace.
