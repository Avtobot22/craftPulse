#!/usr/bin/env python3
"""Exercise the actual secrets profile image without a network or real credentials."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import xml.etree.ElementTree as ET


def run(args, *, cwd=None):
    result = subprocess.run(args, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    if result.returncode:
        # A failing tool must not put the synthetic marker in the job log either.
        raise RuntimeError("secrets fixture command failed (raw output suppressed)")
    return result.stdout


def git(repo, *args):
    return run(["git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                "-C", str(repo), *args]).decode().strip()


def commit(repo, text):
    (repo / "config.go").write_text(text)
    git(repo, "add", ".")
    git(repo, "commit", "--quiet", "-m", "fixture")
    return git(repo, "rev-parse", "HEAD")


def assert_safe(data, marker):
    if any(marker[i:i + 8].encode() in data for i in range(len(marker) - 7)):
        raise RuntimeError("secret fragment escaped the profile")


def check(image, platform):
    marker = base64.urlsafe_b64encode(hashlib.sha256(b"craftpulse fixture marker, not a credential").digest()).decode()[:32]
    with tempfile.TemporaryDirectory(prefix="craftpulse-secrets-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        repo = root / "repo"
        repo.mkdir()
        git(repo, "init", "--quiet")
        base = commit(repo, "package config\n// clean\n")
        added = commit(repo, 'package config\nconst defaultPartnerAPIKey = "' + marker + '"\n')
        head = commit(repo, '// adjacent edit\npackage config\nconst defaultPartnerAPIKey = "' + marker + '"\n')
        shallow = root / "shallow"
        git(root, "clone", "--quiet", "--depth=1", repo.as_uri(), str(shallow))
        (repo / ".gitattributes").write_text("config.go -diff\n")
        binary_base = commit(repo, "package config\n// clean before binary addition\n")
        commit(repo, '\x00package config\nconst defaultPartnerAPIKey = "' + marker + '"\n')
        removed = commit(repo, "package config\n// removed binary secret\n")
        common = ["docker", "run", "--rm", "--network", "none", "--user", f"{os.getuid()}:{os.getgid()}"]
        if platform:
            common += ["--platform", platform]
        info = json.loads(run(common + [image, "craftpulse-ci", "secrets-info"]))
        if info["gitleaks_version"] != "8.30.1":
            raise RuntimeError("unexpected scanner version")
        fingerprints = []
        for name, source, scan_base, sha, expected_head, expected_diff, truncated in [
            ("clean", repo, base, base, 0, 0, False),
            ("head", repo, base, added, 1, 1, False),
            ("diff", repo, base, head, 1, 1, False),
            ("shallow", shallow, base, head, 1, 1, True),
            ("binary-removed", repo, binary_base, removed, 0, 1, False),
        ]:
            state = root / ("state-" + name)
            state.mkdir()
            profile = {"entries": [{"component_id": "0193a5b0-7777-7000-8000-000000000001",
                "path_prefix": "", "language": "go", "analysis_kind": "secrets", "mode": "builtin",
                "profile_id": "gitleaks-default", "image_digest": "sha256:" + "a" * 64}]}
            (state / "profile.json").write_text(json.dumps(profile))
            # This other report repeats the full marker, a fragment, and an entity-escaped value.
            entities = "".join("&#" + str(ord(c)) + ";" for c in marker)
            (state / "junit.xml").write_text('<testsuites xmlns="urn:fixture" xmlns:x="urn:other"><testsuite name="fixture"><x:testcase><failure>' +
                marker + " " + marker[4:12] + " " + entities + '</failure></x:testcase></testsuite></testsuites>')
            env = {"CI_PROJECT_DIR": "/repo", "CI_COMMIT_SHA": sha, "CI_MERGE_REQUEST_DIFF_BASE_SHA": scan_base,
                "CRAFTPULSE_WORK_DIR": "/state", "CRAFTPULSE_URL": "https://craftpulse.example",
                "CRAFTPULSE_UPLOAD_TOKEN": "fixture-token", "CI_SERVER_HOST": "gitlab.example",
                "CI_PROJECT_ID": "42", "CI_PIPELINE_ID": "5001", "CI_JOB_ID": "7002",
                "CI_COMMIT_REF_NAME": "demo/fixture" if name == "clean" else "demo/" + marker}
            args = common + ["--mount", f"type=bind,source={source},target=/repo,readonly",
                             "--mount", f"type=bind,source={state},target=/state"]
            for key, value in env.items():
                args += ["--env", key + "=" + value]
            args += [image, "craftpulse-ci"]
            logs = run(args + ["secrets"])
            if name != "clean":
                logs += run(args + ["import", "--report", "junit:junit_xml:/state/junit.xml"])
            logs += run(args + ["upload", "--dry-run", "--save", "/state/package.tar.gz"])
            assert_safe(logs, marker)
            for stored in state.rglob("*"):
                # junit.xml is the pre-existing client report; all copies written by CLI must be safe.
                if stored.is_file() and stored.name != "junit.xml" and stored.suffix != ".gz":
                    assert_safe(stored.read_bytes(), marker)
            with tarfile.open(state / "package.tar.gz") as package:
                files = {f.name: package.extractfile(f).read() for f in package if f.isfile()}
            for path, data in files.items():
                assert_safe(path.encode() + data, marker)
                if path.endswith(".xml"):
                    ET.fromstring(data)
            scan = json.loads(files["scan-manifest.json"])
            tools = {tool["id"].rsplit("-", 1)[-1]: tool for tool in scan["tools"] if tool["producer"] == "gitleaks"}
            for scope, count in [("head", expected_head), ("diff", expected_diff)]:
                tool = tools[scope]
                findings = json.loads(files[tool["reports"][0]["file"]["path"]])
                training = [f for f in findings if f["RuleID"] == "craftpulse-demo-secret"]
                if len(training) != count:
                    raise RuntimeError("unexpected fixture findings: " + name + "/" + scope)
                if tool["rules_hash"] != info["rules_hash"]:
                    raise RuntimeError("rules hash mismatch")
                for finding in training:
                    if set(finding) != {"RuleID", "File", "StartLine", "Commit", "Fingerprint"}:
                        raise RuntimeError("unsafe finding shape")
                    if finding["File"] != "config.go" or finding["StartLine"] < 2:
                        raise RuntimeError("incorrect finding coordinates")
                if scope == "head" and training:
                    fingerprints.append(training[0]["Fingerprint"])
            if tools["diff"]["history"]["truncated"] != truncated:
                raise RuntimeError("incorrect range completeness")
            if truncated and tools["diff"]["completeness"] != "partial":
                raise RuntimeError("shallow scan incorrectly promises completeness")
            print("secrets fixture " + name + ": ok")
        if len(set(fingerprints)) != 1:
            raise RuntimeError("fingerprint changed after an adjacent edit")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", default="")
    args = parser.parse_args()
    check(args.image, args.platform)


if __name__ == "__main__":
    main()
