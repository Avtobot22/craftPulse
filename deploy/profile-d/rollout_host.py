
#!/usr/bin/env python3
"""Run a serialized profile D deployment on the Linux Docker host."""
import base64
import http.client
import json
import os
import socket
import ssl
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from uuid import UUID, uuid4

from rollout import (ADAPTER, ADAPTER_FILES, CORE_FILES, HERE, PREVIOUS_SERVICE_KEY, SHA, SLUG, RolloutError,
                     require, run, validate_manifest)


INFRASTRUCTURE = ("postgres", "seaweedfs")
OBSERVABILITY = ("victoriametrics", "victorialogs", "grafana", "cadvisor")
OBSERVABILITY_CONFIG = {
    "victoriametrics": {"scrape.yaml": "/etc/victoriametrics/scrape.yaml"},
    "grafana": {
        "grafana-alerts.yaml": "/etc/grafana/provisioning/alerting/craftpulse.yaml",
        "grafana-collection.json": "/etc/grafana/craftpulse-dashboards/collection.json",
    },
}
PROVIDERS = {"jiracloud": "jira_cloud", "confluencecloud": "confluence_cloud"}
PROBE_TIMEOUT = 90


def now():
    return datetime.now(timezone.utc).isoformat()


def write_json(path, data):
    temporary = path.with_suffix(".tmp")
    with temporary.open("w", encoding="utf-8") as stream:
        json.dump(data, stream, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)
    if os.name == "posix":
        fd = os.open(path.parent, os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)


def decode_files(encoded, adapters):
    require(isinstance(encoded, dict), "invalid process files")
    owners = dict(CORE_FILES)
    owners.update({f"adapters/{name}/{file}": 65532 for name in adapters for file in ADAPTER_FILES})
    for adapter in adapters:
        previous = f"adapters/{adapter}/{PREVIOUS_SERVICE_KEY}"
        if previous in encoded:
            owners[previous] = 65532
    if adapters:
        owners["adapter-ca.pem"] = 0
    for name in encoded:
        if name.startswith("oidc/") and SLUG.fullmatch(name.removeprefix("oidc/")):
            owners[name] = 65532
    require(owners.keys() == encoded.keys(), "unexpected or missing process files")
    return {name: (uid, base64.b64decode(encoded[name], validate=True)) for name, uid in owners.items()}


def previous_service_key(directory):
    try:
        return (directory / PREVIOUS_SERVICE_KEY).read_bytes()
    except FileNotFoundError:
        return None


def materialize_files(root, files):
    for name, (uid, value) in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        mode = 0o444 if name == "adapter-ca.pem" else 0o400
        with os.fdopen(os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, mode), "wb") as stream:
            stream.write(value)
            os.fchmod(stream.fileno(), mode)
            os.fchown(stream.fileno(), uid, uid)


def install_config(settings, files):
    config = settings["install"]
    require(isinstance(config, dict) and all(isinstance(v, str) and v for v in config.values()),
            "invalid installation parameters")
    slugs = config["INSTALL_ORGANIZATIONS"].split(",")
    require(len(slugs) == len(set(slugs)) and all(SLUG.fullmatch(slug) for slug in slugs),
            "invalid installation organization list")
    expected = {"INSTALL_ORGANIZATIONS"}
    for slug in slugs:
        prefix = "ORG_" + slug.upper().replace("-", "_") + "_"
        expected.update(prefix + field for field in ("NAME", "OIDC_ISSUER", "OIDC_CLIENT_ID", "ADMIN_SUBJECT"))
    require(config.keys() == expected and {name for name in files if name.startswith("oidc/")} ==
            {f"oidc/{slug}" for slug in slugs}, "installation settings and OIDC files must match")
    return config


def overrides(settings, adapters, manifest, secret_dir):
    network_env = {
        "INSTALLATION_NETWORKS": settings.get("installation_networks", ""),
        "SOURCE_PRIVATE_NETWORKS": settings.get("source_private_networks", ""),
    }
    services = {"nginx": {"volumes": [
        f"{settings['tls_dir']}/tls-cert.pem:/run/secrets/tls-cert.pem:ro",
        f"{settings['tls_dir']}/tls-key.pem:/run/secrets/tls-key.pem:ro",
    ]}}
    core = {"environment": dict(network_env)}
    if adapters:
        keys = "/run/secrets/adapter-keys"
        core["environment"].update({
            "ADAPTER_CA_FILE": "/run/secrets/adapter-ca.pem",
            "BUILTIN_ADAPTERS": ",".join(
                f"{PROVIDERS.get(name, name)}=https://adapter-{name}:8443" for name in adapters
            ),
            "ADAPTER_SERVICE_KEYS_DIR": keys,
        })
        core["volumes"] = [f"{secret_dir}/adapter-ca.pem:/run/secrets/adapter-ca.pem:ro"] + [
            f"{secret_dir}/adapters/{name}/service-key:{keys}/{PROVIDERS.get(name, name)}:ro"
            for name in adapters
        ]
    services.update(api=core, worker=core)
    for name in adapters:
        service = f"adapter-{name}"
        database = f"adapter_{name}"
        common_env = {
            "DATABASE_MAX_CONNS": "2",
            "GOMEMLIMIT": "96MiB" if name == "git" else "36MiB",
            "DATABASE_URL": f"postgres://{database}@postgres:5432/{database}?sslmode=disable",
            "DATABASE_PASSWORD_FILE": "/run/secrets/db-password",
            "ADAPTER_MASTER_KEY_VERSION": str(settings.get("adapter_master_key_versions", {}).get(name, 1)),
            "ADAPTER_MASTER_KEY_FILE": "/run/secrets/master-key",
        }
        def volume(file, target):
            return f"{secret_dir}/adapters/{name}/{file}:/run/secrets/{target}:ro"

        memory = "256m" if name == "git" else "48m"
        common = {
            "image": manifest["images"][ADAPTER + name],
            "cgroup_parent": "${CGROUP_PARENT:?Set the resource slice for the stand}",
            "cpus": "${CONTAINER_CPUS:-4}",
            "mem_limit": memory, "memswap_limit": memory,
        }
        services[service] = {
            **common, "restart": "unless-stopped", "stop_grace_period": "40s", "networks": ["data", "edge"],
            "environment": {
                **common_env, "HTTP_ADDR": ":8443",
                "TLS_CERT_FILE": "/run/secrets/tls-cert.pem", "TLS_KEY_FILE": "/run/secrets/tls-key.pem",
                "ADAPTER_SERVICE_KEY_FILE": "/run/secrets/service-key",
                **network_env,
            },
            "volumes": [volume("app-password", "db-password")] + [volume(file, file) for file in
                        ("master-key", "service-key", "tls-cert.pem", "tls-key.pem")],
        }
        if (Path(secret_dir) / "adapters" / name / PREVIOUS_SERVICE_KEY).is_file():
            services[service]["environment"]["ADAPTER_PREVIOUS_SERVICE_KEY_FILE"] = f"/run/secrets/{PREVIOUS_SERVICE_KEY}"
            services[service]["volumes"].append(volume(PREVIOUS_SERVICE_KEY, PREVIOUS_SERVICE_KEY))
        if name == "git":
            services[service]["cpus"] = 1.0
        if name == "jiracloud":
            services[service]["environment"]["SOURCE_PRIVATE_NETWORKS"] = ""
        if name in ("gitlab", "jiracloud"):
            services[service]["environment"]["ADAPTER_METRICS_ADDR"] = ":9090"
            services[service]["healthcheck"] = {
                "test": ["CMD", "/usr/local/bin/adapter", "healthcheck"],
                "interval": "10s", "timeout": "3s", "retries": 5,
            }
        services[f"{service}-migrate"] = {
            **common, "profiles": ["migration"], "networks": ["data"], "command": ["migrate"],
            "environment": {**common_env, "DATABASE_URL":
                            f"postgres://{database}_owner@postgres:5432/{database}?sslmode=disable"},
            "volumes": [volume("owner-password", "db-password"), volume("master-key", "master-key")],
        }
    override = {"services": services}
    data_volumes = settings.get("stand_data_volumes")
    if data_volumes is not None:
        require(isinstance(data_volumes, dict) and data_volumes.keys() == {"postgres", "objects"} and
                all(isinstance(name, str) and name for name in data_volumes.values()) and
                len(set(data_volumes.values())) == 2,
                "stand_data_volumes must name distinct existing postgres and objects volumes")
        override["volumes"] = {
            role: {"name": name, "external": True} for role, name in data_volumes.items()
        }
    return override


def probe_http(ip, dns, port, path, tls):
    with socket.create_connection((ip, port), timeout=3) as connection:
        conn = tls.wrap_socket(connection, server_hostname=dns) if tls else connection
        with conn:
            conn.sendall(f"GET {path} HTTP/1.1\r\nHost: {dns}\r\nConnection: close\r\n\r\n".encode("ascii"))
            with http.client.HTTPResponse(conn) as response:
                response.begin()
                if response.status != 200:
                    raise ValueError("unexpected status")
                return response.read(65536)


class Stand:
    def __init__(self, root, config_dir=HERE):
        self.root = root
        self.config_dir = config_dir
        self.state_path = root / "state.json"
        self.state = (json.loads(self.state_path.read_bytes()) if self.state_path.exists() else
                      {"current": None, "releases": {}, "pending": None})

    def save(self):
        write_json(self.state_path, self.state)

    def journal(self, release, action, outcome, *, rollback=False):
        event = {"time": now(), "action": action, "outcome": outcome, "rollback": rollback,
                 "sha": release["commit_sha"], "images": release["images"]}
        with (self.root / "journal.jsonl").open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(event, sort_keys=True) + "\n")
            stream.flush()
            os.fsync(stream.fileno())

    def path(self, release):
        return self.root / "releases" / release["snapshot"]

    def compose_args(self, release, *arguments):
        return ["docker", "compose", "--project-directory", str(self.config_dir), "--env-file", "/dev/null",
                "-f", str(self.path(release) / "compose.json"), *arguments]

    def compose(self, release, label, *args, **kwargs):
        return run(label, self.compose_args(release, *args), **kwargs)

    def prepare(self, release, files):
        path = self.path(release)
        path.mkdir(parents=True)
        settings = json.loads((self.config_dir / "rollout.json").read_bytes())
        required = settings.get("required_adapters", [])
        require(isinstance(required, list) and all(isinstance(name, str) and name in release["adapters"]
                                                  for name in required),
                "a required stand adapter is missing from the release or SOPS bundle")
        versions = settings.get("adapter_master_key_versions", {})
        require(isinstance(versions, dict) and all(
            type(v) is int and 0 < v < 2**31 for v in versions.values()
        ), "adapter master key versions must be positive PostgreSQL integers")
        release["install"] = install_config(settings, files)
        tls_dir, catalog = Path(settings["tls_dir"]), Path(settings["check_profiles"])
        require(tls_dir.is_absolute() and all((tls_dir / name).is_file() for name in ("tls-cert.pem", "tls-key.pem")),
                "install the edge TLS certificate and key first")
        require(catalog.is_absolute() and catalog.is_file(), "provide the check profile catalog")
        release["backup_recipient"] = settings["backup_recipient"]
        require(isinstance(release["backup_recipient"], str) and
                release["backup_recipient"].startswith("age1"), "provide the public age backup recipient")
        current = self.state["current"]
        if current:
            old = self.path(current) / "secrets"
            stable = ["postgres-password", "postgres-owner-password", "postgres-app-password",
                      "postgres-backup-password", "core-master-key", "seaweedfs.json", "grafana-password"]
            for adapter in set(current["adapters"]) & set(release["adapters"]):
                stable.extend(f"adapters/{adapter}/{name}" for name in ("owner-password", "app-password", "master-key"))
            require(all((old / name).read_bytes() == files[name][1] for name in stable),
                    "database, storage or master keys differ from the active release; rotate separately")
            require(all(self.adapter_key_version(current, name) == str(versions.get(name, 1))
                        for name in set(current["adapters"]) & set(release["adapters"])),
                    "adapter master key versions differ from the active release; rotate separately")
        secret_dir = path / "secrets"
        materialize_files(secret_dir, files)
        catalog_snapshot = path / "check-profiles.yaml"
        catalog_snapshot.write_bytes(catalog.read_bytes())
        catalog_snapshot.chmod(0o444)
        override = overrides(settings, release["adapters"], release, secret_dir)
        # New bind sources make Compose reload scrape targets and provision Grafana on deploy.
        for service, mounts in OBSERVABILITY_CONFIG.items():
            volumes = []
            for name, target in mounts.items():
                snapshot = path / name
                snapshot.write_bytes((self.config_dir / "observability" / name).read_bytes())
                snapshot.chmod(0o444)
                volumes.append(f"{snapshot}:{target}:ro")
            override["services"][service] = {"volumes": volumes}
        override_file = path / "override.json"
        write_json(override_file, override)

        env_file = path / "release.env"
        env_file.write_text("\n".join((
            f"SECRETS_DIR={secret_dir}", f"CHECK_PROFILES_PATH={catalog_snapshot}",
            f"CRAFTPULSE_SERVER_IMAGE={release['images']['craftpulse-server']}",
            f"CRAFTPULSE_FRONTEND_IMAGE={release['images']['craftpulse-frontend']}",
        )) + "\n", encoding="utf-8")
        rendered = run("validate Compose configuration", [
            "docker", "compose", "--project-directory", str(self.config_dir),
            "--env-file", str(self.config_dir / ".env"), "--env-file", str(env_file),
            "-f", str(self.config_dir / "compose.yaml"), "-f", str(self.config_dir / "compose.stand.yaml"),
            "-f", str(override_file),
            "--profile", "migration", "config", "--format", "json",
        ])
        compose = json.loads(rendered)
        if current:
            previous = json.loads((self.path(current) / "compose.json").read_bytes())
            require(all(previous["volumes"][name]["name"] == compose["volumes"][name]["name"]
                        for name in ("postgres", "objects")),
                    "stand data volumes differ from the active release")
        for volume in override.get("volumes", {}).values():
            name = volume["name"]
            containers = run("check existing stand data volume", [
                "docker", "ps", "--filter", f"volume={name}", "--format",
                '{{.ID}} {{.Label "com.docker.compose.project"}}',
            ]).decode().splitlines()
            require(all(row.split()[1:] == [compose["name"]] for row in containers),
                    f"stand data volume {name} is in use outside the rollout project")
        write_json(path / "compose.json", compose)
        override_file.unlink()
        env_file.unlink()

    def fetch(self, release):
        names = ["craftpulse-server", "craftpulse-frontend", *(ADAPTER + name for name in release["adapters"])]
        for name in names:
            run(f"pull {name} by digest", ["docker", "pull", release["images"][name]], timeout=900)

    def adapter_key_version(self, release, name):
        compose = json.loads((self.path(release) / "compose.json").read_bytes())
        return compose["services"][f"adapter-{name}"]["environment"]["ADAPTER_MASTER_KEY_VERSION"]

    def check_rollback_keys(self, release, current):
        for name in set(release["adapters"]) & set(current["adapters"]):
            previous_keys = self.path(release) / "secrets/adapters" / name
            active_keys = self.path(current) / "secrets/adapters" / name
            require(self.adapter_key_version(release, name) == self.adapter_key_version(current, name)
                    and (previous_keys / "master-key").read_bytes() == (active_keys / "master-key").read_bytes(),
                    f"adapter {name} master key was rotated; deploy the older SHA with the active bundle")
            require((previous_keys / "service-key").read_bytes() == (active_keys / "service-key").read_bytes(),
                    f"adapter {name} service key was rotated; deploy the older SHA with the active bundle")
            require(previous_service_key(previous_keys) in (None, previous_service_key(active_keys)),
                    f"adapter {name} would accept a retired service key; deploy the older SHA with the active bundle")

    def database(self, release, name, sql):
        return self.compose(release, "query PostgreSQL", "exec", "-T", "postgres", "psql",
                            "-XAtq", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", name, "-c", sql).strip()

    def schema(self, release, name):
        if self.database(release, name, "SELECT to_regclass('public.goose_db_version') IS NOT NULL") == b"f":
            return 0
        return int(self.database(release, name, "SELECT COALESCE(max(version_id), 0) "
                                               "FROM public.goose_db_version WHERE is_applied"))

    def backup(self, release, database):
        directory = self.root / "backups"
        directory.mkdir(exist_ok=True)
        target = directory / f"{release['snapshot']}-{database}.dump.age"
        temporary = target.with_suffix(".tmp")
        command = self.compose_args(release, "exec", "-T", "postgres", "sh", "-ec",
            'PGPASSWORD=$(cat "$CRAFTPULSE_BACKUP_PASSWORD_FILE") '
            'exec pg_dump -h 127.0.0.1 -U craftpulse_backup --enable-row-security -Fc -d "$1"', "backup", database)
        try:
            with temporary.open("xb") as stream, subprocess.Popen(
                    command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL) as dump:
                # Feed age directly, keeping the unencrypted archive off disk.
                try:
                    result = subprocess.run(["age", "-r", release["backup_recipient"]], stdin=dump.stdout,
                                            stdout=stream, stderr=subprocess.DEVNULL, timeout=900, check=False)
                    require(result.returncode == 0 and dump.wait(timeout=30) == 0 and stream.tell() > 0,
                            f"backup failed for {database}")
                    os.fsync(stream.fileno())
                finally:
                    if dump.poll() is None:
                        dump.kill()
            temporary.replace(target)
        finally:
            temporary.unlink(missing_ok=True)

    def migrate(self, release):
        self.compose(release, "start infrastructure", "up", "-d", "--no-recreate", "--wait", "--wait-timeout", "180",
                     *INFRASTRUCTURE, timeout=210)
        targets = [("craftpulse", "migrate", release["core_schema_version"])]
        for name in release["adapters"]:
            database = f"adapter_{name}"
            source = self.path(release) / "secrets" / "adapters" / name
            self.compose(release, f"prepare database for {name}", "run", "--rm", "--no-deps", "-T",
                         "--user", "0:0", "--entrypoint", "sh",
                         "-v", f"{self.config_dir.parent / 'postgres/adapter-db.sh'}:/adapter-db.sh:ro",
                         "-v", f"{source / 'owner-password'}:/run/adapter-owner:ro",
                         "-v", f"{source / 'app-password'}:/run/adapter-app:ro",
                         "postgres", "-ec",
                         'export PGHOST=postgres PGPASSWORD="$(cat "$POSTGRES_PASSWORD_FILE")"; '
                         'export ADAPTER_OWNER_PASSWORD_FILE=/run/adapter-owner ADAPTER_PASSWORD_FILE=/run/adapter-app; '
                         'exec sh /adapter-db.sh "$1"', "init", name)
            targets.append((database, f"adapter-{name}-migrate", release["adapter_schema_versions"][ADAPTER + name]))
        for database, service, expected in targets:
            current = self.schema(release, database)
            if current < expected:
                self.backup(release, database)
            if current < expected or (service == "migrate" and current == expected):
                self.compose(release, f"migrate {database}", "run", "--rm", "--no-deps", "-T", service, timeout=900)
                require(self.schema(release, database) == expected, f"migration did not reach expected schema: {database}")

    def address(self, release, service):
        container = self.compose(release, "find service", "ps", "-q", service).decode().strip()
        require(container, f"{service} is not running")
        networks = json.loads(run("inspect service network", ["docker", "inspect", "--format",
                              "{{json .NetworkSettings.Networks}}", container], timeout=10))
        for name, network in networks.items():
            if name.endswith("_edge") and network["IPAddress"]:
                return network["IPAddress"]
        raise RolloutError(f"{service} has no edge address")

    def verify(self, release, service):
        adapter = service.removeprefix("adapter-") if service.startswith("adapter-") else None
        tls = None
        if adapter:
            tls = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
            tls.load_verify_locations(cafile=str(self.path(release) / "secrets/adapter-ca.pem"))
        minimum = release["adapter_schema_versions"][ADAPTER + adapter] if adapter else release["core_schema_version"]
        deadline = time.monotonic() + PROBE_TIMEOUT
        while time.monotonic() < deadline:
            try:
                ip = self.address(release, service)
                port = 8443 if adapter else 8080
                version = json.loads(probe_http(ip, service, port, "/version", tls))
                schema = version["schema_version"]
                if (version["commit_sha"] == release["commit_sha"] and type(schema) is int and schema >= minimum
                        and (not adapter or version["contract_version"] == release["adapter_contract_version"])):
                    probe_http(ip, service, port, "/readyz", tls)
                    return
            except (OSError, ValueError, KeyError, http.client.HTTPException, RolloutError):
                pass
            time.sleep(1)
        raise RolloutError(f"{service} did not pass release version and readiness checks")

    def check_installation(self, release):
        flags = ["-e", "RELEASE_IMAGES=" + ",".join(f"{key}={value}" for key, value in release["images"].items())]
        for key, value in release["install"].items():
            flags.extend(("-e", f"{key}={value}"))
        for slug in release["install"]["INSTALL_ORGANIZATIONS"].split(","):
            key = "ORG_" + slug.upper().replace("-", "_") + "_OIDC_CLIENT_SECRET_FILE"
            target = f"/run/secrets/install-oidc-{slug}"
            flags.extend(("-e", f"{key}={target}", "-v", f"{self.path(release) / 'secrets/oidc' / slug}:{target}:ro"))
        output = self.compose(release, "installation check", "run", "--rm", "--no-deps", "-T", *flags,
                              "api", "/usr/local/bin/api", "install")
        for line in output.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if (isinstance(event, dict) and event.get("message") == "installation protocol"
                    and event.get("release_sha") == release["commit_sha"] and event.get("outcome") == "passed"
                    and isinstance(event.get("protocol_id"), str)):
                protocol_id = UUID(event["protocol_id"])
                print(f"Installation protocol {protocol_id} passed", flush=True)
                return
        raise RolloutError("installation check did not issue a passed protocol for the release")

    def switch(self, release, previous):
        for core in ("worker", "api"):
            self.compose(release, f"restart {core}", "up", "-d", "--no-deps", "--pull", "never", "--force-recreate", core)
        for name in release["adapters"]:
            service = f"adapter-{name}"
            if self.adapter_changed(release, previous, name):
                self.compose(release, f"restart {service}", "up", "-d", "--no-deps", "--pull", "never",
                             "--force-recreate", service)
            self.verify(release, service)
        if previous:
            removed = sorted(set(previous["adapters"]) - set(release["adapters"]))
            for name in removed:
                self.compose(previous, f"stop removed adapter-{name}", "stop", f"adapter-{name}")
        self.compose(release, "start frontend", "up", "-d", "--no-deps", "--pull", "never", "--wait",
                     "--wait-timeout", str(PROBE_TIMEOUT), "nginx")
        for core in ("worker", "api"):
            self.verify(release, core)
        self.compose(release, "apply observability configuration", "up", "-d", "--no-deps", "--wait",
                     "--wait-timeout", "180", *OBSERVABILITY, timeout=210)

    def adapter_changed(self, release, previous, name):
        if not previous or name not in previous["adapters"]:
            return True
        if release["images"][ADAPTER + name] != previous["images"][ADAPTER + name]:
            return True
        relative = f"secrets/adapters/{name}"
        before, after = self.path(previous), self.path(release)
        if any((before / relative / file).read_bytes() != (after / relative / file).read_bytes()
               for file in ("master-key", "app-password", "service-key", "tls-cert.pem", "tls-key.pem")):
            return True
        if previous_service_key(before / relative) != previous_service_key(after / relative):
            return True
        if (before / "secrets/adapter-ca.pem").read_bytes() != (after / "secrets/adapter-ca.pem").read_bytes():
            return True
        return not self.compose(release, f"find adapter-{name}", "ps", "-q", f"adapter-{name}").strip()

    def recover(self):
        pending = self.state["pending"]
        if pending is None:
            return
        previous = self.state["current"]
        if not previous:
            self.compose(pending, "stop failed first deployment", "stop", "worker", "api", "nginx",
                         *(f"adapter-{name}" for name in pending["adapters"]))
            self.journal(pending, "recover", "stopped")
        else:
            print(f"Restoring {previous['commit_sha']}", flush=True)
            try:
                self.switch(previous, pending)
            except (RolloutError, OSError, ValueError, KeyError, TypeError):
                self.journal(previous, "automatic-rollback", "failed", rollback=True)
                raise RolloutError("automatic rollback failed; pending release retained for retry") from None
            self.journal(previous, "automatic-rollback", "passed", rollback=True)
        self.state["pending"] = None
        self.save()

    def apply(self, request):
        action, sha = request["action"], request["sha"]
        require(action in ("deploy", "rollback") and SHA.fullmatch(sha), "invalid release request")
        self.recover()
        rollback = action == "rollback"
        if rollback:
            release = self.state["releases"].get(sha)
            require(release is not None, "rollback requires a previously successful release on this stand")
            if self.state["current"]:
                self.check_rollback_keys(release, self.state["current"])
        else:
            release = validate_manifest(request["manifest"], sha)
            adapters = request["adapters"]
            require(isinstance(adapters, list) and adapters == sorted(set(adapters)) and
                    {ADAPTER + name for name in adapters} <= release["adapter_schema_versions"].keys(), "invalid deployed adapters")
            release.update(adapters=adapters, snapshot=f"{sha}-{uuid4().hex[:12]}")
        self.journal(release, action, "started", rollback=rollback)
        try:
            self.fetch(release)
            if not rollback:
                self.prepare(release, decode_files(request["files"], release["adapters"]))
                self.migrate(release)
            self.state["pending"] = release
            self.save()
            self.switch(release, self.state["current"])
            self.check_installation(release)
        except (RolloutError, OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as exc:
            self.journal(release, action, "failed", rollback=rollback)
            if self.state["pending"] is not None:
                self.recover()
            if isinstance(exc, RolloutError):
                raise
            raise RolloutError("deployment failed while preparing configuration or accessing stand state") from None
        self.state["current"] = release
        self.state["releases"][sha] = release
        self.state["pending"] = None
        self.save()
        self.journal(release, action, "passed", rollback=rollback)
        print(f"{action} {sha} passed", flush=True)


def main():
    import fcntl

    os.umask(0o077)
    try:
        require(os.geteuid() == 0 and len(sys.argv) == 2, "run the stand command as root with its state directory")
        root = Path(sys.argv[1]).resolve()
        require(not root.is_relative_to(HERE.parents[1]), "keep deploy state outside the checkout")
        root.mkdir(mode=0o700, parents=True, exist_ok=True)
        require(root.stat().st_uid == 0 and not root.stat().st_mode & 0o077, "state directory must be root-only")
        limit = 4 * 1024 * 1024
        payload = sys.stdin.buffer.read(limit + 1)
        require(len(payload) <= limit, "release request is too large")
        request = json.loads(payload)
        with (root / "lock").open("a") as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise RolloutError("another stand operation is running") from None
            Stand(root).apply(request)
    except RolloutError as exc:
        print(exc, file=sys.stderr)
    except (KeyError, OSError, TypeError, ValueError):
        print("Invalid rollout request or inaccessible stand state", file=sys.stderr)
    else:
        return 0
    return 1


if __name__ == "__main__":
    sys.exit(main())
