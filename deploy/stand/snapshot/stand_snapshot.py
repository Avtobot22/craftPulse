#!/usr/bin/env python3
"""Build, verify, capture or restore the initial craftPulse demo snapshot."""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import sys
import time
from uuid import uuid4

sys.path.insert(0, str(Path(__file__).resolve().parent))

from stand_sources import (
    HERE, Sources, StandError, apply_cloud, build_manifest, cloud_plan, differences,
    github, github_call, json_bytes, read_json, require,
)
from stand_storage import (
    AWS_CLI, CI, CI_VOLUMES, DB_NAME, INIT, OVERRIDE, ROOT, Archive, Bucket, Compose, Postgres,
    await_check, digest, paused, restore_ci, restore_gitlab, run, runtime_images, save_ci,
    save_gitlab, verify_images,
)


MANIFEST = HERE.parent / "manifest.json"
SECRET_DIR = Path("/etc/craftpulse-product")
COMMENT_AUTHOR = "craftpulse-mvp-bot[bot]"
CHECKPOINT = "checkpoint.json"
SHA256 = re.compile(r"[0-9a-f]{64}")
SOURCE_ARCHIVES = (
    "gitlab.tar", "gitlab-config.tar.gz", "ci-config.tar.gz",
    *(name + ".tar.gz" for name in CI_VOLUMES),
)


class Stand:
    def __init__(self, manifest, password_directory, local=False):
        self.manifest = manifest
        self.sources = Sources(manifest, local=local)
        self.product = Compose(ROOT / "product", "craftpulse-product")
        self.gitlab = Compose(ROOT / "gitlab", "craftpulse-gitlab")
        self.runner = Compose(ROOT / "gitlab/runner", "craftpulse-gitlab-runner")
        self.ci = Compose(ROOT / "ci90", "craftpulse-ci90")
        self.postgres = Postgres(self.product, manifest["craftpulse"])
        self.secret_dir = password_directory

    def images(self, services):
        images = self.manifest["runtime_images"]
        return (
            (self.product, runtime_images(self.manifest, services)),
            (self.gitlab, {"gitlab": images["craftpulse-gitlab-gitlab-1"]}),
            (self.runner, {"runner": images["craftpulse-gitlab-runner"]}),
            (self.ci, {name: images[f"craftpulse-ci90-{name}"] for name in CI}),
        )

    def verify_versions(self, *, only=None):
        errors = self.sources.compare_versions(only=only)
        for error in errors:
            print(error, file=sys.stderr, flush=True)
        require(not errors, "stand version check failed")

    def capture(self, root, recipient):
        self.verify_versions()
        databases = self.postgres.databases()
        services = self.postgres.product_services(databases)
        for compose, images in self.images(services):
            verify_images(compose, images)
        run("fetch pinned snapshot tool", ["docker", "pull", AWS_CLI], capture=False)
        clients = self.sources.clients()
        cloud = self.sources.cloud_state(clients)
        local = self.sources.local_state(clients)
        github_state = github_call(github.snapshot)
        github.validate(github_state)

        require(not root.exists(), "snapshot directory already exists")
        temporary = root.with_name(f".{root.name}.{uuid4().hex}")
        temporary.mkdir(mode=0o700)
        archive = Archive(temporary, recipient=recipient)
        try:
            archive.save("cloud.json", json_bytes(cloud))
            archive.save("github.json", json_bytes(github_state))
            print("Capturing craftPulse databases and bucket", flush=True)
            with paused(self.product, *services):
                rows = self.postgres.checkpoint(databases)
                self.postgres.backup(archive, databases)
                objects = Bucket(self.product, self.secret_dir).backup(archive)
                print("Capturing GitLab, Runner, Jenkins and SonarQube", flush=True)
                save_gitlab(archive, self.gitlab, self.runner)
                save_ci(archive, self.ci)
                await_check("self-managed source checkpoint",
                            lambda: verify_equal(local, self.sources.local_state(clients)))
                await_check("Atlassian fixture checkpoint",
                            lambda: verify_equal(cloud, self.sources.cloud_state(clients)))
                github_call(lambda api: verify_equal(github_state, github.snapshot(api)))
            (temporary / "manifest.json").write_bytes(json_bytes(self.manifest))
            checkpoint = {
                "format": 1, "rows": rows, "object_count": objects, "sources": local,
                "files": {path.name: digest(path) for path in sorted(temporary.iterdir())},
            }
            (temporary / CHECKPOINT).write_bytes(json_bytes(checkpoint))
            temporary.rename(root)
        finally:
            if temporary.is_dir():
                shutil.rmtree(temporary)
        print(json.dumps({"snapshot": "captured", "objects": objects, "checkpoint_sha256": digest(root / CHECKPOINT)}))

    def restore(self, archive, checkpoint):
        started = time.monotonic()
        databases = sorted(checkpoint["rows"])
        require("craftpulse" in databases and all(DB_NAME.fullmatch(name) for name in databases),
                "invalid checkpoint database list")
        services = self.postgres.product_services(databases)
        images = self.images(services)
        passwords = self.postgres.passwords(databases, self.secret_dir)
        cloud = json.loads(archive.read("cloud.json"))
        github_state = json.loads(archive.read("github.json"))
        objects = json.loads(archive.read("bucket.json"))
        require_objects(objects, checkpoint, databases)
        require(b"CREATE ROLE postgres;\n" in archive.read("roles.sql"), "invalid roles dump")
        self.verify_versions(only={"jira", "confluence", "github"})
        clients = self.sources.clients()
        cloud_plan(cloud, self.sources.cloud_state(clients))
        github_call(lambda api: github.reset(api, github_state, False, (COMMENT_AUTHOR,)))
        for compose, _ in images:
            check_override(compose)
        for image in sorted({image for _, pinned in images for image in pinned.values()} | {AWS_CLI}):
            run("fetch pinned image", ["docker", "pull", image], capture=False)
        for name in checkpoint["files"]:
            if name.endswith(".age"):
                run("authenticate archive contents", archive.decryption(name.removesuffix(".age")), capture=False)

        suffix = uuid4().hex[:12]
        print("Restoring craftPulse into fresh storage volumes", flush=True)
        self.product.stop(*services)
        overrides = {name: {"image": ref} for name, ref in images[0][1].items()}
        overrides["postgres"]["volumes"] = ["/dev/null:" + INIT + ":ro"]
        self.product.override(overrides, {
            "database": {"name": f"craftpulse-product-database-snapshot-{suffix}"},
            "object-store": {"name": f"craftpulse-product-objects-snapshot-{suffix}"},
        })
        self.product.up("postgres", "seaweedfs")
        self.postgres.restore(archive, databases, passwords)
        bucket = Bucket(self.product, self.secret_dir)
        await_check("SeaweedFS", bucket.keys)
        bucket.restore(archive, objects)
        verify_equal(checkpoint["rows"], self.postgres.checkpoint(databases))
        bucket.verify(objects)

        print("Restoring self-managed sources", flush=True)
        for compose, pinned in images[1:3]:
            compose.override({name: {"image": ref} for name, ref in pinned.items()})
        restore_gitlab(archive, self.gitlab, self.runner)
        restore_ci(archive, self.ci, self.manifest["runtime_images"], suffix)
        await_check("self-managed source checkpoint",
                    lambda: verify_equal(checkpoint["sources"], self.sources.local_state(clients)), timeout=900)

        print("Resetting Atlassian and GitHub demo fixtures", flush=True)
        apply_cloud(clients, cloud_plan(cloud, self.sources.cloud_state(clients)))
        await_check("Atlassian fixture checkpoint", lambda: verify_equal(cloud, self.sources.cloud_state(clients)))
        github_call(lambda api: github.reset(api, github_state, True, (COMMENT_AUTHOR,)))
        github_call(lambda api: verify_equal(github_state, github.snapshot(api)))

        self.product.up(*services)
        for plant, pins in images:
            verify_images(plant, pins)
        await_check("craftPulse readiness", lambda: clients["craftpulse"].request("/readyz"))
        self.verify_versions()
        print(json.dumps({"snapshot": "restored", "seconds": round(time.monotonic() - started, 3),
                          "rows": checkpoint["rows"], "objects": len(objects),
                          "source_checkpoints": "matched", "version_check": "matched"}))


def check_override(compose):
    path = compose.directory / OVERRIDE
    require(not path.exists() or read_json(path).get("x-craftpulse-snapshot") is True,
            f"{compose.project}: existing Compose override is managed externally")


def verify_equal(expected, actual):
    # Report no fixture names, field values, comments, or object keys.
    require(not differences(expected, actual), "checkpoint differs")


def required_files(databases, object_names):
    return {
        "manifest.json", "roles.sql.age", "bucket.json.age", "cloud.json.age", "github.json.age",
        *(name + ".age" for name in SOURCE_ARCHIVES),
        *(name + ".dump.age" for name in databases), *(name + ".age" for name in object_names),
    }


def require_objects(objects, checkpoint, databases):
    require(isinstance(objects, list) and len(objects) == checkpoint["object_count"], "invalid bucket inventory")
    names = []
    keys = set()
    for index, item in enumerate(objects):
        name = f"object-{index:08d}"
        require(item["file"] == name and SHA256.fullmatch(item["sha256"]) and
                type(item["size"]) is int and item["size"] >= 0 and isinstance(item["key"], str) and
                item["key"] and "\0" not in item["key"] and item["key"] not in keys, "invalid bucket object entry")
        keys.add(item["key"])
        names.append(name)
    require(set(checkpoint["files"]) == required_files(databases, names), "unexpected checkpoint file inventory")


def open_snapshot(root, checksum):
    require(SHA256.fullmatch(checksum), "provide the recorded checkpoint SHA-256")
    path = root / CHECKPOINT
    require(not path.is_symlink() and digest(path) == checksum, "checkpoint SHA-256 differs")
    checkpoint = read_json(path)
    files = checkpoint["files"]
    require(checkpoint["format"] == 1 and "manifest.json" in files, "invalid checkpoint")
    require({path.name for path in root.iterdir()} == {CHECKPOINT, *files}, "snapshot file inventory differs")
    for name, expected in files.items():
        require(re.fullmatch(r"[a-z0-9_.-]+\.(?:json|age)", name) and SHA256.fullmatch(expected),
                "invalid checkpoint file name or SHA-256")
        entry = root / name
        require(entry.is_file() and not entry.is_symlink() and digest(entry) == expected,
                "snapshot file SHA-256 differs")
    return checkpoint


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    manifest = commands.add_parser("manifest", help="compose a manifest from source and OCI release records")
    manifest.add_argument("--release", required=True, type=Path)
    manifest.add_argument("--runtime", type=Path, default=HERE.parent / "instance.json")
    manifest.add_argument("--output", type=Path)
    verify = commands.add_parser("verify", help="compare actual source and craftPulse versions")
    verify.add_argument("--manifest", type=Path, default=MANIFEST)
    verify.add_argument("--local", action="store_true", help="use local Caddy on the stand, with TLS verification")
    capture = commands.add_parser("capture", help="capture initial stand state")
    capture.add_argument("--manifest", type=Path, default=MANIFEST)
    capture.add_argument("--snapshot", required=True, type=Path)
    capture.add_argument("--recipient", required=True)
    restore = commands.add_parser("restore", help="restore and verify initial stand state")
    restore.add_argument("--snapshot", required=True, type=Path)
    restore.add_argument("--checkpoint-sha256", required=True)
    restore.add_argument("--identity", required=True, type=Path)
    restore.add_argument("--apply", action="store_true", help="replace the demo stand state")
    for subcommand in (capture, restore):
        subcommand.add_argument("--password-directory", type=Path, default=SECRET_DIR)
    args = parser.parse_args()
    os.umask(0o077)
    if args.command == "manifest":
        content = json_bytes(build_manifest(read_json(args.release), read_json(args.runtime)))
        if args.output:
            args.output.write_bytes(content)
        else:
            sys.stdout.buffer.write(content)
        return
    if args.command == "verify":
        stand = Stand(read_json(args.manifest), SECRET_DIR, local=args.local)
        stand.verify_versions()
        print("Stand versions match the manifest")
        return

    # Serialize snapshot operations on the Linux host even when the backup directories differ.
    import fcntl
    for command in ("docker", "age", "tar"):
        require(shutil.which(command), f"required command is unavailable: {command}")
    with (ROOT / ".snapshot.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.command == "capture":
            require(re.fullmatch(r"age1[a-z0-9]+", args.recipient), "provide an age public recipient")
            Stand(read_json(args.manifest), args.password_directory, local=True).capture(args.snapshot.resolve(), args.recipient)
        else:
            require(args.apply, "restore requires --apply")
            require(args.identity.is_file(), "age identity file is missing")
            checkpoint = open_snapshot(args.snapshot, args.checkpoint_sha256)
            archive = Archive(args.snapshot.resolve(), identity=args.identity.resolve())
            Stand(read_json(args.snapshot / "manifest.json"), args.password_directory, local=True).restore(archive, checkpoint)


if __name__ == "__main__":
    try:
        main()
    except StandError as error:
        sys.exit(str(error))
    except BlockingIOError:
        sys.exit("Another stand snapshot operation holds the lock")
    except Exception:
        sys.exit("Stand snapshot failed while reading configuration, data, or running a dependency")
