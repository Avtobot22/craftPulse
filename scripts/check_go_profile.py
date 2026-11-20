#!/usr/bin/env python3
"""Exercise the Go code profile image without a network: tool versions, rule fixtures, a job run and a timeout."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PROFILE = ROOT / "backend/internal/ci/profiles/go-default"
IMAGE_PROFILE = "/opt/craftpulse/profiles/go-default"
DIGEST = "sha256:" + "a" * 64
SHA = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"


def run(args, *, check=True):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False)
    if check and result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(args[-4:])}\n{result.stdout[-4000:]}")
    return result.stdout


def docker(image, platform, *command, mounts=(), env=None):
    args = ["docker", "run", "--rm", "--network", "none", "--user", f"{os.getuid()}:{os.getgid()}"]
    if platform:
        args += ["--platform", platform]
    for source, target in mounts:
        args += ["--mount", f"type=bind,source={source},target={target}"]
    for key, value in (env or {}).items():
        args += ["--env", f"{key}={value}"]
    return run(args + [image, *command])


def check_image_env(image):
    config = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config}}", image]))
    names = [entry.split("=", 1)[0] for entry in config.get("Env") or []]
    if any(word in name for name in names for word in ("TOKEN", "SECRET", "PASSWORD", "KEY")):
        raise RuntimeError("profile image carries a credential variable")
    if config.get("User") != "65532:65532":
        raise RuntimeError("profile image does not run as nonroot")


def check_versions(image, platform, tools):
    got = {
        "golangci-lint": docker(image, platform, "golangci-lint", "version", "--short").strip(),
        "lizard": docker(image, platform, "lizard", "--version").strip(),
        "jscpd": docker(image, platform, "jscpd", "--version").strip().removeprefix("jscpd "),
    }
    if got != tools:
        raise RuntimeError(f"tool versions {got} differ from profile.json {tools}")


def check_fixtures(image, platform):
    out = docker(image, platform, "craftpulse-ci", "fixtures", IMAGE_PROFILE + "/fixtures", "--",
                 "craftpulse-ci", "code", "--fixture")
    cases = [line for line in out.splitlines() if line.startswith("fixture ")]
    if len(cases) != 4 or not all(": ok," in line for line in cases):
        raise RuntimeError("rule fixtures differ:\n" + out[-4000:])
    print("\n".join(cases))


def job(image, platform, root, name, *code_args):
    """Run the profile as a job on the repository at root/repo and return the files of the package."""
    state = root / ("state-" + name)
    state.mkdir()
    entry = {"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "go",
             "analysis_kind": "code", "mode": "builtin", "profile_id": "go-default", "image_digest": DIGEST}
    (state / "profile.json").write_text(json.dumps({"entries": [entry]}))
    env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": SHA, "CRAFTPULSE_WORK_DIR": "/state",
           "CRAFTPULSE_URL": "https://craftpulse.example", "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token",
           "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
           "CI_COMMIT_REF_NAME": "demo/fixture"}
    mounts = [(root / "repo", "/repo"), (state, "/state")]
    docker(image, platform, "craftpulse-ci", "code", *code_args, mounts=mounts, env=env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=env)
    with tarfile.open(state / "package.tar.gz") as package:
        return {f.name: package.extractfile(f).read() for f in package if f.isfile()}


def check_job(image, platform, root, tools):
    repo = root / "repo"
    shutil.copytree(PROFILE / "fixtures/scope/src", repo)
    for name in ["orders.go", "invoices.go"]:
        text = (PROFILE / "fixtures/clones/src" / name).read_text()
        (repo / name).write_text(text)
    files = job(image, platform, root, "full")
    scan = json.loads(files["scan-manifest.json"])
    runs = {run["producer"]: run for run in scan["tools"]}
    if sorted(runs) != sorted(tools):
        raise RuntimeError("scan manifest does not have one run per tool")
    rules = hashlib.sha256(b"".join((PROFILE / n).read_bytes() for n in ["profile.json", "golangci.yml", "jscpd.json"])).hexdigest()
    configs = {"golangci-lint": PROFILE / "golangci.yml", "jscpd": PROFILE / "jscpd.json"}
    for producer, tool in runs.items():
        expected_config = hashlib.sha256(configs[producer].read_bytes()).hexdigest() if producer in configs else None
        if (tool["producer_version"] != tools[producer] or tool["profile_image_digest"] != DIGEST or tool["rules_hash"] != rules
                or tool["config_hash"] != expected_config or tool["completeness"] != "complete"):
            raise RuntimeError("incomplete provenance of " + producer + ": " + json.dumps(tool))
        excluded = {(t["path"], t["reason"]) for t in tool["excluded_targets"]}
        if excluded != {("vendor", "vendor"), ("build", "build"), ("api.gen.go", "generated")}:
            raise RuntimeError("unexpected excluded targets of " + producer)
        if tool["test_targets"] != ["service_test.go", "testdata/sample.go"]:
            raise RuntimeError("test code is not marked for " + producer)
    reports = {producer: files[tool["reports"][0]["file"]["path"]] for producer, tool in runs.items()}
    for producer, report in reports.items():
        for excluded in [b"vendor/", b"build/", b"api.gen.go", b".hidden"]:
            if excluded in report:
                raise RuntimeError(producer + " report covers an excluded path")
    clones = json.loads(reports["jscpd"])
    if len(clones["duplicates"]) != 1 or b"fragment" in reports["jscpd"] or b"values[" in reports["jscpd"]:
        raise RuntimeError("clone report must keep the copy coordinates without its code")
    if json.loads(reports["golangci-lint"])["version"] != "2.1.0" or csv_rows(reports["lizard"]) != 5:
        raise RuntimeError("unexpected linter or function report")
    print("go profile job: ok")


def csv_rows(csv):
    return len([line for line in csv.decode().splitlines() if line])


def check_timeout(image, platform, root):
    files = job(image, platform, root, "timeout", "--timeout", "1s")
    scan = json.loads(files["scan-manifest.json"])
    for tool in scan["tools"]:
        if tool["exit_class"] != "timeout" or tool["completeness"] != "partial" or tool["reports"]:
            raise RuntimeError("a profile out of time must give timeout and partial: " + tool["id"])
    print("go profile timeout 1s: ok")


def check(image, platform):
    tools = json.loads((PROFILE / "profile.json").read_text())["tools"]
    check_image_env(image)
    check_versions(image, platform, tools)
    check_fixtures(image, platform)
    with tempfile.TemporaryDirectory(prefix="craftpulse-go-profile-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        check_job(image, platform, root, tools)
        check_timeout(image, platform, root)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform)


if __name__ == "__main__":
    main()
