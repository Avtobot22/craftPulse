"""Run the released server against disposable PostgreSQL and check /version."""
import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.request
import uuid

from oci_release import ROOT, schema_version


def docker(*args, check=True):
    return subprocess.run(["docker", *args], check=check, text=True, timeout=300,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def main():
    parser = argparse.ArgumentParser()
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--manifest", type=Path)
    source.add_argument("--image")
    parser.add_argument("--sha")
    parser.add_argument("--ci-image", help="craftpulse-ci image by digest for the catalog of check profiles")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.manifest:
        manifest = json.loads(args.manifest.read_text())
        args.image = manifest["images"]["craftpulse-server"]
        args.ci_image = manifest["images"]["craftpulse-ci"]
        args.sha = manifest["commit_sha"]
        expected_schema = manifest["core_schema_version"]
    else:
        if not args.sha or not args.ci_image:
            parser.error("--sha and --ci-image are required with --image")
        expected_schema = schema_version(ROOT, "core")
    step_image, _, step_digest = args.ci_image.partition("@")
    if not step_digest.startswith("sha256:"):
        parser.error("the craftpulse-ci image must be given by digest")
    prefix = "craftpulse-83-" + uuid.uuid4().hex[:10]
    db, api = prefix + "-db", prefix + "-api"
    try:
        with tempfile.TemporaryDirectory(prefix="craftpulse-83-secrets-") as temporary:
            directory = Path(temporary)
            directory.chmod(0o755)
            (directory / "password").write_text(secrets.token_urlsafe(24))
            (directory / "master").write_text(base64.b64encode(os.urandom(32)).decode())
            (directory / "check-profiles.yaml").write_text(json.dumps(
                {"version": 1, "step": {"image": step_image, "digest": step_digest}, "profiles": []}) + "\n")
            for path in directory.iterdir():
                path.chmod(0o444)
            docker("network", "create", prefix)
            docker("run", "-d", "--name", db, "--network", prefix,
                   "-v", str(directory) + ":/smoke:ro",
                   "-v", str(ROOT / "deploy/postgres/init.sh") + ":/docker-entrypoint-initdb.d/init.sh:ro",
                   "-e", "POSTGRES_PASSWORD_FILE=/smoke/password",
                   "-e", "CRAFTPULSE_OWNER_PASSWORD_FILE=/smoke/password",
                   "-e", "CRAFTPULSE_APP_PASSWORD_FILE=/smoke/password",
                   "-e", "CRAFTPULSE_BACKUP_PASSWORD_FILE=/smoke/password",
                   "postgres:18-alpine")
            for _ in range(90):
                if docker("exec", db, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", check=False).returncode == 0:
                    break
                time.sleep(1)
            else:
                raise RuntimeError("PostgreSQL did not become ready")
            common = ["--network", prefix, "-v", str(directory) + ":/smoke:ro",
                      "-e", "DATABASE_PASSWORD_FILE=/smoke/password",
                      "-e", "CORE_MASTER_KEY_FILE=/smoke/master",
                      "-e", "CORE_MASTER_KEY_VERSION=1",
                      "-e", "OBJECT_STORE_ENDPOINT=http://127.0.0.1:9000",
                      "-e", "OBJECT_STORE_BUCKET=smoke",
                      "-e", "OBJECT_STORE_ACCESS_KEY_ID=smoke",
                      "-e", "OBJECT_STORE_SECRET_ACCESS_KEY_FILE=/smoke/password",
                      "-e", "SITE_URL=http://127.0.0.1",
                      "-e", "CHECK_PROFILES_FILE=/smoke/check-profiles.yaml"]
            migration = docker("run", "--rm", *common,
                               "-e", "DATABASE_URL=postgres://craftpulse_owner@" + db + ":5432/craftpulse?sslmode=disable",
                               args.image, "/usr/local/bin/api", "migrate", check=False)
            if migration.returncode:
                raise RuntimeError(migration.stdout + migration.stderr)
            docker("run", "-d", "--name", api, *common,
                   "-p", "127.0.0.1::8080",
                   "-e", "DATABASE_URL=postgres://craftpulse_app@" + db + ":5432/craftpulse?sslmode=disable",
                   args.image)
            port = docker("port", api, "8080/tcp").stdout.strip().rsplit(":", 1)[1]
            for _ in range(60):
                try:
                    with urllib.request.urlopen("http://127.0.0.1:" + port + "/version", timeout=2) as response:
                        version = json.load(response)
                    break
                except (OSError, ValueError):
                    time.sleep(1)
            else:
                raise RuntimeError("API did not start: " + docker("logs", api, check=False).stdout)
            if version["commit_sha"] != args.sha or version["schema_version"] != expected_schema:
                raise RuntimeError("Server version disagrees with release: " + json.dumps(version))
            result = json.dumps({"image": args.image, "version": version})
            if args.output:
                args.output.write_text(result + "\n")
            print(result)
    finally:
        docker("rm", "-fv", api, db, check=False)
        docker("network", "rm", prefix, check=False)


if __name__ == "__main__":
    main()
