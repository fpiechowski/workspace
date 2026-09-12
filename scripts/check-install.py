"""Smoke-test a built workspace executable in an isolated Git project.

Usage: python3 scripts/check-install.py /absolute/path/to/workspace
No model client, supervisor, network or user configuration is used.
"""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    executable = str(Path(sys.argv[1]).resolve(strict=True))
    env = {k: v for k, v in os.environ.items() if not k.startswith("WORKSPACE_")}
    with tempfile.TemporaryDirectory(prefix="workspace-install-") as directory:
        root = Path(directory)
        responses = []

        def command(*args):
            return subprocess.run(args, cwd=root, env=env, text=True,
                                  capture_output=True, check=True, timeout=30).stdout

        def cli(*args):
            response = json.loads(command(executable, "--json", "--non-interactive", *args))
            assert response["ok"], response
            responses.append(response)
            return response["data"]

        command("git", "init")
        command("git", "-c", "user.name=Workspace Smoke", "-c",
                "user.email=smoke@example.invalid", "commit", "--allow-empty", "-m", "initial")
        cli("project", "init", "--operation-key", "init")
        assert responses[-1]["operation_id"].startswith("op_")
        skill = cli("skill", "install", "--client", "codex", "--operation-key", "skill")
        assert Path(skill["path"]).is_file()
        issue = root / "issue.md"
        issue.write_text("Fix the reproducible checkout error.\n", encoding="utf-8")
        args = ("create", "--input-file", str(issue), "--workflow", "plan-first",
                "--operation-key", "install-smoke")
        created = cli(*args)
        repeated = cli(*args)
        assert responses[-1]["operation_id"] == responses[-2]["operation_id"]
        assert responses[-1]["revision"] == responses[-2]["revision"]
        workspace_id = created["workspace"]["id"]
        assert repeated["workspace"]["id"] == workspace_id
        assert len(cli("list")) == 1
        state = cli("--workspace", workspace_id, "status")
        assert state["workspace"]["workflow"]["phase"] == "planning"
        for name in ("WORKSPACE.md", "AGENTS.md", "WORKFLOW.md", "inputs/issue.md"):
            assert (Path(state["directory"]) / name).is_file(), name
        assert state["agents"][0]["role"] == "orchestrator"
        assert not state["sessions"]
        cli("--workspace", workspace_id, "menu")
        patch = root / "patch.json"
        patch.write_text(json.dumps({"title": "Updated title"}), encoding="utf-8")
        update_args = ("--workspace", workspace_id, "state", "update", "--patch-file",
                       str(patch), "--expected-revision", str(state["workspace"]["revision"]),
                       "--operation-key", "edit")
        updated = cli(*update_args)
        receipt = responses[-1]
        cli("--workspace", workspace_id, "pause", "--operation-key", "pause")
        replayed = cli(*update_args)
        assert replayed == updated
        assert responses[-1] == receipt
        assert cli("--workspace", workspace_id, "status")["workspace"]["status"] == "paused"
        # Discovery also works without an explicit project/workspace selector.
        discovered = subprocess.run([executable, "--json", "status"],
                                    cwd=state["directory"], env=env, text=True,
                                    capture_output=True, check=True, timeout=30)
        assert json.loads(discovered.stdout)["data"]["workspace"]["id"] == workspace_id
    print("PASS: executable, init, skill, create/replay, documents, identities, menu, discovery, mutation replay and receipts")


if __name__ == "__main__":
    main()
