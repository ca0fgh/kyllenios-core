#!/usr/bin/env python3

import argparse
import os
import re
import shutil
import signal
import socket
import subprocess
import sys
import time
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve()
REPO_ROOT = SCRIPT_PATH.parent.parent
FRONTEND_DIR = REPO_ROOT / "frontend"
BACKEND_DIR = REPO_ROOT / "backend"
DEFAULT_APP_DIR = REPO_ROOT / ".hermes-proxy-runtime"
NODE_EXTRA_PATHS = ["/Users/money/.local/node/bin/node", "/opt/homebrew/bin/node", "/usr/local/bin/node"]
PNPM_EXTRA_PATHS = ["/Users/money/.local/node/bin/pnpm", "/opt/homebrew/bin/pnpm", "/usr/local/bin/pnpm"]
GO_EXTRA_PATHS = [
    "/opt/homebrew/bin/go",
    "/usr/local/go/bin/go",
    "/usr/local/bin/go",
    str(Path.home() / "go" / "bin" / "go"),
]
POSTGRES_EXTRA_PATHS = [
    "/opt/homebrew/bin/postgres",
    "/opt/homebrew/bin/pg_ctl",
    "/usr/local/bin/postgres",
    "/usr/local/bin/pg_ctl",
    "/opt/homebrew/opt/postgresql@17/bin/postgres",
    "/opt/homebrew/opt/postgresql@17/bin/pg_ctl",
    "/opt/homebrew/opt/postgresql@16/bin/postgres",
    "/opt/homebrew/opt/postgresql@16/bin/pg_ctl",
    "/opt/homebrew/opt/postgresql@15/bin/postgres",
    "/opt/homebrew/opt/postgresql@15/bin/pg_ctl",
    "/usr/local/opt/postgresql@17/bin/postgres",
    "/usr/local/opt/postgresql@17/bin/pg_ctl",
    "/usr/local/opt/postgresql@16/bin/postgres",
    "/usr/local/opt/postgresql@16/bin/pg_ctl",
    "/usr/local/opt/postgresql@15/bin/postgres",
    "/usr/local/opt/postgresql@15/bin/pg_ctl",
]
REDIS_EXTRA_PATHS = [
    "/opt/homebrew/bin/redis-server",
    "/opt/homebrew/bin/redis-cli",
    "/usr/local/bin/redis-server",
    "/usr/local/bin/redis-cli",
]


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


def find_tool(cli_name: str, env_var: str, override: str, extra_paths: list[str]) -> str:
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

    return ""


def resolve_tool(cli_name: str, env_var: str, override: str, extra_paths: list[str]) -> str:
    tool_path = find_tool(cli_name, env_var, override, extra_paths)
    if tool_path:
        return tool_path

    joined = ", ".join(extra_paths)
    fail(
        f"cannot find `{cli_name}`. Set `{env_var}` or pass `--{cli_name}-bin`. "
        f"Checked PATH and common locations: {joined}"
    )


def bootstrap_runtime_files(app_dir: Path) -> None:
    if not app_dir.exists():
        app_dir.mkdir(parents=True, exist_ok=True)
        print_step(f"created app dir: {app_dir}")


def command_output(command: list[str]) -> str:
    result = subprocess.run(command, check=False, capture_output=True, text=True)
    if result.returncode != 0:
        return ""
    return (result.stdout or result.stderr).strip()


def parse_major_minor(version_output: str) -> tuple[int, int]:
    match = re.search(r"(\d+)\.(\d+)", version_output)
    if not match:
        return (0, 0)
    return (int(match.group(1)), int(match.group(2)))


def missing_dependency_message(
    cli_name: str,
    env_var: str,
    cli_flag: str,
    extra_paths: list[str],
    install_hint: str,
) -> str:
    checked_locations = ", ".join(["PATH", *extra_paths])
    return (
        f"`{cli_name}`: not found. {install_hint}. "
        f"Set `{env_var}` or pass `--{cli_flag}` if it is installed elsewhere. "
        f"Checked {checked_locations}"
    )


def broken_dependency_message(cli_name: str, env_var: str, cli_flag: str, tool_path: str, command_hint: str) -> str:
    return (
        f"`{cli_name}`: found at `{tool_path}` but `{command_hint}` failed. "
        f"Reinstall it or point `{env_var}` / `--{cli_flag}` to a working binary"
    )


def outdated_dependency_message(cli_name: str, detected_version: str, minimum_version: str) -> str:
    return f"`{cli_name}`: version `{detected_version}` is too old. `{minimum_version}` or newer is required"


def missing_file_message(path: Path, hint: str) -> str:
    return f"`{path}`: not found. {hint}"


def frontend_dependencies_installed() -> bool:
    return (FRONTEND_DIR / "node_modules").exists()


def local_runtime_dependency_installed(display_name: str) -> bool:
    candidates: list[tuple[str, list[str]]] = []
    if display_name == "PostgreSQL":
        candidates = [
            ("postgres", POSTGRES_EXTRA_PATHS),
            ("pg_ctl", POSTGRES_EXTRA_PATHS),
            ("psql", []),
        ]
    elif display_name == "Redis":
        candidates = [
            ("redis-server", REDIS_EXTRA_PATHS),
            ("redis-cli", REDIS_EXTRA_PATHS),
        ]

    for cli_name, extra_paths in candidates:
        if find_tool(cli_name, "", "", extra_paths):
            return True
    return False


def read_config_sections(config_path: Path) -> dict[str, dict[str, str]]:
    sections: dict[str, dict[str, str]] = {}
    if not config_path.exists():
        return sections

    current_section = ""
    for raw_line in config_path.read_text(encoding="utf-8").splitlines():
        stripped = raw_line.strip()
        if not stripped or stripped.startswith("#"):
            continue

        if not raw_line.startswith((" ", "\t")):
            current_section = stripped[:-1] if stripped.endswith(":") else ""
            if current_section:
                sections.setdefault(current_section, {})
            continue

        if not current_section or ":" not in stripped:
            continue

        key, value = stripped.split(":", 1)
        sections[current_section][key.strip()] = value.strip().strip("'\"")

    return sections


def read_section_host_port(config_path: Path, section_name: str, default_host: str, default_port: int) -> tuple[str, int]:
    sections = read_config_sections(config_path)
    values = sections.get(section_name, {})
    host = values.get("host", default_host) or default_host
    port = default_port
    if values.get("port"):
        try:
            port = int(values["port"])
        except ValueError as exc:
            raise ValueError(f"invalid port in {config_path} -> {section_name}.port: {values['port']}") from exc
    return host, port


def collect_build_tool_issues(args: argparse.Namespace) -> list[str]:
    issues: list[str] = []

    node_path = find_tool("node", "NODE_BIN", args.node_bin, NODE_EXTRA_PATHS)
    if not node_path:
        issues.append(
            missing_dependency_message(
                "node",
                "NODE_BIN",
                "node-bin",
                NODE_EXTRA_PATHS,
                "Install Node.js 18+ first",
            )
        )
    else:
        node_version = command_output([node_path, "--version"])
        if not node_version:
            issues.append(broken_dependency_message("node", "NODE_BIN", "node-bin", node_path, "`node --version`"))
        elif parse_major_minor(node_version) < (18, 0):
            issues.append(outdated_dependency_message("node", node_version, "Node.js 18"))

    pnpm_path = find_tool("pnpm", "PNPM_BIN", args.pnpm_bin, PNPM_EXTRA_PATHS)
    if not pnpm_path:
        issues.append(
            missing_dependency_message(
                "pnpm",
                "PNPM_BIN",
                "pnpm-bin",
                PNPM_EXTRA_PATHS,
                "Install pnpm first (for example: `npm install -g pnpm` after installing Node.js 18+)",
            )
        )
    elif not command_output([pnpm_path, "--version"]):
        issues.append(broken_dependency_message("pnpm", "PNPM_BIN", "pnpm-bin", pnpm_path, "`pnpm --version`"))

    go_path = find_tool("go", "GO_BIN", args.go_bin, GO_EXTRA_PATHS)
    if not go_path:
        issues.append(
            missing_dependency_message(
                "go",
                "GO_BIN",
                "go-bin",
                GO_EXTRA_PATHS,
                "Install Go 1.21+ first",
            )
        )
    else:
        go_version = command_output([go_path, "version"])
        if not go_version:
            issues.append(broken_dependency_message("go", "GO_BIN", "go-bin", go_path, "`go version`"))
        elif parse_major_minor(go_version) < (1, 21):
            issues.append(outdated_dependency_message("go", go_version, "Go 1.21"))

    if not frontend_dependencies_installed():
        issues.append(
            f"frontend dependencies are not installed in `{FRONTEND_DIR}`. "
            f"Run `pnpm install` in `{FRONTEND_DIR}` first"
        )

    return issues


def collect_runtime_installation_issues() -> list[str]:
    issues: list[str] = []
    if not local_runtime_dependency_installed("PostgreSQL"):
        issues.append(
            "`PostgreSQL`: local installation not detected. Install PostgreSQL 15+ first and ensure `postgres`, `pg_ctl`, or `psql` is available"
        )
    if not local_runtime_dependency_installed("Redis"):
        issues.append(
            "`Redis`: local installation not detected. Install Redis 7+ first and ensure `redis-server` or `redis-cli` is available"
        )

    return issues


def collect_preflight_issues(
    args: argparse.Namespace,
    binary_path: Path,
    config_path: Path,
) -> list[str]:
    issues: list[str] = []

    if args.restart_only and not binary_path.exists():
        issues.append(missing_file_message(binary_path, "Run without `--restart-only` once to build it first"))

    if not config_path.exists():
        if args.restart_only:
            issues.append(
                missing_file_message(
                    config_path,
                    "Run once without `--restart-only` to enter the Setup Wizard, or create the config manually first",
                )
            )

    issues.extend(collect_runtime_installation_issues())

    if args.restart_only:
        return issues

    issues.extend(collect_build_tool_issues(args))
    return issues


def ensure_preflight_ready(
    args: argparse.Namespace,
    binary_path: Path,
    config_path: Path,
) -> None:
    missing = collect_preflight_issues(args, binary_path, config_path)
    if not missing:
        return

    fail("missing required dependencies before restart:\n  - " + "\n  - ".join(missing))


def run_command(command: list[str], cwd: Path) -> None:
    print_step(f"run: {' '.join(command)} (cwd={cwd})")
    subprocess.run(command, cwd=str(cwd), check=True)


def read_server_config(config_path: Path) -> tuple[str, int]:
    try:
        return read_section_host_port(config_path, "server", "127.0.0.1", 8080)
    except ValueError as exc:
        fail(str(exc))


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
    parser = argparse.ArgumentParser(description="Rebuild and restart the local hermes-proxy runtime binary.")
    parser.add_argument(
        "--restart-only",
        action="store_true",
        help="skip frontend/backend builds and only restart the current runtime binary",
    )
    parser.add_argument(
        "--app-dir",
        help="runtime state directory for the built binary, config.yaml, and logs (default: .hermes-proxy-runtime)",
    )
    parser.add_argument("--go-bin", help="path to the Go executable")
    parser.add_argument("--node-bin", help="path to the Node.js executable")
    parser.add_argument("--pnpm-bin", help="path to the pnpm executable")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    app_dir = resolve_app_dir(args.app_dir)
    binary_path = app_dir / "hermes-proxy"
    log_path = app_dir / "data" / "hermes-proxy.stdout.log"
    config_path = app_dir / "config.yaml"

    bootstrap_runtime_files(app_dir)
    ensure_preflight_ready(args, binary_path, config_path)

    print_step(f"using app dir: {app_dir}")

    host, port = read_server_config(config_path)
    print_step(f"runtime config: host={host} port={port}")

    if not args.restart_only:
        pnpm_bin = resolve_tool(
            "pnpm",
            "PNPM_BIN",
            args.pnpm_bin,
            PNPM_EXTRA_PATHS,
        )
        go_bin = resolve_tool(
            "go",
            "GO_BIN",
            args.go_bin,
            GO_EXTRA_PATHS,
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
