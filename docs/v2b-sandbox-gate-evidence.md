# V2b — sandbox spike GATE evidence (2026-09-07)

**Gate:** PLAN §10 **V2b** — evaluate a real OS/container sandbox for command
execution on the release host against the threat model in
`docs/run-command-containment.md`. GO → V2c (reinstate `run_command` behind the
sandbox); NO-GO → sandboxed execution stays out of v0.2.

**Release host:** `CALVINPC` — WSL2 (kernel `6.18.33.2-microsoft-standard-WSL2`),
Ubuntu 24.04.4, unprivileged userns enabled (`user.max_user_namespaces = 96091`),
systemd 255 with a running user manager + linger, cgroup v2.
All experiments below were run in this session on the actual host.

## Candidates inventoried

| Candidate | Status on host |
|---|---|
| bubblewrap | **present** — bwrap 0.9.0 (`/usr/bin/bwrap`) |
| `systemd-run` slices | **present** — systemd 255, user manager running, `Linger=yes` |
| Rootless containers | **present** — Docker 29.6.0 in **rootless mode** (active context `rootless`, socket `/run/user/1000/docker.sock`, cgroup v2). Podman absent. |
| Docker rootful | also available (`default` context, `docker` group) — not needed; rootless preferred |

## Results matrix (vs the containment threat model)

Threats to contain (from `docs/run-command-containment.md`): credential reads,
network access, writes outside the workspace, surviving children, resource
exhaustion, env-secret leakage.

| Requirement | bwrap 0.9.0 | systemd-run (user) | Docker rootless |
|---|---|---|---|
| FS isolation (no credential reads) | **PASS** — selective binds; `/home` not mounted; `~/.ssh` and `/etc/shadow` invisible from inside | **none by itself** | **PASS** — container FS is the image; host `/home` empty/unmounted by default (verified) |
| Write jail (only workspace rw) | **PASS** — writes outside workspace fail; a `> /etc/...` escape wrote only to sandbox-internal tmpfs (host verified clean) | **none by itself** | **PASS** — `--read-only` root + explicit mounts; container writes land as host `calvin` (rootless uid mapping: container-root ≡ host user) |
| Network isolation | **PASS** — `--unshare-net`: DNS fails (`getent` rc=2), HTTP fails (curl exit 6) | **FAIL** — `IPAddressDeny=any` ignored on this host: DNS resolved **and** a full HTTPS fetch succeeded inside the scope | **PASS** — `--network none`: `nslookup` → "Network is unreachable" (control with network resolves) |
| Env scrub | **PASS** — `--clearenv` + allowlist → exactly `PATH/HOME/GOCACHE/PWD` visible | partial (env passes through) | **PASS** — `-e` allowlist (image baseline env remains) |
| Memory/CPU limits | none inherent | **FAIL** — `MemoryMax=64M` **not enforced** in user scope (independently re-verified by the gate audit) or system scope (observed in the gate session's privileged run; auditor could not re-run without privilege): a ~133 MB allocation survived both; `memory.max` unreadable in the delegated cgroup — WSL2 cgroup-delegation quirk | **PASS** — `--memory 64m` OOM-kills a ~128 MB hog (exit 137); no-limit control survives |
| Kill / cancellation | **PASS** — group SIGTERM kills bwrap *and* the inner process (`--die-with-parent --new-session` verified); integrates with the existing process-group executor | PASS (unit stop) | PASS (`docker kill`; bounded by image lifecycle) |
| **Real workload** — `go test -count=1 ./internal/session`, offline | **PASS** (~5 s incl. compile; tmpfs `GOCACHE`; pinned toolchain bind) | n/a alone (no FS/network isolation of its own) | **PASS** (4.5 s; `golang:1.27-alpine`; `--tmpfs /tmp:exec` required — default tmpfs is `noexec`) |
| Read-only `git` argv | **PASS** (`git log` inside, with `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null`) | n/a | PASS (git available in a suitable image) |
| Per-invocation overhead | **~10 ms** (plus workload) | ~10 ms | **~2–5 s** per `docker run` (amortizable with `docker exec` into a long-lived container) |

### Host-specific findings worth keeping

1. **systemd `IPAddressDeny` and `MemoryMax` are silently unenforced on this
   WSL2 host** — in *both* the user and the system manager. systemd-run is
   therefore useful only as a cgroup/tasks sidecar, never as a containment
   layer. (The failures are silent: the scope runs fine and the probes succeed.)
2. **Docker `--tmpfs` defaults to `noexec`** on this daemon — compiled-output
   directories need `--tmpfs /tmp:exec`.
3. **Rootless uid mapping:** container-root maps to host `calvin`; `--user
   1000:1000` maps to a subuid *not* to host uid 1000 (writes as that user fail
   against host-owned dirs). V2c must mount the workspace and use the rootless
   mapping, never `--user 1000`.
4. **Go toolchain reality:** the effective toolchain (go1.27.1) is
   GOTOOLCHAIN-managed and lives inside `GOMODCACHE`
   (`golang.org/toolchain@v0.0.1-go1.27.1...`). Sandbox runs bind the module
   cache read-only and either bind the extracted toolchain (bwrap) or use
   `golang:1.27-alpine` (docker), with `GOPROXY=off GOTOOLCHAIN=local`.
   `GOTOOLCHAIN=local` against the distro go (1.22) or `/usr/local/go` (1.25.0)
   fails the `go.mod >= 1.25.8` requirement.
5. **bwrap `--setenv` cannot take a value starting with `-`** (parsed as an
   option) — irrelevant to the argv allowlist but worth knowing when writing
   flags.

## Verdict: **GO**

Two viable sandbox stacks were validated end-to-end on the release host against
the threat model:

- **Primary (recommended for V2c): bubblewrap.** Selective read-only binds
  (`/usr`, `/lib*`, minimal `/etc` files — never `/home`, never secret paths) +
  read-only or read-write workspace bind + `--tmpfs /tmp` + `--unshare-net` +
  `--clearenv` + allowlist env + `--die-with-parent --new-session`. Near-zero
  overhead fits an interactive TUI per-command model. Gap: no CPU/memory caps —
  accepted, because the existing containment layer already enforces timeout
  (30 s default / 60 s cap), output caps, serialization, and process-group kill,
  and the workload is bounded `go`/`git` argv.
- **Alternative (strongest isolation): rootless Docker.** `--network none`,
  `--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges`,
  `--memory` (enforced), ro module-cache mount — full threat-model coverage
  including memory caps, at the cost of per-command latency and image
  management. Viable as an opt-in engine.

Both stacks passed the real offline `go test` workload. The gate's precondition
from `docs/run-command-containment.md` — "reintroducing command execution
requires a real OS/container sandbox first" — is now satisfied on this host.

**Per-call confirmation, argv allowlist (no shell/interpreter), scrubbed env,
timeout + process-group kill stay mandatory in V2c** — the sandbox augments,
not replaces, the deferred containment design.

**Next step → V2c** (sandboxed `run_command`, per PLAN §10; one session per the
binding rule).

**Independent audit:** a read-only reality-checker pass re-ran the four load-bearing
probes (bwrap net isolation, bwrap credential hiding, docker `--network none`,
systemd `IPAddressDeny` non-enforcement) and reproduced **4/4**, plus the docker
`--memory` OOM-kill; verdict **ENDORSE GO** with conditions carried to V2c:

1. V2c must ship **and test** the full mitigation stack the GO leans on —
   timeout (≤60 s), output caps, single-flight serialization, process-group kill,
   argv allowlist, per-call confirm — since the v0.1 executor was deleted and the
   sandbox alone does not cover CPU/memory.
2. V2c's design note must record the residual risk: unbounded RSS for up to one
   timeout window (possible host-wide OOM pressure on WSL2); keep rootless Docker
   (`--memory` enforced) as the documented opt-in engine for less-trusted workloads.
3. V2c evidence must include verbatim command outputs/exit codes.
4. V2c test cases must include the bwrap group-kill path and the offline
   `go test` workload (the two gate-session results the audit did not re-run).

NO-GO was a live outcome had bwrap failed under WSL2 or the real workload been
unable to run offline; recorded for the ledger that the gate was genuinely
evaluated, not assumed.
