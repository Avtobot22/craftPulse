import importlib.util
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("export_srs", ROOT / "scripts/export_srs.py")
export = importlib.util.module_from_spec(spec)
spec.loader.exec_module(export)


class ExportSrsTest(unittest.TestCase):
    def test_github_anchors(self):
        cases = {
            "UC-01: Развернуть craftPulse в контуре компании": "uc-01-развернуть-craftpulse-в-контуре-компании",
            "9.4. Сквозная трассировка M0, M1 и M2": "94-сквозная-трассировка-m0-m1-и-m2",
            "Пакет `calc` и [ссылка](x.md)": "пакет-calc-и-ссылка",
        }
        for title, anchor in cases.items():
            with self.subTest(title=title):
                self.assertEqual(export.slug(title), anchor)

    def test_duplicate_headings_and_code_blocks(self):
        lines = ["# Итог", "```md", "# Не заголовок", "```", "## Итог", "### Итог"]
        anchors = [github for _, _, _, github in export.headings(lines)]
        self.assertEqual(anchors, ["итог", "итог-1", "итог-2"])

    def test_column_weights_follow_content(self):
        text = "\n".join([
            "| ID | Описание |",
            "|---|---|",
            "| F-01 | " + "длинное описание " * 10 + "|",
            "",
            "| A | B | C |",
            "|---|---|---|",
            "| 1 | 2 | 3 |",
        ])
        first, second = export.column_weights(text)
        self.assertEqual(len(first), 2)
        self.assertGreater(first[1], first[0] * 3)
        self.assertEqual(len(second), 3)
        self.assertEqual(second[0], second[2])


if __name__ == "__main__":
    unittest.main()
