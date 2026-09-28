# State and lifetime pass

Apply [discovery.md](discovery.md), especially the distinction between displayed,
in-flight, and persisted state. Include competing writes as well as stale reads;
trace atomicity across the full operation, not just each store method.

For changed forms or callbacks, choose a concrete edit sequence involving the
changed field/event and another independently editable value. Track:

1. A value is changed but not committed. Where does this draft live?
2. Another action starts and completes an asynchronous operation.
3. State/render/morph updates run. Which actual value survives?
4. Save, Cancel, or navigation occurs. What is persisted or displayed?

Record values at each step using the DOM and state ownership rules. Read
state.js, activeSheetData preservation, and the relevant morph.js branches;
focus preservation is not automatically preservation of all unsaved inputs.
Inspect actual callback ownership checks for navigation/household changes.

Compare the same user sequence and affected data on base and head. If a changed
field or callback did not exist on base, say so. An older similar failure in
another field is not a reason to dismiss silent loss in the new flow. A test gap
may accompany a product defect; it does not replace the product finding.

For non-UI mutable state/background work, trace concrete concurrent callers,
lock acquisition order, cancellation, and shutdown. Establish a reachable race,
lock cycle, stale callback, or lifetime violation rather than extrapolating from
syntax. Read the actual implementations wired in app/server.go.

Return evidence and candidates; do not invent a bug to fill the pass. Runtime
reproduction may be needed for unclear browser behavior, but a complete source
trace can establish deterministic data replacement without claiming execution.
