"""Exercise installation in disposable directories with mocked host tools."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


PROJECT = Path(__file__).resolve().parents[1]


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="p8cec-install-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.output = self.root / "custom bin %"
        self.env = dict(os.environ, PATH=f"{self.tools}:{os.environ['PATH']}",
                        XDG_CONFIG_HOME=str(self.root / "config"),
                        XDG_DATA_HOME=str(self.root / "data"),
                        P8CEC_INSTALL_BIN=str(self.output),
                        P8CEC_SOURCE_DIR=str(PROJECT),
                        P8CEC_TEST_SYSTEMCTL=str(self.root / "systemctl.log"),
                        P8CEC_TEST_BUILD=str(self.root / "build.log"))
        self.tool("systemctl", '''printf '%s\\n' "$*" >> "$P8CEC_TEST_SYSTEMCTL"
if [ "$2" = is-active ]; then exit "${P8CEC_TEST_ACTIVE_STATUS:-3}"; fi
''')
        self.tool("cec-client", "exit 99\n")  # Installation must never execute it.
        self.tool("go", '''pwd > "$P8CEC_TEST_BUILD"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; output="$1"; fi
  shift
done
printf '#!/bin/sh\\nexit 0\\n' > "$output"
chmod +x "$output"
''')

    def tool(self, name, body):
        path = self.tools / name
        path.write_text("#!/bin/sh\nset -eu\n" + body)
        path.chmod(0o755)

    def install(self, script=None):
        return subprocess.run(["bash", str(script or PROJECT / "scripts/install.sh")],
                              cwd=self.root, env=self.env, capture_output=True, text=True,
                              timeout=10)

    def assert_installed_stopped(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((self.output / "pulse-eight-cec").is_file())
        self.assertEqual(os.readlink(self.output / "pulse-eight-cecctl"), "pulse-eight-cec")
        calls = (self.root / "systemctl.log").read_text()
        self.assertIn("--user daemon-reload", calls)
        self.assertNotIn("enable", calls)
        self.assertNotIn("--now", calls)
        unit = (self.root / "config/systemd/user/pulse-eight-cec.service").read_text()
        self.assertIn(f'ExecStart="{str(self.output).replace("%", "%%")}/pulse-eight-cec" daemon', unit)

    def test_source_build_uses_module_directory_and_does_not_start_service(self):
        self.assert_installed_stopped(self.install())
        self.assertEqual((self.root / "build.log").read_text().strip(), str(PROJECT))

    def test_portable_archive_uses_packaged_binary(self):
        archive = self.root / "archive"
        (archive / "scripts").mkdir(parents=True)
        shutil.copy2(PROJECT / "scripts/install.sh", archive / "scripts/install.sh")
        shutil.copytree(PROJECT / "packaging", archive / "packaging")
        (archive / "pulse-eight-cec").write_text("#!/bin/sh\nexit 0\n")
        self.env.pop("P8CEC_SOURCE_DIR")
        self.assert_installed_stopped(self.install(archive / "scripts/install.sh"))
        self.assertFalse((self.root / "build.log").exists())

    def test_running_service_is_left_untouched(self):
        self.env["P8CEC_TEST_ACTIVE_STATUS"] = "0"
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("stop it before installing", result.stderr)
        self.assertFalse(self.output.exists())

    def test_unavailable_user_session_is_not_treated_as_inactive(self):
        self.env["P8CEC_TEST_ACTIVE_STATUS"] = "1"
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("could not inspect the user service", result.stderr)
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
