# V2c sandboxed `run_command` evidence

**Date:** 2026-09-07  
**Repository:** `/home/calvin/SelfTUI`  
**Gate:** PLAN §10 V2c, after V2b GO

V2c reinstates `run_command` only behind bubblewrap, with the deferred design's
argv allowlist, no shell/interpreter, scrubbed environment, timeout/output
limits, process-group teardown, single-flight serialization, and per-call
confirmation.

## Host and focused mitigation tests

The release host has bubblewrap 0.9.0:

```text
$ bwrap --version
bubblewrap 0.9.0
```

Focused V2c tests were run against the real `/usr/bin/bwrap` and returned exit
code 0:

```text
$ go test ./internal/agent -run 'TestRunCommand|TestCommandOutput|TestRunnerConfirms' -count=1 -v
=== RUN   TestRunCommandRejectsUnsafeArgv
--- PASS: TestRunCommandRejectsUnsafeArgv (0.00s)
=== RUN   TestRunCommandWaitsForSingleFlightSlotWithCancellation
--- PASS: TestRunCommandWaitsForSingleFlightSlotWithCancellation (0.02s)
=== RUN   TestRunCommandUsesBubblewrapAndScrubsEnvironment
--- PASS: TestRunCommandUsesBubblewrapAndScrubsEnvironment (0.01s)
=== RUN   TestCommandOutputCapsEachStream
--- PASS: TestCommandOutputCapsEachStream (0.00s)
=== RUN   TestRunCommandExecutesOfflineGoTestInsideSandbox
--- PASS: TestRunCommandExecutesOfflineGoTestInsideSandbox (5.66s)
=== RUN   TestRunCommandAllowsReadOnlyGit
--- PASS: TestRunCommandAllowsReadOnlyGit (0.01s)
=== RUN   TestRunnerConfirmsAndExecutesSandboxedCommand
--- PASS: TestRunnerConfirmsAndExecutesSandboxedCommand (0.01s)
=== RUN   TestRunCommandEnforcesTimeout
--- PASS: TestRunCommandEnforcesTimeout (1.00s)
=== RUN   TestRunCommandCancellationKillsSandboxProcessGroup
--- PASS: TestRunCommandCancellationKillsSandboxProcessGroup (0.28s)
PASS
ok  	selftui/internal/agent	6.997s
```

These tests cover:

- fixed `go`/read-only `git` argv allowlisting and shell/interpreter/path
  rejection;
- cancellation-aware single-flight serialization;
- `--clearenv` plus the fixed environment and read-only module-cache mount;
- 256 KiB per-stream output caps;
- a real offline `go test` workload inside the bwrap filesystem/network
  boundary;
- explicit `ToolConfirmMsg` approval before execution;
- the 30-second default/60-second maximum timeout contract and process-group
  cancellation teardown.

## Repository gates

The canonical repository gate returned exit code 0:

```text
$ make check
go build -o bin/selftui ./cmd/self-tui
go test -count=1 ./...
ok  	selftui/cmd/self-tui	0.147s
?   	selftui/cmd/size-probe	[no test files]
ok  	selftui/internal/agent	6.754s
ok  	selftui/internal/config	0.175s
ok  	selftui/internal/ollama	1.083s
ok  	selftui/internal/session	0.205s
ok  	selftui/internal/ui	6.929s
go vet ./...
```

The race suite returned exit code 0:

```text
$ go test -race -count=1 ./...
ok  	selftui/cmd/self-tui	1.454s
?   	selftui/cmd/size-probe	[no test files]
ok  	selftui/internal/agent	7.609s
ok  	selftui/internal/config	1.269s
ok  	selftui/internal/ollama	4.994s
ok  	selftui/internal/session	1.187s
ok  	selftui/internal/ui	12.847s
```

## Residual risk

Bubblewrap has no CPU or memory cap. A hostile allowlisted Go workload can
create host-wide memory pressure until the 30-second default/60-second maximum
timeout fires. Rootless Docker's enforced memory limit was independently
validated in the V2b gate and remains the stronger, slower alternative for a
future selectable engine; the current binary uses bwrap as its default and only
engine.
