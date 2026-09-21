"""Exercise the local development setup in isolated temporary directories."""

import json
import os
from pathlib import Path
import subprocess
import tempfile


SCRIPT = Path(__file__).with_name("setup-dev.sh").resolve()


def run_setup(environment, cwd):
    return subprocess.run(
        ["sh", str(SCRIPT)],
        cwd=cwd,
        env=environment,
        text=True,
        capture_output=True,
        timeout=180,
    )


def isolated_environment(build_dir, install_dir):
    environment = {key: value for key, value in os.environ.items()
                   if not key.startswith("WORKSPACE_")}
    environment.pop("GOOS", None)
    environment.pop("GOARCH", None)
    environment.pop("CGO_ENABLED", None)
    environment.update(
        WORKSPACE_DEV_BUILD_DIR=str(build_dir),
        WORKSPACE_DEV_INSTALL_DIR=str(install_dir),
    )
    return environment


def assert_success(result):
    assert result.returncode == 0, f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"


def run_installed(installed, environment, *args):
    return subprocess.run(
        [str(installed), *args],
        cwd=installed.parent.parent,
        env=environment,
        text=True,
        capture_output=True,
        check=True,
        timeout=30,
    )


def main():
    with tempfile.TemporaryDirectory(prefix="workspace-dev-setup-") as directory:
        root = Path(directory)
        outside = root / "outside working directory"
        outside.mkdir()
        build_dir = root / "build directory with spaces"
        install_dir = root / "install directory with spaces"
        environment = isolated_environment(build_dir, install_dir)

        first = run_setup(environment, outside)
        assert_success(first)
        artifact = build_dir / "workspace"
        installed = install_dir / "workspace"
        assert artifact.is_file()
        assert installed.is_symlink()
        assert os.readlink(installed) == str(artifact.resolve())
        assert Path(os.path.realpath(installed)) == artifact.resolve()
        assert str(artifact) in first.stdout
        assert str(installed) in first.stdout
        assert "PATH hint:" in first.stdout

        version = run_installed(installed, environment, "version")
        assert "dev" in version.stdout, version.stdout
        version_json = json.loads(run_installed(installed, environment, "--json", "version").stdout)
        assert version_json["ok"]
        assert version_json["data"]["version"] == "dev"

        path_environment = environment.copy()
        path_environment["PATH"] = f"{install_dir}{os.pathsep}{path_environment['PATH']}"
        path_setup = run_setup(path_environment, outside)
        assert_success(path_setup)
        assert "PATH hint:" not in path_setup.stdout
        path_check = subprocess.run(
            ["sh", "-c", "command -v workspace && workspace version"],
            cwd=outside,
            env=path_environment,
            text=True,
            capture_output=True,
            check=True,
            timeout=30,
        )
        assert path_check.stdout.splitlines()[0] == str(installed)
        assert "dev" in path_check.stdout

        first_inode = artifact.stat().st_ino
        artifact.write_bytes(b"stale development artifact\n")
        second = run_setup(environment, outside)
        assert_success(second)
        assert artifact.stat().st_ino != first_inode
        assert "dev" in run_installed(installed, environment, "version").stdout

        previous_bytes = artifact.read_bytes()
        previous_inode = artifact.stat().st_ino
        fake_go_dir = root / "fake go"
        fake_go_dir.mkdir()
        fake_go = fake_go_dir / "go"
        fake_go.write_text(
            "#!/bin/sh\n"
            "case \"$1\" in\n"
            "  version) printf '%s\\n' 'go version go1.24.0 linux/amd64' ;;\n"
            "  env) printf '%s\\n' linux ;;\n"
            "  *) printf '%s\\n' 'intentional development build failure' >&2; exit 42 ;;\n"
            "esac\n",
            encoding="utf-8",
        )
        fake_go.chmod(0o755)
        failed_environment = environment.copy()
        failed_environment["PATH"] = f"{fake_go_dir}{os.pathsep}{environment['PATH']}"
        failed = run_setup(failed_environment, outside)
        assert failed.returncode != 0
        assert "development build failed" in failed.stderr
        assert artifact.read_bytes() == previous_bytes
        assert artifact.stat().st_ino == previous_inode
        assert "dev" in run_installed(installed, environment, "version").stdout

        file_install_dir = root / "file conflict install"
        file_install_dir.mkdir()
        existing_file = file_install_dir / "workspace"
        existing_file.write_text("unrelated file\n", encoding="utf-8")
        file_environment = isolated_environment(build_dir, file_install_dir)
        file_result = run_setup(file_environment, outside)
        assert file_result.returncode != 0
        assert "refusing to replace existing file" in file_result.stderr
        assert existing_file.read_text(encoding="utf-8") == "unrelated file\n"

        symlink_install_dir = root / "symlink conflict install"
        symlink_install_dir.mkdir()
        unrelated_target = root / "unrelated target"
        unrelated_target.write_text("unrelated target\n", encoding="utf-8")
        unrelated_link = symlink_install_dir / "workspace"
        unrelated_link.symlink_to(unrelated_target)
        symlink_environment = isolated_environment(build_dir, symlink_install_dir)
        symlink_result = run_setup(symlink_environment, outside)
        assert symlink_result.returncode != 0
        assert "refusing to replace unrelated symlink" in symlink_result.stderr
        assert unrelated_link.is_symlink()
        assert os.readlink(unrelated_link) == str(unrelated_target)

    print("PASS: development build, version invocation, stable symlink, idempotent refresh, failure preservation, space paths, and conflict protection")


if __name__ == "__main__":
    main()
