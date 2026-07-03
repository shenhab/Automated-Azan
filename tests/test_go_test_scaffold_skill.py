"""
Tests for the go-test-scaffold repo skill (theloop-skills/go-test-scaffold),
which generates table-driven Go test stubs for go/internal/* packages.
"""
import importlib.util
import os

import pytest

_SKILL_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "theloop-skills",
    "go-test-scaffold",
    "scaffold_go_test.py",
)
_spec = importlib.util.spec_from_file_location("scaffold_go_test", _SKILL_PATH)
scaffold_go_test = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(scaffold_go_test)


@pytest.fixture
def go_source(tmp_path):
    src = tmp_path / "widget.go"
    src.write_text(
        "package widget\n"
        "\n"
        "func Add(a, b int) int {\n"
        "\treturn a + b\n"
        "}\n"
        "\n"
        "func (w *Widget) Reset() {\n"
        "\tw.value = 0\n"
        "}\n"
    )
    return src


def test_scaffold_creates_test_file_with_stub_for_each_function(go_source):
    added = scaffold_go_test.scaffold(go_source)

    assert added == ["Add", "Reset"]

    test_path = go_source.with_name("widget_test.go")
    assert test_path.exists()
    content = test_path.read_text()

    assert content.startswith("package widget\n")
    assert 'import "testing"' in content
    assert "func TestAdd(t *testing.T) {" in content
    assert "func TestReset(t *testing.T) {" in content
    # The generated stub must fail if run as-is (until a real implementer
    # fills it in) rather than silently passing.
    assert 't.Fatalf("TODO: call Add' in content


def test_scaffold_skips_functions_that_already_have_a_test(go_source):
    test_path = go_source.with_name("widget_test.go")
    test_path.write_text(
        "package widget\n"
        "\n"
        "import \"testing\"\n"
        "\n"
        "func TestAdd(t *testing.T) {\n"
        "\tif Add(2, 3) != 5 {\n"
        "\t\tt.Fatal(\"bad\")\n"
        "\t}\n"
        "}\n"
    )

    added = scaffold_go_test.scaffold(go_source)

    assert added == ["Reset"]
    content = test_path.read_text()
    # The existing, hand-written TestAdd must be preserved verbatim, not
    # duplicated or clobbered.
    assert content.count("func TestAdd(t *testing.T) {") == 1
    assert "Add(2, 3) != 5" in content
    assert "func TestReset(t *testing.T) {" in content


def test_scaffold_is_noop_when_every_function_already_tested(go_source):
    test_path = go_source.with_name("widget_test.go")
    test_path.write_text(
        "package widget\n"
        "\n"
        "import \"testing\"\n"
        "\n"
        "func TestAdd(t *testing.T) {}\n"
        "func TestReset(t *testing.T) {}\n"
    )
    before = test_path.read_text()

    added = scaffold_go_test.scaffold(go_source)

    assert added == []
    assert test_path.read_text() == before


def test_scaffold_can_target_a_single_function(go_source):
    added = scaffold_go_test.scaffold(go_source, wanted_funcs={"Add"})

    assert added == ["Add"]
    content = go_source.with_name("widget_test.go").read_text()
    assert "func TestAdd(t *testing.T) {" in content
    assert "func TestReset" not in content


def test_scaffold_raises_without_a_package_clause(tmp_path):
    src = tmp_path / "nopkg.go"
    src.write_text("func Add(a, b int) int { return a + b }\n")

    with pytest.raises(ValueError):
        scaffold_go_test.scaffold(src)


def test_cli_main_reports_added_stubs_and_exits_zero(go_source, capsys):
    rc = scaffold_go_test.main(["scaffold_go_test.py", str(go_source)])

    assert rc == 0
    out = capsys.readouterr().out
    assert "TestAdd" in out
    assert "TestReset" in out


def test_cli_main_reports_noop_when_nothing_to_add(go_source, capsys):
    scaffold_go_test.scaffold(go_source)  # first run creates both stubs
    capsys.readouterr()  # discard first run's output

    rc = scaffold_go_test.main(["scaffold_go_test.py", str(go_source)])

    assert rc == 0
    out = capsys.readouterr().out
    assert "no stubs needed" in out
