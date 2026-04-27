#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_INSTALL_BIN:-bin/sis}"
root="${SIS_INSTALL_ROOT:-}"
user="${SIS_INSTALL_USER:-sis}"
group="${SIS_INSTALL_GROUP:-sis}"

if [[ ! -x "${bin}" ]]; then
  echo "install-linux-service: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if [[ -z "${root}" && "${EUID}" -ne 0 ]]; then
  echo "install-linux-service: run as root or set SIS_INSTALL_ROOT for a staged install" >&2
  exit 1
fi

target() {
  printf '%s%s\n' "${root}" "$1"
}

owner_args() {
  if [[ -z "${root}" ]]; then
    printf -- '-o %s -g %s' "$1" "$2"
  fi
}

install_file() {
  local mode="$1"
  local owner="$2"
  local group_name="$3"
  local src="$4"
  local dst="$5"
  # shellcheck disable=SC2086
  install -m "${mode}" $(owner_args "${owner}" "${group_name}") "${src}" "${dst}"
}

install_if_missing() {
  local mode="$1"
  local owner="$2"
  local group_name="$3"
  local src="$4"
  local dst="$5"
  if [[ -e "${dst}" ]]; then
    install_file "${mode}" "${owner}" "${group_name}" "${src}" "${dst}.example"
    echo "install-linux-service: kept existing ${dst}; wrote ${dst}.example"
    return
  fi
  install_file "${mode}" "${owner}" "${group_name}" "${src}" "${dst}"
}

if [[ -z "${root}" ]] && ! id -u "${user}" >/dev/null 2>&1; then
  useradd --system --home /var/lib/sis --shell /usr/sbin/nologin "${user}"
fi

# shellcheck disable=SC2086
install -d $(owner_args root root) "$(target /etc/sis)"
# shellcheck disable=SC2086
install -d $(owner_args "${user}" "${group}") "$(target /var/lib/sis)"
# shellcheck disable=SC2086
install -d $(owner_args root root) "$(target /usr/local/bin)"
# shellcheck disable=SC2086
install -d $(owner_args root root) "$(target /etc/systemd/system)"

install_if_missing 0640 root "${group}" examples/sis.yaml "$(target /etc/sis/sis.yaml)"
install_if_missing 0640 root "${group}" examples/sis.env "$(target /etc/sis/sis.env)"
install_file 0755 root root "${bin}" "$(target /usr/local/bin/sis)"
install_file 0644 root root examples/sis.service "$(target /etc/systemd/system/sis.service)"

"$(target /usr/local/bin/sis)" config check -config "$(target /etc/sis/sis.yaml)"

if [[ -z "${root}" && "${SIS_INSTALL_SKIP_SYSTEMD_RELOAD:-0}" != "1" ]]; then
  systemctl daemon-reload
fi

echo "install-linux-service: installed Sis service files"
