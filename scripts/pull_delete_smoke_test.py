#!/usr/bin/env python3
"""Unit tests for scripts/pull-delete-smoke.py (H-04 non-destructive smoke,
M-11 private unique captures).

Fake-host tests drive the script with the Ollama HTTP surface (/api/tags and
/api/delete) and the pty/TUI machinery replaced by fakes, so no live host and
no pty are ever touched. They pin the H-04 contract:

  * initial host state is captured BEFORE any mutation;
  * an already-installed target aborts with a clear nonzero result and no
    DELETE, pull, or TUI action (never restore by re-pulling a tag);
  * cleanup deletes a model only after this run provably created it —
    including failure cleanup — and never a pre-existing one.

and the M-11 capture contract:

  * the run's capture lives in a fresh unique private temp directory (0700)
    as one exclusive 0600 file — never the old fixed /tmp/selftui-smoke.log,
    so a pre-placed symlink at that path is never followed;
  * a failed run retains the capture and prints its exact path;
  * a successful run removes the capture by default, or retains it (path
    printed) when SMOKE_KEEP_CAPTURE=1.

Only the Python standard library is used. Run from the repository root:

    python3 -m unittest -v scripts/pull_delete_smoke_test.py
"""
import contextlib
import importlib.util
import io
import os
import re
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
SMOKE_PATH = os.path.join(HERE, "pull-delete-smoke.py")
TEST_MODEL = "h04:fake-target"
# The old fixed capture path (M-11 finding): scripts used to write the full
# TUI capture here with an ordinary open(..., "w").
OLD_LOG = "/tmp/selftui-smoke.log"
CAPTURE_RE = re.compile(r"capture: ([^)\s]+)\)")


def load_smoke():
    """Import scripts/pull-delete-smoke.py (hyphens: importlib, no argv/env
    side effects after the H-04 import-safe refactor)."""
    spec = importlib.util.spec_from_file_location("pull_delete_smoke", SMOKE_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeHost:
    """Recorded /api/tags + /api/delete stand-ins driven by a tag script.

    tags_for_call(call_index) returns the list of installed model names that
    the fake host reports on that /api/tags call; every call and every delete
    is appended to `log` so tests can assert call ordering (state capture
    before mutation, cleanup after creation).
    """

    def __init__(self, tags_for_call):
        self.tags_for_call = tags_for_call
        self.log = []
        self.calls = 0
        self.deletes = []

    def tags(self):
        names = self.tags_for_call(self.calls)
        self.calls += 1
        self.log.append(("tags", list(names)))
        return list(names)

    def delete(self, name):
        self.deletes.append(name)
        self.log.append(("delete", name))
        return True


class FakeProc:
    def __init__(self):
        self.kills = 0
        self.waits = 0

    def poll(self):
        return None

    def wait(self, timeout=None):
        self.waits += 1
        return 0

    def kill(self):
        self.kills += 1


class FakeClock:
    """Deterministic time.time()/time.sleep() so pty wait loops terminate
    instantly instead of sleeping for the real WATCH/deadline durations."""

    def __init__(self):
        self.now = 1000.0

    def time(self):
        self.now += 1.0
        return self.now

    def sleep(self, _seconds):
        self.now += 0.25


@contextlib.contextmanager
def keep_capture_env(keep):
    """Set (or clear) SMOKE_KEEP_CAPTURE for the duration of the run and
    restore the caller's environment afterwards."""
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


def cleanup_module_capture(module):
    """Remove the capture dir/file a failed (or kept) run retained, so unit
    runs leave no /tmp litter. A no-op for modules without capture state
    (e.g. the pre-fix fixed-path code)."""
    path = getattr(module, "capture_path", None)
    directory = getattr(module, "capture_dir", None)
    if path:
        try:
            os.unlink(path)
        except OSError:
            pass
    if directory:
        try:
            os.rmdir(directory)
        except OSError:
            pass


class SmokeSafetyTest(unittest.TestCase):
    def run_main(self, host, wait_for_value=True, refuse_pty=False,
                 keep_capture=False):
        """Load a fresh module and run smoke.main() with every host + pty
        surface faked and SMOKE_KEEP_CAPTURE under test control.

        Returns (module, exit_code_or_None, host, printed_text). A SystemExit
        is caught and its code returned; a normal return yields None. Any
        capture the run retained is removed after the test.
        """
        smoke = load_smoke()
        stdout = io.StringIO()
        patchers = [
            mock.patch.object(smoke, "tags", host.tags),
            mock.patch.object(smoke, "api_delete", host.delete),
            mock.patch.object(smoke, "MODEL", TEST_MODEL),
            mock.patch.object(smoke, "wait_for", return_value=wait_for_value),
            mock.patch.object(smoke, "pump"),
        ]
        if refuse_pty:
            def never(*_a, **_k):
                raise AssertionError(
                    "pty/TUI machinery reached though the run must abort before "
                    "any pull or TUI action")
            patchers += [
                mock.patch.object(smoke.pty, "openpty", side_effect=never),
                mock.patch.object(smoke.subprocess, "Popen", side_effect=never),
            ]
        else:
            clock = FakeClock()
            proc = FakeProc()
            patchers += [
                mock.patch.object(smoke.pty, "openpty", return_value=(1234, 1235)),
                mock.patch.object(smoke.fcntl, "ioctl"),
                mock.patch.object(smoke.os, "write"),
                mock.patch.object(smoke.os, "read", return_value=b""),
                mock.patch.object(smoke.os, "close"),
                mock.patch.object(smoke.subprocess, "Popen", return_value=proc),
                mock.patch.object(smoke.time, "time", clock.time),
                mock.patch.object(smoke.time, "sleep", clock.sleep),
            ]
        with contextlib.ExitStack() as stack:
            for patcher in patchers:
                stack.enter_context(patcher)
            with keep_capture_env(keep_capture):
                with mock.patch.object(sys, "stdout", stdout):
                    try:
                        smoke.main()
                        code = None
                    except SystemExit as exc:  # fail() exits 1
                        code = exc.code
        self.addCleanup(cleanup_module_capture, smoke)
        return smoke, code, host, stdout.getvalue()

    def capture_path(self, printed):
        """The exact capture path the run printed (None when it printed none)."""
        match = CAPTURE_RE.search(printed)
        return match.group(1) if match else None

    def assert_private_capture(self, path):
        """A retained capture must be an exclusive 0600 file inside a 0700
        temp dir that is not the old fixed /tmp path."""
        self.assertIsNotNone(path, "run must print its exact capture path")
        self.assertNotEqual(path, OLD_LOG,
                            "capture must not live at the old fixed /tmp path")
        self.assertTrue(os.path.isfile(path), f"capture missing: {path}")
        self.assertEqual(os.stat(path).st_mode & 0o777, 0o600,
                         "capture file must be 0600")
        parent = os.path.dirname(path)
        self.assertEqual(os.stat(parent).st_mode & 0o777, 0o700,
                         "capture dir must be 0700")

    # --- H-04: never mutate a pre-existing model --------------------------

    def test_pre_existing_target_aborts_before_any_delete_pull_or_tui(self):
        # The fake host already has the target installed on every /api/tags
        # call. Any api_delete, pty open, or Popen is an H-04 violation.
        host = FakeHost(lambda _call: [TEST_MODEL])
        _smoke, code, host, printed = self.run_main(host, refuse_pty=True)

        self.assertEqual(code, 1, "pre-existing target must exit nonzero")
        self.assertIn("already installed", printed)
        self.assertIn("refus", printed)
        self.assertEqual(host.deletes, [], "must not DELETE a pre-existing model")
        # State capture came first: the log must open with /api/tags, and no
        # mutation entry may precede it.
        self.assertEqual(host.log[0], ("tags", [TEST_MODEL]))

    # --- H-04: cleanup only a model this run created ----------------------

    def test_failure_before_creation_cleans_nothing(self):
        # Target never appears (the app fails to boot before the pull). The
        # run created nothing, so cleanup must delete nothing.
        host = FakeHost(lambda _call: [])
        _smoke, code, host, printed = self.run_main(host, wait_for_value=False)

        self.assertEqual(code, 1)
        self.assertIn("did not boot", printed)
        self.assertEqual(host.deletes, [], "nothing was created, so nothing "
                                           "may be cleaned up")
        self.assertEqual(host.log, [("tags", [])],
                         "only the initial state capture should reach the host")

    def test_failure_after_creation_cleans_only_what_the_run_created(self):
        # Target is absent at start, appears after the pull (this run created
        # it), then the delete step never completes — the failure cleanup must
        # delete exactly that model, and only after creation was observed.
        def installed_after_first(call):
            return [TEST_MODEL] if call >= 1 else []

        host = FakeHost(installed_after_first)
        _smoke, code, host, printed = self.run_main(host)

        self.assertEqual(code, 1)
        self.assertEqual(host.deletes, [TEST_MODEL],
                         "failure cleanup deletes exactly the model this run "
                         "created, and only once")
        present_seen = host.log.index(("tags", [TEST_MODEL]))
        self.assertGreater(
            host.log.index(("delete", TEST_MODEL)), present_seen,
            "cleanup delete must happen only after the run observed the model "
            "it created")

    def test_success_creates_then_removes_only_its_own_model(self):
        # Absent at start -> present after pull -> the TUI delete removes it.
        # Full run exits 0 and cleanup stays limited to the created model.
        def lifecycle(call):
            if call == 0:
                return []
            if call == 1:
                return [TEST_MODEL]
            return []

        host = FakeHost(lifecycle)
        _smoke, code, host, printed = self.run_main(host)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assertEqual(host.deletes, [TEST_MODEL],
                         "cleanup touches only the model this run created")
        present_seen = host.log.index(("tags", [TEST_MODEL]))
        self.assertGreater(
            host.log.index(("delete", TEST_MODEL)), present_seen,
            "cleanup delete must follow creation observation")

    # --- M-11: private unique 0600 captures, never the fixed /tmp path -----

    def test_old_fixed_log_path_symlink_is_never_followed(self):
        # A hostile/previous process pre-places a symlink at the old fixed
        # capture path pointing at a canary file. The smoke must never open
        # or modify it (an ordinary open(..., "w") through the symlink would
        # truncate the canary).
        fd, canary = tempfile.mkstemp(prefix="pull-delete-canary-")
        os.write(fd, b"canary-payload")
        os.close(fd)
        try:
            if os.path.lexists(OLD_LOG):  # leftover from an earlier run
                os.unlink(OLD_LOG)
            os.symlink(canary, OLD_LOG)
            host = FakeHost(lambda _call: [])
            _smoke, code, _host, printed = self.run_main(
                host, wait_for_value=False)  # boot failure writes a capture
            self.assertEqual(code, 1)
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

    def test_failure_retains_private_capture_and_prints_exact_path(self):
        host = FakeHost(lambda _call: [])
        smoke, code, host, printed = self.run_main(host, wait_for_value=False)

        self.assertEqual(code, 1)
        self.assertIn("SMOKE FAIL", printed)
        self.assert_private_capture(self.capture_path(printed))
        # The retained capture is discoverable at the printed path.
        self.assertTrue(os.path.exists(self.capture_path(printed)))

    def test_consecutive_runs_get_distinct_capture_dirs(self):
        # Every run must capture into its own private unique temp dir; two
        # runs may never collide on one shared path.
        paths = []
        for _ in range(2):
            host = FakeHost(lambda _call: [])
            smoke, code, _host, printed = self.run_main(
                host, wait_for_value=False)
            self.assertEqual(code, 1)
            path = self.capture_path(printed)
            self.assert_private_capture(path)
            paths.append(path)
        self.assertNotEqual(paths[0], paths[1],
                            "consecutive runs must use distinct capture dirs")

    def test_success_removes_capture_by_default(self):
        # Absent at start -> present after pull -> removed through the TUI.
        def lifecycle(call):
            if call == 0:
                return []
            if call == 1:
                return [TEST_MODEL]
            return []

        host = FakeHost(lifecycle)
        smoke, code, host, printed = self.run_main(host)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assertNotIn("capture:", printed,
                         "a successful run must not advertise a removed "
                         "capture path")
        # Nothing may persist from the run.
        self.assertIsNone(smoke.capture_path,
                          "capture must be removed after a successful run")
        self.assertIsNone(smoke.capture_dir)

    def test_success_keeps_capture_when_env_set(self):
        # Absent at start -> present after pull -> removed through the TUI.
        def lifecycle(call):
            if call == 0:
                return []
            if call == 1:
                return [TEST_MODEL]
            return []

        host = FakeHost(lifecycle)
        smoke, code, host, printed = self.run_main(host, keep_capture=True)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assert_private_capture(self.capture_path(printed))
        # Retained capture actually exists at the printed path.
        self.assertTrue(os.path.exists(smoke.capture_path))


if __name__ == "__main__":
    unittest.main(verbosity=2)
