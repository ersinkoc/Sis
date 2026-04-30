#!/usr/bin/env bash
set -euo pipefail

repo="${SIS_INSTALL_REPO:-ersinkoc/Sis}"
github_api="${SIS_INSTALL_GITHUB_API:-https://api.github.com}"
github_raw="${SIS_INSTALL_GITHUB_RAW:-https://raw.githubusercontent.com}"
download_base="${SIS_INSTALL_RELEASE_BASE_URL:-https://github.com/${repo}/releases/download}"
version="${SIS_INSTALL_VERSION:-${1:-latest}}"
work_dir="${SIS_INSTALL_WORK_DIR:-}"
install_user="${SIS_INSTALL_USER:-sis}"
install_group="${SIS_INSTALL_GROUP:-sis}"
service_name="${SIS_INSTALL_SERVICE:-sis}"
config_dir="${SIS_INSTALL_CONFIG_DIR:-/etc/sis}"
data_dir_default="${SIS_INSTALL_DATA_DIR:-/var/lib/sis}"
bin_dir="${SIS_INSTALL_BIN_DIR:-/usr/local/bin}"
systemd_dir="${SIS_INSTALL_SYSTEMD_DIR:-/etc/systemd/system}"
noninteractive="${SIS_INSTALL_NONINTERACTIVE:-0}"

usage() {
  cat <<'EOF'
Usage:
  sudo ./install.sh [latest|vMAJOR.MINOR.PATCH]

Environment overrides:
  SIS_INSTALL_VERSION=v0.1.2
  SIS_INSTALL_NONINTERACTIVE=1
  SIS_DNS_LISTEN=0.0.0.0:53,[::]:53
  SIS_HTTP_LISTEN=127.0.0.1:8080
  SIS_STORE_BACKEND=sqlite
  SIS_PRIVACY_LOG_MODE=hashed
  SIS_INSTALL_ENABLE_SERVICE=1
  SIS_INSTALL_START_SERVICE=1
EOF
}

die() {
  echo "install.sh: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

is_tty() {
  [[ "${noninteractive}" != "1" && -t 0 && -t 1 ]]
}

prompt_value() {
  local prompt="$1"
  local default="$2"
  local answer
  if ! is_tty; then
    printf '%s\n' "${default}"
    return
  fi
  read -r -p "${prompt} [${default}]: " answer
  printf '%s\n' "${answer:-${default}}"
}

prompt_yes_no() {
  local prompt="$1"
  local default="$2"
  local answer suffix
  if ! is_tty; then
    printf '%s\n' "${default}"
    return
  fi
  if [[ "${default}" == "yes" ]]; then
    suffix="Y/n"
  else
    suffix="y/N"
  fi
  while true; do
    read -r -p "${prompt} [${suffix}]: " answer
    answer="${answer:-${default}}"
    case "${answer}" in
      y | Y | yes | YES) printf 'yes\n'; return ;;
      n | N | no | NO) printf 'no\n'; return ;;
      *) echo "Please answer yes or no." ;;
    esac
  done
}

prompt_choice() {
  local prompt="$1"
  local default="$2"
  shift 2
  local answer option
  if ! is_tty; then
    printf '%s\n' "${default}"
    return
  fi
  echo "${prompt}"
  for option in "$@"; do
    echo "  ${option}"
  done
  while true; do
    read -r -p "Choice [${default}]: " answer
    answer="${answer:-${default}}"
    for option in "$@"; do
      if [[ "${answer}" == "${option%%)*}" ]]; then
        printf '%s\n' "${answer}"
        return
      fi
    done
    echo "Please choose one of: $*"
  done
}

shell_quote() {
  local value="$1"
  printf "'%s'" "${value//\'/\'\\\'\'}"
}

target_arch_asset() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'sis_linux_amd64\n' ;;
    aarch64 | arm64) printf 'sis_linux_arm64\n' ;;
    *) die "unsupported Linux architecture: $(uname -m)" ;;
  esac
}

latest_version() {
  local body tag
  body="$(curl --fail --location --show-error --silent "${github_api}/repos/${repo}/releases/latest")"
  tag="$(printf '%s\n' "${body}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
  [[ -n "${tag}" ]] || die "could not resolve latest release for ${repo}"
  printf '%s\n' "${tag}"
}

download() {
  local url="$1"
  local out="$2"
  echo "install.sh: downloading ${url}"
  curl --fail --location --show-error --silent --output "${out}" "${url}"
}

verify_release_bundle() {
  local dir="$1"
  (
    cd "${dir}"
    sha256sum -c SHA256SUMS
    grep -q '"spdxVersion": "SPDX-2.3"' sis.spdx.json
    grep -q '"name": "sis"' sis.spdx.json
    if [[ -f SHA256SUMS.asc && -f release-signing-public-key.asc ]]; then
      if command -v gpg >/dev/null 2>&1; then
        verify_home="$(mktemp -d)"
        cleanup_verify_home() {
          rm -rf "${verify_home}"
        }
        trap cleanup_verify_home EXIT
        export GNUPGHOME="${verify_home}"
        chmod 700 "${GNUPGHOME}"
        gpg --batch --import release-signing-public-key.asc >/dev/null
        gpg --batch --verify SHA256SUMS.asc SHA256SUMS
      else
        echo "install.sh: gpg not found; skipped checksum signature verification" >&2
      fi
    fi
  )
  echo "install.sh: release artifacts verified"
}

install_file_keep_existing() {
  local mode="$1"
  local owner="$2"
  local group_name="$3"
  local src="$4"
  local dst="$5"
  if [[ -e "${dst}" ]]; then
    install -m "${mode}" -o "${owner}" -g "${group_name}" "${src}" "${dst}.example"
    echo "install.sh: kept existing ${dst}; wrote ${dst}.example"
    return
  fi
  install -m "${mode}" -o "${owner}" -g "${group_name}" "${src}" "${dst}"
}

write_managed_env() {
  local env_file="$1"
  local tmp_file backup
  tmp_file="$(mktemp)"
  backup="${env_file}.bak.$(date -u +%Y%m%dT%H%M%SZ)"

  if [[ -f "${env_file}" ]]; then
    cp "${env_file}" "${backup}"
    awk '
      $0 == "# sis-install: begin managed settings" {skip=1; next}
      $0 == "# sis-install: end managed settings" {skip=0; next}
      skip != 1 {print}
    ' "${env_file}" > "${tmp_file}"
  fi

  {
    cat "${tmp_file}"
    [[ ! -s "${tmp_file}" ]] || printf '\n'
    echo "# sis-install: begin managed settings"
    printf 'SIS_DNS_LISTEN=%s\n' "$(shell_quote "${dns_listen}")"
    printf 'SIS_HTTP_LISTEN=%s\n' "$(shell_quote "${http_listen}")"
    printf 'SIS_DATA_DIR=%s\n' "$(shell_quote "${data_dir}")"
    printf 'SIS_STORE_BACKEND=%s\n' "$(shell_quote "${store_backend}")"
    printf 'SIS_PRIVACY_LOG_MODE=%s\n' "$(shell_quote "${privacy_log_mode}")"
    echo "# sis-install: end managed settings"
  } > "${tmp_file}.new"

  install -m 0640 -o root -g "${install_group}" "${tmp_file}.new" "${env_file}"
  rm -f "${tmp_file}" "${tmp_file}.new"
  [[ ! -f "${backup}" ]] || echo "install.sh: backed up previous env to ${backup}"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

[[ "$(uname -s)" == "Linux" ]] || die "this installer currently supports Linux only"
[[ "${EUID}" -eq 0 ]] || die "run as root, for example: curl -fsSL https://raw.githubusercontent.com/${repo}/main/install.sh | sudo bash"

need_cmd curl
need_cmd install
need_cmd sha256sum
need_cmd sed
need_cmd awk

if [[ "${version}" == "latest" ]]; then
  version="$(latest_version)"
fi
[[ "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "invalid version: ${version}"

linux_asset="$(target_arch_asset)"
if [[ -z "${work_dir}" ]]; then
  work_dir="$(mktemp -d)"
  cleanup_work_dir() {
    rm -rf "${work_dir}"
  }
  trap cleanup_work_dir EXIT
else
  mkdir -p "${work_dir}"
fi

echo "install.sh: installing Sis ${version} for ${linux_asset}"

dns_mode="$(prompt_choice "DNS listener:" "1" "1) LAN DNS on 0.0.0.0:53,[::]:53" "2) Local development on 127.0.0.1:5353" "3) Custom")"
case "${dns_mode}" in
  1) dns_default="0.0.0.0:53,[::]:53" ;;
  2) dns_default="127.0.0.1:5353" ;;
  3) dns_default="${SIS_DNS_LISTEN:-127.0.0.1:5353}" ;;
esac
dns_listen="${SIS_DNS_LISTEN:-$(prompt_value "DNS listen address list" "${dns_default}")}"

http_mode="$(prompt_choice "WebUI/API listener:" "1" "1) Localhost on 127.0.0.1:8080" "2) Trusted LAN on 0.0.0.0:8080" "3) Custom")"
case "${http_mode}" in
  1) http_default="127.0.0.1:8080" ;;
  2) http_default="0.0.0.0:8080" ;;
  3) http_default="${SIS_HTTP_LISTEN:-127.0.0.1:8080}" ;;
esac
http_listen="${SIS_HTTP_LISTEN:-$(prompt_value "HTTP listen address" "${http_default}")}"

store_choice="$(prompt_choice "Store backend:" "1" "1) sqlite" "2) json")"
case "${store_choice}" in
  1) store_default="sqlite" ;;
  2) store_default="json" ;;
esac
store_backend="${SIS_STORE_BACKEND:-$(prompt_value "Store backend value" "${store_default}")}"

privacy_choice="$(prompt_choice "Query log privacy:" "1" "1) hashed" "2) anonymous" "3) full")"
case "${privacy_choice}" in
  1) privacy_default="hashed" ;;
  2) privacy_default="anonymous" ;;
  3) privacy_default="full" ;;
esac
privacy_log_mode="${SIS_PRIVACY_LOG_MODE:-$(prompt_value "Privacy log mode" "${privacy_default}")}"
data_dir="${SIS_DATA_DIR:-$(prompt_value "Data directory" "${data_dir_default}")}"
enable_service="${SIS_INSTALL_ENABLE_SERVICE:-$(prompt_yes_no "Enable systemd service at boot" "yes")}"
start_service="${SIS_INSTALL_START_SERVICE:-$(prompt_yes_no "Start/restart service now" "yes")}"

case "${store_backend}" in json | sqlite) ;; *) die "unsupported store backend: ${store_backend}" ;; esac
case "${privacy_log_mode}" in full | hashed | anonymous) ;; *) die "unsupported privacy log mode: ${privacy_log_mode}" ;; esac

assets=(
  "sis_linux_amd64"
  "sis_linux_arm64"
  "sis_darwin_amd64"
  "sis_darwin_arm64"
  "sis.spdx.json"
  "SHA256SUMS"
  "SHA256SUMS.asc"
  "release-signing-public-key.asc"
)
for asset in "${assets[@]}"; do
  download "${download_base}/${version}/${asset}" "${work_dir}/${asset}"
done
chmod +x "${work_dir}/sis_linux_amd64" "${work_dir}/sis_linux_arm64"
verify_release_bundle "${work_dir}"

download "${github_raw}/${repo}/${version}/examples/sis.yaml" "${work_dir}/sis.yaml"
download "${github_raw}/${repo}/${version}/examples/sis.env" "${work_dir}/sis.env"
download "${github_raw}/${repo}/${version}/examples/sis.service" "${work_dir}/sis.service"

if ! getent group "${install_group}" >/dev/null 2>&1; then
  groupadd --system "${install_group}"
fi
if ! id -u "${install_user}" >/dev/null 2>&1; then
  useradd --system --home "${data_dir}" --shell /usr/sbin/nologin --gid "${install_group}" "${install_user}"
fi

install -d -o root -g root "${config_dir}"
install -d -o "${install_user}" -g "${install_group}" "${data_dir}"
install -d -o root -g root "${bin_dir}"
install -d -o root -g root "${systemd_dir}"

install_file_keep_existing 0640 root "${install_group}" "${work_dir}/sis.yaml" "${config_dir}/sis.yaml"
install_file_keep_existing 0640 root "${install_group}" "${work_dir}/sis.env" "${config_dir}/sis.env"
write_managed_env "${config_dir}/sis.env"
install -m 0755 -o root -g root "${work_dir}/${linux_asset}" "${bin_dir}/sis"
install -m 0644 -o root -g root "${work_dir}/sis.service" "${systemd_dir}/${service_name}.service"

(
  set -a
  # shellcheck disable=SC1090
  . "${config_dir}/sis.env"
  set +a
  "${bin_dir}/sis" config check -config "${config_dir}/sis.yaml"
)

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  if [[ "${enable_service}" == "yes" || "${enable_service}" == "1" ]]; then
    systemctl enable "${service_name}"
  fi
  if [[ "${start_service}" == "yes" || "${start_service}" == "1" ]]; then
    systemctl restart "${service_name}"
    systemctl --no-pager --full status "${service_name}" || true
  fi
else
  echo "install.sh: systemctl not found; installed files but did not manage service" >&2
fi

echo
echo "Sis ${version} installed."
echo "Binary: ${bin_dir}/sis"
echo "Config: ${config_dir}/sis.yaml"
echo "Env: ${config_dir}/sis.env"
echo "Data: ${data_dir}"
echo "WebUI/API: http://${http_listen}"
echo "DNS: ${dns_listen}"
echo
echo "Complete first-run setup in the WebUI, or run:"
echo "  sudo ${bin_dir}/sis user add -config ${config_dir}/sis.yaml admin 'change-me-now'"
