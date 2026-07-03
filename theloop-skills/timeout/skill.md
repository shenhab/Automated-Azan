# skill: timeout
description: Run a command under a bounded wall-clock timeout, guaranteeing its whole process group is killed if it overruns — for commands in this repo that can hang or leak background processes (`uv run python -m pytest`, `go test ./...`, the Flask/SocketIO web interface, the scheduler's infinite loop, Chromecast mDNS discovery).
run: `python3 theloop-skills/timeout/run_with_timeout.py <seconds> <command> [args...]`

## When to use

Any time an agent would otherwise reach for the shell `timeout` command to
bound a test run or a manual app-verification step in this repo — a
recurring pattern (10+ occurrences across recent cycles). Prefer this
script over raw `timeout <n> <cmd>`: on expiry it kills the *entire*
process group the command spawned, not just the immediate child, so
nothing is left running in the background — satisfying the "never leave
background test processes running when you finish" rule every implementer
works under.

Examples:

```
python3 theloop-skills/timeout/run_with_timeout.py 60 uv run python -m pytest tests/ -v
python3 theloop-skills/timeout/run_with_timeout.py 30 bash -c "cd go && go test ./..."
python3 theloop-skills/timeout/run_with_timeout.py 10 uv run python main.py
```

Exit code is the wrapped command's own exit code, or `124` if it was
killed for exceeding the timeout (matching the standard `timeout(1)`
convention). Nothing needs to be imported — it's a standalone CLI script.
