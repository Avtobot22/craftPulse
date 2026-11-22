"""Write Lizard CSV function records and the file inventory from one analysis."""

import csv
import sys

from lizard import analyze_files


def main(file_list, output):
    with open(file_list, encoding="utf-8") as paths:
        files = [path for path in paths.read().split("\n") if path]
    with open(output, "w", encoding="utf-8", newline="") as report:
        writer = csv.writer(report)
        writer.writerow(("craftpulse-lizard", 2))
        for file in analyze_files(files):
            writer.writerow(("file", file.filename, file.nloc))
            for fn in file.function_list:
                location = f"{fn.name}@{fn.start_line}-{fn.end_line}@{file.filename}"
                writer.writerow((
                    fn.nloc, fn.cyclomatic_complexity, fn.token_count,
                    fn.parameter_count, fn.length, location, file.filename,
                    fn.name, fn.long_name, fn.start_line, fn.end_line,
                ))


if __name__ == "__main__":
    main(*sys.argv[1:])
