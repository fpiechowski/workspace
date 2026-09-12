#!/usr/bin/env python3
"""Deliver one workspace mailbox message to an existing OpenCode session.

The workspace supervisor invokes this program with:
    opencode-deliver.py THREAD_ID MESSAGE_FILE MESSAGE_ID

OpenCode's ``run --session`` command is synchronous, while the workspace
delivery contract must return quickly.  We therefore start the OpenCode turn
as a detached child and return an acceptance receipt after the process has
survived a short startup window.  A marker and lock make retries idempotent.
"""

from __future__ import annotations

import fcntl
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time


STARTUP_WINDOW_SECONDS = 0.75
POLL_INTERVAL_SECONDS = 0.05


def fail(message: str) -> int:
    print(message, file=sys.stderr)
    return 1


def main() -> int:
    if len(sys.argv) != 4:
        return fail("usage: opencode-deliver.py THREAD_ID MESSAGE_FILE MESSAGE_ID")

    thread_id, message_file, message_id = sys.argv[1:]
    if not thread_id.startswith("ses_"):
        return fail("OpenCode delivery requires a native session ID starting with ses_")

    path = Path(message_file)
    try:
        message = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return fail(f"cannot read workspace message: {exc}")

    if message.get("id") != message_id:
        return fail("message ID argument does not match the message file")
    body = message.get("body")
    if not isinstance(body, str) or not body.strip():
        return fail("workspace message body is empty")

    delivery_dir = path.parent / "opencode"
    try:
        delivery_dir.mkdir(parents=True, exist_ok=True)
        lock_path = delivery_dir / f"{message_id}.lock"
        marker_path = delivery_dir / f"{message_id}.accepted"
        log_path = delivery_dir / f"{message_id}.log"
        with lock_path.open("a+", encoding="utf-8") as lock_file:
            fcntl.flock(lock_file.fileno(), fcntl.LOCK_EX)
            if marker_path.exists():
                print(json.dumps({"accepted": True}))
                return 0

            executable = os.environ.get("OPENCODE_BIN") or shutil.which("opencode")
            if not executable:
                bundled = Path.home() / ".opencode" / "bin" / "opencode"
                if bundled.is_file() and os.access(bundled, os.X_OK):
                    executable = str(bundled)
            if not executable:
                return fail("OpenCode executable not found; set OPENCODE_BIN or add opencode to PATH")
            command = [
                executable,
                "run",
                "--auto",
                "--session",
                thread_id,
                "--format",
                "json",
                "--",
                body,
            ]
            log = log_path.open("ab")
            try:
                process = subprocess.Popen(
                    command,
                    cwd=os.getcwd(),
                    stdin=subprocess.DEVNULL,
                    stdout=log,
                    stderr=log,
                    start_new_session=True,
                    close_fds=True,
                )
            except (OSError, ValueError) as exc:
                log.close()
                return fail(f"cannot start OpenCode delivery: {exc}")

            deadline = time.monotonic() + STARTUP_WINDOW_SECONDS
            while time.monotonic() < deadline:
                status = process.poll()
                if status is not None:
                    log.close()
                    if status != 0:
                        return fail(
                            f"OpenCode delivery exited during startup with status {status}; "
                            f"see {log_path}"
                        )
                    marker_path.write_text(
                        json.dumps({"message_id": message_id, "thread_id": thread_id}),
                        encoding="utf-8",
                    )
                    print(json.dumps({"accepted": True}))
                    return 0
                time.sleep(POLL_INTERVAL_SECONDS)
            log.close()
            marker_path.write_text(
                json.dumps({"message_id": message_id, "thread_id": thread_id}),
                encoding="utf-8",
            )
            print(json.dumps({"accepted": True}))
            return 0
    except OSError as exc:
        return fail(f"cannot prepare OpenCode delivery state: {exc}")


if __name__ == "__main__":
    raise SystemExit(main())
