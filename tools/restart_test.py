import argparse
import contextlib
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path
from unittest import mock


RESTART_PATH = Path(__file__).resolve().parent / "restart.py"
SPEC = importlib.util.spec_from_file_location("restart", RESTART_PATH)
restart = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(restart)


def make_args(*, restart_only: bool = False, app_dir=None, go_bin=None, node_bin=None, pnpm_bin=None):
    return argparse.Namespace(
        restart_only=restart_only,
        app_dir=app_dir,
        go_bin=go_bin,
        node_bin=node_bin,
        pnpm_bin=pnpm_bin,
    )


class CollectMissingDependenciesTest(unittest.TestCase):
    def test_build_mode_reports_all_missing_dependencies_before_start(self):
        args = make_args()

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir)
            binary_path = app_dir / "kyllenios-core"
            config_path = app_dir / "config.yaml"

            with mock.patch.object(restart, "find_tool", return_value="") as find_tool:
                with mock.patch.object(restart, "frontend_dependencies_installed", return_value=False):
                    with mock.patch.object(restart, "local_runtime_dependency_installed", return_value=False):
                        issues = restart.collect_preflight_issues(args, binary_path, config_path)

        self.assertEqual(3, find_tool.call_count)
        joined = "\n".join(issues)
        self.assertIn("`node`", joined)
        self.assertIn("`pnpm`", joined)
        self.assertIn("`go`", joined)
        self.assertIn("PostgreSQL", joined)
        self.assertIn("Redis", joined)
        self.assertIn("frontend dependencies", joined)
        self.assertNotIn("config.yaml", joined)

    def test_restart_only_skips_build_tool_checks_but_requires_runtime_files(self):
        args = make_args(restart_only=True)

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir)
            binary_path = app_dir / "kyllenios-core"
            config_path = app_dir / "config.yaml"

            with mock.patch.object(restart, "find_tool") as find_tool:
                with mock.patch.object(restart, "frontend_dependencies_installed") as frontend_dependencies_installed:
                    with mock.patch.object(restart, "local_runtime_dependency_installed", return_value=False):
                        issues = restart.collect_preflight_issues(args, binary_path, config_path)

        joined = "\n".join(issues)
        self.assertIn("kyllenios-core`: not found", joined)
        self.assertIn("config.yaml", joined)
        self.assertIn("PostgreSQL", joined)
        self.assertIn("Redis", joined)
        find_tool.assert_not_called()
        frontend_dependencies_installed.assert_not_called()

    def test_runtime_checks_report_postgres_and_redis_installation_before_start(self):
        args = make_args(restart_only=True)

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir)
            binary_path = app_dir / "kyllenios-core"
            binary_path.write_text("", encoding="utf-8")
            config_path = app_dir / "config.yaml"
            config_path.write_text(
                "\n".join(
                    [
                        "database:",
                        "    host: 127.0.0.1",
                        "    port: 5432",
                        "redis:",
                        "    host: 127.0.0.1",
                        "    port: 6379",
                    ]
                ),
                encoding="utf-8",
            )

            with mock.patch.object(restart, "local_runtime_dependency_installed", return_value=False):
                issues = restart.collect_preflight_issues(args, binary_path, config_path)

        joined = "\n".join(issues)
        self.assertIn("PostgreSQL", joined)
        self.assertIn("Redis", joined)

    def test_runtime_checks_use_installation_hints(self):
        args = make_args(restart_only=True)

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir)
            binary_path = app_dir / "kyllenios-core"
            binary_path.write_text("", encoding="utf-8")
            config_path = app_dir / "config.yaml"
            config_path.write_text("server:\n    port: 8080\n", encoding="utf-8")

            with mock.patch.object(restart, "local_runtime_dependency_installed", return_value=False):
                issues = restart.collect_preflight_issues(args, binary_path, config_path)

        joined = "\n".join(issues)
        self.assertIn("Install PostgreSQL 15+ first", joined)
        self.assertIn("Install Redis 7+ first", joined)


class BootstrapRuntimeFilesTest(unittest.TestCase):
    def test_bootstrap_runtime_files_creates_app_dir_without_forcing_config(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir) / ".kyllenios-core-runtime"
            config_path = app_dir / "config.yaml"

            restart.bootstrap_runtime_files(app_dir)

            self.assertTrue(app_dir.exists())
            self.assertFalse(config_path.exists())

    def test_collect_preflight_issues_allows_first_run_without_config_in_build_mode(self):
        args = make_args()

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir) / ".kyllenios-core-runtime"
            binary_path = app_dir / "kyllenios-core"
            config_path = app_dir / "config.yaml"

            restart.bootstrap_runtime_files(app_dir)
            with mock.patch.object(restart, "find_tool", return_value="/mock/tool"):
                with mock.patch.object(
                    restart,
                    "command_output",
                    side_effect=["v18.0.0", "9.0.0", "go version go1.21.5 darwin/arm64"],
                ):
                    with mock.patch.object(restart, "frontend_dependencies_installed", return_value=True):
                        with mock.patch.object(restart, "local_runtime_dependency_installed", return_value=True):
                            issues = restart.collect_preflight_issues(args, binary_path, config_path)

        self.assertEqual([], issues)


class EnsurePreflightReadyTest(unittest.TestCase):
    def test_ensure_preflight_ready_fails_with_combined_hint(self):
        args = make_args()

        with tempfile.TemporaryDirectory() as tmpdir:
            app_dir = Path(tmpdir)
            binary_path = app_dir / "kyllenios-core"
            config_path = app_dir / "config.yaml"

            with mock.patch.object(
                restart,
                "collect_preflight_issues",
                return_value=["`pnpm`: missing", "`go`: missing"],
            ):
                stderr = io.StringIO()
                with contextlib.redirect_stderr(stderr):
                    with self.assertRaises(SystemExit) as exc:
                        restart.ensure_preflight_ready(args, binary_path, config_path)

        self.assertEqual(1, exc.exception.code)
        self.assertIn("missing required dependencies before restart", stderr.getvalue())
        self.assertIn("`pnpm`: missing", stderr.getvalue())
        self.assertIn("`go`: missing", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
