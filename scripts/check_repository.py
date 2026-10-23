#!/usr/bin/env python3
"""Small, dependency-free checks for this repository, not an application test suite."""

import argparse
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


ROOT = Path(__file__).resolve().parents[1]
TITLE = re.compile(
    r"(?:feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)"
    r"(?:\([a-z0-9][a-z0-9/-]*\))?!?: \S[^\r\n]*"
)
BRANCH = re.compile(
    r"(?:feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|copilot)/"
    r"[a-z0-9]+(?:-[a-z0-9]+)*"
)
LOCAL_DIRS = {
    ".agents", ".claude", ".codex", ".venv", ".uv-cache", ".hf-cache",
    ".idea", "node_modules", "__pycache__",
}
RECORDINGS = {".m4a", ".mp3", ".wav", ".mp4", ".webm", ".flac", ".ogg"}
LINK = re.compile(r"!?\[[^\]\n]*\]\(\s*(<[^>]+>|[^\s)]+)(?:\s+[^)]*)?\)")


def valid_title(title):
    return bool(TITLE.fullmatch(title)) and len(title) <= 100


def valid_branch(branch, author=""):
    if author == "dependabot[bot]" and branch.startswith("dependabot/"):
        return True
    return bool(BRANCH.fullmatch(branch)) and len(branch) <= 100


def commit_title_errors(root, base, head):
    # Only full event SHAs are accepted; no revisions or flags from a PR author.
    if not all(re.fullmatch(r"[0-9a-f]{40}", sha) for sha in (base, head)):
        return ["Некорректный SHA в событии GitHub"]
    result = subprocess.run(
        ["git", "log", "--format=%H%x00%s", base + ".." + head, "--"],
        cwd=root, check=True, stdout=subprocess.PIPE, text=True,
    )
    errors = []
    for line in result.stdout.splitlines():
        sha, title = line.split("\0", 1)
        if not valid_title(title):
            if title == "Initial plan":
                # Copilot creates an empty bootstrap commit before editing files.
                # Compare to the first parent explicitly, including merge commits.
                diff = subprocess.run(["git", "diff", "--quiet", sha + "^", sha, "--"], cwd=root)
                if diff.returncode == 0:
                    continue
            errors.append("Коммит " + sha[:12] + ": ожидается type(scope): описание до 100 символов")
    return errors


def forbidden_path(name):
    path = PurePosixPath(name)
    if set(path.parts) & LOCAL_DIRS:
        return True
    if path.parts[0] in {"shared", "artifacts", "transcripts"}:
        return True
    if path.name in {".DS_Store", "Thumbs.db"}:
        return True
    if path.name == ".env" or (
        path.name.startswith(".env.") and not path.name.endswith(".example")
        and path.name != ".env.example"
    ):
        return True
    if path.suffix in {".pem", ".key"}:
        return True
    return len(path.parts) == 1 and path.suffix.lower() in RECORDINGS


def link_errors(root, source, content, known):
    """Check inline Markdown file links. Remote URLs and fragments are not checked."""
    errors = []
    fence = None
    for line in content.splitlines():
        stripped = line.lstrip()
        marker = re.match(r"(`{3,}|~{3,})", stripped)
        if marker:
            token = marker.group(1)
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            continue
        if fence:
            continue
        line = re.sub(r"`+[^`]*`+", "", line)
        for match in LINK.finditer(line):
            dest = match.group(1).strip("<>")
            parsed = urlsplit(dest)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            target_path = unquote(parsed.path)
            target = ((root if target_path.startswith("/") else source.parent)
                      / target_path.lstrip("/")).resolve()
            try:
                relative = target.relative_to(root.resolve()).as_posix()
            except ValueError:
                errors.append("ссылка за пределы репозитория")
                continue
            if relative not in known and not any(p.startswith(relative + "/") for p in known):
                errors.append("нет файла в Git: " + relative)
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--event", type=Path, help="GitHub event JSON; checks PR title")
    args = parser.parse_args()
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=ROOT, check=True, stdout=subprocess.PIPE,
    )
    names = set(result.stdout.decode("utf-8").strip("\0").split("\0")) - {""}
    errors = []
    for name in sorted(names):
        path = ROOT / name
        if forbidden_path(name):
            errors.append(name + ": локальный или секретный файл не должен быть в Git")
            continue
        if path.is_symlink():
            errors.append(name + ": символическая ссылка требует отдельного решения")
            continue
        if not path.is_file():
            errors.append(name + ": файл отсутствует")
            continue
        if path.stat().st_size > 10 * 1024 * 1024:
            errors.append(name + ": файл больше 10 MiB; нужен отдельный способ хранения")
        if path.suffix == ".json":
            try:
                json.loads(path.read_text(encoding="utf-8"))
            except (ValueError, UnicodeError) as error:
                errors.append(name + ": невалидный JSON: " + str(error))
        if path.suffix == ".md":
            try:
                content = path.read_text(encoding="utf-8")
            except UnicodeError:
                errors.append(name + ": ожидается UTF-8")
                continue
            errors.extend(name + ": " + error for error in link_errors(ROOT, path, content, names))
    if args.event:
        event = json.loads(args.event.read_text(encoding="utf-8"))
        if "pull_request" in event:
            pr = event["pull_request"]
            if not valid_title(pr["title"]):
                errors.append("Заголовок PR: type(scope): описание, одна строка до 100 символов")
            if not valid_branch(pr["head"]["ref"], pr["user"]["login"]):
                errors.append("Ветка: type/123-short-description или type/short-description до 100 символов")
            errors.extend(commit_title_errors(ROOT, pr["base"]["sha"], pr["head"]["sha"]))
        elif event.get("after"):
            result = subprocess.run(["git", "log", "-1", "--format=%s"], cwd=ROOT,
                                    check=True, stdout=subprocess.PIPE, text=True)
            if not valid_title(result.stdout.rstrip("\n")):
                errors.append("Последний коммит main не соответствует Conventional Commits")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("Repository checks passed ({} files).".format(len(names)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
