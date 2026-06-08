---
description: Check PWA and iOS client parity for the current branch
agent: build
subtask: true
---

Run `bash scripts/check-parity.sh` and report the results.

Then read `docs/plans/client-parity.md` and identify any rows marked "iOS pending" or "PWA pending".

For each pending row, check whether the recent commits on this branch affect it. If so, list the specific files that need to be changed in the other client to bring it up to parity.
