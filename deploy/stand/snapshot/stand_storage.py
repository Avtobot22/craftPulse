"""Encrypted stand artifacts and Compose-backed storage operations."""

from contextlib import contextmanager
from hashlib import file_digest, sha256
import json
import os
from pathlib import Path
import re
import subprocess as sp
import sys
import time
from uuid import uuid4

from stand_sources import IMAGE, StandError, json_bytes, require


ROOT = Path("/opt/craftpulse")
GITLAB_DATA = Path("/srv/craftpulse-gitlab")
GITLAB_CONFIG = (GITLAB_DATA / "config", Path("/etc/craftpulse-gitlab/secrets"),
                 Path("/etc/craftpulse-gitlab-runner"))
CI_CONFIG = (Path("/etc/craftpulse-ci90"), Path("/etc/craftpulse-ci90-agent"))
INIT = "/docker-entrypoint-initdb.d/10-craftpulse.sh"
OVERRIDE = "compose.override.yaml"
AWS_CLI = ("public.ecr.aws/aws-cli/aws-cli:2.40.0@"
           "sha256:0da258e32b17cc468b2e8812c8d93f5732561b901357c43a207baae0567b195a")
PG = ("psql", "-X", "-w", "-v", "ON_ERROR_STOP=1", "-U", "postgres")
DB_NAME = re.compile(r"craftpulse|adapter_[a-z][a-z0-9]{0,20}")
TABLE = re.compile(r"[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*")
CI_VOLUMES = {
    "jenkins-home": "craftpulse-ci90_jenkins-home",
    "db-data": "craftpulse-ci90_db-data",
    "sonar-data": "craftpulse-ci90_sonar-data",
    "sonar-extensions": "craftpulse-ci90_sonar-extensions",
    "sonar-logs": "craftpulse-ci90_sonar-logs",
}
CORE_TABLES = ("platform.tenant", "org.organization", "org.project", "org.app_user",
               "connect.source_instance", "connect.connection", "ingest.receipt", "ingest.record")
ADAPTER_TABLES = ("adapter.org_key", "adapter.credential", "adapter.user_token")
APP = ("server", "worker", "frontend")
CI = ("jenkins", "agent", "db", "sonarqube")


def digest(path):
    with path.open("rb") as stream:
        return file_digest(stream, "sha256").hexdigest()


def run(label, args, *, env=None, data=None, capture=True, timeout=3600):
    try:
        result = sp.run(args, input=data, stdout=sp.PIPE if capture else sp.DEVNULL,
                        stderr=sp.DEVNULL, env=env, timeout=timeout, check=False)
    except (OSError, sp.TimeoutExpired):
        raise StandError(f"{label}: command failed to start or timed out") from None
    require(result.returncode == 0, f"{label}: exit {result.returncode}")
    return result.stdout


def stream_digest(command):
    with sp.Popen(command, stdout=sp.PIPE, stderr=sp.DEVNULL) as process:
        with process.stdout:
            value = file_digest(process.stdout, "sha256").hexdigest()
        require(process.wait(timeout=3600) == 0, "SeaweedFS: download for SHA-256 failed")
    return value


def pipe(label, source, destination, *, checksum=False):
    """Stream without writing plaintext and check both ends of the pipeline."""
    digestor = sha256()
    with sp.Popen(source, stdout=sp.PIPE, stderr=sp.DEVNULL) as reader:
        try:
            with sp.Popen(destination, stdin=sp.PIPE if checksum else reader.stdout,
                          stdout=sp.DEVNULL, stderr=sp.DEVNULL) as writer:
                try:
                    if checksum:
                        with writer.stdin:
                            for block in iter(lambda: reader.stdout.read(1024 * 1024), b""):
                                digestor.update(block)
                                writer.stdin.write(block)
                    else:
                        reader.stdout.close()
                    require(writer.wait(timeout=3600) == 0, f"{label}: destination failed")
                finally:
                    if writer.poll() is None:
                        writer.kill()
        finally:
            reader.stdout.close()
            if reader.poll() is None and sys.exc_info()[0]:
                reader.kill()
        require(reader.wait(timeout=3600) == 0, f"{label}: source failed")
    return digestor.hexdigest() if checksum else None


class Archive:
    def __init__(self, root, *, recipient=None, identity=None):
        self.root = root
        self.recipient = recipient
        self.identity = identity

    def encryption(self, name):
        return ["age", "-r", self.recipient, "-o", str(self.root / (name + ".age"))]

    def decryption(self, name):
        return ["age", "-d", "-i", str(self.identity), str(self.root / (name + ".age"))]

    def capture(self, name, command, *, checksum=False):
        return pipe(name, command, self.encryption(name), checksum=checksum)

    def save(self, name, data):
        run(name, self.encryption(name), data=data)

    def read(self, name):
        return run(name, self.decryption(name))

    def restore(self, name, command):
        pipe(name, self.decryption(name), command)


class Compose:
    def __init__(self, directory, project):
        self.directory = directory
        self.project = project

    def command(self, *args):
        base = ["docker", "compose", "--project-directory", str(self.directory),
                "-p", self.project, "-f", str(self.directory / "compose.yaml")]
        override = self.directory / OVERRIDE
        if override.exists():
            base += ["-f", str(override)]
        return base + list(args)

    def run(self, label, *args, **options):
        return run(label, self.command(*args), **options)

    def services(self):
        return json.loads(self.run(self.project, "config", "--format", "json"))["services"]

    def exec(self, service, *args):
        return self.command("exec", "-T", service, *args)

    def up(self, *services):
        self.run(self.project, "up", "-d", "--wait", "--wait-timeout", "900", *services)

    def stop(self, *services):
        self.run(self.project, "stop", "-t", "60", *services)

    def override(self, services, volumes=None):
        config = {"services": services}
        if volumes:
            config["volumes"] = volumes
        target = self.directory / OVERRIDE
        if target.exists():
            require(json.loads(target.read_bytes()).get("x-craftpulse-snapshot") is True,
                    f"{self.project}: existing Compose override is managed externally")
        config["x-craftpulse-snapshot"] = True
        tmp = target.with_name(f".{OVERRIDE}.{uuid4().hex}")
        tmp.write_bytes(json_bytes(config))
        tmp.replace(target)

    def image(self, service):
        container = self.run(service, "ps", "-q", service).decode().strip()
        require(container and "\n" not in container, f"{service}: expected one running container")
        return run(service, ["docker", "inspect", "--format", "{{.Config.Image}}", container]).decode().strip()


@contextmanager
def paused(compose, *services):
    compose.stop(*services)
    try:
        yield
    finally:
        compose.up(*services)


def await_check(label, check, timeout=240):
    end = time.monotonic() + timeout
    while True:
        try:
            return check()
        except StandError:
            if time.monotonic() >= end:
                raise StandError(f"{label}: did not reach the expected state") from None
            time.sleep(3)


class Postgres:
    def __init__(self, compose, release):
        self.compose = compose
        self.release = release

    def sql(self, database, sql):
        return self.compose.run("PostgreSQL", "exec", "-T", "postgres", *PG,
                                "-At", "-d", database, data=sql.encode()).decode().strip()

    def databases(self):
        values = self.sql("postgres", "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate "
                          "AND datname <> 'postgres' ORDER BY datname").splitlines()
        require("craftpulse" in values and all(DB_NAME.fullmatch(name) for name in values),
                "PostgreSQL: non-stand database or missing core database")
        return values

    def product_services(self, databases):
        services = list(APP)
        configured = self.compose.services()
        for database in databases:
            if database.startswith("adapter_"):
                service = "adapter-" + database.removeprefix("adapter_")
                require(service in configured, "PostgreSQL: adapter database has no matching Compose service")
                services.append(service)
        require(set(configured) - {"postgres", "seaweedfs", "migrate", "install"}
                - {name for name in configured if name.endswith("-migrate")} == set(services),
                "craftPulse: unexpected running service in product Compose configuration")
        return services

    def checkpoint(self, databases):
        rows = {}
        for database in databases:
            core = database == "craftpulse"
            expected = (self.release["core_schema_version"] if core else
                        self.release["adapter_schema_versions"]["craftpulse-" + database.replace("_", "-", 1)])
            schema = int(self.sql(database, "SELECT coalesce(max(version_id), 0) "
                                  "FROM public.goose_db_version WHERE is_applied"))
            require(schema == expected, f"PostgreSQL: unexpected schema version in {database}")
            tables = CORE_TABLES if core else ADAPTER_TABLES
            counts = self.sql(database, " UNION ALL ".join(
                f"SELECT '{table}', count(*) FROM {table}" for table in tables
            ))
            rows[database] = {table: int(count) for table, count in
                              (line.split("|") for line in counts.splitlines())}
        return rows

    def backup(self, archive, databases):
        archive.capture("roles.sql", self.compose.exec(
            "postgres", "pg_dumpall", "-w", "-U", "postgres", "--roles-only", "--no-role-passwords"
        ))
        for database in databases:
            archive.capture(database + ".dump", self.compose.exec(
                "postgres", "sh", "-ec",
                'export PGPASSWORD="$(cat "$CRAFTPULSE_BACKUP_PASSWORD_FILE")"; '
                'exec pg_dump -w -h 127.0.0.1 -U craftpulse_backup --enable-row-security -Fc --create -d "$1"',
                "pg_dump", database,
            ))

    def passwords(self, databases, directory):
        files = {"craftpulse_owner": directory / "owner-password",
                 "craftpulse_app": directory / "app-password",
                 "craftpulse_backup": directory / "backup-password"}
        for name in databases:
            if name.startswith("adapter_"):
                files[name] = directory / f"{name}-password"
                files[name + "_owner"] = directory / f"{name}-owner-password"
        statements = []
        for role, path in files.items():
            password = path.read_bytes().rstrip(b"\r\n")
            require(password and not any(byte in password for byte in b"\r\n\0"),
                    "PostgreSQL: missing or invalid role password file")
            statements.append("SELECT format('ALTER ROLE %I PASSWORD %L', "
                              f"'{role}', convert_from(decode('{password.hex()}', 'hex'), 'UTF8')) \\gexec")
        return ("SET log_min_duration_statement = -1;\nSET log_statement = 'none';\n"
                "SET log_min_error_statement = 'panic';\n" + "\n".join(statements))

    def restore(self, archive, databases, passwords):
        roles = archive.read("roles.sql")
        create_postgres = b"CREATE ROLE postgres;\n"
        require(roles.count(create_postgres) == 1, "PostgreSQL: unexpected bootstrap role in dump")
        self.compose.run("PostgreSQL roles", "exec", "-T", "postgres", *PG, "-d", "postgres",
                         data=roles.replace(create_postgres, b"", 1), capture=False)
        self.sql("postgres", passwords)
        for database in databases:
            archive.restore(database + ".dump", self.compose.exec(
                "postgres", "pg_restore", "-w", "-U", "postgres", "--exit-on-error", "--create", "-d", "postgres"
            ))
        require(self.databases() == databases, "PostgreSQL: restored database list differs")


class Bucket:
    def __init__(self, compose, directory):
        container = compose.run("SeaweedFS", "ps", "-q", "seaweedfs").decode().strip()
        require(re.fullmatch(r"[a-f0-9]{12,64}", container), "SeaweedFS: expected one running container")
        secret = (directory / "object-secret").resolve()
        require(secret.is_file(), "SeaweedFS: object-secret is missing")
        self.cli = [
            "docker", "run", "--rm", "-i", "--network", "container:" + container,
            "--mount", f"type=bind,source={secret},target=/run/secrets/object-secret,readonly",
            "-e", "AWS_ACCESS_KEY_ID=craftpulse", "-e", "AWS_DEFAULT_REGION=us-east-1",
            "-e", "AWS_EC2_METADATA_DISABLED=true", "-e", "AWS_REQUEST_CHECKSUM_CALCULATION=WHEN_REQUIRED",
            "-e", "AWS_RESPONSE_CHECKSUM_VALIDATION=WHEN_REQUIRED",
            "--entrypoint", "/bin/sh", AWS_CLI, "-ec",
            'export AWS_SECRET_ACCESS_KEY="$(tr -d \'\\r\\n\' < /run/secrets/object-secret)"; '
            'exec aws --no-cli-pager --endpoint-url http://127.0.0.1:8333 "$@"', "aws",
        ]

    def keys(self):
        result = run("SeaweedFS inventory", self.cli + [
            "s3api", "list-objects-v2", "--bucket", "craftpulse", "--output", "json",
        ])
        return {item["Key"]: item["Size"] for item in json.loads(result).get("Contents", [])}

    def download(self, key):
        return self.cli + ["s3", "cp", "--only-show-errors", "--", "s3://craftpulse/" + key, "-"]

    def backup(self, archive):
        records = []
        for index, (key, size) in enumerate(sorted(self.keys().items())):
            name = f"object-{index:08d}"
            if size == 0:
                # SeaweedFS rejects the ranged GET that aws s3 cp uses for empty objects.
                archive.save(name, b"")
                digest = sha256(b"").hexdigest()
            else:
                digest = archive.capture(name, self.download(key), checksum=True)
            records.append({"key": key, "size": size, "file": name, "sha256": digest})
        archive.save("bucket.json", json_bytes(records))
        return len(records)

    def restore(self, archive, records):
        require(self.keys() == {}, "SeaweedFS: expected an empty restore bucket")
        for entry in records:
            archive.restore(entry["file"], self.cli + [
                "s3", "cp", "--only-show-errors", "--expected-size", str(entry["size"]),
                "--", "-", "s3://craftpulse/" + entry["key"],
            ])

    def verify(self, records):
        require(self.keys() == {entry["key"]: entry["size"] for entry in records}, "SeaweedFS: object inventory differs")
        for entry in records:
            actual = sha256(b"").hexdigest() if entry["size"] == 0 else stream_digest(self.download(entry["key"]))
            require(actual == entry["sha256"], "SeaweedFS: object SHA-256 differs")


def rotate(paths):
    for path in paths:
        if path.exists():
            path.rename(path.with_name(f"{path.name}.before-snapshot-{uuid4().hex[:12]}"))


def tar_capture(archive, name, paths):
    require(all(path.exists() for path in paths), f"{name}: state directory is missing")
    archive.capture(name, ["tar", "--numeric-owner", "-czf", "-", "-C", "/",
                           *[str(path).lstrip("/") for path in paths]])


def tar_restore(archive, name, root=Path("/")):
    archive.restore(name, ["tar", "--numeric-owner", "-xzf", "-", "-C", str(root)])


def volume_path(name):
    item, = json.loads(run("inspect CI source volume", ["docker", "volume", "inspect", name]))
    path = Path(item["Mountpoint"])
    require(item["Driver"] == "local" and not item["Options"] and path.is_absolute() and
            path.name == "_data" and path.parent.name == name, "CI sources: unexpected Docker volume driver or path")
    return path


def save_gitlab(archive, gitlab, runner):
    backup_id = "cp92_" + uuid4().hex
    with paused(runner, "runner"):
        gitlab.run("GitLab data backup", "exec", "-T", "gitlab", "gitlab-backup", "create", "BACKUP=" + backup_id)
        name = f"/var/opt/gitlab/backups/{backup_id}_gitlab_backup.tar"
        try:
            archive.capture("gitlab.tar", gitlab.exec("gitlab", "cat", name))
            tar_capture(archive, "gitlab-config.tar.gz", GITLAB_CONFIG)
        finally:
            gitlab.run("remove temporary GitLab archive", "exec", "-T", "gitlab", "rm", "-f", "--", name)


def restore_gitlab(archive, gitlab, runner):
    runner.stop("runner")
    gitlab.stop("gitlab")
    rotate((*GITLAB_CONFIG, GITLAB_DATA / "data"))
    tar_restore(archive, "gitlab-config.tar.gz")
    gitlab.run("GitLab pinned start", "up", "-d", "--force-recreate", "--wait", "--wait-timeout", "900", "gitlab")
    name = "/var/opt/gitlab/backups/cp92_restore_gitlab_backup.tar"
    archive.restore("gitlab.tar", gitlab.exec("gitlab", "sh", "-ec", 'cat > "$1"; chown git:git "$1"', "restore", name))
    gitlab.run("GitLab Puma stop", "exec", "-T", "gitlab", "gitlab-ctl", "stop", "puma")
    gitlab.run("GitLab Sidekiq stop", "exec", "-T", "gitlab", "gitlab-ctl", "stop", "sidekiq")
    try:
        gitlab.run("GitLab native restore", "exec", "-T", "-e", "GITLAB_ASSUME_YES=1", "gitlab",
                   "gitlab-backup", "restore", "BACKUP=cp92_restore")
    finally:
        gitlab.run("remove temporary GitLab archive", "exec", "-T", "gitlab", "rm", "-f", "--", name)
    gitlab.run("GitLab reconfigure", "exec", "-T", "gitlab", "gitlab-ctl", "reconfigure")
    gitlab.run("GitLab restart", "exec", "-T", "gitlab", "gitlab-ctl", "restart")
    runner.run("GitLab Runner start", "up", "-d", "--force-recreate", "runner")


def ci_volume_names(ci):
    config = json.loads(ci.run("CI volume config", "config", "--format", "json"))
    return {logical: config["volumes"][logical]["name"] for logical in CI_VOLUMES}


def save_ci(archive, ci):
    with paused(ci, *CI):
        tar_capture(archive, "ci-config.tar.gz", CI_CONFIG)
        for logical, name in ci_volume_names(ci).items():
            archive.capture(logical + ".tar.gz",
                            ["tar", "--numeric-owner", "-czf", "-", "-C", str(volume_path(name)), "."])


def restore_ci(archive, ci, images, suffix):
    ci.stop(*CI)
    vols = {}
    for logical, name in CI_VOLUMES.items():
        target = f"{name}-snapshot-{suffix}"
        run("create CI source volume", ["docker", "volume", "create", target])
        tar_restore(archive, logical + ".tar.gz", volume_path(target))
        vols[logical] = {"name": target}
    rotate(CI_CONFIG)
    tar_restore(archive, "ci-config.tar.gz")
    ci.override({name: {"image": images[f"craftpulse-ci90-{name}"]} for name in CI}, vols)
    ci.up(*CI)


def runtime_images(manifest, services):
    images = manifest["runtime_images"]
    product = {name: images[f"craftpulse-product-{name}-1"] for name in ("postgres", "seaweedfs")}
    release = manifest["craftpulse"]["images"]
    product.update({name: release["craftpulse-server" if name in ("server", "worker")
                                  else "craftpulse-" + name] for name in services})
    require(all(IMAGE.fullmatch(ref) for ref in (*images.values(), *product.values())),
            "stand manifest contains an unpinned runtime image")
    return product


def verify_images(compose, expected):
    for service, image in expected.items():
        require(compose.image(service) == image, f"{compose.project}: {service} image differs from manifest")
