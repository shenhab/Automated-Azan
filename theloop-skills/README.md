# Skills — deterministic, repo-owned

Each subdirectory is one skill: a `skill.md` manifest plus the script(s) that
implement it. Skills are deterministic code, never agent prompts — see
`../theloop.md` for the principle.

`skill.md` format:

    # skill: <name>
    description: one line — what it does and when to use it
    run: `<command, from the repo root>`
