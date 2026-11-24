#!/usr/bin/env python3
"""Prepare release references and private runtime files for the Compose stand."""
import argparse
import base64
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
import tempfile


_ROOT = Path(__file__).resolve().parents[2]
_BUNDLE = "deploy/profile-d/stand.sops.json"
_CORE_UID = 65532
_POSTGRES_UID = 70
_SEAWEEDFS_UID = 1000
_GRAFANA_UID = 472
_PASSWORDS = (
    "postgres_password",
    "owner_password",
    "app_password",
    "backup_password",
    "object_key",
    "grafana_password",
)


def generated_values():
    values = {name: secrets.token_urlsafe(36) for name in _PASSWORDS}
    values["core_master_key"] = base64.b64encode(secrets.token_bytes(32)).decode()
    return values


def materialize(values, destination):
    if os.name != "posix" or os.geteuid() != 0:
        raise ValueError("materialize on Linux as root so container UID restrictions are applied")
    for name in (*_PASSWORDS, "core_master_key"):
        value = values.get(name)
        if not isinstance(value, str) or not value or any(ch in value for ch in "\r\n\0"):
            raise ValueError(f"invalid required secret: {name}")
    if len(base64.b64decode(values["core_master_key"], validate=True)) != 32:
        raise ValueError("core_master_key must encode 32 bytes")
    destination = destination.resolve()
    if destination.is_relative_to(_ROOT) or destination.exists():
        raise ValueError("use a new secrets directory outside the checkout")
    if not destination.parent.is_dir():
        raise ValueError("create the parent directory before materializing")
    objects = {"identities": [{
        "name": "craftpulse",
        "credentials": [{"accessKey": "craftpulse", "secretKey": values["object_key"]}],
        "actions": ["Read:craftpulse", "Write:craftpulse", "List:craftpulse", "Tagging:craftpulse"],
    }]}
    files = {
        "postgres-password": (_POSTGRES_UID, values["postgres_password"]),
        "postgres-owner-password": (_POSTGRES_UID, values["owner_password"]),
        "postgres-app-password": (_POSTGRES_UID, values["app_password"]),
        "postgres-backup-password": (_POSTGRES_UID, values["backup_password"]),
        "owner-password": (_CORE_UID, values["owner_password"]),
        "app-password": (_CORE_UID, values["app_password"]),
        "core-master-key": (_CORE_UID, values["core_master_key"]),
        "object-key": (_CORE_UID, values["object_key"]),
        "grafana-password": (_GRAFANA_UID, values["grafana_password"]),
        "seaweedfs.json": (_SEAWEEDFS_UID, json.dumps(objects)),
    }
    with tempfile.TemporaryDirectory(prefix=".profile-d-", dir=destination.parent) as tmp:
        staged = Path(tmp) / "secrets"
        staged.mkdir(mode=0o700)
        for name, (uid, value) in files.items():
            fd = os.open(staged / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o400)
            with os.fdopen(fd, "w", encoding="utf-8") as stream:
                stream.write(value + "\n")
                os.fchown(stream.fileno(), uid, uid)
        if destination.exists():
            raise ValueError("destination appeared during materialization")
        staged.rename(destination)


def release_env(path):
    data = json.loads(path.read_text(encoding="utf-8"))
    images = data["images"]
    for image, env in (
        ("craftpulse-server", "CRAFTPULSE_SERVER_IMAGE"),
        ("craftpulse-frontend", "CRAFTPULSE_FRONTEND_IMAGE"),
    ):
        reference = images[image]
        if not re.fullmatch(rf"ghcr\.io/avtobot22/{image}@sha256:[0-9a-f]{{64}}", reference):
            raise ValueError(f"invalid OCI release reference: {image}")
        print(f"{env}={reference}")


def encrypt(binary, destination):
    completed = subprocess.run(
        [binary, "--config", str(_ROOT / ".sops.yaml"), "encrypt", "--input-type", "json",
         "--output-type", "json", "--filename-override", _BUNDLE],
        cwd=_ROOT, input=json.dumps(generated_values()).encode(),
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if completed.returncode:
        raise RuntimeError("SOPS could not encrypt the bundle")
    with destination.open("xb") as stream:
        stream.write(completed.stdout)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    operations = parser.add_subparsers(dest="operation", required=True)
    for name in ("init-local", "materialize", "encrypt"):
        operations.add_parser(name).add_argument("destination", type=Path)
    operations.choices["encrypt"].add_argument("--sops", default="sops")
    operations.add_parser("release-env").add_argument("manifest", type=Path)
    args = parser.parse_args()
    try:
        if args.operation == "materialize":
            materialize(json.load(sys.stdin), args.destination)
        elif args.operation == "init-local":
            materialize(generated_values(), args.destination)
        elif args.operation == "encrypt":
            encrypt(args.sops, args.destination)
        else:
            release_env(args.manifest)
    except (KeyError, OSError, RuntimeError, ValueError) as exc:
        parser.exit(1, f"{exc}\n")


if __name__ == "__main__":
    main()
