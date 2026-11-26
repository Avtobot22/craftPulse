
#!/usr/bin/env python3
"""Deploy or roll back a published craftPulse release from the operator workstation."""
import argparse
import base64
import json
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path, PurePosixPath

from prepare import runtime_files


HERE = Path(__file__).resolve().parent
REGISTRY = "ghcr.io/avtobot22"
SHA = re.compile(r"[0-9a-f]{40}")
IMAGE = re.compile(r"craftpulse-(?:server|frontend|ci(?:-[a-z0-9-]+)?|adapter-[a-z][a-z0-9]{0,20})")
ADAPTER = "craftpulse-adapter-"
SLUG = re.compile(r"[a-z0-9-]{2,40}")
CORE_FILES = {
    "postgres-password": 70, "postgres-owner-password": 70, "postgres-app-password": 70,
    "postgres-backup-password": 70, "owner-password": 65532, "app-password": 65532,
    "core-master-key": 65532, "object-key": 65532, "grafana-password": 472, "seaweedfs.json": 1000,
}
ADAPTER_FILES = ("owner-password", "app-password", "master-key", "service-key", "tls-cert.pem", "tls-key.pem")
PREVIOUS_SERVICE_KEY = "previous-service-key"


class RolloutError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise RolloutError(message)


def run(label, args, *, data=None, output=subprocess.PIPE, timeout=300):
    try:
        result = subprocess.run(args, input=data, stdout=output, stderr=subprocess.DEVNULL,
                                timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise RolloutError(f"{label} failed to run or timed out") from None
    if result.returncode:
        raise RolloutError(f"{label} failed (exit {result.returncode})")
    return result.stdout


def validate_manifest(raw, sha):
    require(isinstance(raw, dict) and raw.get("schema_version") == 1 and raw.get("commit_sha") == sha,
            "manifest does not identify the requested release")
    images = raw.get("images")
    require(isinstance(images, dict) and {"craftpulse-server", "craftpulse-frontend"} <= images.keys(),
            "manifest must contain server and frontend images")
    for name, ref in images.items():
        require(IMAGE.fullmatch(name) and isinstance(ref, str) and
                re.fullmatch(rf"{re.escape(REGISTRY)}/{name}@sha256:[0-9a-f]{{64}}", ref),
                "invalid image digest in manifest")
    schemas = raw.get("adapter_schema_versions")
    require(isinstance(schemas, dict) and
            set(schemas) == {name for name in images if name.startswith(ADAPTER)},
            "manifest must specify a schema for every published adapter")
    core = raw.get("core_schema_version")
    require(all(type(value) is int and value >= 0 for value in (core, *schemas.values())),
            "invalid schema version in manifest")
    contract = raw.get("adapter_contract_version")
    require(not schemas or (isinstance(contract, str) and re.fullmatch(r"[0-9]+(?:\.[0-9]+){0,2}", contract)),
            "invalid adapter contract version in manifest")
    return {key: raw.get(key) for key in ("schema_version", "commit_sha", "images",
            "core_schema_version", "adapter_schema_versions", "adapter_contract_version")}


def fetch_manifest(sha, oras):
    with tempfile.TemporaryDirectory(prefix="craftpulse-release-") as directory:
        run("download manifest", [oras, "pull", "--output", directory, f"{REGISTRY}/craftpulse-release:{sha}"])
        path = Path(directory) / "release-manifest.json"
        return validate_manifest(json.loads(path.read_bytes()), sha)


def secret(values, name):
    value = values.get(name)
    require(isinstance(value, str) and value and not any(ch in value for ch in "\r\n\0"),
            f"invalid secret field {name}")
    return value.encode("utf-8") + b"\n"


def certificates(openssl, ca_cert, ca_key, adapters):
    # Only the public CA and signed leaf credentials cross the SSH connection.
    with tempfile.TemporaryDirectory(prefix="craftpulse-ca-") as directory:
        path = Path(directory)
        ca = path / "ca.pem"
        ca.write_bytes(ca_cert)
        key = path / "ca.key"
        with open(key, "xb", opener=private_file) as stream:
            stream.write(ca_key)
        run("check installation CA lifetime", [openssl, "x509", "-in", str(ca), "-noout", "-checkend", "31536000"])
        run("check installation CA", [openssl, "verify", "-check_ss_sig", "-CAfile", str(ca), str(ca)])
        result = {"adapter-ca.pem": ca_cert}
        for adapter in adapters:
            dns = f"adapter-{adapter}"
            leaf = path / "leaf.key"
            csr = path / "leaf.csr"
            cert = path / "leaf.pem"
            extensions = path / "extensions.cnf"
            extensions.write_text(
                f"basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\n"
                f"extendedKeyUsage=serverAuth\nsubjectAltName=DNS:{dns}\n", encoding="ascii",
            )
            run("generate adapter key", [openssl, "genpkey", "-algorithm", "ED25519", "-out", str(leaf)])
            run("generate adapter request", [openssl, "req", "-new", "-key", str(leaf),
                                              "-subj", f"/CN={dns}", "-out", str(csr)])
            run("sign adapter certificate", [openssl, "x509", "-req", "-in", str(csr),
                "-CA", str(ca), "-CAkey", str(key), "-CAserial", str(path / "ca.srl"), "-CAcreateserial",
                "-days", "365", "-extfile", str(extensions), "-out", str(cert)])
            run("verify issued adapter certificate", [openssl, "verify", "-purpose", "sslserver",
                "-verify_hostname", dns, "-CAfile", str(ca), str(cert)])
            result[f"adapters/{adapter}/tls-cert.pem"] = cert.read_bytes()
            result[f"adapters/{adapter}/tls-key.pem"] = leaf.read_bytes()
        return result


def private_file(path, flags):
    return os.open(path, flags, 0o600)


def process_files(bundle, manifest, openssl):
    require(isinstance(bundle, dict), "invalid SOPS bundle")
    files = {name: value.encode("utf-8") + b"\n" for name, (_, value) in runtime_files(bundle).items()}
    orgs = bundle.get("oidc_client_secrets")
    require(isinstance(orgs, dict) and orgs and all(SLUG.fullmatch(slug) for slug in orgs),
            "oidc_client_secrets must map organization slugs to secrets")
    for slug in orgs:
        files[f"oidc/{slug}"] = secret(orgs, slug)
    adapters = bundle.get("adapters", {})
    require(isinstance(adapters, dict) and {ADAPTER + name for name in adapters} <= manifest["adapter_schema_versions"].keys(),
            "configured adapter is absent from the release")
    if adapters:
        for name, values in adapters.items():
            require(isinstance(values, dict), "invalid adapter secret fields")
            for field in ("owner_password", "app_password", "master_key", "service_key"):
                files[f"adapters/{name}/{field.replace('_', '-')}"] = secret(values, field)
            if "previous_service_key" in values:
                previous = secret(values, "previous_service_key")
                require(previous != files[f"adapters/{name}/service-key"],
                        "previous adapter service key must differ from the current key")
                files[f"adapters/{name}/{PREVIOUS_SERVICE_KEY}"] = previous
            require(len(base64.b64decode(values["master_key"], validate=True)) == 32,
                    "adapter master key must encode 32 bytes")
        ca_cert, ca_key = bundle.get("adapter_ca_cert"), bundle.get("adapter_ca_key")
        require(all(isinstance(value, str) and value for value in (ca_cert, ca_key)),
                "adapter_ca_cert and adapter_ca_key are required with adapters")
        files.update(certificates(openssl, ca_cert.encode(), ca_key.encode(), sorted(adapters)))
    return sorted(adapters), {name: base64.b64encode(value).decode("ascii") for name, value in files.items()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="defects", help="SSH host or alias")
    parser.add_argument("--stand-dir", type=PurePosixPath, default=PurePosixPath("/opt/craftpulse/profile-d"))
    parser.add_argument("--state", type=PurePosixPath, default=PurePosixPath("/var/lib/craftpulse-profile-d/rollout"))
    commands = parser.add_subparsers(dest="action", required=True)
    deploy = commands.add_parser("deploy")
    deploy.add_argument("--bundle", type=Path, default=HERE / "stand.sops.json")
    deploy.add_argument("--oras", default="oras")
    deploy.add_argument("--sops", default="sops")
    deploy.add_argument("--openssl", default="openssl")
    deploy.add_argument("sha")
    commands.add_parser("rollback").add_argument("sha")
    args = parser.parse_args()
    if not SHA.fullmatch(args.sha):
        parser.error("specify the full 40-character SHA")
    if not re.fullmatch(r"[A-Za-z0-9_.@-]+", args.host) or args.host.startswith("-"):
        parser.error("invalid SSH host")
    if not args.stand_dir.is_absolute() or not args.state.is_absolute():
        parser.error("stand directory and state must be absolute paths")
    try:
        request = {"action": args.action, "sha": args.sha}
        if args.action == "deploy":
            manifest = fetch_manifest(args.sha, args.oras)
            raw = run("decrypt SOPS bundle", [args.sops, "decrypt", "--output-type", "json", str(args.bundle)])
            request["adapters"], request["files"] = process_files(json.loads(raw), manifest, args.openssl)
            request["manifest"] = manifest
        command = ["sudo", "-n", "python3", str(args.stand_dir / "deploy/profile-d/rollout_host.py"), str(args.state)]
        ssh_env = {key: value for key, value in os.environ.items() if not key.upper().startswith("SOPS_AGE_")}
        result = subprocess.run(["ssh", "-T", "-a", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
                                 "-o", "ConnectTimeout=10", args.host, shlex.join(command)],
                                input=json.dumps(request).encode("utf-8"), env=ssh_env, check=False)
        return result.returncode
    except RolloutError as exc:
        print(exc, file=sys.stderr)
    except (KeyError, OSError, ValueError, TypeError):
        print("Could not prepare the release or contact the stand", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
