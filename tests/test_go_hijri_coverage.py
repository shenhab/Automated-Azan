"""
Verifies go/internal/hijri/hijri_test.go carries real table-driven coverage.

The Go module's own toolchain (go.mod pins `go 1.25.11`) isn't guaranteed to be
available wherever this Python suite runs, so this checks the test file's
content directly rather than shelling out to `go test`. The actual behavioral
assertions live in the Go table-driven tests themselves.
"""
import re
from pathlib import Path

HIJRI_TEST_PATH = (
    Path(__file__).resolve().parent.parent / "go" / "internal" / "hijri" / "hijri_test.go"
)

# Matches a table-driven case entry, e.g. `{"name", ...}` (compact struct
# literal) or `name: "...",` (expanded struct literal).
CASE_ENTRY_RE = re.compile(r'^\s*(?:\{"|name:\s*")', re.MULTILINE)

EXPECTED_COVERAGE = [
    "Eid al-Fitr",
    "Eid al-Adha",
    "Day of Arafat",
    "Islamic New Year",
    "Mawlid al-Nabi",
    "leap",
]


def test_hijri_test_file_exists():
    assert HIJRI_TEST_PATH.exists(), "go/internal/hijri/hijri_test.go is missing"


def test_hijri_test_file_has_table_driven_cases():
    content = HIJRI_TEST_PATH.read_text(encoding="utf-8")
    case_count = len(CASE_ENTRY_RE.findall(content))
    assert case_count >= 5, f"expected at least 5 table-driven cases, found {case_count}"


def test_hijri_test_file_covers_known_dates_and_boundaries():
    content = HIJRI_TEST_PATH.read_text(encoding="utf-8")
    missing = [case for case in EXPECTED_COVERAGE if case not in content]
    assert not missing, f"hijri_test.go is missing coverage for: {missing}"
