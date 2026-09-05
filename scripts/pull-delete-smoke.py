#!/usr/bin/env python3
"""Live M1b smoke: drive the SelfTUI Models tab over a pty and verify
delete-with-confirm and streamed pull against a real Ollama host.

Usage: scripts/pull-delete-smoke.py [model]
Defaults to qwen3:0.6b — a real registry model. NON-DESTRUCTIVE: the script
captures the host's initial state before any mutation and refuses to run when
the target model is already installed — it never deletes (or re-pulls to
"restore") a model this run did not create, because a tag can move and the
original digest is not recorded. Point make smoke at a disposable model/tag
or run it against an isolated Ollama store. When the target is absent the run
pulls it through the TUI (waiting on the UI's own completion signals), deletes
it through the TUI, verifies it left /api/tags, and cleans up (including on
failure) only a model this run provably created.

Exit 0 only when, in order: the target was absent at start, the pull name
input opens, the pull dialog renders (spinner/status/progress), the UI leaves
the dialog with the model landed in /api/tags, delete confirm renders naming
the model, and the model leaves /api/tags after `y`.
"""
import fcntl
import json
import os
import pty
import re
import select
import struct
import subprocess
import sys
import time
import urllib.request

# Import-safe defaults: argv and SMOKE_* are read inside main() only, never
# at import time, so scripts/pull_delete_smoke_test.py can exec_module() and
# drive every function through unittest.mock fakes.
MODEL = "qwen3:0.6b"
LOG = "/tmp/selftui-smoke.log"

out = bytearray()
seen = ""


def api_delete(name):
    req = urllib.request.Request(
        "http://localhost:11434/api/delete",
        data=json.dumps({"name": name}).encode(),
        method="DELETE",
    )
    try:
        urllib.request.urlopen(req, timeout=10)
        return True
    except Exception:
        return False


def tags():
    """Live /api/tags names in Ollama's order; a string on failure."""
    try:
        d = json.load(urllib.request.urlopen("http://localhost:11434/api/tags", timeout=5))
        return [m["name"] for m in d["models"]]
    except Exception as e:  # noqa: BLE001
        return f"tags error: {e}"


def preflight(model):
    """Capture initial host state before any mutation; refuse a live target.

    Runs before any DELETE, pull, or TUI action. Aborts (exit 1) when the
    target is already installed: deleting it would destroy something this run
    did not create, and re-pulling the tag is not a safe restore because a tag
    can move and the original digest is not recorded.
    """
    initial = tags()
    if not isinstance(initial, list):
        fail(f"ollama not reachable: {initial}")
    if model in initial:
        fail(
            f"{model} is already installed; refusing to run — pull-delete-smoke "
            "never deletes a pre-existing model (it would remove something this "
            "run did not create). Use a disposable model/tag or an isolated "
            "Ollama store."
        )
    return initial


def pump(fd, deadline):
    """Drain the pty into `out` until deadline."""
    global seen
    while time.time() < deadline:
        r, _, _ = select.select([fd], [], [], 0.05)
        if not r:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            return
        if not data:
            return
        out.extend(data)
        seen = out.decode("utf-8", "replace")


def wait_for(fd, pattern, timeout, step=0.25):
    """Block until pattern appears in the pty capture or timeout expires."""
    end = time.time() + timeout
    while time.time() < end:
        pump(fd, time.time() + step)
        if re.search(pattern, seen, re.MULTILINE):
            return True
    return False


def fail(msg):
    with open(LOG, "w") as f:
        f.write(seen)
    print(f"SMOKE FAIL: {msg} (capture: {LOG})")
    sys.exit(1)


def main(argv=None):
    model = argv[0] if argv else MODEL
    watch = float(os.environ.get("SMOKE_WATCH", "300"))  # max wait for the pull
    size = (int(os.environ.get("SMOKE_COLS", "110")), int(os.environ.get("SMOKE_ROWS", "36")))

    # H-04: capture initial state and refuse an already-installed target
    # BEFORE any mutation. A pre-existing model is never deleted and never
    # "restored" by re-pulling its tag — the whole run aborts instead.
    preflight(model)
    created = False  # becomes True only after this run's pull installed it

    master, slave = pty.openpty()
    # target geometry: wide (110x36)
    fcntl.ioctl(slave, 0x5414, struct.pack("HHHH", size[1], size[0], 0, 0))
    proc = subprocess.Popen(
        ["./bin/selftui"],
        stdin=slave, stdout=slave, stderr=slave, close_fds=True,
    )
    os.close(slave)
    started = time.time()

    def keys(s):
        os.write(master, s.encode())

    try:
        # 1. Boot + list load.
        if not wait_for(master, r"Models", 8):
            fail("app did not boot / render the Models tab")
        if not wait_for(master, r"qwen3:8b", 8):
            fail("model list did not load (live host)")
        pump(master, time.time() + 0.5)

        # 2. Pull: p -> name -> enter -> streamed dialog.
        keys("p")
        if not wait_for(master, r"Pull a model", 3):
            fail("pull name input did not open on p")
        keys(MODEL + "\r")
        if not wait_for(master, r"Pulling %s" % re.escape(MODEL), 3):
            fail("pull dialog did not open after enter")
        if not wait_for(master, r"esc cancel", 2):
            fail("pull dialog missing the cancel hint")
        if not wait_for(master, r"pulling manifest|pulling [0-9a-f]{6,}|success", 30):
            fail("pull streamed no status lines")
        # Progress bar/bytes are evidence; a fully-cached layer can render
        # very few frames, so absence is not a failure (statuses are).
        progress_seen = wait_for(master, r"\d+%|\d+ B / \d+ B", 8)

        # 3. UI completion: the pull dialog closes on its own after the final
        #    stream event. bubbletea v2 redraws incrementally, so the pull
        #    dialog's constantly-updated lines (status/percent/bytes/spinner)
        #    stay in the recent capture while it is open; when they fall
        #    silent for a sustained beat the dialog is gone. A surfaced
        #    "⚠ <error>" aborts first.
        in_dialog = re.compile(r"Pulling %s|esc cancel|pulling [0-9a-f]{6,}|verifying sha256|writing manifest|success|B / |%%" % re.escape(MODEL))
        end = time.time() + watch
        quiet_since = None
        while time.time() < end:
            pump(master, time.time() + 0.5)
            m = re.search(r"⚠ ([^\r\n]{3,})", seen)
            if m:
                fail(f"pull errored in the UI: {m.group(1).strip()}")
            if in_dialog.search(seen[-2500:]):
                quiet_since = None
            elif quiet_since is None:
                quiet_since = time.time()
            elif time.time() - quiet_since >= 1.0:
                break
            if os.environ.get("SMOKE_DEBUG"):
                clean = re.sub(r"\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07", "", seen[-2500:])
                print(f"[debug] t={time.time()-started:.0f}s quiet={quiet_since is not None} tail: {clean[-160:]!r}", flush=True)
            time.sleep(0.2)
        else:
            states = sorted(set(re.findall(r"pulling [^\r\n]*|\d+(\.\d+)? ?[KMG]?B / \d+(\.\d+)? ?[KMG]?B|\d+%%", seen)))
            fail("pull dialog never exited within %ds (UI last showed: %s)" % (watch, " | ".join(states)))
        pump(master, time.time() + 0.6)

        # 4. Server truth: the model is installed.
        after = tags()
        if not isinstance(after, list):
            fail(f"tags query after pull failed: {after}")
        if MODEL not in after:
            fail(f"{MODEL} missing from /api/tags after the UI reported done")
        created = True  # this run's pull provably installed the model
        try:
            jump = after.index(MODEL)
        except ValueError:
            fail("model index lookup failed")

        # 5. Delete: navigate to the model, x -> confirm -> y.
        keys("j" * jump)  # cursor starts at 0 after the reload
        pump(master, time.time() + 0.4)
        keys("x")
        if not wait_for(master, r"Delete model", 3):
            fail("delete confirm dialog did not open on x")
        if not wait_for(master, r"Delete %s\?" % re.escape(MODEL), 2):
            fail("confirm dialog does not name the model")
        keys("y")
        deleted_seen = wait_for(master, r"deleted %s" % re.escape(MODEL), 8)
        end = time.time() + 20
        while time.time() < end:
            t = tags()
            if isinstance(t, list) and MODEL not in t:
                break
            pump(master, time.time() + 0.5)
            time.sleep(0.5)
        else:
            fail("model still present in /api/tags after y (delete did not complete)")

        # 6. Quit with ctrl+c.
        keys("\x03")
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            fail("app did not exit on ctrl+c")

        with open(LOG, "w") as f:
            f.write(seen)
        print(f"SMOKE PASS in {time.time()-started:.0f}s (progress bar evidence: {progress_seen}, "
              f"deleted-notice seen: {deleted_seen}; capture: {LOG})")
    finally:
        if proc is not None and proc.poll() is None:
            proc.kill()
        # H-04: clean up only a model this run provably created (its own
        # pull), never a pre-existing one. On the success path the model was
        # already deleted through the TUI, so this is a harmless no-op safety
        # net; on failure it removes only this run's leftover model.
        if created:
            api_delete(model)


if __name__ == "__main__":
    main(sys.argv[1:] if len(sys.argv) > 1 else None)