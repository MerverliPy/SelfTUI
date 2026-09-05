#!/usr/bin/env python3
"""M6 reconnect smoke — fresh SSH re-connect semantics (docs/reconnect.md).

Simulates an SSH drop while bin/selftui is mid-turn against the local Ollama
host, then a reconnect. Canonical transport decision (owner, 2026-09-06): a
drop on plain SSH kills the host-side TUI process; reconnect = a fresh SSH
session + relaunch. Checks, in order:

  1. boot at the canonical device geometry 72x30 renders the Models tab and
     the status bar shows the negotiated geometry (72x30 compact);
  2. an agent chat turn starts against the smallest chat model on the host
     (informational: whether tokens streamed before the drop is recorded);
  3. the transport drops (pty master closes -> SIGHUP to the app session,
     like sshd closing the connection); the app must die promptly and cleanly;
  4. the Ollama host recovers: /api/tags is fast and a fresh short generation
     completes — the dropped client's in-flight job must not wedge the server;
  5. reconnect (fresh process at 72x30): clean boot, geometry restored, and
     the config file is applied (light theme from a scratch config) — config
     persistence across the reconnect;
  6. ctrl+c exits 0.

Exit 0 only if every step passes. Each run keeps its scratch config/state and
capture in a fresh private unique 0700 temp dir with one exclusive 0600
capture file: the scratch is retained on failure (the exact capture path is
printed) and removed after a successful run unless SMOKE_KEEP_CAPTURE=1.
Usage: scripts/reconnect-smoke.py [model]  (default: smallest chat model).
"""
import fcntl
import json
import os
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
import urllib.request

SIZE = (int(os.environ.get("SMOKE_COLS", "72")), int(os.environ.get("SMOKE_ROWS", "30")))

# M-11: the whole run is private and unique, and nothing is created at
# import time. main() makes one fresh 0700 scratch temp dir holding the
# scratch config, the hermetic XDG state dir, and one exclusive 0600 capture
# file — never a fixed /tmp path, so a symlink pre-placed at the old
# /tmp/selftui-reconnect.log is never followed or overwritten. The scratch
# is removed after a successful run unless SMOKE_KEEP_CAPTURE=1; a failed
# run retains it and prints the exact capture path.
CAPTURE_PREFIX = "selftui-reconnect-"
scratch_dir = None
state_dir = None
capture_path = None


def tags():
    """Live /api/tags; a string message on failure."""
    try:
        d = json.load(urllib.request.urlopen("http://localhost:11434/api/tags", timeout=5))
        return d["models"]
    except Exception as e:  # noqa: BLE001
        return f"tags error: {e}"


def pick_model(requested):
    """The chat model to drive: the requested one, else the smallest
    non-embedding model on the host (fastest round trip)."""
    models = tags()
    if isinstance(models, str):
        return None, models
    named = [m for m in models if "embed" not in m["name"]]
    if requested:
        return requested, None
    if not named:
        return None, "no chat-capable model on the host"
    return min(named, key=lambda m: m["size"])["name"], None


def short_generation(model):
    """A complete short chat round trip against `model` (stream=false,
    num_predict=8). Returns elapsed seconds or raises on failure."""
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": "Say OK."}],
        "stream": False,
        "options": {"num_predict": 8},
    }).encode()
    start = time.time()
    req = urllib.request.Request(
        "http://localhost:11434/api/chat", data=body,
        headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=90) as resp:
        d = json.load(resp)
    if not d.get("message", {}).get("content", "").strip():
        raise RuntimeError("generation returned empty content")
    return time.time() - start


class App:
    """One selftui launch inside a pty at the target geometry. The child is
    made its own session with the pty slave as its controlling terminal, so
    closing the master delivers SIGHUP exactly like sshd on a dropped
    connection."""

    def __init__(self, args):
        self.master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", SIZE[1], SIZE[0], 0, 0))

        def session_leader():
            os.setsid()
            fcntl.ioctl(0, termios.TIOCSCTTY, 0)

        env = dict(os.environ)
        env["XDG_STATE_HOME"] = state_dir
        self.proc = subprocess.Popen(
            args, stdin=slave, stdout=slave, stderr=slave,
            close_fds=True, preexec_fn=session_leader, env=env)
        os.close(slave)
        self.out = bytearray()
        self.seen = ""

    def pump(self, until):
        while time.time() < until:
            r, _, _ = select.select([self.master], [], [], 0.05)
            if not r:
                continue
            try:
                data = os.read(self.master, 65536)
            except OSError:
                return
            if not data:
                return
            self.out.extend(data)
            self.seen = self.out.decode("utf-8", "replace")

    def wait_for(self, pattern, timeout, step=0.25):
        end = time.time() + timeout
        while time.time() < end:
            self.pump(time.time() + step)
            if re.search(pattern, self.seen, re.MULTILINE | re.DOTALL):
                return True
        return False

    def keys(self, s):
        os.write(self.master, s.encode())

    def drop(self):
        """Close the pty master — the transport drop. Returns the exit code
        once the child is gone, or None if it is still alive."""
        try:
            os.close(self.master)
        except OSError:
            pass
        try:
            return self.proc.wait(timeout=6)
        except subprocess.TimeoutExpired:
            return None

    def quit(self, timeout=6):
        try:
            self.keys("\x03")
        except OSError:
            pass
        try:
            return self.proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            return self.proc.wait()


def keep_capture():
    """True when SMOKE_KEEP_CAPTURE=1 — retain scratch + capture after a pass."""
    return os.environ.get("SMOKE_KEEP_CAPTURE") == "1"


def open_scratch():
    """Create (once) the run's private scratch + exclusive capture file.

    Returns the scratch dir: a fresh 0700 temp dir (mkdtemp) holding the
    XDG state dir and one freshly created exclusive 0600 capture file
    (mkstemp opens with O_CREAT|O_EXCL and pins 0600). Nothing derives from
    a fixed caller-visible path, so a symlink pre-placed at the old /tmp
    capture path is never opened.
    """
    global scratch_dir, state_dir, capture_path
    if scratch_dir is None:
        scratch_dir = tempfile.mkdtemp(prefix=CAPTURE_PREFIX)
        state_dir = os.path.join(scratch_dir, "state")
        fd, capture_path = tempfile.mkstemp(prefix="capture-", dir=scratch_dir)
        os.close(fd)
    return scratch_dir


def write_capture(text):
    """Persist `text` into the run's capture file; returns its path."""
    with open(capture_path, "w") as f:
        f.write(text)
    return capture_path


def discard_scratch():
    """Remove the run's scratch — config, state, capture (success default)."""
    global scratch_dir, state_dir, capture_path
    if scratch_dir is not None:
        shutil.rmtree(scratch_dir, ignore_errors=True)
        scratch_dir = None
    state_dir = None
    capture_path = None


def fail(msg, capture=None):
    path = write_capture(capture if capture is not None else "")
    print(f"SMOKE FAIL: {msg} (capture: {path})")
    sys.exit(1)


def main():
    scratch = open_scratch()
    cfg = os.path.join(scratch, "config.toml")
    with open(cfg, "w") as f:
        f.write('theme = "light"\n')  # reconnect must re-apply the file config

    requested = sys.argv[1] if len(sys.argv) > 1 else ""
    model, err = pick_model(requested)
    if err:
        fail(err)
    print(f"model: {model}  geometry: {SIZE[0]}x{SIZE[1]}  state dir: {state_dir}")

    args = ["./bin/selftui", "-config", cfg, "-default-model", model]

    # --- 1. first session boots at 72x30, Models + geometry ---
    app = App(args)
    started = time.time()
    try:
        if not app.wait_for(r"Models", 8):
            fail("app did not boot / render the Models tab", app.seen)
        if not app.wait_for(r"72x30[\s\S]{0,200}?compact", 8):
            fail("status bar did not show the negotiated 72x30 compact geometry",
                 app.seen)
        if not app.wait_for(re.escape(model), 8):
            fail("model list did not load from the live host", app.seen)
        print(f"  boot ok (72x30 compact, list loaded) in {time.time()-started:.1f}s")

        # --- 2. start a chat turn; a drop mid-turn is the in-flight case ---
        app.keys("2")                      # Agent tab
        if not app.wait_for(r"shift\+enter", 4):
            fail("Agent tab did not render on 2", app.seen)
        prompt = "Count slowly from 1 to 300, one number per line."
        app.keys(prompt + "\r")
        if not app.wait_for(re.escape("Count slowly"), 5):
            fail("prompt was not echoed into the conversation", app.seen)
        # Mid-generation evidence: the echoed prompt itself contains digits,
        # so only count digits that appear AFTER the last echo — i.e. model
        # tokens streaming in. Informational; a slow CPU may still answer
        # only after the drop, and the post-drop host round trip below is the
        # real recovery assertion.
        echo_at = app.seen.rfind("Count slowly")
        end = time.time() + 12
        stream_started = False
        while time.time() < end:
            app.pump(time.time() + 0.25)
            after = app.seen[echo_at:]
            if re.search(r"[0-9]", after):
                stream_started = True
                break
            if "⚠" in after:  # a surfaced error aborts the turn early
                break
        # stay mid-stream for a beat, then drop
        time.sleep(1.5)
        print(f"  chat turn started; tokens streamed before drop: {stream_started}")

        # --- 3. transport drop: pty master closes -> SIGHUP to the session ---
        rc = app.drop()
        if rc is None:
            fail("app survived the transport drop (still running after 6s)", app.seen)
        note = f" (rc={rc})" if rc < 0 else ""
        print(f"  drop: app exited promptly with {rc}{note}")
    finally:
        if app.proc.poll() is None:
            app.proc.kill()

    # --- 4. host recovery: fast tags + a complete short generation ---
    time.sleep(1.5)  # let ollama reap the dropped client's runner
    t0 = time.time()
    if not isinstance(tags(), list):
        fail("ollama /api/tags not responsive after the drop")
    tags_s = time.time() - t0
    try:
        gen_s = short_generation(model)
    except Exception as e:  # noqa: BLE001
        fail(f"post-reconnect generation failed ({e}) — dropped client left a stuck job?")
    print(f"  host recovered: tags in {tags_s:.2f}s, generation round trip in {gen_s:.1f}s")

    # --- 5. reconnect: fresh process, geometry restored, config re-applied ---
    app2 = App(args)
    try:
        if not app2.wait_for(r"72x30[\s\S]{0,200}?compact", 8):
            fail("reconnected session did not render 72x30 compact", app2.seen)
        print(f"  reconnect boot ok (72x30 compact) in {time.time()-started:.1f}s total")
        rc = app2.quit()
        if rc != 0:
            fail(f"ctrl+c exit = {rc}, want 0", app2.seen)
    finally:
        if app2.proc.poll() is None:
            app2.proc.kill()

    # config persistence evidence: the app log must carry both sessions with
    # the light theme applied from the scratch config file.
    logfile = os.path.join(state_dir, "selftui", "log.txt")
    try:
        with open(logfile) as f:
            lines = f.read().splitlines()
    except OSError as e:
        fail(f"app log not found ({e})")
    starts = [l for l in lines if "starting" in l]
    if len(starts) < 2:
        fail(f"expected >=2 app sessions in the log, found {len(starts)}")
    if not all("theme=light" in l for l in starts[-2:]):
        fail("reconnected session did not re-apply the light theme from the config file")
    combined = app.seen + "\n\n--- second session ---\n" + app2.seen
    if keep_capture():
        cap = write_capture(combined)
        print(f"SMOKE PASS in {time.time()-started:.0f}s "
              f"(streamed-before-drop: {stream_started}; app log: {logfile}; "
              f"capture: {cap})")
    else:
        discard_scratch()
        print(f"SMOKE PASS in {time.time()-started:.0f}s "
              f"(streamed-before-drop: {stream_started})")


if __name__ == "__main__":
    main()
