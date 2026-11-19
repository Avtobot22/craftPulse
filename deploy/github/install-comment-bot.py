#!/usr/bin/env python3
"""Install a local Unix-socket capability for fixture PR comments (root on Linux)."""

import argparse
import json
import os
from pathlib import Path
import pwd
import subprocess

from manage import validate

BOT_USER = "cp89-comment-bot"
BOT_HOME = Path("/var/lib/craftpulse-github89-bot")
BOT_CONFIG = Path("/etc/craftpulse-github89-bot")
CODE = Path("/opt/craftpulse/github89")


def install(app_config_path, private_key_path, baseline_path):
    if os.geteuid() != 0:
        raise RuntimeError("Run as root on the test stand")
    baseline = json.loads(baseline_path.read_text())
    validate(baseline)
    app_config = json.loads(app_config_path.read_text())
    private_key = private_key_path.read_text()
    if not private_key.startswith("-----BEGIN RSA PRIVATE KEY-----"):
        raise ValueError("Expected the App RSA private key")
    actor = app_config["slug"] + "[bot]"
    app_config["key_file"] = str(BOT_CONFIG / "app.pem")
    try:
        account = pwd.getpwnam(BOT_USER)
    except KeyError:
        subprocess.run(["useradd", "--system", "--home-dir", str(BOT_HOME),
                        "--shell", "/usr/sbin/nologin", BOT_USER], check=True)
        account = pwd.getpwnam(BOT_USER)
    if account.pw_dir != str(BOT_HOME):
        raise RuntimeError("Unexpected pre-existing account")
    if account.pw_shell != "/usr/sbin/nologin":
        subprocess.run(["usermod", "--shell", "/usr/sbin/nologin", BOT_USER], check=True)
    BOT_CONFIG.mkdir(mode=0o750, exist_ok=True)
    os.chmod(BOT_CONFIG, 0o750)
    os.chown(BOT_CONFIG, 0, account.pw_gid)
    config = {"actor": actor, "baseline": str(BOT_CONFIG / "baseline.json"),
              "app_config": str(BOT_CONFIG / "app-config.json")}
    for name, content in (("app.pem", private_key), ("baseline.json", json.dumps(baseline)),
                          ("app-config.json", json.dumps(app_config)), ("config.json", json.dumps(config))):
        path = BOT_CONFIG / name
        path.write_text(content)
        os.chmod(path, 0o640)
        os.chown(path, 0, account.pw_gid)
    for name in ("craftpulse-github89-comment.socket", "craftpulse-github89-comment@.service"):
        source = CODE / name
        target = Path("/etc/systemd/system") / name
        target.write_text(source.read_text())
        target.chmod(0o644)
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    subprocess.run(["systemctl", "enable", "--now", "craftpulse-github89-comment.socket"], check=True)
    print("Comment-only local Unix socket installed; service account has no login shell")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app-config", type=Path, required=True)
    parser.add_argument("--private-key-file", type=Path, required=True)
    parser.add_argument("--baseline", type=Path, required=True)
    args = parser.parse_args()
    install(args.app_config, args.private_key_file, args.baseline)
