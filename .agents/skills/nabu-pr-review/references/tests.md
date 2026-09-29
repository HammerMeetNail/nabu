# Test validity pass

Review important tests as claims about a specific path, including the fixtures
and the implementation under test. Identify eligibility/setup, operation,
completion signal, and assertion. State a concrete broken behavior that the test
would detect, then try to construct one it would miss.

For async negative assertions, write an event ordering. Include fixture writes,
worker reads, per-item processing, the observed signal, and the negative read.
Add ordering edges only when code establishes them (await, transaction boundary,
joined task, signal after the target, etc.). Wall-clock closeness does not add an
edge. A background worker can run before the test begins polling. Being in one
loop/batch does not imply later work completed before an earlier signal.
An elapsed-time estimate, sleep, or timeout budget is not a completion barrier;
only an observed signal whose source ordering follows the target work can prove
that work completed before the assertion.

If an allowed ordering makes broken behavior pass, keep that counterexample
unless a cited guard rules it out. Do not call it negligible as a substitute for
proof. Repeated polls or eventual delivery in a later phase do not retroactively
establish that target work ran before the earlier negative assertion.

Distinguish real SQL from sqlmock, UI events from direct API calls, and render
checks from state transitions. Trace fixture configuration versus ordinary
client values. Synchronous memory tests can validate that path without proving
an asynchronous PostgreSQL/E2E claim. A missing test is not automatically a
product bug; conversely, demonstrable product harm is not merely a test gap.

For a clean outcome, cite what orders the target evaluation before the assertion
or explain the narrower guarantee actually established. Do not infer author
intent, measured runtime from a timeout, or a required fix from a test's name.
