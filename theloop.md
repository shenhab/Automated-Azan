# theloop.md — the autonomous loop's manifest for this repo

This repository is continuously improved by TheLoop, an autonomous multi-agent
system. This file tells its agents (and humans) how the loop works here and
where its deterministic skills live.

## Principles

1. **Deterministic code beats agent work.** It is faster, cheaper, and
   testable. Any action an agent performs repeatedly that code could do must
   be promoted into a *skill* — a deterministic script in the skills directory
   below — so the loop speeds up over time instead of re-deriving the same
   work with AI.

## Skills

<!-- skills-dir: theloop-skills -->
Registered skills live in `theloop-skills/`. Each skill is a directory holding
a `skill.md` (what it does, when to use it, how to run it) and the script(s)
that implement it. Skills are deterministic code — never prompts.

**Agents:** before doing multi-step mechanical work, check the skills listed in
your briefing — if a skill covers it, RUN THE SKILL instead of improvising.
When you notice you are repeating mechanizable work, register a new skill:
create `theloop-skills/<name>/skill.md` plus its script and ship it with your
change.
