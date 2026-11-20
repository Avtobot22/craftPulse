#!/usr/bin/env python3
"""Exercise the Trivy profile image without a network: version, rule fixtures, a job with the database in the scan
manifest, a missing database, a failed database update, new dependencies of a change and a timeout."""
import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PROFILE = ROOT / "backend/internal/ci/profiles/trivy-default"
IMAGE_PROFILE = "/opt/craftpulse/profiles/trivy-default"
IMAGE_CACHE = "/opt/craftpulse/trivy"
DIGEST = "sha256:" + "e" * 64
SHA = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"


def run(args, *, check=True, cwd=None):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False, cwd=cwd)
    if check and result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(args[-4:])}\n{result.stdout[-4000:]}")
    return result


def docker(image, platform, *command, mounts=(), env=None, check=True):
    args = ["docker", "run", "--rm", "--network", "none", "--user", f"{os.getuid()}:{os.getgid()}"]
    if platform:
        args += ["--platform", platform]
    for source, target in mounts:
        args += ["--mount", f"type=bind,source={source},target={target}"]
    for key, value in (env or {}).items():
        args += ["--env", f"{key}={value}"]
    return run(args + [image, *command], check=check).stdout


def check_image(image, platform, version):
    config = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config}}", image]).stdout)
    names = [entry.split("=", 1)[0] for entry in config.get("Env") or []]
    if any(word in name for name in names for word in ("TOKEN", "SECRET", "PASSWORD", "KEY")):
        raise RuntimeError("profile image carries a credential variable")
    if config.get("User") != "65532:65532":
        raise RuntimeError("profile image does not run as nonroot")
    got = docker(image, platform, "trivy", "--version").splitlines()[0].removeprefix("Version: ").strip()
    if got != version:
        raise RuntimeError(f"Trivy {got} differs from profile.json {version}")


def image_db(image, platform):
    out = docker(image, platform, "sh", "-c", f"cat {IMAGE_CACHE}/db/metadata.json; echo; cat {IMAGE_CACHE}/db/craftpulse.json")
    metadata, snapshot = (json.loads(line) for line in out.splitlines() if line.strip())
    return metadata, snapshot


def check_fixtures(image, platform):
    out = docker(image, platform, "craftpulse-ci", "fixtures", IMAGE_PROFILE + "/fixtures", "--", "craftpulse-ci", "trivy", "--fixture")
    cases = [line for line in out.splitlines() if line.startswith("fixture ")]
    want = len([d for d in (PROFILE / "fixtures").iterdir() if (d / "expected.txt").is_file()])
    if len(cases) != want or not all(": ok," in line for line in cases):
        raise RuntimeError("rule fixtures differ:\n" + out[-4000:])
    print("\n".join(cases))


def job(image, platform, root, name, repo, *trivy_args, env=None, sha=SHA):
    state = root / ("state-" + name)
    state.mkdir()
    entries = [{"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "",
                "analysis_kind": kind, "mode": "builtin", "profile_id": "trivy-default", "image_digest": DIGEST} for kind in ["sca", "iac"]]
    (state / "profile.json").write_text(json.dumps({"entries": entries}))
    job_env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": sha, "CRAFTPULSE_WORK_DIR": "/state",
               "CRAFTPULSE_URL": "https://craftpulse.example", "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token",
               "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
               "CI_COMMIT_REF_NAME": "demo/fixture", **(env or {})}
    mounts = [(repo, "/repo"), (state, "/state")]
    docker(image, platform, "craftpulse-ci", "trivy", *trivy_args, mounts=mounts, env=job_env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=job_env)
    with tarfile.open(state / "package.tar.gz") as package:
        files = {f.name: package.extractfile(f).read() for f in package if f.isfile()}
    runs = {run["report_kind"]: run for run in json.loads(files["scan-manifest.json"])["tools"]}
    reports = {kind: json.loads(files[r["reports"][0]["file"]["path"]]) if r["reports"] else None for kind, r in runs.items()}
    return runs, reports, files


def check_job(image, platform, root, version, metadata, snapshot):
    repo = root / "repo"
    repo.mkdir()
    for case in ["go", "python", "javascript", "dockerfile", "kubernetes"]:
        shutil.copytree(PROFILE / "fixtures" / case / "src", repo / case)
    for excluded in ["vendor/lib", "node_modules/dep"]:
        shutil.copytree(PROFILE / "fixtures/javascript/src", repo / excluded)
    runs, reports, files = job(image, platform, root, "full", repo)
    rules = hashlib.sha256(b"".join((PROFILE / n).read_bytes() for n in ["profile.json", "trivy.yaml", "trivyignore"])).hexdigest()
    config = hashlib.sha256((PROFILE / "profile.json").read_bytes()).hexdigest()
    for kind, tool in runs.items():
        if (tool["producer"] != "trivy" or tool["producer_version"] != version or tool["profile_image_digest"] != DIGEST
                or tool["rules_hash"] != rules or tool["config_hash"] != config or tool["completeness"] != "complete"
                or tool["exit_class"] != "findings" or tool["reports"][0]["report_format"] != "trivy_json"):
            raise RuntimeError("incomplete provenance of the " + kind + " run: " + json.dumps(tool))
        if {(t["path"], t["reason"]) for t in tool["excluded_targets"]} != {("vendor", "vendor"), ("node_modules", "vendor")}:
            raise RuntimeError("unexpected excluded targets of the " + kind + " run")
    sca, iac = runs["sca"], runs["iac"]
    published = metadata["UpdatedAt"]
    if sca["db_version"] != f"{metadata['Version']}@sha256:{snapshot['sha256']}" or not same_time(sca["db_published_at"], published):
        raise RuntimeError(f"sca run does not name the database snapshot: {sca['db_version']} {sca['db_published_at']}")
    if iac["db_version"] is not None:
        raise RuntimeError("iac run names a vulnerability database")
    vulns = [(r["Target"], v) for r in reports["sca"]["Results"] for v in r.get("Vulnerabilities", [])]
    pyyaml = [v for target, v in vulns if v["PkgName"] == "pyyaml" and target == "python/requirements.txt"]
    if not pyyaml or not pyyaml[0]["FixedVersion"] or not pyyaml[0]["VendorIDs"] or pyyaml[0]["InstalledVersion"] != "5.3.1":
        raise RuntimeError("sca report lacks the package, versions, lockfile or aliases of an advisory")
    if any(target.startswith(("vendor/", "node_modules/")) for target, _ in vulns):
        raise RuntimeError("sca report covers an excluded directory")
    misconfigs = [m for r in reports["iac"]["Results"] for m in r.get("Misconfigurations", [])]
    if not misconfigs or any("Code" in m["CauseMetadata"] or not m["CauseMetadata"].get("Resource") for m in misconfigs):
        raise RuntimeError("iac report keeps configuration lines or misses a resource")
    if b'"Lines"' in b"".join(files.values()):
        raise RuntimeError("package carries lines of configuration")
    print(f"trivy job: ok, {len(vulns)} vulnerability(ies), {len(misconfigs)} misconfiguration(s), db {sca['db_version'][:20]}... {sca['db_published_at']}")
    return repo


def same_time(a, b):
    def instant(s):
        return datetime.fromisoformat(re.sub(r"(\.\d{6})\d+", r"\1", s).replace("Z", "+00:00"))
    return instant(a) == instant(b)


def check_missing_db(image, platform, root, repo):
    empty = root / "empty-cache"
    empty.mkdir()
    # The cache directory of the job is mounted empty: the sca run must fail without a report.
    state = root / "state-missing"
    state.mkdir()
    entries = [{"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "", "analysis_kind": kind,
                "mode": "builtin", "profile_id": "trivy-default", "image_digest": DIGEST} for kind in ["sca", "iac"]]
    (state / "profile.json").write_text(json.dumps({"entries": entries}))
    env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": SHA, "CRAFTPULSE_WORK_DIR": "/state", "CRAFTPULSE_URL": "https://craftpulse.example",
           "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token", "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42",
           "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002", "CRAFTPULSE_TRIVY_CACHE": "/empty"}
    mounts = [(repo, "/repo"), (state, "/state"), (empty, "/empty")]
    docker(image, platform, "craftpulse-ci", "trivy", mounts=mounts, env=env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=env)
    with tarfile.open(state / "package.tar.gz") as package:
        manifest = json.loads(package.extractfile("scan-manifest.json").read())
    runs = {run["report_kind"]: run for run in manifest["tools"]}
    if runs["sca"]["exit_class"] != "failed" or runs["sca"]["reports"] or runs["sca"]["db_version"] is not None:
        raise RuntimeError("sca without a database must fail without a report: " + json.dumps(runs["sca"]))
    if runs["iac"]["completeness"] != "complete":
        raise RuntimeError("iac does not need the vulnerability database")
    print("trivy without database: ok, sca failed without a report, iac complete")


def check_failed_update(image, platform, metadata):
    script = (f"craftpulse-ci trivy-db --repository 127.0.0.1:9/missing/trivy-db:2; echo exit=$?; "
              f"cat {IMAGE_CACHE}/db/metadata.json; ls {IMAGE_CACHE}")
    out = docker(image, platform, "sh", "-c", script)
    lines = out.splitlines()
    kept = json.loads(next(line for line in lines if line.startswith("{")))
    if "exit=1" not in lines or kept["UpdatedAt"] != metadata["UpdatedAt"] or "db" not in lines:
        raise RuntimeError("a failed update must keep the database and fail the command:\n" + out[-2000:])
    print("trivy-db update without a network: ok, exit 1, database of " + kept["UpdatedAt"] + " kept")


def check_new_dependencies(image, platform, root):
    repo = root / "change"
    (repo / "svc").mkdir(parents=True)
    git = ["git", "-c", "user.name=craftPulse", "-c", "user.email=ci@craftpulse.invalid", "-c", "commit.gpgsign=false"]
    run(git + ["init", "-q", "-b", "main"], cwd=repo)
    (repo / "svc/requirements.txt").write_text("pyyaml==5.4\nsix==1.16.0\n")
    run(git + ["add", "-A"], cwd=repo)
    run(git + ["commit", "-q", "-m", "base"], cwd=repo)
    base = run(["git", "rev-parse", "HEAD"], cwd=repo).stdout.strip()
    (repo / "svc/requirements.txt").write_text("pyyaml==5.3.1\nsix==1.16.0\n")
    shutil.copy(PROFILE / "fixtures/javascript/src/package-lock.json", repo / "svc/package-lock.json")
    run(git + ["add", "-A"], cwd=repo)
    run(git + ["commit", "-q", "-m", "head"], cwd=repo)
    head = run(["git", "rev-parse", "HEAD"], cwd=repo).stdout.strip()
    runs, reports, _ = job(image, platform, root, "change", repo, env={"CI_COMMIT_BEFORE_SHA": base}, sha=head)
    report = reports["sca"]
    marks = {(r["Target"], p["Name"], p["Version"]): p.get("CraftPulseDelta") for r in report["Results"] for p in r.get("Packages", [])}
    want = {("svc/requirements.txt", "pyyaml", "5.3.1"): "changed", ("svc/requirements.txt", "six", "1.16.0"): None,
            ("svc/package-lock.json", "minimist", "1.2.5"): "added"}
    if report["CraftPulse"] != {"BaseSHA": base, "Delta": "computed"} or any(marks.get(k, "absent") != v for k, v in want.items()):
        raise RuntimeError(f"new dependencies are not marked: {report['CraftPulse']} {marks}")
    vulns = {v["PkgName"]: v.get("CraftPulseDelta") for r in report["Results"] for v in r.get("Vulnerabilities", [])}
    if vulns.get("minimist") != "added" or vulns.get("pyyaml") != "changed" or runs["sca"]["sha"] != head:
        raise RuntimeError(f"advisories of new dependencies are not marked: {vulns}")
    print(f"trivy new dependencies: ok, base {base[:12]} head {head[:12]}: minimist added, pyyaml changed, six unchanged")


def check_timeout(image, platform, root, repo):
    runs, _, _ = job(image, platform, root, "timeout", repo, "--timeout", "1ms")
    for kind, tool in runs.items():
        if tool["exit_class"] != "timeout" or tool["completeness"] != "partial" or tool["reports"]:
            raise RuntimeError("a profile out of time must give timeout and partial: " + kind)
    print("trivy timeout 1ms: ok")


def check(image, platform):
    version = json.loads((PROFILE / "profile.json").read_text())["tools"]["trivy"]
    check_image(image, platform, version)
    metadata, snapshot = image_db(image, platform)
    check_fixtures(image, platform)
    with tempfile.TemporaryDirectory(prefix="craftpulse-trivy-profile-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        repo = check_job(image, platform, root, version, metadata, snapshot)
        check_missing_db(image, platform, root, repo)
        check_failed_update(image, platform, metadata)
        check_new_dependencies(image, platform, root)
        check_timeout(image, platform, root, repo)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform)


if __name__ == "__main__":
    main()
