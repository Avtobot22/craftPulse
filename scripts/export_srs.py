#!/usr/bin/env python3
"""Export part I of the technical specification with its appendices as a standalone SRS.

The DOCX and PDF are built from the current commit: part I of
docs/technical-specification/technical-specification.md plus the requirement
sources of its section 1.5. Links between included documents become internal
links; links to other repository files point to the same commit on GitHub.
Requires pandoc; the PDF also needs LibreOffice (soffice).
"""

import argparse
import hashlib
import os
import posixpath
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
REPO_URL = "https://github.com/Avtobot22/craftPulse/blob"
SPEC = "docs/technical-specification/technical-specification.md"
APPENDICES = [
    "docs/requirements/use-cases.md",
    "docs/requirements/features.md",
    "docs/technical-specification/mvp-scope.md",
    "docs/product/metrics.md",
    "docs/product/metrics/model.md",
    "docs/product/metrics/tasks.md",
    "docs/product/metrics/activity.md",
    "docs/product/metrics/code.md",
    "docs/product/metrics/security.md",
    "docs/product/metrics/ci-cd.md",
    "docs/product/metrics/documentation.md",
    "docs/product/connectors.md",
    "docs/product/scenarios.md",
    "docs/product/vision.md",
]
LETTERS = "АБВГДЕЖИКЛМНПРСТУФХЦШЭЮЯ"
HEADING = re.compile(r"^(#{1,6})\s+(.*?)\s*#*\s*$")
LINK = re.compile(r"(\]\()(<[^>]+>|[^)\s]+)(\))")
FENCE = re.compile(r"^\s*(`{3,}|~{3,})")


def git(*args):
    return subprocess.run(["git", *args], cwd=ROOT, check=True, capture_output=True, text=True).stdout.strip()


def slug(text):
    """GitHub-style heading anchor."""
    text = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)
    text = re.sub(r"[`*]", "", text)
    return re.sub(r"[^\w\- ]", "", text.lower()).replace(" ", "-")


def outside_code(lines):
    """Yield (index, line) for lines outside fenced code blocks."""
    fence = None
    for index, line in enumerate(lines):
        marker = FENCE.match(line)
        if marker:
            token = marker.group(1)
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            continue
        if fence is None:
            yield index, line


def headings(lines):
    """(index, level, title, github slug) with GitHub duplicate suffixes."""
    seen = {}
    for index, line in outside_code(lines):
        match = HEADING.match(line)
        if not match:
            continue
        base = slug(match.group(2))
        count = seen.get(base, 0)
        seen[base] = count + 1
        yield index, len(match.group(1)), match.group(2), base if count == 0 else f"{base}-{count}"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=ROOT / "build" / "srs")
    parser.add_argument("--no-pdf", action="store_true", help="build only DOCX")
    args = parser.parse_args()
    if not shutil.which("pandoc"):
        sys.exit("pandoc не найден")

    sha = git("rev-parse", "HEAD")
    dirty = bool(git("status", "--porcelain", "--", "docs"))
    revision = sha[:12] + (" с незафиксированными изменениями docs/" if dirty else "")
    spec_text = (ROOT / SPEC).read_text(encoding="utf-8")
    version = re.search(r"Версия требований - \*\*([\d.]+)\*\* от \*\*([^*]+)\*\*", spec_text)
    if not version:
        sys.exit("В разделе 1.1 не найдена версия требований")

    # Документы экспорта: часть I и приложения в порядке раздела 1.5.
    documents = []
    part_two = spec_text.index("\n## Часть II.")
    documents.append((SPEC, spec_text[:part_two].splitlines(), None))
    for letter, name in zip(LETTERS, APPENDICES):
        documents.append((name, (ROOT / name).read_text(encoding="utf-8").splitlines(), letter))

    # Короткие ASCII-идентификаторы: Word ограничивает имена закладок.
    # Часть I - начало файла ТЗ, поэтому её якоря совпадают с якорями полного файла.
    anchors, tops, counter = {}, {}, 0
    for name, lines, _ in documents:
        for _, _, _, github in headings(lines):
            counter += 1
            anchors[(name, github)] = f"h{counter}"
            tops.setdefault(name, f"h{counter}")

    def resolve(source, destination):
        destination = destination.strip("<>")
        if re.match(r"[a-z][a-z0-9+.-]*:", destination) or destination.startswith("//"):
            return destination
        path, _, fragment = destination.partition("#")
        fragment = unquote(fragment)
        target = posixpath.normpath(posixpath.join(posixpath.dirname(source), unquote(path))) if path else source
        if target in tops:
            anchor = anchors.get((target, fragment)) if fragment else tops[target]
            if anchor:
                return "#" + anchor
        url = f"{REPO_URL}/{sha}/{target}"
        return url + ("#" + fragment if fragment else "")

    parts = [
        "---",
        f"title: \"Спецификация требований к ПО craftPulse (SRS)\"",
        f"subtitle: \"Часть I технического задания, версия {version.group(1)} от {version.group(2)}\"",
        f"date: \"Коммит {revision}\"",
        "lang: ru-RU",
        "---",
        "",
        f"Документ собран из файла `{SPEC}` и приложений раздела 1.5 на коммите `{sha}`. "
        "Часть II технического задания (проектирование) и контракт `openapi.yaml` находятся в репозитории на том же коммите; "
        "ссылки на них ведут туда.",
        "",
        "TOC-PLACEHOLDER",
        "",
    ]
    toc = []
    for name, lines, letter in documents:
        lines = list(lines)
        ids = {index: github for index, _, _, github in headings(lines)}
        first = True
        for index, line in outside_code(lines):
            if index in ids:
                match = HEADING.match(line)
                level, title = match.group(1), match.group(2)
                anchor = anchors[(name, ids[index])]
                if letter and first:
                    level, title = "#", f"Приложение {letter}. {title}"
                elif letter and len(level) == 1:
                    level = "##"
                elif not letter and len(level) == 1:
                    lines[index] = ""
                    continue
                elif not letter:
                    level = level[1:]
                first = False
                plain = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", title)
                if len(level) <= (3 if not letter else 1):
                    toc.append(f"{'  ' * (len(level) - 1)}- [{plain}](#{anchor})")
                lines[index] = f"{level} {title} {{#{anchor}}}"
            else:
                pieces = re.split(r"(`+[^`]*`+)", line)
                pieces = [piece if piece.startswith("`") else LINK.sub(lambda m: m.group(1) + resolve(name, m.group(2)) + m.group(3), piece)
                          for piece in pieces]
                lines[index] = "".join(pieces)
        text = "\n".join(lines)
        text = re.sub(r"<!--.*?-->", "", text, flags=re.S)
        parts.append(text)
        parts.append("")

    body = "\n".join(parts).replace("TOC-PLACEHOLDER", "**Содержание**\n\n" + "\n".join(toc))
    args.out.mkdir(parents=True, exist_ok=True)
    stem = f"craftpulse-srs-{version.group(1)}"
    markdown = args.out / f"{stem}.md"
    markdown.write_text(body, encoding="utf-8")

    with tempfile.TemporaryDirectory() as temp:
        reference = Path(temp) / "reference.docx"
        build_reference(reference)
        docx = args.out / f"{stem}.docx"
        # Дата коммита вместо текущей делает DOCX воспроизводимым.
        environment = {**os.environ, "SOURCE_DATE_EPOCH": git("log", "-1", "--format=%ct")}
        subprocess.run(["pandoc", str(markdown), "-f", "gfm+attributes+yaml_metadata_block", "-t", "docx",
                        "--reference-doc", str(reference), "-o", str(docx)], check=True, env=environment)
        full_width(docx, column_weights(body))
    outputs = [markdown, docx]
    if not args.no_pdf:
        soffice = shutil.which("soffice") or shutil.which("libreoffice")
        if not soffice:
            sys.exit("Для PDF нужен LibreOffice; используйте --no-pdf")
        with tempfile.TemporaryDirectory() as profile:
            subprocess.run([soffice, f"-env:UserInstallation=file://{profile}", "--headless",
                            "--convert-to", "pdf", "--outdir", str(args.out), str(docx)],
                           check=True, capture_output=True)
        outputs.append(args.out / f"{stem}.pdf")
    for path in outputs:
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        print(f"{digest}  {path.relative_to(ROOT) if path.is_relative_to(ROOT) else path}")
    if dirty:
        print("Внимание: в docs/ есть незафиксированные изменения, экспорт не соответствует коммиту.", file=sys.stderr)


def column_weights(text):
    """Relative column widths of each pipe table, in document order, by average cell length."""
    lines, tables, index = text.split("\n"), [], 0
    while index < len(lines) - 1:
        if lines[index].startswith("|") and re.fullmatch(r"\|(?:\s*:?-+:?\s*\|)+", lines[index + 1].strip()):
            end = index + 2
            while end < len(lines) and lines[end].startswith("|"):
                end += 1
            rows = [re.split(r"(?<!\\)\|", row.strip().strip("|")) for row in [lines[index], *lines[index + 2:end]]]
            count = len(rows[0])
            weights = []
            for column in range(count):
                sizes = [len(re.sub(r"\]\([^)]*\)", "]", row[column]).strip()) for row in rows if column < len(row)]
                header = max((len(word) for word in rows[0][column].split()), default=0) if column < len(rows[0]) else 0
                weights.append(max(6, header * 1.6, min(sum(sizes) / max(len(sizes), 1), 300)) ** 0.7)
            tables.append(weights)
            index = end
        else:
            index += 1
    return tables


def full_width(docx, tables):
    """Stretch every table to the landscape text width with content-based column widths."""
    width = 16838 - 2 * 850
    temp = docx.with_suffix(".tmp.docx")
    with zipfile.ZipFile(docx) as original, zipfile.ZipFile(temp, "w", zipfile.ZIP_DEFLATED) as result:
        for item in original.infolist():
            content = original.read(item.filename)
            if item.filename == "word/document.xml":
                xml = content.decode()

                order = iter(tables)

                def grid(match):
                    columns = [int(value) for value in re.findall(r'w:w="(\d+)"', match.group(0))]
                    weights = next(order, None)
                    if weights and len(weights) == len(columns):
                        columns = weights
                    total = sum(columns) or 1
                    return "<w:tblGrid>" + "".join(f'<w:gridCol w:w="{round(width * value / total)}" />' for value in columns) + "</w:tblGrid>"

                xml = re.sub(r"<w:tblGrid>.*?</w:tblGrid>", grid, xml)
                xml = xml.replace('<w:tblW w:type="auto" w:w="0" />', '<w:tblW w:type="pct" w:w="5000" />')
                content = xml.encode()
            result.writestr(item, content)
    temp.replace(docx)


def build_reference(path):
    """Default pandoc reference.docx with landscape A4, compact text, table borders and DejaVu fonts."""
    source = path.with_suffix(".orig.docx")
    subprocess.run(["pandoc", "-o", str(source), "--print-default-data-file", "reference.docx"], check=True)
    with zipfile.ZipFile(source) as original, zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as result:
        for item in original.infolist():
            content = original.read(item.filename)
            if item.filename == "word/document.xml":
                xml = content.decode()
                section = ('<w:sectPr><w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>'
                           '<w:pgMar w:top="850" w:right="850" w:bottom="850" w:left="850" '
                           'w:header="567" w:footer="567" w:gutter="0"/></w:sectPr>')
                xml = xml.replace("<w:sectPr />", section)
                content = xml.encode()
            if item.filename == "word/styles.xml":
                xml = content.decode()
                xml = xml.replace('<w:sz w:val="24" />', '<w:sz w:val="18" />').replace('<w:sz w:val="22" />', '<w:sz w:val="16" />')
                borders = ('<w:tblBorders>' + ''.join(
                    f'<w:{side} w:val="single" w:sz="4" w:space="0" w:color="999999"/>'
                    for side in ("top", "left", "bottom", "right", "insideH", "insideV")) + '</w:tblBorders>')
                xml = re.sub(r'(<w:style w:type="table" w:default="1" w:styleId="Table">.*?<w:tblPr>)', r'\1' + borders, xml, count=1, flags=re.S)
                content = xml.encode()
            if item.filename == "word/theme/theme1.xml":
                # Шрифты с кириллицей и стрелками; у замен Cambria и Calibri нет части символов.
                content = content.decode().replace('typeface="Cambria"', 'typeface="DejaVu Serif"') \
                    .replace('typeface="Calibri"', 'typeface="DejaVu Sans"').encode()
            if item.filename == "word/styles.xml":
                content = content.decode().replace('"Consolas"', '"DejaVu Sans Mono"').encode()
            result.writestr(item, content)


if __name__ == "__main__":
    main()
