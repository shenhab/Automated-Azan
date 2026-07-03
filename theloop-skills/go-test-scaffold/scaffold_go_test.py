#!/usr/bin/env python3
"""Scaffold table-driven Go test stubs for a source file's top-level functions.

Usage:
    scaffold_go_test.py <path/to/file.go> [FuncName ...]

Parses top-level func/method declarations in the given Go source file and,
for every one that doesn't already have a `Test<Func>` in the sibling
`<file>_test.go`, appends a table-driven stub (with TODO markers) to that
test file — creating it with the correct `package` clause and `testing`
import if it doesn't exist yet. Never overwrites an existing stub or test.

If one or more FuncName arguments are given, only those functions are
considered; otherwise every top-level func/method in the file is.
"""
import re
import sys
from pathlib import Path

FUNC_RE = re.compile(
    r'^func\s+(?:\(\s*\w+\s+\*?[\w.]+\s*\)\s+)?([A-Za-z_]\w*)\s*\('
)
PACKAGE_RE = re.compile(r'^package\s+(\w+)')
TEST_FUNC_RE = re.compile(r'^func\s+(Test[A-Za-z_]\w*)\s*\(')


def parse_functions(src_path):
    """Return (package_name, [func_name, ...]) for top-level declarations."""
    package = None
    funcs = []
    for line in src_path.read_text().splitlines():
        if package is None:
            m = PACKAGE_RE.match(line)
            if m:
                package = m.group(1)
        m = FUNC_RE.match(line)
        if m:
            funcs.append(m.group(1))
    return package, funcs


def existing_test_names(test_path):
    """Return the set of TestXxx names already declared in test_path."""
    if not test_path.exists():
        return set()
    return {
        m.group(1)
        for line in test_path.read_text().splitlines()
        for m in [TEST_FUNC_RE.match(line)]
        if m
    }


def test_name_for(func_name):
    return f"Test{func_name[0].upper()}{func_name[1:]}"


def stub_for(func_name):
    test_name = test_name_for(func_name)
    return f'''
// TODO: replace with real cases exercising {func_name}'s behavior.
func {test_name}(t *testing.T) {{
	tests := []struct {{
		name string
		// TODO: input field(s)
		// TODO: want field(s)
	}}{{
		// {{name: "description of case", ...}},
	}}

	for _, tt := range tests {{
		t.Run(tt.name, func(t *testing.T) {{
			t.Fatalf("TODO: call {func_name} and assert against tt.want")
		}})
	}}
}}
'''


def scaffold(src_path, wanted_funcs=None):
    """Append missing test stubs for src_path's functions.

    Returns the list of function names a stub was generated for.
    """
    package, funcs = parse_functions(src_path)
    if package is None:
        raise ValueError(f"no package clause found in {src_path}")

    if wanted_funcs:
        funcs = [f for f in funcs if f in wanted_funcs]

    test_path = src_path.with_name(src_path.stem + "_test.go")
    tested = existing_test_names(test_path)
    missing = [f for f in funcs if test_name_for(f) not in tested]

    if not missing:
        return []

    if not test_path.exists():
        test_path.write_text(f'package {package}\n\nimport "testing"\n')

    with test_path.open("a") as f:
        for func_name in missing:
            f.write(stub_for(func_name))

    return missing


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 1

    src_path = Path(argv[1])
    wanted = set(argv[2:])

    try:
        added = scaffold(src_path, wanted)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    test_path = src_path.with_name(src_path.stem + "_test.go")
    if not added:
        print(f"no stubs needed: every targeted function in {src_path} already has a Test* in {test_path}")
        return 0

    for func_name in added:
        print(f"added stub {test_name_for(func_name)} for {func_name}() to {test_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
