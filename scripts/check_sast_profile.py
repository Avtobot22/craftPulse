#!/usr/bin/env python3
"""Exercise the SAST profile image without a network: Opengrep version, rule fixtures, a job with an unparsed file."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PROFILE = ROOT / "backend/internal/ci/profiles/opengrep-default"
IMAGE_PROFILE = "/opt/craftpulse/profiles/opengrep-default"
DIGEST = "sha256:" + "b" * 64
SHA = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"


def run(args):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False)
    if result.returncode:
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


def check_image(image, platform, version):
    config = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config}}", image]))
    names = [entry.split("=", 1)[0] for entry in config.get("Env") or []]
    if any(word in name for name in names for word in ("TOKEN", "SECRET", "PASSWORD", "KEY")):
        raise RuntimeError("profile image carries a credential variable")
    if config.get("User") != "65532:65532":
        raise RuntimeError("profile image does not run as nonroot")
    got = docker(image, platform, "opengrep", "--version").strip()
    if got != version:
        raise RuntimeError(f"Opengrep {got} differs from profile.json {version}")


def check_fixtures(image, platform):
    out = docker(image, platform, "craftpulse-ci", "fixtures", IMAGE_PROFILE + "/fixtures", "--", "craftpulse-ci", "sast", "--fixture")
    cases = [line for line in out.splitlines() if line.startswith("fixture ")]
    if len(cases) != 3 or not all(": ok," in line for line in cases):
        raise RuntimeError("rule fixtures differ:\n" + out[-4000:])
    print("\n".join(cases))


def check_job(image, platform, root, version):
    """A repository with every fixture, a vendored copy and an unparsable file gives a partial run that names it."""
    repo = root / "repo"
    for case in ["go", "python", "javascript"]:
        shutil.copytree(PROFILE / "fixtures" / case / "src", repo / case)
    shutil.copytree(PROFILE / "fixtures/python/src", repo / "vendor/lib")
    (repo / "javascript/broken.js").write_text("export function broken( {\n  return eval(x\n")
    state = root / "state"
    state.mkdir()
    entry = {"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "go",
             "analysis_kind": "sast", "mode": "builtin", "profile_id": "opengrep-default", "image_digest": DIGEST}
    (state / "profile.json").write_text(json.dumps({"entries": [entry]}))
    env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": SHA, "CRAFTPULSE_WORK_DIR": "/state",
           "CRAFTPULSE_URL": "https://craftpulse.example", "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token",
           "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
           "CI_COMMIT_REF_NAME": "demo/fixture"}
    mounts = [(repo, "/repo"), (state, "/state")]
    docker(image, platform, "craftpulse-ci", "sast", mounts=mounts, env=env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=env)
    with tarfile.open(state / "package.tar.gz") as package:
        files = {f.name: package.extractfile(f).read() for f in package if f.isfile()}
    [tool] = json.loads(files["scan-manifest.json"])["tools"]
    if tool["producer_version"] != version or tool["profile_image_digest"] != DIGEST or not tool["rules_hash"]:
        raise RuntimeError("incomplete provenance: " + json.dumps(tool))
    if tool["completeness"] == "complete" or tool["skipped_targets"] != [{"path": "javascript/broken.js", "reason": "unsupported"}]:
        raise RuntimeError("an unparsed file must be a skipped target of a partial run: " + json.dumps(tool))
    if tool["excluded_targets"] != [{"path": "vendor", "reason": "vendor"}]:
        raise RuntimeError("unexpected excluded targets: " + json.dumps(tool["excluded_targets"]))
    report = files[tool["reports"][0]["file"]["path"]]
    sarif = json.loads(report)
    results = [(r["ruleId"], r["locations"][0]["physicalLocation"]["artifactLocation"]["uri"]) for run in sarif["runs"] for r in run["results"]]
    if len(results) != 9 or any("." in rule or path.startswith("vendor/") for rule, path in results):
        raise RuntimeError("unexpected findings: " + json.dumps(results))
    rules = {rule["id"]: rule for run in sarif["runs"] for rule in run["tool"]["driver"]["rules"]}
    if any("security-severity" not in rule["properties"] or not any(tag.startswith("CWE-") for tag in rule["properties"]["tags"])
           for rule in rules.values()):
        raise RuntimeError("a rule of the report has no CWE or severity")
    if b"snippet" in report or b"yaml.load(stream)" in report:
        raise RuntimeError("the report keeps lines of code")
    print("sast profile job: ok")


def check(image, platform):
    version = json.loads((PROFILE / "profile.json").read_text())["tools"]["opengrep"]
    check_image(image, platform, version)
    check_fixtures(image, platform)
    with tempfile.TemporaryDirectory(prefix="craftpulse-sast-profile-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        check_job(image, platform, root, version)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform)


if __name__ == "__main__":
    main()
