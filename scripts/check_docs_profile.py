#!/usr/bin/env python3
"""Exercise the Git documentation profile image without a network: Spectral version, rule fixtures and the
docs root of the demo dataset with 25 documents, before and after its broken anchor is fixed."""
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
PROFILE = ROOT / "backend/internal/ci/profiles/git-markdown-openapi"
IMAGE_PROFILE = "/opt/craftpulse/profiles/git-markdown-openapi"
GO_IMAGE = "golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf"
DIGEST = "sha256:" + "d" * 64
ROOT_ID = "0193a5b0-e4e4-7000-8000-000000000001"
# The broken links and the invalid specification of the docs root of demo/orders-service (demo-dataset/README.md).
BROKEN = {("missing-file", "docs/runbooks/deploy.md"), ("missing-anchor", "docs/api/overview.md")}
INVALID = {"docs/api/legacy-quotes.openapi.yaml"}


def run(args, *, cwd=None):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False, cwd=cwd)
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


def check_image(image, platform, tools):
    config = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config}}", image]))
    names = [entry.split("=", 1)[0] for entry in config.get("Env") or []]
    if any(word in name for name in names for word in ("TOKEN", "SECRET", "PASSWORD", "KEY")):
        raise RuntimeError("profile image carries a credential variable")
    if config.get("User") != "65532:65532":
        raise RuntimeError("profile image does not run as nonroot")
    got = docker(image, platform, "spectral", "--version").strip()
    if got != tools["spectral"]:
        raise RuntimeError(f"Spectral {got} differs from profile.json {tools['spectral']}")
    out = docker(image, platform, "craftpulse-ci", "fixtures", IMAGE_PROFILE + "/fixtures", "--", "craftpulse-ci", "docs", "--fixture")
    cases = [line for line in out.splitlines() if line.startswith("fixture ")]
    want = len([d for d in (PROFILE / "fixtures").iterdir() if (d / "expected.txt").is_file()])
    if len(cases) != want or not all(": ok," in line for line in cases):
        raise RuntimeError("rule fixtures differ:\n" + out[-4000:])
    print("\n".join(cases))


def dataset_repo(root):
    """Generate the demo dataset and clone demo/orders-service, whose docs root has 25 documents."""
    out = root / "dataset"
    out.mkdir()
    # The generator needs the Go module proxy, so this one container keeps the network.
    args = ["docker", "run", "--rm", "--user", f"{os.getuid()}:{os.getgid()}", "-e", "HOME=/tmp", "-e", "GOCACHE=/tmp/gocache",
            "-e", "GOPATH=/tmp/gopath", "-v", f"{ROOT / 'demo-dataset'}:/src:ro", "-v", f"{out}:/out", "-w", "/src", GO_IMAGE,
            "go", "run", "./cmd/dataset", "generate", "--out", "/out/model.json", "--repo-dir", "/out/repos"]
    run(args)
    repo = root / "repo"
    run(["git", "clone", "-q", str(out / "repos/repos/org-1/gitlab-1/repo-1.git"), str(repo)])
    return repo


def job(image, platform, root, name, repo):
    state = root / ("state-" + name)
    state.mkdir()
    sha = run(["git", "rev-parse", "HEAD"], cwd=repo).strip()
    entries = [{"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": "", "analysis_kind": kind,
                "mode": "builtin", "profile_id": "git-markdown-openapi", "image_digest": DIGEST} for kind in ["docs", "openapi"]]
    roots = [{"root_id": ROOT_ID, "kind": kind, "path_prefix": "docs"} for kind in ["docs", "openapi"]]
    (state / "profile.json").write_text(json.dumps({"entries": entries, "documentation_roots": roots}))
    env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": sha, "CRAFTPULSE_WORK_DIR": "/state", "CRAFTPULSE_URL": "https://craftpulse.example",
           "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token", "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42",
           "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002", "CI_COMMIT_REF_NAME": "main"}
    mounts = [(repo, "/repo"), (state, "/state")]
    docker(image, platform, "craftpulse-ci", "docs", mounts=mounts, env=env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=env)
    with tarfile.open(state / "package.tar.gz") as package:
        files = {f.name: package.extractfile(f).read() for f in package if f.isfile()}
    runs = {run["report_kind"]: run for run in json.loads(files["scan-manifest.json"])["tools"]}
    reports = {kind: {r["report_format"]: json.loads(files[r["file"]["path"]]) for r in tool["reports"]} for kind, tool in runs.items()}
    return sha, runs, reports


def findings(sarif):
    return {(r["ruleId"], r["locations"][0]["physicalLocation"]["artifactLocation"]["uri"]) for run in sarif["runs"] for r in run["results"]}


def check_dataset(image, platform, root, tools):
    repo = dataset_repo(root)
    files = sorted(str(p.relative_to(repo)) for p in (repo / "docs").rglob("*") if p.is_file())
    unowned = sorted(f for f in files if f.endswith(".md") and "\nowner:" not in (repo / f).read_text())
    sha, runs, reports = job(image, platform, root, "dataset", repo)
    rules = hashlib.sha256((PROFILE / "profile.json").read_bytes() + (PROFILE / "spectral.yaml").read_bytes()).hexdigest()
    for kind, tool in runs.items():
        producer, version = ("spectral", tools["spectral"]) if kind == "openapi" else ("craftpulse-docs", tools["craftpulse-docs"])
        if (tool["producer"], tool["producer_version"]) != (producer, version) or tool["profile_image_digest"] != DIGEST \
                or tool["rules_hash"] != rules or tool["sha"] != sha or tool["completeness"] != "complete":
            raise RuntimeError("incomplete provenance of the " + kind + " run: " + json.dumps(tool))
    inventory = reports["docs"]["docs_json"]["documents"]
    paths = sorted(d["document"]["path"] for d in inventory)
    if len(files) != 25 or paths != files or runs["docs"]["scanned_count"] != 25:
        raise RuntimeError(f"inventory {len(paths)} documents differs from the 25 files of the root")
    for d in inventory:
        doc = d["document"]
        if doc["version_ref"] != sha or doc["content_sha256"] != hashlib.sha256((repo / doc["path"]).read_bytes()).hexdigest():
            raise RuntimeError("inventory entry without the SHA or content hash: " + doc["path"])
    got_unowned = sorted(d["document"]["path"] for d in inventory if d["document"]["doc_format"] == "markdown" and d["owner"] is None)
    if got_unowned != unowned or len(unowned) != 5:
        raise RuntimeError(f"documents without an owner {got_unowned} differ from {unowned}")
    links = reports["docs"]["docs_json"]["links"]
    if findings(reports["docs"]["sarif"]) != BROKEN or links["broken"] != 2:
        raise RuntimeError(f"broken links {findings(reports['docs']['sarif'])} differ from {BROKEN}")
    specs = reports["openapi"]["docs_json"]["documents"]
    if {d["document"]["path"] for d in specs if not d["spec_valid"]} != INVALID or len(specs) != 3:
        raise RuntimeError("specification validity differs from the dataset")
    if not any(rule == "oas3-schema" and path in INVALID for rule, path in findings(reports["openapi"]["sarif"])):
        raise RuntimeError("the invalid specification has no schema finding")
    print(f"docs root of demo/orders-service: ok, {len(paths)} documents, {len(unowned)} without owner, "
          f"{links['checked']} local links, 2 broken, invalid {sorted(INVALID)[0]}")
    return repo


def check_fixed_anchor(image, platform, root, repo):
    fixed = root / "fixed"
    shutil.copytree(repo, fixed)
    overview = fixed / "docs/api/overview.md"
    overview.write_text(overview.read_text().replace("(errors.md#rate-limits)", "(errors.md#rate-limiting)"))
    _, _, reports = job(image, platform, root, "fixed", fixed)
    if findings(reports["docs"]["sarif"]) != {("missing-file", "docs/runbooks/deploy.md")}:
        raise RuntimeError("the fixed anchor still gives a finding")
    print("docs root with the fixed anchor: ok, only the missing file remains")


def check(image, platform):
    tools = json.loads((PROFILE / "profile.json").read_text())["tools"]
    check_image(image, platform, tools)
    with tempfile.TemporaryDirectory(prefix="craftpulse-docs-profile-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        repo = check_dataset(image, platform, root, tools)
        check_fixed_anchor(image, platform, root, repo)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform)


if __name__ == "__main__":
    main()
