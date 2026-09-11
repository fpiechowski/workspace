"""Offline protocol smoke test. Initializes app-server; never starts a model turn."""
import argparse
import json
import os
import queue
import subprocess
import threading
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument("--client", default="codex")
args = parser.parse_args()
runtime_dir = tempfile.TemporaryDirectory(prefix="workspace-codex-protocol-")
environment = dict(os.environ, CODEX_HOME=runtime_dir.name)
process = subprocess.Popen(
    [args.client, "app-server", "--stdio"],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE,
    text=True, env=environment,
)
lines = queue.Queue()


def read_lines():
    for line in process.stdout:
        lines.put(line)
    lines.put(None)


threading.Thread(target=read_lines, daemon=True).start()
try:
    request = {
        "id": 1, "method": "initialize",
        "params": {"clientInfo": {"name": "workspace_protocol_check", "version": "1.0.0"}},
    }
    process.stdin.write(json.dumps(request) + "\n")
    process.stdin.flush()
    for _ in range(30):
        line = lines.get(timeout=10)
        if line is None:
            raise RuntimeError("app-server exited before initialization")
        response = json.loads(line)
        if response.get("id") != 1:
            continue
        if "error" in response or not isinstance(response.get("result"), dict):
            raise RuntimeError("initialize failed: " + json.dumps(response))
        process.stdin.write(json.dumps({"method": "initialized", "params": {}}) + "\n")
        process.stdin.flush()
        print("PASS: installed Codex app-server accepted initialize; no thread or turn started")
        break
    else:
        raise RuntimeError("initialize response not received")
finally:
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)
    runtime_dir.cleanup()
