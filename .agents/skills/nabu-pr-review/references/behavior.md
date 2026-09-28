# Behavior and compatibility pass

Trace new behavior through the ordinary client writers, request parsing,
defaults/storage, fallback branches, and consumers. Inspect create/edit/clear,
omitted/empty/null values, existing data, and older clients where relevant.
Use a compact value table: writer → actual ordinary value → stored/default value
→ each consumer's selected branch. Read the helper implementations. A nonempty
default can still select a fallback branch. Check actual memory/PostgreSQL paths
and PWA/iOS request models rather than relying on the parity matrix.

For time calculations, identify each clock/zone source and evaluate one ordinary
non-UTC scenario through ALL predicates. Use a specified date and an executable
calculation when presenting numeric conversions; otherwise state the source
relationship without inventing numbers. Check relevant overnight boundaries,
lead/eligibility windows, and persistence semantics. Nabu's user and reminder
preferences have distinct timezone fields: determine which client writes each.
Do not silently substitute the test fixture's default for the ordinary value.

Preserve deliberate older policies. Evaluate the new flow's contract separately;
a new control can expose an old helper's behavior for the first time. Partial
settings may intentionally be inactive: read the documented update semantics
and per-field callers before claiming completeness validation is required.

Report the actual unsupported behavior, not just a difference between helpers.
No speculative fix is needed. In particular, changing a shared helper can alter
older flows outside the finding. Return candidates and exact validation limits.
