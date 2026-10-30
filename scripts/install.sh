#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${P8CEC_REPO_URL:-https://github.com/niski84/pulse-eight-cec-linux.git}"
SOURCE_DIR="${P8CEC_SOURCE_DIR:-}"
INSTALL_BIN="${P8CEC_INSTALL_BIN:-$HOME/.local/bin}"
USER_SERVICE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

fail() { printf '✗ %s\n' "$*" >&2; exit 1; }
ok() { printf '✓ %s\n' "$*"; }

[[ "$(uname -s)" == "Linux" ]] || fail "Linux is required"
[[ "$INSTALL_BIN" == /* && "$INSTALL_BIN" != *$'\n'* ]] || fail "P8CEC_INSTALL_BIN must be an absolute, single-line path"
command -v systemctl >/dev/null 2>&1 || fail "systemd user services are required"
command -v cec-client >/dev/null 2>&1 || fail "libCEC cec-client is required; install the distribution's cec-utils package"
if systemctl --user is-active --quiet pulse-eight-cec.service; then
  fail "pulse-eight-cec.service is running; stop it before installing an update"
else
  service_status=$?
  [[ "$service_status" == 3 || "$service_status" == 4 ]] || fail "could not inspect the user service; check the systemd user session"
fi

temporary_dir=""
cleanup() {
  if [[ -n "$temporary_dir" ]]; then rm -rf "$temporary_dir"; fi
}
trap cleanup EXIT

# Archives contain a binary and this installer; checkouts contain go.mod.
# When piped into bash, clone the source instead.
if [[ -z "$SOURCE_DIR" && -f "${BASH_SOURCE[0]:-}" ]]; then
  candidate="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if [[ -f "$candidate/go.mod" || -f "$candidate/pulse-eight-cec" ]]; then
    SOURCE_DIR="$candidate"
  fi
fi
if [[ -z "$SOURCE_DIR" ]]; then
  command -v git >/dev/null 2>&1 || fail "git is required for source installation"
  temporary_dir="$(mktemp -d)"
  SOURCE_DIR="$temporary_dir/source"
  git clone --depth 1 "$REPO_URL" "$SOURCE_DIR"
fi
SOURCE_DIR="$(cd "$SOURCE_DIR" && pwd)"
[[ -f "$SOURCE_DIR/packaging/systemd/user/pulse-eight-cec.service" ]] || fail "missing packaged user service"
mkdir -p "$INSTALL_BIN" "$USER_SERVICE_DIR"

if [[ -f "$SOURCE_DIR/go.mod" ]]; then
  command -v go >/dev/null 2>&1 || fail "Go is required for source installation"
  (cd "$SOURCE_DIR" && go build -trimpath -o "$INSTALL_BIN/pulse-eight-cec" ./cmd/pulse-eight-cec-linux)
elif [[ -f "$SOURCE_DIR/pulse-eight-cec" ]]; then
  if [[ "$SOURCE_DIR/pulse-eight-cec" != "$INSTALL_BIN/pulse-eight-cec" ]]; then
    install -m 0755 "$SOURCE_DIR/pulse-eight-cec" "$INSTALL_BIN/pulse-eight-cec"
  fi
else
  fail "not a source tree or portable release archive: $SOURCE_DIR"
fi
ln -sfn pulse-eight-cec "$INSTALL_BIN/pulse-eight-cecctl"

# Quote custom binary paths using systemd's syntax, including percent specifiers.
unit_binary="$INSTALL_BIN/pulse-eight-cec"
unit_binary="${unit_binary//\\/\\\\}"
unit_binary="${unit_binary//\"/\\\"}"
unit_binary="${unit_binary//%/%%}"
while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" == ExecStart=* ]]; then
    printf 'ExecStart="%s" daemon\n' "$unit_binary"
  else
    printf '%s\n' "$line"
  fi
done < "$SOURCE_DIR/packaging/systemd/user/pulse-eight-cec.service" > "$USER_SERVICE_DIR/pulse-eight-cec.service"
systemctl --user daemon-reload
ok "installed pulse-eight-cec and pulse-eight-cecctl in $INSTALL_BIN"
ok "installed user service; left stopped and did not change boot enablement"

rules_file="$SOURCE_DIR/packaging/udev/70-pulse-eight-cec.rules"
if [[ -n "$temporary_dir" ]]; then
  rules_dir="${XDG_DATA_HOME:-$HOME/.local/share}/pulse-eight-cec"
  mkdir -p "$rules_dir"
  install -m 0644 "$rules_file" "$rules_dir/70-pulse-eight-cec.rules"
  rules_file="$rules_dir/70-pulse-eight-cec.rules"
fi
printf '\nHardware input remains experimental. The installer does not stop other CEC handlers,\nchange permissions, or claim the adapter. Verify permissions and schedule a test first.\n'
printf '\nPermission setup (administrator, if needed):\n  sudo install -m 0644 %q /etc/udev/rules.d/70-pulse-eight-cec.rules\n  sudo udevadm control --reload-rules\n' "$rules_file"
printf '\nWith the current adapter owner stopped for a scheduled test:\n  %q daemon --shadow\n' "$INSTALL_BIN/pulse-eight-cec"
printf '\nAfter physical receive-to-keyboard validation:\n  systemctl --user enable --now pulse-eight-cec.service\n'
