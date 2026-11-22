#!/usr/bin/env python3
"""Exercise a code profile image without a network: tool versions, rule fixtures, a job run with dependency,
build and broken files, a timeout and the base and head commits of a fixture project."""
import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PROFILES = ROOT / "backend/internal/ci/profiles"
PROJECTS = ROOT / "backend/internal/ci/testdata/code-projects"
DIGEST = "sha256:" + "a" * 64
SHA = "d2126fca9ecbc14c5b15c22bdd35268aabb5cdac"
LINTERS = {"go": ("golangci-lint", "golangci.yml"), "python": ("ruff", "ruff.toml"), "javascript": ("eslint", "eslint.config.mjs")}
# Files the job run adds to the repository: dependencies the profile must exclude and a file the linter cannot
# parse. The repository check forbids node_modules and .venv in Git, so they exist only here.
EXTRA = {
    "go": {"files": {}, "excluded": set(), "broken": None},
    "python": {"files": {"venv/pyvenv.cfg": "home = /usr/bin\n", "venv/lib/python3.12/site-packages/dep.py": "import os\n",
                         ".venv/lib/dep.py": "import os\n", "broken.py": "import os\n\n\ndef broken(:\n    pass\n"},
               "excluded": {("venv", "vendor")}, "broken": "broken.py"},
    "javascript": {"files": {"node_modules/dep/index.js": "export const unused = 1;\n",
                             "broken.ts": "export function broken(value: number {\n  return value;\n}\n"},
                   "excluded": {("node_modules", "vendor")}, "broken": "broken.ts"},
}


def run(args, *, check=True, cwd=None):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, check=False, cwd=cwd)
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


class Profile:
    def __init__(self, name):
        self.name = name
        self.dir = PROFILES / name
        self.image_dir = "/opt/craftpulse/profiles/" + name
        self.spec = json.loads((self.dir / "profile.json").read_text())
        self.tools = self.spec["tools"]
        self.language = self.spec["language"]
        self.linter, self.config = LINTERS[self.language]
        self.rules_hash = hashlib.sha256(b"".join((self.dir / n).read_bytes() for n in ["profile.json", self.config, "jscpd.json"])).hexdigest()

    def expected(self, case, kind):
        lines = (self.dir / "fixtures" / case / "expected.txt").read_text().splitlines()
        return [line.split()[1:] for line in lines if line.startswith(kind + " ")]


def check_image_env(image):
    config = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config}}", image]))
    names = [entry.split("=", 1)[0] for entry in config.get("Env") or []]
    if any(word in name for name in names for word in ("TOKEN", "SECRET", "PASSWORD", "KEY")):
        raise RuntimeError("profile image carries a credential variable")
    if config.get("User") != "65532:65532":
        raise RuntimeError("profile image does not run as nonroot")


def check_versions(image, platform, p):
    def node_version(package):
        return docker(image, platform, "node", "-p", f"require('{p.image_dir}/node_modules/{package}/package.json').version").strip()

    commands = {
        "golangci-lint": lambda: docker(image, platform, "golangci-lint", "version", "--short").strip(),
        "ruff": lambda: docker(image, platform, "ruff", "--version").strip().removeprefix("ruff "),
        "eslint": lambda: docker(image, platform, "eslint", "--version").strip().removeprefix("v"),
        "typescript-eslint": lambda: node_version("typescript-eslint"),
        "typescript": lambda: node_version("typescript"),
        "lizard": lambda: docker(image, platform, "lizard", "--version").strip(),
        "jscpd": lambda: docker(image, platform, "jscpd", "--version").strip().removeprefix("jscpd "),
    }
    got = {tool: commands[tool]() for tool in p.tools}
    if got != p.tools:
        raise RuntimeError(f"tool versions {got} differ from profile.json {p.tools}")


def check_fixtures(image, platform, p):
    out = docker(image, platform, "craftpulse-ci", "fixtures", p.image_dir + "/fixtures", "--", "craftpulse-ci", "code", "--fixture")
    cases = [line for line in out.splitlines() if line.startswith("fixture ")]
    want = len([d for d in (p.dir / "fixtures").iterdir() if (d / "expected.txt").is_file()])
    if len(cases) != want or not all(": ok," in line for line in cases):
        raise RuntimeError("rule fixtures differ:\n" + out[-4000:])
    print("\n".join(cases))


def job(image, platform, root, name, repo, p, sha=SHA, *code_args):
    """Run the profile as a job on the repository and return the files of the package."""
    state = root / ("state-" + name)
    state.mkdir()
    entry = {"component_id": "0193a5b0-7777-7000-8000-000000000001", "path_prefix": "", "language": p.language,
             "analysis_kind": "code", "mode": "builtin", "profile_id": p.name, "image_digest": DIGEST}
    (state / "profile.json").write_text(json.dumps({"entries": [entry]}))
    env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": sha, "CRAFTPULSE_WORK_DIR": "/state",
           "CRAFTPULSE_URL": "https://craftpulse.example", "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token",
           "CI_SERVER_HOST": "gitlab.example", "CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
           "CI_COMMIT_REF_NAME": "demo/fixture"}
    mounts = [(repo, "/repo"), (state, "/state")]
    docker(image, platform, "craftpulse-ci", "code", *code_args, mounts=mounts, env=env)
    docker(image, platform, "craftpulse-ci", "upload", "--dry-run", "--save", "/state/package.tar.gz", mounts=mounts, env=env)
    with tarfile.open(state / "package.tar.gz") as package:
        return {f.name: package.extractfile(f).read() for f in package if f.isfile()}


def fixture_repo(root, p):
    """The scope and clones cases in one repository, with the extra dependency and broken files."""
    repo = root / "repo"
    shutil.copytree(p.dir / "fixtures/scope/src", repo)
    for source in (p.dir / "fixtures/clones/src").iterdir():
        if not (repo / source.name).exists():
            shutil.copy(source, repo / source.name)
    for name, content in EXTRA[p.language]["files"].items():
        (repo / name).parent.mkdir(parents=True, exist_ok=True)
        (repo / name).write_text(content)
    return repo


def check_job(image, platform, root, p):
    extra = EXTRA[p.language]
    files = job(image, platform, root, "full", fixture_repo(root, p), p)
    runs = {run["producer"]: run for run in json.loads(files["scan-manifest.json"])["tools"]}
    if sorted(runs) != sorted([p.linter, "lizard", "jscpd"]):
        raise RuntimeError("scan manifest does not have one run per tool")
    excluded = {(path, reason) for path, reason in p.expected("scope", "excluded")} | extra["excluded"]
    tests = sorted(path for (path,) in p.expected("scope", "test"))
    configs = {p.linter: p.dir / p.config, "jscpd": p.dir / "jscpd.json", "lizard": p.dir / "profile.json"}
    for producer, tool in runs.items():
        config = hashlib.sha256(configs[producer].read_bytes()).hexdigest() if producer in configs else None
        complete = "partial" if producer == p.linter and extra["broken"] else "complete"
        if (tool["producer_version"] != p.tools[producer] or tool["profile_image_digest"] != DIGEST or tool["rules_hash"] != p.rules_hash
                or tool["config_hash"] != config or tool["completeness"] != complete or tool["sha"] != SHA):
            raise RuntimeError("incomplete provenance of " + producer + ": " + json.dumps(tool))
        if {(t["path"], t["reason"]) for t in tool["excluded_targets"]} != excluded:
            raise RuntimeError("unexpected excluded targets of " + producer + ": " + json.dumps(tool["excluded_targets"]))
        if tool["test_targets"] != tests:
            raise RuntimeError("test code is not marked for " + producer)
        skipped = [{"path": extra["broken"], "reason": "unsupported"}] if complete == "partial" else []
        if tool["skipped_targets"] != skipped:
            raise RuntimeError("unexpected skipped targets of " + producer + ": " + json.dumps(tool["skipped_targets"]))
    if p.language == "javascript" and runs["eslint"]["parser_version"] != (
            f"typescript-eslint {p.tools['typescript-eslint']}, typescript {p.tools['typescript']}"):
        raise RuntimeError("eslint run does not name the TypeScript parser")
    reports = {producer: files[tool["reports"][0]["file"]["path"]] for producer, tool in runs.items()}
    for producer, report in reports.items():
        if producer == "jscpd":
            continue
        for path, _ in excluded:
            if (path + "/").encode() in report or path.encode() + b'"' in report:
                raise RuntimeError(producer + " report covers the excluded path " + path)
        if b".hidden" in report or b".venv" in report:
            raise RuntimeError(producer + " report covers a hidden directory")
    clone_report = runs["jscpd"]["reports"][0]
    if clone_report["report_format"] != "jscpd_json" or clone_report["format_version"] != "2":
        raise RuntimeError("unexpected jscpd report format")
    clones = json.loads(reports["jscpd"])
    if set(clones.get("scope_excluded_paths", [])) != {path for path, _ in excluded}:
        raise RuntimeError("jscpd report scope differs from the scan manifest")
    if len(clones["duplicates"]) != 1 or b"fragment" in reports["jscpd"]:
        raise RuntimeError("clone report must keep the copy coordinates without its code")
    for clone in clones["duplicates"]:
        for member in (clone["firstFile"]["name"], clone["secondFile"]["name"]):
            if any(member == path or member.startswith(path + "/") for path, _ in excluded):
                raise RuntimeError("jscpd clone covers an excluded path: " + member)
            if ".hidden" in member or ".venv" in member:
                raise RuntimeError("jscpd clone covers a hidden directory: " + member)
    lizard = runs["lizard"]
    report = lizard["reports"][0]
    if report["report_format"] != "lizard_csv" or report["format_version"] != "2":
        raise RuntimeError("unexpected Lizard report format")
    if lizard.get("function_thresholds") != p.spec["functions"]:
        raise RuntimeError("Lizard run thresholds differ from the profile")
    rows = csv.reader(reports["lizard"].decode().splitlines())
    if next(rows, None) != ["craftpulse-lizard", "2"]:
        raise RuntimeError("Lizard report has no version 2 header")
    inventory, functions = set(), set()
    for row in rows:
        if len(row) == 3 and row[0] == "file":
            if int(row[2]) < 0:
                raise RuntimeError("negative file NLOC")
            inventory.add(row[1])
        elif len(row) == 11:
            functions.add(row[6])
        else:
            raise RuntimeError("unexpected record in Lizard report")
    if not functions or not (functions | set(tests)) <= inventory:
        raise RuntimeError("Lizard file inventory does not cover the reported functions and test files")
    if any(file == path or file.startswith(path + "/") for file in inventory for path, _ in excluded):
        raise RuntimeError("Lizard file inventory contains an excluded path")
    lint = json.loads(reports[p.linter])
    if lint["version"] != "2.1.0" or (extra["broken"] and extra["broken"].encode() in reports[p.linter]):
        raise RuntimeError("unexpected linter report")
    print(p.name + " job: ok, " + ", ".join(f"{t['producer']} {t['exit_class']}/{t['completeness']}" for t in runs.values()))


def check_timeout(image, platform, root, p):
    files = job(image, platform, root, "timeout", root / "repo", p, SHA, "--timeout", "1ms")
    for tool in json.loads(files["scan-manifest.json"])["tools"]:
        if tool["exit_class"] != "timeout" or tool["completeness"] != "partial" or tool["reports"]:
            raise RuntimeError("a profile out of time must give timeout and partial: " + tool["id"])
    print(p.name + " timeout 1ms: ok")


def lint_findings(files, p):
    run = next(t for t in json.loads(files["scan-manifest.json"])["tools"] if t["producer"] == p.linter)
    report = json.loads(files[run["reports"][0]["file"]["path"]])
    return run["sha"], sorted((r["ruleId"], r["locations"][0]["physicalLocation"]["artifactLocation"]["uri"])
                              for r in report["runs"][0]["results"])


def check_base_head(image, platform, root, p):
    """Commit the base and head snapshots of the fixture project and run the profile on each commit."""
    project = PROJECTS / p.language
    if not project.is_dir():
        return
    repo = root / "project"
    repo.mkdir()
    git = ["git", "-c", "user.name=craftPulse", "-c", "user.email=ci@craftpulse.invalid", "-c", "commit.gpgsign=false"]
    run(git + ["init", "-q", "-b", "main"], cwd=repo)
    shas = {}
    for snapshot in ["base", "head"]:
        for child in repo.iterdir():
            if child.name != ".git":
                shutil.rmtree(child) if child.is_dir() else child.unlink()
        shutil.copytree(project / snapshot, repo, dirs_exist_ok=True)
        run(git + ["add", "-A"], cwd=repo)
        run(git + ["commit", "-q", "-m", snapshot], cwd=repo)
        shas[snapshot] = run(["git", "rev-parse", "HEAD"], cwd=repo).strip()
    found = {}
    for snapshot, sha in shas.items():
        run(["git", "checkout", "-q", sha], cwd=repo)
        files = job(image, platform, root, "project-" + snapshot, repo, p, sha)
        if any(tool["sha"] != sha for tool in json.loads(files["scan-manifest.json"])["tools"]):
            raise RuntimeError(f"{snapshot} reports do not carry the SHA {sha}")
        found[snapshot] = lint_findings(files, p)[1]
    expected = json.loads((project / "expected.json").read_text())
    for snapshot in shas:
        if found[snapshot] != sorted(tuple(f) for f in expected[snapshot]):
            raise RuntimeError(f"{snapshot} findings {found[snapshot]} differ from expected.json")
    print(f"{p.name} base {shas['base'][:12]} and head {shas['head'][:12]}: ok, {len(found['base'])} and {len(found['head'])} finding(s)")


def check(image, platform, name):
    p = Profile(name)
    check_image_env(image)
    check_versions(image, platform, p)
    check_fixtures(image, platform, p)
    with tempfile.TemporaryDirectory(prefix="craftpulse-code-profile-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        check_job(image, platform, root, p)
        check_timeout(image, platform, root, p)
        check_base_head(image, platform, root, p)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", required=True, choices=sorted(d.name for d in PROFILES.iterdir() if (d / "profile.json").is_file()
                                                                    and "language" in json.loads((d / "profile.json").read_text())))
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform, args.profile)


if __name__ == "__main__":
    main()
