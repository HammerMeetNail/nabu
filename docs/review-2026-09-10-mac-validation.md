# Mac validation record

Continuation of the [Mac handoff](review-2026-09-10-mac-handoff.md), in the
`pwa-review-20260910` worktree. Fetched source:
`cd9431395e0f56b839c804ea3da4a76021a92d26`. The fixes and evidence below belong to
the following Mac follow-up commit; no merge, release tag or deployment was run.

## Environment

- Xcode 26.6 (17F113), iPhone Simulator SDK 26.5.
- iPhone 17 Pro, iOS 26.0 (23A343), dedicated simulator
  `D32A6D1E-55A0-4488-93AB-7C4B5CB2FAFD` (`Nabu review 26.0`).
- PostgreSQL 17 stack built with `make local-fresh`, local API `:8080`, Mailpit
  `:8025`; synthetic accounts only. Fixture transports fail closed at `:9998`.
- Results, logs and temporary execution scripts:
  `/tmp/nabu-review-xcode.AW02tx`. These artifacts are local, not committed.

## Corrections

- Fixed the SwiftUI export Section initializer that prevented the original app
  from compiling.
- Password requests now explicitly encode `current_password` and `new_password`,
  matching the server and existing PWA. A production-encoder regression, native
  setup flow and browser request assertion cover this contract.
- Seeded UI launches now adopt a managed fixture identity before restoring
  fixture data. All fixture routes stay local and retain saved logs across
  refreshes. The unit-test host defaults to an inert local URL in the scheme.
- Account deletion confirmation now belongs to the root view. Canonical session
  refresh previously destroyed the Settings sheet and its ownership-error text.
  The coordinator retains guidance only for the initiating server/account/
  household; cancellation and superseded presentations reject late UI results.
  Identity invalidation and canonical confirmation remain enforced.
- Schedule end dates now use the server's UTC-midnight timestamp contract.
  Creation previously sent an invalid plain date; editing/summary rendering
  could shift the selected day west of UTC, and unchecking the end date omitted
  the required explicit null. Unit, screen reload and browser regressions cover
  setting and clearing the date, including New York/Tokyo and a DST boundary.
- Corrected native accessibility selectors and the stopped timer's retry label.
  Added actual screen baselines, fixed date inputs, and stable synthetic receipt
  ages. Existing unit-snapshot baselines were retained. UI saves now scroll to
  reachable controls, the Feed Baby flow explicitly selects its required amount,
  and export/recent screenshots wait for settled presentation. Browser preference
  and schedule persistence checks wait for the save response before reading or
  reloading, avoiding races with optimistic UI updates.

## Validation

App build and test build passed in `build-r20.xcresult` and
`build-tests-r20.xcresult`. The complete `NabuTests` run passed **335 tests**,
with zero failures, skips or expected failures (`unit-final-r16.xcresult`).
The 18 modified app/project/unit-test files are byte-for-byte unchanged since
that unit gate; subsequent edits only adjust UI/browser test synchronization
and the newly recorded UI baselines.

The complete `NabuUITests` run passed **53 tests**, with zero failures, skips or
expected failures and exit 0 (`ui-final-r20.xcresult`). Snapshot recording was
disabled; all 16 new screen baselines matched. The final five adjusted captures
also matched in the preceding focused verification. No temporary export CSV
remained in the dedicated simulator after the completed suite
(`export-cleanup-final.json`).

The source manifest includes the fetched base SHA and SHA-256 hashes of all 38
changed/new files under `ios/` and `tests/`, including the 16 screen PNGs.
`source-manifest-final.txt` has SHA-256
`c6eb85556bfe04802d73ffbb6bbf91ec48feaace524f5e8870450b671f1412fc`.

| Local check | Result |
| --- | --- |
| `make local-fresh` / PostgreSQL readiness | Passed |
| `go build ./...` | Passed |
| `go vet ./...` | Passed |
| `make test-go` | Passed |
| `make test-js` | 111 passed, 0 failed/skipped |
| `make lint` | Passed, 0 issues |
| `make check-parity` | Passed; no pending rows |
| Focused account-claim Playwright tests | 3 passed |
| `make e2e` | 382 passed, 0 failed/skipped; exit 0 (`e2e-final-r3.log`) |

GNU coreutils was installed for the operations tests' `timeout` command; the JS
rerun used `/opt/homebrew/opt/coreutils/libexec/gnubin` on PATH. Playwright Chromium
was installed before browser execution. An existing simulator's test service
failed before executing tests; the dedicated simulator above replaced it.
The installed Mac Git hook predates the matrix linter and rejects native-only
production changes. The push uses its documented `SKIP_PARITY=1` override: the
matrix is updated and passes, the PWA already implements these contracts, and
all required build/test/lint checks above ran independently.

Snapshot recording runs intentionally report recording assertions and are not
passing validation gates. Original compile/fixture/contract failures and all
rerun exit codes remain in `exits.tsv`.

Coverage includes gram/count/volume inputs and previous-day type prefill;
Activity error/retry/search; Stats loading, failure, empty controls, household
switching and gated stale widget responses; notification pagination, retained
rows, read/delete/all-read/refresh; export range/error/cancel/share dismissal;
successful password setup; rejected journal retry/discard across termination;
stopped timer retry across termination; schedule recurrence/end-date reload;
deletion ownership guidance and cancellation; enlarged controls and the existing
Home accessibility audit.

The opt-in `AuthTests.testLocalServerPasswordRotationAndFailedLogoutRecovery`
uses real URLSession cookies against PostgreSQL. It verifies rotated credentials,
old-session revocation, failed logout retaining its cookie and durable pending
marker, reconstruction of identity from disk, and successful logout retry.

## Reproduction and boundaries

Build the app, then `build-for-testing`, before `test-without-building`, as in the
handoff. Enable the scheme's disabled `NABU_TEST_SERVER_URL=http://localhost:8080`
test environment variable to include the real-cookie integration test. Leave the
unit host's `NABU_BASE_URL=http://localhost:9998` override enabled. The local runs
used a copied `.xctestrun` with absolute test-product paths and those variables.

The UI PNGs require the named iPhone/iOS runtime, light appearance, English and
the Mac's America/New_York timezone. Before comparing them, set simulator status
time to 9:41, Wi-Fi to three bars, and battery to charged/100% with `simctl
status_bar override`. `NABU_RECORD_SNAPSHOTS=1` in the UI test runner records new
images; remove it for verification. Date inputs use the explicit review launch
timestamp. Behavioral UI assertions still run on other OS versions; these screen
image comparisons are limited to iOS 26.

No available physical iPhone was connected. Protected-file behavior while locked,
APNs reassignment/stale actions and multi-device delivery, device background
recovery, and real VoiceOver navigation remain physical-device release checks.
Simulator cookie and relaunch evidence does not establish those device results.
