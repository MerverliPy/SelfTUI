#!/usr/bin/env python3
"""Unit tests for scripts/reconnect-smoke.py (M-11 private unique captures).

Fake-host/fake-app tests drive the script's main() with every live surface
replaced — no live Ollama host, no pty, deterministic clock. They pin the
M-11 contract for the reconnect smoke:

  * the run's scratch config/state and its capture live in a fresh unique
    0700 temp dir with one exclusive 0600 capture file — never the old
    fixed /tmp/selftui-reconnect.log, so a pre-placed symlink at that path
    is never followed (nor modified);
  * a failed run retains the scratch + capture and prints the exact capture
    path;
  * a successful run removes scratch + capture by default, or retains them
    (capture path printed) when SMOKE_KEEP_CAPTURE=1.

Only the Python standard library is used. Run from the repository root:

    python3 -m unittest -v scripts/reconnect_smoke_test.py
"""
import contextlib
import importlib.util
import io
import os
import re
import shutil
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
SMOKE_PATH = os.path.join(HERE, "reconnect-smoke.py")
MODEL = "reconnect:fake-model"
# The old fixed capture path (M-11 finding): the script used to write the
# combined session capture here with an ordinary open(..., "w").
OLD_LOG = "/tmp/selftui-reconnect.log"
STATE_DIR_RE = re.compile(r"state dir: (\S+)")
CAPTURE_RE = re.compile(r"capture: ([^)\s]+)\)")
# The two 'starting … theme=light' lines the fake app writes to the state
# log, mirroring what the real binary logs for each session.
APP_LOG = ("starting version=dev theme=light program running\n"
           "starting version=dev theme=light program running\n")


def load_smoke():
    """Import scripts/reconnect-smoke.py (hyphens: importlib). After the
    M-11 refactor the module performs no filesystem work at import time."""
    spec = importlib.util.spec_from_file_location("reconnect_smoke", SMOKE_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeTags:
    """Scripted /api/tags stand-in; pick_model only reads the names."""

    def __init__(self, names=("reconnect:fake-a", "reconnect:fake-b")):
        self.names = list(names)
        self.calls = 0

    def __call__(self):
        self.calls += 1
        return [{"name": name, "size": 1_000_000 + i}
                for i, name in enumerate(self.names)]


class FakeClock:
    """Deterministic time.time()/time.sleep() so wait loops terminate
    instantly instead of sleeping for the real durations."""

    def __init__(self):
        self.now = 5000.0

    def time(self):
        self.now += 1.0
        return self.now

    def sleep(self, _seconds):
        self.now += 0.1


class FakeProc:
    def __init__(self, rc):
        self.rc = rc

    def poll(self):
        return self.rc

    def kill(self):
        self.rc = -9


class FakeApp:
    """Scripted selftui session standing in for the pty App: boots, echoes
    the prompt and streams model digits on pump, and drops/quits with the
    scripted return code. On construction it also writes the app log the
    real binary leaves under the run's XDG state dir, so the success path's
    two-session evidence check has real content to read.

    index 0 = first session (transport drop → rc -1); index 1 = reconnected
    session (quit → quit_rc).
    """

    def __init__(self, args, state_dir, quit_rc, index):
        self.args = args
        self.proc = FakeProc(rc=-1 if index == 0 else quit_rc)
        self.quit_rc = quit_rc
        self.seen = (
            "Models  list  72x30 compact  " + MODEL +
            " Count slowly from 1 to 300, one number per line.")
        self._streamed = False
        logdir = os.path.join(state_dir, "selftui")
        os.makedirs(logdir, exist_ok=True)
        with open(os.path.join(logdir, "log.txt"), "w") as f:
            f.write(APP_LOG)

    def wait_for(self, _pattern, _timeout, _step=0.25):
        return True

    def keys(self, _s):
        pass

    def pump(self, _until):
        if not self._streamed:
            self.seen += " 1 2 3"  # model digits streaming in
            self._streamed = True

    def drop(self):
        return -1  # SIGHUP death, like sshd closing the connection

    def quit(self, _timeout=6):
        return self.quit_rc


def app_factory(module, quit_rc):
    """Return a factory for smoke.App that hands out scripted FakeApp
    sessions (first session = transport drop, second = reconnect)."""
    state = {"count": 0}

    def factory(args):
        index = state["count"]
        state["count"] += 1
        return FakeApp(args, module.state_dir, quit_rc, index)

    return factory


@contextlib.contextmanager
def keep_capture_env(keep):
    """Set (or clear) SMOKE_KEEP_CAPTURE for the run and restore the
    caller's environment afterwards."""
    previous = os.environ.get("SMOKE_KEEP_CAPTURE")
    try:
        if keep:
            os.environ["SMOKE_KEEP_CAPTURE"] = "1"
        else:
            os.environ.pop("SMOKE_KEEP_CAPTURE", None)
        yield
    finally:
        if previous is None:
            os.environ.pop("SMOKE_KEEP_CAPTURE", None)
        else:
            os.environ["SMOKE_KEEP_CAPTURE"] = previous


def cleanup_module_scratch(module):
    """Remove the scratch dir a failed (or kept) run retained, so unit runs
    leave no /tmp litter. Handles the pre-fix module SCRATCH constant too."""
    scratch = getattr(module, "scratch_dir", None)
    if scratch is None:
        scratch = getattr(module, "SCRATCH", None)
    if scratch:
        shutil.rmtree(scratch, ignore_errors=True)


class ReconnectCaptureTest(unittest.TestCase):
    def setUp(self):
        self.stdout = io.StringIO()

    def run_main(self, quit_rc=0, keep_capture=False):
        """Drive reconnect-smoke.main() with every live surface faked.

        quit_rc = the reconnected session's ctrl+c exit code (nonzero makes
        the run fail with a capture). Returns (module, code_or_None,
        printed_text); a retained scratch dir is removed after the test.
        """
        smoke = load_smoke()
        patchers = [
            mock.patch.object(smoke, "tags", FakeTags()),
            mock.patch.object(smoke, "short_generation", lambda _model: 0.25),
            mock.patch.object(smoke, "App", side_effect=app_factory(smoke, quit_rc)),
            mock.patch.object(smoke.sys, "argv", ["reconnect-smoke.py", MODEL]),
        ]
        clock = FakeClock()
        patchers += [
            mock.patch.object(smoke.time, "time", clock.time),
            mock.patch.object(smoke.time, "sleep", clock.sleep),
        ]
        with contextlib.ExitStack() as stack:
            for patcher in patchers:
                stack.enter_context(patcher)
            with keep_capture_env(keep_capture):
                with mock.patch.object(sys, "stdout", self.stdout):
                    try:
                        smoke.main()
                        code = None
                    except SystemExit as exc:  # fail() exits 1
                        code = exc.code
        self.addCleanup(cleanup_module_scratch, smoke)
        return smoke, code, self.stdout.getvalue()

    def capture_path(self, printed):
        match = CAPTURE_RE.search(printed)
        return match.group(1) if match else None

    def assert_private_capture(self, path):
        """A retained capture must be an exclusive 0600 file directly inside
        a 0700 scratch dir, not the old fixed /tmp path."""
        self.assertIsNotNone(path, "run must print its exact capture path")
        self.assertNotEqual(path, OLD_LOG,
                            "capture must not live at the old fixed /tmp path")
        self.assertTrue(os.path.isfile(path), f"capture missing: {path}")
        self.assertEqual(os.stat(path).st_mode & 0o777, 0o600,
                         "capture file must be 0600")
        scratch = os.path.dirname(path)
        self.assertEqual(os.stat(scratch).st_mode & 0o777, 0o700,
                         "scratch dir must be 0700")

    # --- M-11: private unique 0600 captures, never the fixed /tmp path -----

    def test_old_fixed_log_path_symlink_is_never_followed(self):
        # A hostile/previous process pre-places a symlink at the old fixed
        # capture path pointing at a canary file. The smoke must never open
        # or modify it (the old success/fail code truncated it with an
        # ordinary open(..., "w")).
        fd, canary = tempfile.mkstemp(prefix="reconnect-canary-")
        os.write(fd, b"canary-payload")
        os.close(fd)
        try:
            if os.path.lexists(OLD_LOG):  # leftover from an earlier run
                os.unlink(OLD_LOG)
            os.symlink(canary, OLD_LOG)
            # A failure carrying session text is exactly where the old code
            # wrote the capture (and followed the symlink).
            _smoke, code, printed = self.run_main(quit_rc=1)
            self.assertEqual(code, 1)
            self.assertIn("SMOKE FAIL", printed)
            with open(canary, "rb") as f:
                self.assertEqual(
                    f.read(), b"canary-payload",
                    "the smoke followed the old fixed capture symlink and "
                    "modified its target")
            self.assertTrue(os.path.islink(OLD_LOG),
                            "the old fixed capture path was replaced")
        finally:
            if os.path.islink(OLD_LOG):
                os.unlink(OLD_LOG)
            if os.path.exists(canary):
                os.unlink(canary)

    def test_failure_retains_private_scratch_and_capture(self):
        _smoke, code, printed = self.run_main(quit_rc=1)

        self.assertEqual(code, 1)
        self.assertIn("SMOKE FAIL", printed)
        self.assert_private_capture(self.capture_path(printed))
        self.assertTrue(os.path.exists(self.capture_path(printed)))
        # Scratch config/state is retained too (failure evidence), under the
        # same 0700 dir as the capture.
        state = STATE_DIR_RE.search(printed).group(1)
        self.assertTrue(os.path.isdir(state))
        self.assertEqual(os.stat(os.path.dirname(state)).st_mode & 0o777,
                         0o700)

    def test_success_removes_scratch_and_capture_by_default(self):
        smoke, code, printed = self.run_main(quit_rc=0)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assertNotIn("capture:", printed,
                         "a successful run must not advertise a removed "
                         "capture")
        self.assertNotIn("app log:", printed)
        # The private scratch (config + state + capture) is gone entirely.
        state = STATE_DIR_RE.search(printed).group(1)
        scratch = os.path.dirname(state)
        self.assertFalse(os.path.exists(scratch),
                         "scratch config/state/capture must be removed after "
                         "a successful run")
        self.assertIsNone(smoke.capture_path)

    def test_success_keeps_scratch_and_capture_when_env_set(self):
        smoke, code, printed = self.run_main(quit_rc=0, keep_capture=True)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assert_private_capture(self.capture_path(printed))
        # Retained capture exists and holds the combined session text.
        self.assertTrue(os.path.exists(smoke.capture_path))
        with open(smoke.capture_path) as f:
            self.assertIn("--- second session ---", f.read())


if __name__ == "__main__":
    unittest.main(verbosity=2)
