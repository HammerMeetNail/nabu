#!/usr/bin/env python3
"""Run bounded source reviews and an evidence audit with the configured model.

Record validation checks structure, source locations, and candidate accounting.
It does not establish the truth of an LLM's interpretation.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time


PASSES = ("access", "behavior", "state", "tests")
PRIORITIES = ("P0", "P1", "P2", "P3")


class ReviewInterrupted(RuntimeError):
    pass


def interrupt(signum: int, _frame: object) -> None:
    raise ReviewInterrupted(f"Received signal {signum}")


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n")


def command(argv: list[str], cwd: Path | None = None) -> str:
    result = subprocess.run(argv, cwd=cwd, capture_output=True, text=True, check=True)
    return result.stdout


def git(repo: Path, *args: str) -> str:
    return command(["git", "-C", str(repo), "--no-pager", *args])


def parse_record(text: str) -> dict:
    value = text.strip()
    fences = list(re.finditer(r"(?m)^```([^\n`]*)\n([\s\S]*?)^```[ \t]*(?:\n|$)", value))
    if fences:
        records = []
        for fence in fences:
            language, content = fence.group(1).strip().lower(), fence.group(2).strip()
            try:
                parsed = json.loads(content)
            except json.JSONDecodeError:
                # Ignore ordinary fenced source examples, but never select a
                # valid record over another malformed JSON-looking record.
                if language == "json" or content.startswith(("{", "[")):
                    raise ValueError("Response contains a malformed structured record") from None
                continue
            if language == "json" or isinstance(parsed, (dict, list)):
                records.append(content)
        if len(records) != 1:
            raise ValueError("Response contains multiple structured records")
        outside, end = "", 0
        for fence in fences:
            outside += value[end:fence.start()]
            end = fence.end()
        outside += value[end:]
        if any(line.lstrip().startswith(("{", "[")) for line in outside.splitlines()):
            raise ValueError("Response contains another possible structured record")
        value = records[0]
    result = json.loads(value)
    if not isinstance(result, dict):
        raise ValueError("Record must be a JSON object")
    return result


def nonempty(value: object, label: str) -> None:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"Missing nonempty {label}")


def array(value: object, label: str) -> list:
    if not isinstance(value, list):
        raise ValueError(f"{label} must be an array")
    return value


class Source:
    def __init__(self, repo: Path, base: str, head: str):
        self.repo, self.revisions = repo, {"base": base, "head": head}
        self.cache: dict[tuple[str, str], list[str]] = {}

    def reference(self, ref: dict) -> None:
        if not isinstance(ref, dict):
            raise ValueError("Source reference must be an object")
        path, revision, line = ref.get("path"), ref.get("revision"), ref.get("line")
        nonempty(path, "reference path")
        if Path(path).is_absolute() or ".." in Path(path).parts or "\0" in path:
            raise ValueError(f"Invalid checkout-relative path: {path!r}")
        if revision not in self.revisions or type(line) is not int or line < 1:
            raise ValueError(f"Invalid source revision/line: {ref}")
        key = (revision, path)
        if key not in self.cache:
            self.cache[key] = git(self.repo, "show", f"{self.revisions[revision]}:{path}").splitlines()
        if line > len(self.cache[key]):
            raise ValueError(f"Source line does not exist: {revision}:{path}:{line}")
        nonempty(ref.get("fact"), "source fact")

    def trace(self, refs: object) -> None:
        values = array(refs, "source trace")
        if not values:
            raise ValueError("Source trace must contain evidence")
        for ref in values:
            self.reference(ref)


def validate_pass(record: dict, name: str, source: Source) -> set[str]:
    coverage = array(record.get("coverage"), "coverage")
    if not coverage:
        raise ValueError("Pass must account for at least one inspected path")
    covered: set[str] = set()
    for row in coverage:
        nonempty(row.get("path"), "coverage path")
        nonempty(row.get("scenario"), "coverage scenario")
        if row.get("result") not in ("candidate", "no_issue_found", "unresolved"):
            raise ValueError("Invalid coverage result")
        source.trace(row.get("evidence"))
        row_ids = array(row.get("candidate_ids"), "coverage candidate_ids")
        if any(not isinstance(cid, str) or not cid for cid in row_ids):
            raise ValueError("Coverage candidate IDs must be nonempty strings")
        if row["result"] == "candidate" and not row_ids:
            raise ValueError("Candidate coverage must identify its candidate records")
        if row["result"] != "candidate" and row_ids:
            raise ValueError("Only candidate coverage may list candidate IDs")
        covered.update(row_ids)
    ids: set[str] = set()
    for item in array(record.get("candidates"), "candidates"):
        cid = item.get("id")
        nonempty(cid, "candidate ID")
        if not cid.startswith(name + "-") or cid in ids:
            raise ValueError(f"Invalid or duplicate candidate ID: {cid}")
        ids.add(cid)
        for field in ("title", "trigger", "expected", "actual", "base_comparison", "validation_limit"):
            nonempty(item.get(field), field)
        if item.get("category") not in ("product", "test"):
            raise ValueError("Invalid candidate category")
        if item.get("priority") not in PRIORITIES:
            raise ValueError("Invalid priority")
        if item.get("evidence_kind") not in ("static", "executed"):
            raise ValueError("Invalid evidence kind")
        source.trace(item.get("trace"))
    if covered != ids:
        raise ValueError(f"Coverage/candidate mismatch: coverage={sorted(covered)}, candidates={sorted(ids)}")
    for value in array(record.get("checks_run"), "checks_run"):
        nonempty(value, "check")
    return ids


def validate_audit(record: dict, expected_ids: set[str], source: Source) -> None:
    found: set[str] = set()
    for decision in array(record.get("decisions"), "decisions"):
        cid = decision.get("id")
        if cid not in expected_ids or cid in found:
            raise ValueError(f"Unknown or duplicate audit ID: {cid}")
        found.add(cid)
        verdict = decision.get("verdict")
        if verdict not in ("confirmed", "dismissed", "unresolved"):
            raise ValueError("Invalid audit verdict")
        nonempty(decision.get("reason"), "audit reason")
        source.trace(decision.get("trace"))
        finding = decision.get("finding")
        if verdict == "confirmed":
            if not isinstance(finding, dict):
                raise ValueError("Confirmed candidate requires a finding")
            for key in ("title", "body"):
                nonempty(finding.get(key), "finding " + key)
            if finding.get("priority") not in PRIORITIES or finding.get("category") not in ("product", "test"):
                raise ValueError("Invalid finding priority/category")
            source.reference(finding.get("location"))
        elif finding is not None:
            raise ValueError("Only confirmed candidates may have final findings")
    if found != expected_ids:
        raise ValueError(f"Audit omitted candidates: {sorted(expected_ids - found)}")
    for key in ("audit_questions", "checks_run"):
        for value in array(record.get(key), key):
            nonempty(value, key)


def select_passes(paths: list[str]) -> list[str]:
    code = [p for p in paths if p.endswith((".go", ".js", ".ts", ".swift", ".sql", ".html", ".css"))]
    if not code:
        return ["behavior", "tests"]
    selected = ["behavior", "tests"]
    if any(p.startswith(("internal/", "migrations/", "cmd/")) for p in code):
        selected.append("access")
    if any(p.startswith(("web/", "ios/", "internal/")) for p in code):
        selected.append("state")
    return [p for p in PASSES if p in selected]


def verify_cli(cli: str) -> None:
    """Reject incompatible export protocols before spending a model turn."""
    help_result = subprocess.run([cli, "session", "--help"], capture_output=True,
                                 text=True, timeout=15, check=True)
    if not re.search(r"(?im)^\s*export\s+Export session data", help_result.stdout):
        raise RuntimeError("This runner requires the OpenCode2 session/export protocol. "
                           "Use opencode2 or a compatible v2 alias. Older OpenCode can "
                           "use the skill's native-host workflow instead.")


def session_runtime(session: dict) -> tuple[list[str], dict | None]:
    assistants = [m for m in session["messages"] if m.get("type") == "assistant"]
    if not assistants:
        raise RuntimeError("Session has no assistant messages")
    last = assistants[-1]
    if (last.get("error") or last.get("finish") != "stop" or
            not last.get("time", {}).get("completed")):
        raise RuntimeError("Final assistant turn is not normally completed; review is incomplete")
    models = sorted({m["model"]["providerID"] + "/" + m["model"]["id"] for m in assistants})
    return models, session["info"].get("tokens")


def run_model(cli: str, repo: Path, out: Path, name: str, prompt: str,
              body: str, expect_model: str | None, timeout: int, reuse: bool = False) -> dict:
    verify_cli(cli)
    directory = out / name
    task = directory / "prompt.md"
    argv = [cli, "run", "--agent", "plan", "--format", "json", "--title",
            f"Nabu review {name}", "--file", str(task),
            "Perform the attached bounded review task. Load nabu-pr-review with the skill tool. "
            "Use the configured default model and return the required JSON record."]
    started = time.monotonic()
    if directory.exists():
        if not reuse or task.read_text() != prompt or json.loads((directory / "command.json").read_text()) != argv:
            raise RuntimeError(f"{name}: refusing to overwrite or reuse different task")
        terminal = json.loads((directory / "exit.json").read_text()) if (directory / "exit.json").exists() else {}
        if terminal.get("exit_code") != 0 or terminal.get("status") == "interrupted":
            raise RuntimeError(f"{name}: cannot resume unfinished/failed model process; inspect server session first")
        print(f"REUSE {name}: completed original model response, no inference retry", flush=True)
    else:
        directory.mkdir(mode=0o700)
        task.write_text(prompt)
        write_json(directory / "command.json", argv)
        print(f"START {name}", flush=True)
        with (directory / "events.jsonl").open("w") as stdout, (directory / "stderr.log").open("w") as stderr:
            proc = subprocess.Popen(argv, cwd=repo, stdout=stdout, stderr=stderr, start_new_session=True)
            try:
                pid_tmp = directory / "pid.tmp"
                pid_tmp.write_text(str(proc.pid))
                pid_tmp.replace(directory / "pid")
                code = proc.wait(timeout=timeout)
            except BaseException:
                try:
                    os.killpg(proc.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.wait()
                write_json(directory / "exit.json", {"status": "interrupted", "exit_code": proc.returncode,
                           "note": "Client stopped; shared server was not terminated. Check server session before any retry."})
                raise
        write_json(directory / "exit.json", {"status": "completed", "exit_code": code, "elapsed_seconds": round(time.monotonic() - started, 2)})
        if code:
            raise RuntimeError(f"{name}: OpenCode exited {code}; inspect stderr.log")
    events = [json.loads(line) for line in (directory / "events.jsonl").read_text().splitlines() if line.strip()]
    loads = [event.get("part", {}).get("state", {}).get("output", "") for event in events
             if event.get("part", {}).get("tool") == "skill"]
    if not any(isinstance(value, str) and body in value for value in loads):
        raise RuntimeError(f"{name}: exact installed skill body not observed in native tool output")
    texts = [event["part"]["text"] for event in events if event.get("type") == "text"]
    if not texts:
        raise RuntimeError(f"{name}: no completed text response")
    (directory / "response.md").write_text(texts[-1] + "\n")
    sid = next((event["sessionID"] for event in events if event.get("sessionID")), None)
    if not sid:
        raise RuntimeError(f"{name}: missing session ID")
    # OpenCode2 can exit before a large piped stdout export drains. A regular
    # output file preserves the complete export; validate it before accepting.
    with (directory / "session.json").open("w") as exported:
        subprocess.run([cli, "session", "export", sid], stdout=exported,
                       stderr=subprocess.PIPE, text=True, check=True)
    session = json.loads((directory / "session.json").read_text())
    models, tokens = session_runtime(session)
    if not models or (expect_model and models != [expect_model]):
        raise RuntimeError(f"{name}: unexpected actual models: {models}")
    write_json(directory / "runtime.json", {"session_id": sid, "actual_models": models,
               "tokens": tokens, "exact_native_skill_body": True, "final_assistant_completed": True})
    record = parse_record(texts[-1])
    write_json(directory / "record.json", record)
    print(f"COMPLETE {name}: {round(time.monotonic() - started, 1)}s", flush=True)
    return record


def render(audit: dict, records: dict, base: str, head: str) -> str:
    confirmed = [d for d in audit["decisions"] if d["verdict"] == "confirmed"]
    confirmed.sort(key=lambda d: PRIORITIES.index(d["finding"]["priority"]))
    unresolved = [f"{d['id']}: {d['reason']}" for d in audit["decisions"] if d["verdict"] == "unresolved"]
    unresolved += audit["audit_questions"]
    for name, record in records.items():
        unresolved += [f"{name}, {c['path']}: {c['scenario']}" for c in record["coverage"] if c["result"] == "unresolved"]
    lines = ["# Nabu review", "", f"Base `{base}`; head `{head}`.", "",
             f"Audit classified {len(confirmed)} findings and retained {len(unresolved)} unresolved items.", "",
             "These are model-reviewed claims, not guarantees established by the record validator.", ""]
    for d in confirmed:
        f, loc = d["finding"], d["finding"]["location"]
        lines += [f"## [{f['priority']}] {f['title']} ({f['category']}; {d['id']})", "", f["body"], "",
                  f"Source: `{loc['path']}:{loc['line']}` ({loc['revision']}).", ""]
    lines += ["## Unresolved / limits", ""]
    lines += [f"- {item}" for item in unresolved] if unresolved else ["No unresolved entries were recorded; this is not proof of completeness."]
    limits = sorted({c["validation_limit"] for record in records.values() for c in record["candidates"]})
    lines += ["", *[f"- {value}" for value in limits], "", "## Checks actually reported", ""]
    checks = [check for record in [*records.values(), audit] for check in record["checks_run"]]
    lines += [f"- {check}" for check in checks] if checks else ["Static source review; no runtime checks reported."]
    lines += ["", "All candidate dispositions and supporting traces are retained in audit.json.", ""]
    return "\n".join(lines)


def main() -> int:
    for signum in {signal.SIGTERM, getattr(signal, "SIGHUP", signal.SIGTERM)}:
        signal.signal(signum, interrupt)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", required=True)
    parser.add_argument("--intent", required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--cli", default="opencode2", help="OpenCode2/v2 executable or compatible alias; older OpenCode uses the native skill workflow")
    parser.add_argument("--expect-model")
    parser.add_argument("--passes", help="Comma-separated focused passes; a source-only omission pass always runs first")
    parser.add_argument("--timeout", type=int, default=900, help="Per-client process deadline in seconds")
    parser.add_argument("--prepare-only", action="store_true", help="Prepare scope/manifest without invoking a model")
    parser.add_argument("--resume", action="store_true", help="Revalidate and reuse completed original responses; never retry a failed model process")
    args = parser.parse_args()
    repo, out = args.repo.resolve(), args.out.resolve()
    skill = Path(__file__).resolve().parents[1]
    base = git(repo, "rev-parse", "--verify", "--end-of-options", args.base + "^{commit}").strip()
    head = git(repo, "rev-parse", "--verify", "--end-of-options", args.head + "^{commit}").strip()
    if git(repo, "rev-parse", "HEAD").strip() != head:
        parser.error("Review checkout HEAD must match --head")
    before = git(repo, "status", "--porcelain=v1", "--untracked-files=all")
    if before:
        parser.error("Use a clean review worktree")
    common = Path(git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir").strip()).parent
    if out.is_relative_to(repo) or out.is_relative_to(common):
        parser.error("Output must be outside the repository")
    paths = git(repo, "-c", "core.quotePath=false", "diff", "--no-ext-diff", "--name-only", base, head, "--").splitlines()
    chosen = args.passes.split(",") if args.passes else select_passes(paths)
    if not chosen or len(set(chosen)) != len(chosen) or any(p not in PASSES for p in chosen):
        parser.error("Invalid --passes")
    if args.timeout < 1:
        parser.error("--timeout must be positive")
    hashes = {str(p.relative_to(skill)): hashlib.sha256(p.read_bytes()).hexdigest()
              for p in sorted(skill.rglob("*")) if p.is_file() and "__pycache__" not in p.parts}
    manifest = {"status": "prepared", "repo": str(repo), "base": base, "head": head,
                "intent": args.intent, "changed_files": paths, "passes": ["omissions", *chosen],
                "skill_files": hashes, "model_override": False, "cli": args.cli}
    if args.resume:
        previous = json.loads((out / "manifest.json").read_text())
        for key in ("repo", "base", "head", "intent", "changed_files", "passes", "cli"):
            if previous.get(key) != manifest[key]:
                parser.error(f"Resume scope changed: {key}")
        old_guidance = {k: v for k, v in previous["skill_files"].items() if not k.startswith("scripts/")}
        new_guidance = {k: v for k, v in hashes.items() if not k.startswith("scripts/")}
        if old_guidance != new_guidance:
            parser.error("Resume requires unchanged skill guidance")
        n = len(list(out.glob("manifest-before-resume-*.json"))) + 1
        write_json(out / f"manifest-before-resume-{n}.json", previous)
        manifest["resumed"] = True
    else:
        out.mkdir(mode=0o700, parents=True, exist_ok=False)
    write_json(out / "manifest.json", manifest)
    if args.prepare_only:
        print(f"Prepared {out}")
        return 0
    source, records = Source(repo, base, head), {}
    skill_text = (skill / "SKILL.md").read_text()
    body = skill_text.split("---", 2)[2].strip()
    scope = (f"Repository: {repo}\nBase: {base}\nHead: {head}\nIntended behavior: {args.intent}\n"
             + "Changed files:\n" + "\n".join(paths) + "\n\n"
             "Load installed nabu-pr-review with the native skill tool. Read repository instructions. "
             "This is a read-only static review. Do not edit source, run builds/tests, install packages, "
             "start services, mutate databases, delegate, switch model, commit, push or post. "
             "Do not checkout, switch, reset, restore, or stash the review worktree; inspect revisions with git show/diff. "
             "Pure calculations and source-reading commands are allowed. Do not read prior reviews, "
             "evaluator files, temporary probes, or sibling task outputs except the "
             "candidate records explicitly attached to an audit. No expected findings are supplied.\n")
    try:
        manifest["status"] = "running"
        write_json(out / "manifest.json", manifest)
        for name in manifest["passes"]:
            prompt = (f"NABU_REVIEW_PASS {name}\n\n" + scope + "\n"
                      "The relevant skill references are attached below; use their text directly "
                      "without reading files outside the checkout. Focus on this pass only; trace its "
                      "coupled paths to a supported result. Return the pass JSON record. "
                      f"Candidate IDs must start with {name}-. Do not propose fixes.\n\n"
                      + (skill / "references/discovery.md").read_text() + "\n\n"
                      + (skill / "references" / (name + ".md")).read_text() + "\n\n"
                      + (skill / "references/record-format.md").read_text())
            record = run_model(args.cli, repo, out, name, prompt, body, args.expect_model, args.timeout, args.resume)
            validate_pass(record, name, source)
            records[name] = record
            if (git(repo, "rev-parse", "HEAD").strip() != head or
                    git(repo, "status", "--porcelain=v1", "--untracked-files=all") != before):
                raise RuntimeError(f"Source HEAD/status changed during {name}; stop and inspect, do not restore automatically")
        write_json(out / "candidates.json", records)
        expected_ids = {c["id"] for record in records.values() for c in record["candidates"]}
        prompt = ("NABU_REVIEW_AUDIT\n\n" + scope + "\n"
                  "The relevant skill references are attached below; use their text directly "
                  "without reading files outside the checkout. "
                  "Independently audit every candidate and the coverage gaps against source. "
                  "Return the audit JSON record; no speculative fixes.\n\n"
                  + (skill / "references/discovery.md").read_text() + "\n\n"
                  + (skill / "references/audit.md").read_text() + "\n\n"
                  + (skill / "references/record-format.md").read_text() + "\n\nCandidate records:\n"
                  + json.dumps(records, indent=2, ensure_ascii=False))
        audit = run_model(args.cli, repo, out, "audit", prompt, body, args.expect_model, args.timeout, args.resume)
        validate_audit(audit, expected_ids, source)
        if (git(repo, "rev-parse", "HEAD").strip() != head or
                git(repo, "status", "--porcelain=v1", "--untracked-files=all") != before):
            raise RuntimeError("Source HEAD/status changed during audit; stop and inspect")
        write_json(out / "audit.json", audit)
        (out / "review.md").write_text(render(audit, records, base, head))
        manifest.update(status="completed", source_unchanged=True,
                        confirmed=sum(d["verdict"] == "confirmed" for d in audit["decisions"]))
        write_json(out / "manifest.json", manifest)
        print(f"COMPLETE review: {out / 'review.md'}", flush=True)
        return 0
    except BaseException as exc:
        manifest.update(status="incomplete", error=f"{type(exc).__name__}: {exc}")
        write_json(out / "manifest.json", manifest)
        raise


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, RuntimeError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
        print(f"Review incomplete: {exc}", file=sys.stderr)
        raise SystemExit(1)
