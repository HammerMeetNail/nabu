# Review records

Return one JSON object as the final response. A single json fence is acceptable;
do not surround the record with a second verdict. Source facts must be concrete.
References use checkout-relative paths, positive line numbers, and revision
`head` or `base`. Do not use wildcard paths or range strings as line numbers.
The helper checks that referenced files/lines exist; it cannot verify the fact.
Never cite a line in a file absent at the named revision. For a newly added file,
cite an existing head line and explain its absence on base in `fact` or
`base_comparison`, supported by the actual diff. Do not invent a base line 1.

A `NABU_REVIEW_PASS` response has this shape (arrays may be empty):

```json
{
  "coverage": [{
    "path": "path/from/checkout",
    "scenario": "concrete input/action sequence and result",
    "result": "candidate",
    "candidate_ids": ["access-1"],
    "evidence": [{"path": "path/from/checkout", "line": 1, "revision": "head", "fact": "decisive observed branch or value"}]
  }],
  "candidates": [{
    "id": "access-1",
    "category": "product",
    "title": "Specific observable failure",
    "priority": "P2",
    "trigger": "supported inputs/actions",
    "expected": "behavior grounded in contract",
    "actual": "actual result and user consequence",
    "base_comparison": "same base user path, or how the new path first becomes reachable",
    "evidence_kind": "static",
    "trace": [{"path": "path/from/checkout", "line": 1, "revision": "head", "fact": "causal evidence"}],
    "validation_limit": "what was not executed or remains unknown"
  }],
  "checks_run": []
}
```

- Candidate IDs start with the assigned pass name and are unique.
- `category`: `product` or `test`. A candidate can be unresolved; the audit decides.
- `priority`: `P0`, `P1`, `P2`, or `P3` (impact, not confidence).
- `coverage.result`: `candidate`, `no_issue_found`, or `unresolved`.
- Name the changed operation and concrete scenario in each coverage row. A
  `no_issue_found` row needs the decisive guard or contract in its evidence; an
  unresolved row states the missing evidence. Coverage is not a completeness proof.
- Each coverage row includes `candidate_ids`. Candidate rows name their matching
  IDs; other rows use `[]`. Every candidate must be linked from coverage. A
  scenario cannot be marked as a candidate and then omitted from the records.
- `evidence_kind`: `static` or `executed`; do not claim execution for reading code.
- `checks_run`: strings naming actual commands/outcomes, or an empty array.
- Report contract ambiguities and incomplete traces as unresolved coverage, not
  invented confirmed defects. No findings is a valid result.

A `NABU_REVIEW_AUDIT` response has this shape:

```json
{
  "decisions": [{
    "id": "access-1",
    "verdict": "confirmed",
    "reason": "causal evidence supporting the disposition",
    "trace": [{"path": "path/from/checkout", "line": 1, "revision": "head", "fact": "independently checked source fact"}],
    "finding": {
      "title": "Specific observable failure",
      "priority": "P2",
      "category": "product",
      "body": "Trigger, expected/actual result, user consequence, and evidence method. No speculative fix.",
      "location": {"path": "path/from/checkout", "line": 1, "revision": "head", "fact": "changed connection to the failure"}
    }
  }],
  "audit_questions": [],
  "checks_run": []
}
```

Every input candidate ID appears exactly once. No new IDs in decisions.
`verdict` is `confirmed`, `dismissed`, or `unresolved`. `finding` is an object
only for confirmed decisions and is null otherwise. `audit_questions` is an
array of strings describing missed/unresolved paths with source references.
If a previously confirmed candidate is dismissed, the reason needs actual
contrary source evidence. The final report is rendered from these decisions;
there is no separate model-written conclusion that can contradict them.
