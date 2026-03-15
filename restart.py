#!/usr/bin/env python3

import argparse
import os
import shutil
import signal
import socket
import subprocess
import sys
import time
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve()
REPO_ROOT = SCRIPT_PATH.parent
FRONTEND_DIR = REPO_ROOT / "frontend"
BACKEND_DIR = REPO_ROOT / "backend"
DEFAULT_APP_DIR = REPO_ROOT / ".kyllenios-core-runtime"


def print_step(message: str) -> None:
    print(f"[restart] {message}")


def fail(message: str) -> "NoReturn":
    print(f"[restart] ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def resolve_app_dir(override: str) -> Path:
    if override:
        return Path(override).expanduser().resolve()

    if os.environ.get("HERMES_APP_DIR"):
        return Path(os.environ["HERMES_APP_DIR"]).expanduser().resolve()

    return DEFAULT_APP_DIR


def resolve_tool(cli_name: str, env_var: str, override: str, extra_paths: list) -> str:
    candidates: list = []
    if override:
        candidates.append(override)
    if os.environ.get(env_var):
        candidates.append(os.environ[env_var])
    if shutil.which(cli_name):
        candidates.append(shutil.which(cli_name) or "")
    candidates.extend(extra_paths)

    for candidate in candidates:
        if not candidate:
            continue
        path = Path(candidate).expanduser()
        if path.exists() and os.access(path, os.X_OK):
            return str(path)

    joined = ", ".join(extra_paths)
    fail(
        f"cannot find `{cli_name}`. Set `{env_var}` or pass `--{cli_name}-bin`. "
        f"Checked PATH and common locations: {joined}"
    )


def run_command(command: list[str], cwd: Path) -> None:
    print_step(f"run: {' '.join(command)} (cwd={cwd})")
    subprocess.run(command, cwd=str(cwd), check=True)


def read_server_config(config_path: Path) -> tuple[str, int]:
    host = "127.0.0.1"
    port = 8080
    if not config_path.exists():
        return host, port

    in_server_block = False
    for raw_line in config_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.rstrip()
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue

        if not raw_line.startswith((" ", "\t")):
            in_server_block = stripped == "server:"
            continue

        if not in_server_block:
            continue

        if stripped.startswith("host:"):
            host = stripped.split(":", 1)[1].strip().strip("'\"") or host
        elif stripped.startswith("port:"):
            value = stripped.split(":", 1)[1].strip().strip("'\"")
            try:
                port = int(value)
            except ValueError:
                fail(f"invalid port in {config_path}: {value}")

    return host, port


def listening_pids(port: int) -> list[int]:
    lsof = shutil.which("lsof")
    if not lsof:
        return []
    result = subprocess.run(
        [lsof, "-t", f"-iTCP:{port}", "-sTCP:LISTEN"],
        check=False,
        capture_output=True,
        text=True,
    )
    pids: list[int] = []
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            pids.append(int(line))
        except ValueError:
            continue
    return pids


def stop_existing_processes(port: int) -> None:
    pids = listening_pids(port)
    if not pids:
        print_step(f"no listening process found on port {port}")
        return

    print_step(f"stopping existing process(es) on port {port}: {', '.join(map(str, pids))}")
    for pid in pids:
        os.kill(pid, signal.SIGTERM)

    deadline = time.time() + 10
    while time.time() < deadline:
        remaining = [pid for pid in pids if process_exists(pid)]
        if not remaining:
            return
        time.sleep(0.2)

    remaining = [pid for pid in pids if process_exists(pid)]
    if remaining:
        print_step(f"force killing remaining process(es): {', '.join(map(str, remaining))}")
        for pid in remaining:
            os.kill(pid, signal.SIGKILL)


def process_exists(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    return True


def probe_host(host: str) -> str:
    return "127.0.0.1" if host in {"0.0.0.0", "", "::"} else host


def wait_until_listening(host: str, port: int, timeout_seconds: float = 15) -> None:
    target = probe_host(host)
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.settimeout(0.5)
            try:
                sock.connect((target, port))
                return
            except OSError:
                time.sleep(0.3)
    fail(f"new process did not start listening on {target}:{port} within {timeout_seconds} seconds")


def start_process(app_dir: Path, binary_path: Path, log_path: Path) -> int:
    log_path.parent.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["DATA_DIR"] = str(app_dir)

    print_step(f"starting {binary_path} (data_dir={app_dir}, log={log_path})")
    with log_path.open("ab") as log_file:
        process = subprocess.Popen(
            [str(binary_path)],
            cwd=str(app_dir),
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=log_file,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    return process.pid


def build_frontend(pnpm_bin: str) -> None:
    run_command([pnpm_bin, "build"], FRONTEND_DIR)


def build_backend(go_bin: str, binary_path: Path) -> None:
    binary_path.parent.mkdir(parents=True, exist_ok=True)
    run_command([go_bin, "build", "-tags", "embed", "-o", str(binary_path), "./cmd/server"], BACKEND_DIR)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Rebuild and restart the local kyllenios-core runtime binary.")
    parser.add_argument(
        "--restart-only",
        action="store_true",
        help="skip frontend/backend builds and only restart the current runtime binary",
    )
    parser.add_argument(
        "--app-dir",
        help="runtime state directory for the built binary, config.yaml, and logs (default: .kyllenios-core-runtime)",
    )
    parser.add_argument("--go-bin", help="path to the Go executable")
    parser.add_argument("--pnpm-bin", help="path to the pnpm executable")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    app_dir = resolve_app_dir(args.app_dir)
    binary_path = app_dir / "kyllenios-core"
    log_path = app_dir / "data" / "kyllenios-core.stdout.log"
    config_path = app_dir / "config.yaml"

    if not app_dir.exists():
        app_dir.mkdir(parents=True, exist_ok=True)
    print_step(f"using app dir: {app_dir}")

    host, port = read_server_config(config_path)
    print_step(f"runtime config: host={host} port={port}")

    if not args.restart_only:
        pnpm_bin = resolve_tool(
            "pnpm",
            "PNPM_BIN",
            args.pnpm_bin,
            ["/Users/money/.local/node/bin/pnpm", "/opt/homebrew/bin/pnpm", "/usr/local/bin/pnpm"],
        )
        go_bin = resolve_tool(
            "go",
            "GO_BIN",
            args.go_bin,
            ["/opt/homebrew/bin/go", "/usr/local/go/bin/go", "/usr/local/bin/go", str(Path.home() / "go" / "bin" / "go")],
        )
        build_frontend(pnpm_bin)
        build_backend(go_bin, binary_path)
    elif not binary_path.exists():
        fail(f"binary not found: {binary_path}")

    stop_existing_processes(port)
    pid = start_process(app_dir, binary_path, log_path)
    wait_until_listening(host, port)
    print_step(f"done: pid={pid}, binary={binary_path}, url=http://{probe_host(host)}:{port}")


if __name__ == "__main__":
    main()
