---
name: Bug report
about: Something in SelfTUI behaves wrongly, renders badly, or crashes
labels:
  - bug
---

**What happened?**

A clear and concise description of the bug.

**Steps to reproduce**

1. …
2. …

**What did you expect to happen?**

**Environment (all of these matter for a TUI bug report)**

- SelfTUI version (`selftui -version`, or `dev` if built from source):
- Ollama host + version (e.g. local `http://localhost:11434`, Ollama `0.33.1`):
- OS / architecture (e.g. Ubuntu 24.04 on WSL2, amd64):
- Terminal + geometry (cols × rows — e.g. Moshi portrait 72×30, wide PC 120×40):
- Workspace tools enabled? (yes/no — Settings → Agent)

**Logs**

Attach the relevant tail of `$XDG_STATE_HOME/selftui/log.txt` (bearer tokens
are already redacted at the sink). The `ctrl+o` logs drawer shows the same
lines from inside the TUI.

**Anything else?**

Golden-render output, screenshots, or related issues.
