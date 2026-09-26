#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${P8CEC_REPO_URL:-https://github.com/niski84/pulse-eight-cec-linux.git}"
SOURCE_DIR="${P8CEC_SOURCE_DIR:-}"
INSTALL_BIN="${P8CEC_INSTALL_BIN:-$HOME/.local/bin}"
USER_SERVICE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

fail() { printf '✗ %s\n' "$*" >&2; exit 1; }
ok() { printf '✓ %s\n' "$*"; }

[[ "$(uname -s)" == "Linux" ]] || fail "Linux is required"
command -v go >/dev/null 2>&1 || fail "Go is required for source installation"
command -v systemctl >/dev/null 2>&1 || fail "systemd user services are required"
command -v cec-client >/dev/null 2>&1 || fail "libCEC cec-client is required; install the distribution's cec-utils package"

temporary_dir=""
cleanup() {
  if [[ -n "$temporary_dir" ]]; then
    rm -rf "$temporary_dir"
  fi
}
trap cleanup EXIT

if [[ -z "$SOURCE_DIR" ]]; then
  command -v git >/dev/null 2>&1 || fail "git is required when P8CEC_SOURCE_DIR is not set"
  temporary_dir="$(mktemp -d)"
  SOURCE_DIR="$temporary_dir/source"
  git clone --depth 1 "$REPO_URL" "$SOURCE_DIR" >/dev/null
fi

[[ -f "$SOURCE_DIR/go.mod" ]] || fail "not a pulse-eight-cec-linux source tree: $SOURCE_DIR"
mkdir -p "$INSTALL_BIN" "$USER_SERVICE_DIR"

go build -trimpath -o "$INSTALL_BIN/pulse-eight-cec" "$SOURCE_DIR/cmd/pulse-eight-cec-linux"
ln -sfn pulse-eight-cec "$INSTALL_BIN/pulse-eight-cecctl"
install -m 0644 "$SOURCE_DIR/packaging/systemd/user/pulse-eight-cec.service" "$USER_SERVICE_DIR/pulse-eight-cec.service"

systemctl --user daemon-reload
systemctl --user enable --now pulse-eight-cec.service
ok "installed pulse-eight-cec in $INSTALL_BIN"
ok "enabled pulse-eight-cec.service"

for group in dialout input; do
  if id -nG "$USER" | tr ' ' '\n' | grep -qx "$group"; then
    ok "user has $group access"
  else
    printf '⚠ user is not in %s; adapter or uinput access may require: sudo usermod -aG %s %s\n' "$group" "$group" "$USER"
  fi
done

udev_rule="$SOURCE_DIR/packaging/udev/70-pulse-eight-cec.rules"
if [[ -w /etc/udev/rules.d || -w /etc/udev ]]; then
  install -m 0644 "$udev_rule" /etc/udev/rules.d/70-pulse-eight-cec.rules
  udevadm control --reload-rules
  udevadm trigger --subsystem-match=tty
  udevadm trigger --subsystem-match=misc
  ok "installed Pulse-Eight udev rule"
else
  printf '⚠ udev rule not installed; run with administrative access or install manually:\n  %s\n' "$udev_rule"
fi

printf '\nInstall complete. Check the adapter with:\n  pulse-eight-cecctl status\n'
