#!/usr/bin/env python3
"""Build, scan and publish the images of one checked main commit."""

import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import urllib.error
import urllib.parse
import urllib.request


ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = "Avtobot22/craftPulse"
SHA = re.compile(r"[0-9a-f]{40}")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
MANIFEST_TYPE = "application/vnd.craftpulse.release.v1+json"


class ReleaseError(Exception):
    pass


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def run(args, *, cwd=ROOT, capture=False, input_text=None, extra_env=None):
    env = {k: v for k, v in os.environ.items() if k not in {"RELEASE_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}}
    env.update(extra_env or {})
    result = subprocess.run(args, cwd=cwd, env=env, text=True, input=input_text,
                            stdout=subprocess.PIPE if capture else None, check=True)
    return result.stdout.strip() if capture else None


PROFILES = "backend/internal/ci/profiles"
SAST_PROFILE = PROFILES + "/opengrep-default"
TRIVY_PROFILE = PROFILES + "/trivy-default"
DOCS_PROFILE = PROFILES + "/git-markdown-openapi"
# Code profiles and the linter configuration each one hashes with profile.json and jscpd.json.
CODE_PROFILES = {"go-default": "golangci.yml", "python-default": "ruff.toml", "js-default": "eslint.config.mjs"}
# Each profile image is exercised by its check script without a network.
PROFILE_CHECKS = {"gitleaks-default": ["scripts/check_secrets_profile.py"], "opengrep-default": ["scripts/check_sast_profile.py"],
                  "trivy-default": ["scripts/check_trivy_profile.py"], "git-markdown-openapi": ["scripts/check_docs_profile.py"],
                  **{name: ["scripts/check_code_profile.py", "--profile", name] for name in CODE_PROFILES}}


def plan(root=ROOT):
    images = [
        {"name": "craftpulse-server", "context": "backend", "dockerfile": "backend/Dockerfile"},
        {"name": "craftpulse-frontend", "context": "frontend", "dockerfile": "frontend/Dockerfile"},
        {"name": "craftpulse-ci", "context": "backend", "dockerfile": "backend/ci.Dockerfile"},
    ]
    if (root / "backend/pkg/secrets/profiles/gitleaks-default/profile.json").is_file():
        images.append({"name": "craftpulse-ci-gitleaks-default", "context": "backend",
                       "dockerfile": "backend/ci.Dockerfile", "target": "gitleaks-default"})
    for name in CODE_PROFILES:
        if (root / PROFILES / name / "profile.json").is_file():
            images.append({"name": f"craftpulse-ci-{name}", "context": "backend",
                           "dockerfile": "backend/ci.Dockerfile", "target": name})
    if (root / SAST_PROFILE / "profile.json").is_file():
        images.append({"name": "craftpulse-ci-opengrep-default", "context": "backend",
                       "dockerfile": "backend/ci.Dockerfile", "target": "opengrep-default"})
    if (root / TRIVY_PROFILE / "profile.json").is_file():
        images.append({"name": "craftpulse-ci-trivy-default", "context": "backend",
                       "dockerfile": "backend/ci.Dockerfile", "target": "trivy-default"})
    if (root / DOCS_PROFILE / "profile.json").is_file():
        images.append({"name": "craftpulse-ci-git-markdown-openapi", "context": "backend",
                       "dockerfile": "backend/ci.Dockerfile", "target": "git-markdown-openapi"})
    for directory in sorted((root / "backend/cmd").glob("adapter-*")):
        # The recorded-answer server explicitly excludes itself from release images.
        if directory.name == "adapter-test":
            continue
        if not directory.is_dir() or not (directory / "main.go").is_file():
            raise ReleaseError(f"invalid adapter command: {directory.name}")
        if not re.fullmatch(r"adapter-[a-z0-9]+(?:-[a-z0-9]+)*", directory.name):
            raise ReleaseError("invalid adapter directory name")
        images.append({"name": f"craftpulse-{directory.name}", "context": "backend",
                       "dockerfile": "backend/adapter.Dockerfile", "adapter": directory.name})
    for image in images:
        if not (root / image["dockerfile"]).is_file():
            raise ReleaseError(f"missing Dockerfile: {image['dockerfile']}")
    return images


def schema_version(root, name):
    versions = [int(path.name.split("_", 1)[0]) for path in (root / f"backend/migrations/{name}").glob("[0-9]*_*.sql")]
    if not versions:
        raise ReleaseError(f"no {name} migrations")
    return max(versions)


def metadata(root, images):
    adapters = [i["name"] for i in images if i.get("adapter")]
    contract = re.search(r'const CurrentContractVersion = "([^"]+)"',
                         (root / "backend/pkg/adaptersdk/errors.go").read_text())
    if adapters and not contract:
        raise ReleaseError("adapter contract version not found")
    ci_versions, rule_hashes = {}, {}
    if any(image.get("target") == "gitleaks-default" for image in images):
        directory = root / "backend/pkg/secrets/profiles/gitleaks-default"
        policy = read_json(directory / "profile.json")
        version = policy["gitleaks_version"]
        if not re.search(r"github.com/zricethezav/gitleaks/v8 v" + re.escape(version) + r"\s", (root / "backend/go.mod").read_text()):
            raise ReleaseError("Gitleaks profile and Go dependency versions disagree")
        ci_versions["gitleaks"] = version
        rule_hashes["gitleaks-default"] = hashlib.sha256(version.encode() + b"\n" +
            (directory / "rules.toml").read_bytes() + (directory / "profile.json").read_bytes()).hexdigest()
    for name in CODE_PROFILES:
        if not any(image.get("target") == name for image in images):
            continue
        tools, rule_hash = code_profile(root, name)
        for tool, version in tools.items():
            if ci_versions.setdefault(tool, version) != version:
                raise ReleaseError(f"code profiles pin {tool} at different versions")
        rule_hashes[name] = rule_hash
    rule_sources = {}
    if any(image.get("target") == "opengrep-default" for image in images):
        version, rule_hash, sources = sast_profile(root)
        ci_versions["opengrep"] = version
        rule_hashes["opengrep-default"] = rule_hash
        rule_sources["opengrep-default"] = sources
    if any(image.get("target") == "trivy-default" for image in images):
        version, database, rule_hash = trivy_profile(root)
        ci_versions["trivy"], ci_versions["trivy-db"] = version, database
        rule_hashes["trivy-default"] = rule_hash
    if any(image.get("target") == "git-markdown-openapi" for image in images):
        tools, rule_hash = docs_profile(root)
        ci_versions.update(tools)
        rule_hashes["git-markdown-openapi"] = rule_hash
    return {"core_schema_version": schema_version(root, "core"),
            "adapter_schema_versions": {name: schema_version(root, "adapter") for name in adapters},
            "adapter_contract_version": contract[1] if adapters else None,
            "ci_tool_versions": ci_versions, "rule_hashes": rule_hashes, "rule_sources": rule_sources}


def code_profile(root, name):
    """Tool versions of a code profile, checked against the image recipe, and its rules hash.

    golangci-lint and jscpd are pinned in the Dockerfile, Python packages by hash in the requirements of the
    profile, npm packages in its package-lock.json."""
    directory = root / PROFILES / name
    tools = read_json(directory / "profile.json")["tools"]
    dockerfile = (root / "backend/ci.Dockerfile").read_text()
    pinned = {
        "golangci-lint": set(re.findall(r"^ARG GOLANGCI_LINT_VERSION=(\S+)$", dockerfile, re.M)),
        "jscpd": set(re.findall(r"/jscpd-linux-(?:x64|arm64)-musl-([0-9][^/]*)\.tgz", dockerfile)),
    }
    for requirements in sorted(directory.glob("*requirements.txt")):
        for package, version in re.findall(r"^([a-z0-9-]+)==(\S+) ", requirements.read_text(), re.M):
            pinned.setdefault(package, set()).add(version)
    lock = directory / "package-lock.json"
    if lock.is_file():
        for path, package in read_json(lock)["packages"].items():
            if path.count("node_modules/") == 1 and path.startswith("node_modules/"):
                pinned.setdefault(path.removeprefix("node_modules/"), set()).add(package["version"])
    if any(pinned.get(tool) != {version} for tool, version in tools.items()):
        raise ReleaseError(f"{name} profile and image tool versions disagree")
    content = b"".join((directory / file).read_bytes() for file in ["profile.json", CODE_PROFILES[name], "jscpd.json"])
    return {tool: tools[tool] for tool in sorted(tools)}, hashlib.sha256(content).hexdigest()


def sast_profile(root):
    """Opengrep version of the SAST profile, checked against the image recipe, its rules hash and rule sources."""
    directory = root / SAST_PROFILE
    profile = (directory / "profile.json").read_bytes()
    version = json.loads(profile)["tools"]["opengrep"]
    pinned = set(re.findall(r"/opengrep/releases/download/v([0-9][^/]*)/opengrep_", (root / "backend/ci.Dockerfile").read_text()))
    if pinned != {version}:
        raise ReleaseError("SAST profile and image Opengrep versions disagree")
    sources = (directory / "sources.json").read_bytes()
    content = profile + sources + b"".join(path.read_bytes() for path in sorted((directory / "rules").glob("*.yaml")))
    return version, hashlib.sha256(content).hexdigest(), json.loads(sources)["rules"]


def trivy_profile(root):
    """Trivy version of the SCA and IaC profile, checked against the image recipe, the pinned database and the
    rules hash of profile.json, trivy.yaml and trivyignore."""
    directory = root / TRIVY_PROFILE
    profile = (directory / "profile.json").read_bytes()
    version = json.loads(profile)["tools"]["trivy"]
    dockerfile = (root / "backend/ci.Dockerfile").read_text()
    pinned = set(re.findall(r"/trivy/releases/download/v([0-9][^/]*)/trivy_", dockerfile))
    if pinned != {version}:
        raise ReleaseError("Trivy profile and image Trivy versions disagree")
    database = re.findall(r"^ARG TRIVY_DB_REPOSITORY=(\S+@sha256:[0-9a-f]{64})$", dockerfile, re.M)
    if len(database) != 1:
        raise ReleaseError("Trivy database is not pinned by digest")
    content = profile + (directory / "trivy.yaml").read_bytes() + (directory / "trivyignore").read_bytes()
    return version, database[0], hashlib.sha256(content).hexdigest()


def docs_profile(root):
    """Spectral and goldmark versions of the Git documentation profile, checked against its lockfile and
    backend/go.mod, and the rules hash of profile.json and spectral.yaml."""
    directory = root / DOCS_PROFILE
    profile = (directory / "profile.json").read_bytes()
    tools = json.loads(profile)["tools"]
    lock = read_json(directory / "package-lock.json")["packages"].get("node_modules/@stoplight/spectral-cli", {})
    goldmark = re.findall(r"^\s*github.com/yuin/goldmark v(\S+)$", (root / "backend/go.mod").read_text(), re.M)
    if lock.get("version") != tools.get("spectral") or goldmark != [tools.get("goldmark")]:
        raise ReleaseError("docs profile and image tool versions disagree")
    content = profile + (directory / "spectral.yaml").read_bytes()
    return {"spectral": tools["spectral"], "goldmark": tools["goldmark"]}, hashlib.sha256(content).hexdigest()


def install_tools(destination, root=ROOT):
    destination.mkdir(parents=True, exist_ok=True)
    for name, tool in read_json(root / ".github/release-tools.json").items():
        with urllib.request.urlopen(tool["url"], timeout=120) as response:
            content = response.read()
        if hashlib.sha256(content).hexdigest() != tool["sha256"]:
            raise ReleaseError(f"checksum mismatch for {name}")
        with tarfile.open(fileobj=io.BytesIO(content), mode="r:gz") as archive:
            member = archive.getmember(name)
            if not member.isfile():
                raise ReleaseError(f"not a regular binary: {name}")
            with archive.extractfile(member) as source, (destination / name).open("wb") as target:
                shutil.copyfileobj(source, target)
        (destination / name).chmod(0o755)


class Registry:
    def __init__(self, actor, password):
        self.authorization = "Basic " + base64.b64encode(f"{actor}:{password}".encode()).decode()

    def resolve(self, name, tag):
        scope = urllib.parse.urlencode({"service": "ghcr.io", "scope": f"repository:avtobot22/{name}:pull,push"})
        request = urllib.request.Request(f"https://ghcr.io/token?{scope}", headers={"Authorization": self.authorization})
        with urllib.request.urlopen(request, timeout=60) as response:
            token = json.load(response)["token"]
        request = urllib.request.Request(f"https://ghcr.io/v2/avtobot22/{name}/manifests/{tag}", method="HEAD", headers={
            "Authorization": f"Bearer {token}",
            "Accept": ", ".join(["application/vnd.oci.image.manifest.v1+json", "application/vnd.oci.image.index.v1+json",
                                  "application/vnd.docker.distribution.manifest.v2+json", "application/vnd.docker.distribution.manifest.list.v2+json"]),
        })
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                digest = response.headers.get("Docker-Content-Digest", "")
        except urllib.error.HTTPError as error:
            error.close()
            if error.code == 404:
                return None
            raise
        if not DIGEST.fullmatch(digest):
            raise ReleaseError("invalid registry digest")
        return digest


def build(image, sha, ref, root=ROOT):
    args = ["docker", "build", "--platform", "linux/amd64", "--build-arg", f"COMMIT_SHA={sha}",
            "--label", f"org.opencontainers.image.revision={sha}", "--tag", ref,
            "--file", str(root / image["dockerfile"])]
    if image.get("target"):
        args += ["--target", image["target"]]
    if image.get("adapter"):
        args += ["--build-arg", f"ADAPTER={image['adapter']}"]
    run(args + [str(root / image["context"])], cwd=root)


def smoke_adapter(ref):
    run(["docker", "run", "--rm", "--platform", "linux/amd64", "--network", "none", "--entrypoint", "/bin/sh", ref, "-ec",
         'test "$(id -u)" = 65532; test "$(id -g)" = 65532; test "$(id -un)" = nonroot; '
         'test "$GIT_CACHE_DIR" = /var/cache/craftpulse-git; '
         'mkdir -p "$GIT_CACHE_DIR/clones" "$GIT_CACHE_DIR/work"; '
         'ssh -G -F /dev/null git@example.invalid >/dev/null'])


def smoke_ci(ref):
    run(["docker", "run", "--rm", "--platform", "linux/amd64", "--network", "none", ref, "sh", "-ec",
         'test "$(id -u)" = 65532; test "$(id -g)" = 65532; craftpulse-ci version'])


def validate_original(manifest, sha, images, resolved, expected_metadata):
    if manifest.get("schema_version") != 1 or manifest.get("commit_sha") != sha:
        raise ReleaseError("stored release belongs to another commit or schema")
    if manifest.get("images") != {i["name"]: f"ghcr.io/avtobot22/{i['name']}@{resolved[i['name']]}" for i in images}:
        raise ReleaseError("stored release and registry image tags disagree")
    for key, value in expected_metadata.items():
        if manifest.get(key) != value:
            raise ReleaseError(f"stored release metadata disagrees: {key}")
    if not manifest.get("licenses", {}).get("images") or not manifest["licenses"].get("frontend_npm"):
        raise ReleaseError("stored release has no license inventory")


def image_licenses(sbom):
    return sorted([{"name": package["name"], "version": package.get("version", ""),
                    "type": package["type"], "purl": package.get("purl", ""),
                    "licenses": sorted({license.get("spdxExpression") or license.get("value") or "NOASSERTION"
                                        for license in package.get("licenses", [])}) or ["NOASSERTION"]}
                   for package in sbom["artifacts"]], key=lambda package: (package["type"], package["name"], package["version"]))


def npm_licenses(lock):
    packages = []
    for path, package in sorted(lock["packages"].items()):
        if "node_modules/" not in path or package.get("link"):
            continue
        packages.append({"name": path.rsplit("node_modules/", 1)[1], "version": package["version"],
                         "license": package.get("license") or "NOASSERTION", "dev": bool(package.get("dev"))})
    if not packages:
        raise ReleaseError("empty npm license inventory")
    return packages


def scan(images, references, output, tools):
    failed = []
    licenses = {}
    for image in images:
        name = image["name"]
        report = output / f"{name}.trivy.json"
        try:
            run([str(tools / "trivy"), "image", "--image-src", "docker", "--scanners", "vuln", "--severity", "HIGH,CRITICAL",
                 "--exit-code", "1", "--format", "json", "--output", str(report), references[name]])
        except subprocess.CalledProcessError:
            failed.append(name)
        sbom = output / f"{name}.syft.json"
        run([str(tools / "syft"), "scan", f"docker:{references[name]}", "-o", f"syft-json={sbom}"],
            extra_env={"SYFT_GOLANG_SEARCH_REMOTE_LICENSES": "true"})
        licenses[name] = image_licenses(read_json(sbom))
    if failed:
        raise ReleaseError("Trivy blocked publication: " + ", ".join(failed))
    return licenses


def publish_images(images, sha, references, resolved, registry):
    for image in images:
        name = image["name"]
        current = registry.resolve(name, sha)
        if resolved[name]:
            if current != resolved[name]:
                raise ReleaseError(f"published tag changed: {name}")
            continue
        if current:
            raise ReleaseError(f"tag appeared during build: {name}; rerun to scan that digest")
        scanned_id = run(["docker", "image", "inspect", "--format", "{{.Id}}", references[name]], capture=True)
        run(["docker", "push", references[name]])
        resolved[name] = registry.resolve(name, sha)
        if not resolved[name]:
            raise ReleaseError(f"published digest missing: {name}")
        published = f"ghcr.io/avtobot22/{name}@{resolved[name]}"
        run(["docker", "pull", "--platform", "linux/amd64", published])
        published_id = run(["docker", "image", "inspect", "--format", "{{.Id}}", published], capture=True)
        if published_id != scanned_id:
            raise ReleaseError(f"published image differs from scanned image: {name}")


def release(sha, output, tools, root=ROOT):
    if not SHA.fullmatch(sha) or run(["git", "rev-parse", "HEAD"], cwd=root, capture=True) != sha:
        raise ReleaseError("checkout does not match release SHA")
    if os.environ.get("GITHUB_REPOSITORY") != REPOSITORY:
        raise ReleaseError("release is limited to its source repository")
    password = os.environ["RELEASE_TOKEN"]
    actor = os.environ["GITHUB_ACTOR"]
    output.mkdir(parents=True, exist_ok=True)
    images = plan(root)
    expected_metadata = metadata(root, images)
    registry = Registry(actor, password)
    run(["docker", "login", "ghcr.io", "--username", actor, "--password-stdin"], input_text=password)
    resolved = {image["name"]: registry.resolve(image["name"], sha) for image in images}
    manifest_digest = registry.resolve("craftpulse-release", sha)
    original = None
    if manifest_digest:
        original_dir = output / "original"
        run([str(tools / "oras"), "pull", "--output", str(original_dir),
             f"ghcr.io/avtobot22/craftpulse-release@{manifest_digest}"])
        original = read_json(original_dir / "release-manifest.json")
        validate_original(original, sha, images, resolved, expected_metadata)
    references = {}
    for image in images:
        name = image["name"]
        reference = f"ghcr.io/avtobot22/{name}"
        if resolved[name]:
            reference += "@" + resolved[name]
            run(["docker", "pull", "--platform", "linux/amd64", reference])
        else:
            reference += ":" + sha
            build(image, sha, reference, root)
        labels = json.loads(run(["docker", "image", "inspect", "--format", "{{json .Config.Labels}}", reference], capture=True))
        if labels.get("org.opencontainers.image.revision") != sha:
            raise ReleaseError(f"image revision does not match release: {name}")
        if image.get("adapter"):
            smoke_adapter(reference)
        if name == "craftpulse-ci":
            smoke_ci(reference)
        if image.get("target") in PROFILE_CHECKS:
            script, *options = PROFILE_CHECKS[image["target"]]
            run([sys.executable, str(root / script), *options, "--image", reference], cwd=root)
        references[name] = reference
    licenses = scan(images, references, output, tools)
    publish_images(images, sha, references, resolved, registry)
    manifest_path = output / "release-manifest.json"
    if original:
        shutil.copyfile(output / "original/release-manifest.json", manifest_path)
    else:
        manifest = {"schema_version": 1, "commit_sha": sha, **expected_metadata,
                    "images": {i["name"]: f"ghcr.io/avtobot22/{i['name']}@{resolved[i['name']]}" for i in images},
                    "licenses": {"images": licenses, "frontend_npm": npm_licenses(read_json(root / "frontend/package-lock.json"))}}
        write_json(manifest_path, manifest)
        if registry.resolve("craftpulse-release", sha):
            raise ReleaseError("release manifest tag appeared during publication")
        run([str(tools / "oras"), "push", "--artifact-type", MANIFEST_TYPE,
             "--annotation", f"org.opencontainers.image.source=https://github.com/{REPOSITORY}",
             "--annotation", f"org.opencontainers.image.revision={sha}",
             f"ghcr.io/avtobot22/craftpulse-release:{sha}", f"release-manifest.json:{MANIFEST_TYPE}"], cwd=output)
    print(f"Release manifest: ghcr.io/avtobot22/craftpulse-release:{sha}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["plan", "check", "build-adapters", "build-ci", "install-tools", "release"])
    parser.add_argument("--sha", default="0" * 40)
    parser.add_argument("--output", type=Path, default=ROOT / "build/release")
    parser.add_argument("--tools", type=Path, default=ROOT / "build/release-tools")
    args = parser.parse_args()
    try:
        if args.command == "plan":
            print(json.dumps(plan(), indent=2))
        elif args.command == "check":
            print(json.dumps(metadata(ROOT, plan()), indent=2))
            npm_licenses(read_json(ROOT / "frontend/package-lock.json"))
        elif args.command == "build-adapters":
            for image in plan():
                if image.get("adapter"):
                    reference = f"{image['name']}:check"
                    build(image, args.sha, reference)
                    smoke_adapter(reference)
        elif args.command == "build-ci":
            for image in plan():
                if image["name"] == "craftpulse-ci" or image.get("target") in PROFILE_CHECKS:
                    reference = f"{image['name']}:check"
                    build(image, args.sha, reference)
                    smoke_ci(reference)
                    if image.get("target") in PROFILE_CHECKS:
                        script, *options = PROFILE_CHECKS[image["target"]]
                        run([sys.executable, str(ROOT / script), *options, "--image", reference])
        elif args.command == "install-tools":
            install_tools(args.tools.resolve())
        else:
            release(args.sha, args.output.resolve(), args.tools.resolve())
    except (ReleaseError, subprocess.CalledProcessError, urllib.error.URLError, OSError, ValueError, KeyError) as error:
        # URLs, response bodies, command environments and credentials never enter this message.
        detail = str(error) if isinstance(error, ReleaseError) else type(error).__name__
        print(f"OCI release failed: {detail}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
