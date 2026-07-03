#!/usr/bin/env python3
"""
Run a command under a bounded wall-clock timeout, guaranteeing its entire
process group is torn down if it overruns.

This repo's commands routinely spawn things that can hang or leak
background processes: `uv run python -m pytest`, `go test ./...`, the
Flask+SocketIO web interface, the Athan scheduler's infinite loop, and
Chromecast mDNS discovery threads. Wrapping any of these in this script
gives a deterministic pass/fail instead of an agent waiting indefinitely
or leaving a stray process running after the shell command returns.
"""
import os
import signal
import subprocess
import sys
import time


def run_command(cmd, timeout, kill_after=5):
    """Run cmd (a list of argv) with a bounded timeout.

    Returns {"success": bool, "returncode": int|None, "timed_out": bool}.
    success is True only if the process exited on its own with code 0
    before the timeout elapsed. On timeout, the command's whole process
    group is sent SIGTERM, then SIGKILL after `kill_after` seconds if it
    hasn't exited yet, so no child process is left running.
    """
    proc = subprocess.Popen(cmd, start_new_session=True)

    try:
        returncode = proc.wait(timeout=timeout)
        return {"success": returncode == 0, "returncode": returncode, "timed_out": False}
    except subprocess.TimeoutExpired:
        pass

    _kill_process_group(proc, kill_after)
    return {"success": False, "returncode": proc.returncode, "timed_out": True}


def _kill_process_group(proc, kill_after):
    try:
        pgid = os.getpgid(proc.pid)
        os.killpg(pgid, signal.SIGTERM)
    except ProcessLookupError:
        return

    deadline = time.time() + kill_after
    while time.time() < deadline:
        if proc.poll() is not None:
            return
        time.sleep(0.05)

    try:
        os.killpg(pgid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    proc.wait()


def main(argv):
    if len(argv) < 3:
        print("usage: run_with_timeout.py <seconds> <command> [args...]", file=sys.stderr)
        return 2

    try:
        seconds = float(argv[1])
    except ValueError:
        print(f"run_with_timeout: invalid seconds value '{argv[1]}'", file=sys.stderr)
        return 2

    cmd = argv[2:]
    result = run_command(cmd, seconds)

    if result["timed_out"]:
        print(
            f"run_with_timeout: '{' '.join(cmd)}' exceeded {seconds}s, process group killed",
            file=sys.stderr,
        )
        return 124

    return result["returncode"] if result["returncode"] is not None else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
