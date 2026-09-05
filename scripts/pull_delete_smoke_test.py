#!/usr/bin/env python3
"""Unit tests for scripts/pull-delete-smoke.py (H-04 non-destructive smoke).

Fake-host tests drive the script with the Ollama HTTP surface (/api/tags and
/api/delete) and the pty/TUI machinery replaced by fakes, so no live host and
no pty are ever touched. They pin the H-04 contract:

  * initial host state is captured BEFORE any mutation;
  * an already-installed target aborts with a clear nonzero result and no
    DELETE, pull, or TUI action (never restore by re-pulling a tag);
  * cleanup deletes a model only after this run provably created it —
    including failure cleanup — and never a pre-existing one.

Only the Python standard library is used. Run from the repository root:

    python3 -m unittest -v scripts/pull_delete_smoke_test.py
"""
import contextlib
import importlib.util
import io
import os
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
SMOKE_PATH = os.path.join(HERE, "pull-delete-smoke.py")
TEST_MODEL = "h04:fake-target"


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


class SmokeSafetyTest(unittest.TestCase):
    def setUp(self):
        self.smoke = load_smoke()
        self.stdout = io.StringIO()
        fd, self.log_path = tempfile.mkstemp(prefix="pull-delete-smoke-test-")
        os.close(fd)

    def tearDown(self):
        if os.path.exists(self.log_path):
            os.unlink(self.log_path)

    def run_main(self, host, wait_for_value=True, refuse_pty=False):
        """Run smoke.main() with every host + pty surface faked.

        Returns (exit_code_or_None, host, printed_text). A SystemExit is
        caught and its code returned; a normal return yields None.
        """
        patchers = [
            mock.patch.object(self.smoke, "tags", host.tags),
            mock.patch.object(self.smoke, "api_delete", host.delete),
            mock.patch.object(self.smoke, "MODEL", TEST_MODEL),
            mock.patch.object(self.smoke, "LOG", self.log_path),
            mock.patch.object(self.smoke, "wait_for", return_value=wait_for_value),
            mock.patch.object(self.smoke, "pump"),
        ]
        if refuse_pty:
            def never(*_a, **_k):
                raise AssertionError(
                    "pty/TUI machinery reached though the run must abort before "
                    "any pull or TUI action")
            patchers += [
                mock.patch.object(self.smoke.pty, "openpty", side_effect=never),
                mock.patch.object(self.smoke.subprocess, "Popen", side_effect=never),
            ]
        else:
            clock = FakeClock()
            proc = FakeProc()
            self.proc = proc
            patchers += [
                mock.patch.object(self.smoke.pty, "openpty", return_value=(1234, 1235)),
                mock.patch.object(self.smoke.fcntl, "ioctl"),
                mock.patch.object(self.smoke.os, "write"),
                mock.patch.object(self.smoke.os, "read", return_value=b""),
                mock.patch.object(self.smoke.os, "close"),
                mock.patch.object(self.smoke.subprocess, "Popen", return_value=proc),
                mock.patch.object(self.smoke.time, "time", clock.time),
                mock.patch.object(self.smoke.time, "sleep", clock.sleep),
            ]
        with contextlib.ExitStack() as stack:
            for patcher in patchers:
                stack.enter_context(patcher)
            with mock.patch.object(sys, "stdout", self.stdout):
                try:
                    self.smoke.main()
                    code = None
                except SystemExit as exc:  # fail() exits 1
                    code = exc.code
        return code, host, self.stdout.getvalue()

    # --- H-04: never mutate a pre-existing model --------------------------

    def test_pre_existing_target_aborts_before_any_delete_pull_or_tui(self):
        # The fake host already has the target installed on every /api/tags
        # call. Any api_delete, pty open, or Popen is an H-04 violation.
        host = FakeHost(lambda _call: [TEST_MODEL])
        code, host, printed = self.run_main(host, refuse_pty=True)

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
        code, host, printed = self.run_main(host, wait_for_value=False)

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
        code, host, printed = self.run_main(host)

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
        code, host, printed = self.run_main(host)

        self.assertIsNone(code, "full run must exit 0 without SystemExit")
        self.assertIn("SMOKE PASS", printed)
        self.assertEqual(host.deletes, [TEST_MODEL],
                         "cleanup touches only the model this run created")
        present_seen = host.log.index(("tags", [TEST_MODEL]))
        self.assertGreater(
            host.log.index(("delete", TEST_MODEL)), present_seen,
            "cleanup delete must follow creation observation")


if __name__ == "__main__":
    unittest.main(verbosity=2)
