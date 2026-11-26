#!/usr/bin/env python3
"""Run one Go integration shard and collect comparable JSON timings."""
import argparse
import json
import math
from pathlib import Path
import re
import statistics
import subprocess
import tempfile
import time


def assign(names, weights, count):
    if len(set(names)) != len(names) or not names or count < 1 or count > len(names):
        raise ValueError("Invalid test inventory or shard count")
    valid = {name: seconds for name, seconds in weights.items()
             if isinstance(seconds, (int, float)) and math.isfinite(seconds) and seconds > 0}
    fallback = max(statistics.median(valid.values()), 0.01) if valid else 1.0
    groups = [[] for _ in range(count)]
    totals = [0.0] * count
    for name in sorted(names, key=lambda name: (-valid.get(name, fallback), name)):
        index = min(range(count), key=lambda i: (totals[i], len(groups[i]), i))
        groups[index].append(name)
        totals[index] += valid.get(name, fallback)
    return groups


def read_weights(path):
    if path is None:
        return {}
    record = json.loads(path.read_text(encoding="utf-8"))
    return record["tests"]


def run_shard(args):
    if not 0 <= args.index < args.count:
        raise ValueError("Shard index must be smaller than the shard count")
    module = Path.cwd()
    package_dir = (module / args.package).resolve()
    package = subprocess.check_output(["go", "list", args.package], text=True).strip()
    with tempfile.TemporaryDirectory() as work:
        binary = Path(work) / "integration.test"
        started = time.monotonic()
        vet = "all" if args.index == 0 else "off"
        subprocess.run(["go", "test", "-c", "-race", "-vet=" + vet, "-o", str(binary), args.package],
                       check=True)
        inventory = subprocess.check_output([str(binary), "-test.list=."], cwd=package_dir,
                                            text=True).splitlines()
        if any(not re.fullmatch(r"(?:(?:Test|Fuzz|Example)\w*)", name) for name in inventory):
            raise ValueError("Unexpected integration test inventory")
        members = assign(inventory, read_weights(args.profile), args.count)[args.index]
        pattern = "^(" + "|".join(re.escape(name) for name in members) + ")$"
        build_seconds = time.monotonic() - started
        started = time.monotonic()
        outcomes = {}
        with subprocess.Popen(
            ["go", "tool", "test2json", "-t", "-p", package, str(binary), "-test.v=test2json",
             "-test.timeout=10m", "-test.paniconexit0", "-test.run=" + pattern],
            cwd=package_dir, stdout=subprocess.PIPE, text=True,
        ) as run:
            for line in run.stdout:
                event = json.loads(line)
                if event["Action"] == "output":
                    print(event["Output"], end="", flush=True)
                test = event.get("Test")
                if test and "/" not in test and event["Action"] in {"pass", "skip", "fail"}:
                    outcomes[test] = event
        test_seconds = time.monotonic() - started
        if run.returncode:
            return run.returncode
    if set(members) != set(outcomes) or any(event["Action"] == "fail" for event in outcomes.values()):
        raise ValueError("Executed test inventory differs from the shard plan")
    result = {"count": args.count, "index": args.index, "build_seconds": build_seconds,
              "test_seconds": test_seconds,
              "tests": {name: outcome.get("Elapsed", 0) for name, outcome in sorted(outcomes.items())}}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(f"Shard {args.index + 1}/{args.count}: {len(members)} tests, "
          f"build {build_seconds:.1f}s, tests {test_seconds:.1f}s")
    return 0


def report(args):
    variants = {}
    for file in sorted(args.directory.rglob("shard.json")):
        item = json.loads(file.read_text(encoding="utf-8"))
        variants.setdefault(item["count"], []).append(item)
    if set(variants) != {1, 2, 4}:
        raise ValueError("Benchmark must contain 1, 2 and 4 shards")
    baseline = set(variants[1][0]["tests"])
    print("| Shards | Tests | Critical test time (s) | Build + test critical time (s) | Total build + test time (s) |")
    print("|---:|---:|---:|---:|---:|")
    for count, jobs in sorted(variants.items()):
        names = [name for job in jobs for name in job["tests"]]
        if sorted(job["index"] for job in jobs) != list(range(count)):
            raise ValueError(f"Missing or duplicate shard in variant {count}")
        if set(names) != baseline or len(names) != len(baseline):
            raise ValueError(f"Test coverage mismatch in variant {count}")
        elapsed = [job["build_seconds"] + job["test_seconds"] for job in jobs]
        print(f"| {count} | {len(names)} | {max(job['test_seconds'] for job in jobs):.1f} | "
              f"{max(elapsed):.1f} | {sum(elapsed):.1f} |")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(required=True)
    run = subparsers.add_parser("run")
    run.add_argument("--package", default="./internal/app")
    run.add_argument("--count", type=int, default=1)
    run.add_argument("--index", type=int, default=0)
    run.add_argument("--profile", type=Path)
    run.add_argument("--output", type=Path, required=True)
    run.set_defaults(handler=run_shard)
    result = subparsers.add_parser("report")
    result.add_argument("directory", type=Path)
    result.set_defaults(handler=report)
    args = parser.parse_args()
    return args.handler(args)


if __name__ == "__main__":
    raise SystemExit(main())
