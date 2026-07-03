"""
Tests for the timeout repo skill (theloop-skills/timeout), which runs a
command under a bounded wall-clock timeout and guarantees the command's
whole process group is killed if it overruns.
"""
import importlib.util
import os
import sys

import pytest

_SKILL_PATH = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "theloop-skills",
    "timeout",
    "run_with_timeout.py",
)
_spec = importlib.util.spec_from_file_location("run_with_timeout", _SKILL_PATH)
run_with_timeout = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(run_with_timeout)


def _pid_alive(pid):
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    return True


def test_fast_successful_command_reports_success():
    result = run_with_timeout.run_command([sys.executable, "-c", "pass"], timeout=5)

    assert result == {"success": True, "returncode": 0, "timed_out": False}


def test_failing_command_reports_failure_without_timeout():
    result = run_with_timeout.run_command(
        [sys.executable, "-c", "import sys; sys.exit(3)"], timeout=5
    )

    assert result["success"] is False
    assert result["returncode"] == 3
    assert result["timed_out"] is False


def test_slow_command_is_killed_on_timeout_and_leaves_no_process_behind():
    result = run_with_timeout.run_command(
        [sys.executable, "-c", "import time; time.sleep(30)"],
        timeout=0.3,
        kill_after=0.3,
    )

    assert result["success"] is False
    assert result["timed_out"] is True


def test_process_group_is_actually_terminated_after_timeout():
    proc = run_with_timeout.subprocess.Popen(
        [sys.executable, "-c", "import time; time.sleep(30)"],
        start_new_session=True,
    )
    pgid = os.getpgid(proc.pid)

    try:
        proc.wait(timeout=0.2)
    except run_with_timeout.subprocess.TimeoutExpired:
        pass
    run_with_timeout._kill_process_group(proc, kill_after=0.3)

    with pytest.raises(ProcessLookupError):
        os.killpg(pgid, 0)


def test_process_ignoring_sigterm_is_hard_killed_after_grace_period():
    ignore_term_script = (
        "import signal, time; "
        "signal.signal(signal.SIGTERM, signal.SIG_IGN); "
        "time.sleep(30)"
    )
    proc = run_with_timeout.subprocess.Popen(
        [sys.executable, "-c", ignore_term_script],
        start_new_session=True,
    )
    pid = proc.pid

    try:
        proc.wait(timeout=0.2)
    except run_with_timeout.subprocess.TimeoutExpired:
        pass
    run_with_timeout._kill_process_group(proc, kill_after=0.3)

    assert not _pid_alive(pid)


def test_cli_main_returns_124_on_timeout(capsys):
    rc = run_with_timeout.main(
        ["run_with_timeout.py", "0.2", sys.executable, "-c", "import time; time.sleep(30)"]
    )

    assert rc == 124
    err = capsys.readouterr().err
    assert "exceeded" in err


def test_cli_main_returns_zero_on_fast_success():
    rc = run_with_timeout.main(["run_with_timeout.py", "5", sys.executable, "-c", "pass"])

    assert rc == 0


def test_cli_main_returns_child_exit_code_on_failure():
    rc = run_with_timeout.main(
        ["run_with_timeout.py", "5", sys.executable, "-c", "import sys; sys.exit(7)"]
    )

    assert rc == 7


def test_cli_main_requires_a_command():
    rc = run_with_timeout.main(["run_with_timeout.py", "5"])

    assert rc == 2
