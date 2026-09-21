"""Run the installer against a local release fixture only.

The fixture exercises target selection, checksum rejection, unsupported target
messages, and install paths containing spaces without contacting GitHub.
"""

import hashlib
import http.server
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import threading


SCRIPT = Path(__file__).with_name("install.sh").resolve()


def archive_bytes(marker: str) -> bytes:
    with tempfile.TemporaryDirectory(prefix="workspace-install-archive-") as directory:
        path = Path(directory) / "workspace"
        path.write_text(f"#!/bin/sh\nprintf '%s\\n' '{marker}'\n", encoding="utf-8")
        path.chmod(0o755)
        output = Path(directory) / "archive.tar.gz"
        with tarfile.open(output, "w:gz") as archive:
            archive.add(path, arcname="workspace", recursive=False)
        return output.read_bytes()


class Fixture:
    def __init__(self, broken_checksum=False):
        self.archives = {
            "workspace_0.1.0_linux_amd64.tar.gz": archive_bytes("linux-amd64"),
            "workspace_0.1.0_darwin_arm64.tar.gz": archive_bytes("darwin-arm64"),
        }
        self.broken_checksum = broken_checksum
        self.paths = []

    def manifest(self):
        lines = []
        for name, data in sorted(self.archives.items()):
            digest = hashlib.sha256(data).hexdigest()
            if self.broken_checksum and "darwin_arm64" in name:
                digest = "0" * 64
            lines.append(f"{digest}  {name}")
        return ("\n".join(lines) + "\n").encode()


class Handler(http.server.BaseHTTPRequestHandler):
    fixture = None

    def do_GET(self):  # noqa: N802 - stdlib handler API
        self.fixture.paths.append(self.path)
        if self.path == "/checksums.txt":
            body = self.fixture.manifest()
        else:
            name = self.path.lstrip("/")
            body = self.fixture.archives.get(name)
            if body is None:
                self.send_error(404)
                return
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


def run_installer(base: str, directory: Path, os_name: str, arch: str):
    environment = os.environ.copy()
    environment.update(
        WORKSPACE_RELEASE_BASE_URL=base,
        WORKSPACE_INSTALL_DIR=str(directory),
        WORKSPACE_INSTALL_OS=os_name,
        WORKSPACE_INSTALL_ARCH=arch,
    )
    return subprocess.run(
        ["sh", str(SCRIPT)],
        text=True,
        capture_output=True,
        env=environment,
        timeout=30,
    )


def main():
    with tempfile.TemporaryDirectory(prefix="workspace-installer-fixture-") as directory:
        root = Path(directory)
        fixture = Fixture()
        Handler.fixture = fixture
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            install_dir = root / "directory with spaces"
            result = run_installer(
                f"http://127.0.0.1:{server.server_port}",
                install_dir,
                "Darwin",
                "arm64",
            )
            assert result.returncode == 0, result.stderr
            installed = install_dir / "workspace"
            assert installed.is_file()
            assert "darwin-arm64" in installed.read_text(encoding="utf-8")
            assert "v0.1.0" in result.stdout
            assert fixture.paths[-1] == "/workspace_0.1.0_darwin_arm64.tar.gz"

            broken = Fixture(broken_checksum=True)
            Handler.fixture = broken
            broken_dir = root / "broken"
            result = run_installer(
                f"http://127.0.0.1:{server.server_port}",
                broken_dir,
                "Darwin",
                "arm64",
            )
            assert result.returncode != 0
            assert "checksum" in result.stderr.lower()
            assert not (broken_dir / "workspace").exists()

            result = run_installer(
                f"http://127.0.0.1:{server.server_port}",
                root / "unsupported",
                "FreeBSD",
                "amd64",
            )
            assert result.returncode != 0
            assert "supported" in result.stderr.lower()
        finally:
            server.shutdown()
            thread.join(timeout=5)
            server.server_close()
    print("PASS: installer fixture target selection, success, checksum rejection, unsupported target, and space path")


if __name__ == "__main__":
    main()
