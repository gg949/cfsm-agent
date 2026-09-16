#!/bin/sh
set -eu

REPO="gg949/cfsm-agent"
GITHUB_PROXY=""
INSTALL_VERSION="latest"

log() {
    printf '%s\n' "$*"
}

die() {
    printf '[ERROR] %s\n' "$*" >&2
    exit 1
}

need_value_for=""
for arg in "$@"; do
    if [ -n "$need_value_for" ]; then
        case "$need_value_for" in
            proxy) GITHUB_PROXY="$arg" ;;
            version) INSTALL_VERSION="$arg" ;;
        esac
        need_value_for=""
        continue
    fi
    case "$arg" in
        --install-ghproxy=*) GITHUB_PROXY="${arg#*=}" ;;
        --install-ghproxy) need_value_for="proxy" ;;
        --install-version=*) INSTALL_VERSION="${arg#*=}" ;;
        --install-version) need_value_for="version" ;;
    esac
done

detect_os() {
    os="$(uname -s 2>/dev/null || printf unknown)"
    case "$os" in
        Linux) printf linux ;;
        Darwin) printf darwin ;;
        FreeBSD) printf freebsd ;;
        MINGW*|MSYS*|CYGWIN*) printf windows ;;
        *) die "unsupported OS: $os" ;;
    esac
}

detect_arch() {
    arch="$(uname -m 2>/dev/null || printf unknown)"
    case "$arch" in
        x86_64|amd64) printf amd64 ;;
        aarch64|arm64) printf arm64 ;;
        i386|i686) printf 386 ;;
        armv5*) if [ "${os_name:-}" = "freebsd" ]; then printf arm; else printf armv5; fi ;;
        armv6*) if [ "${os_name:-}" = "freebsd" ]; then printf arm; else printf armv6; fi ;;
        armv7*|armv8l) if [ "${os_name:-}" = "freebsd" ]; then printf arm; else printf armv7; fi ;;
        loongarch64|loong64) printf loong64 ;;
        *) die "unsupported architecture: $arch" ;;
    esac
}

download() {
    url="$1"
    out="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fL --connect-timeout 10 -m 120 -o "$out" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$out" "$url"
    else
        die "curl or wget is required for bootstrap download"
    fi
}

run_payload() {
    bin="$1"
    shift
    case "${cmd:-${1:-install}}" in
        uninstall|remove|delete|purge)
            "$bin" "$cmd"
            return
            ;;
    esac
    if [ "$#" -eq 0 ]; then
        "$bin" install
        return
    fi
    "$bin" "$@"
}

run_payload_checked() {
    err_file="$1"
    shift
    if run_payload "$@" 2>"$err_file"; then
        return 0
    else
        rc=$?
        return "$rc"
    fi
}

dir_has_noexec() {
    dir="$1"
    if command -v findmnt >/dev/null 2>&1; then
        opts="$(findmnt -no OPTIONS -T "$dir" 2>/dev/null || true)"
        case ",$opts," in
            *,noexec,*) return 0 ;;
        esac
    fi
    return 1
}

stage_and_run_payload() {
    dir="$1"
    shift
    [ -n "$dir" ] || return 125
    if dir_has_noexec "$dir"; then
        return 126
    fi
    mkdir -p "$dir" 2>/dev/null || return 125
    stage="$dir/.cf-probe-bootstrap.$$"
    stage_err="$stage.err"
    cp "$tmp" "$stage" 2>/dev/null || return 125
    chmod +x "$stage" 2>/dev/null || {
        rm -f "$stage"
        return 125
    }
    if run_payload_checked "$stage_err" "$stage" "$@"; then
        rc=0
    else
        rc=$?
        if [ "$rc" -ne 126 ]; then
            cat "$stage_err" >&2 2>/dev/null || true
        fi
    fi
    rm -f "$stage" "$stage_err"
    return "$rc"
}

cmd="${1:-install}"
case "$cmd" in
    uninstall|remove|delete|purge)
        log "[INFO] downloading temporary uninstaller"
        ;;
esac

os_name="$(detect_os)"
arch_name="$(detect_arch)"
asset="cf-probe-${os_name}-${arch_name}"
if [ "$os_name" = "windows" ]; then
    asset="${asset}.exe"
fi

if [ "$INSTALL_VERSION" = "latest" ]; then
    path="latest/download"
else
    path="download/$INSTALL_VERSION"
fi
url="https://github.com/$REPO/releases/$path/$asset"
if [ -n "$GITHUB_PROXY" ]; then
    url="${GITHUB_PROXY%/}/$url"
fi

tmp_dir="${TMPDIR:-/tmp}"
tmp="${tmp_dir%/}/cf-probe-bootstrap.$$"
tmp_err="$tmp.err"
trap 'rm -f "$tmp" "$tmp_err"' EXIT INT TERM

log "ProbeDeck Go Probe bootstrap"
log "  repo    : $REPO"
log "  version : $INSTALL_VERSION"
log "  target  : $os_name/$arch_name"
log "  asset   : $asset"
log "  url     : $url"

download "$url" "$tmp"
chmod +x "$tmp"

status=126
if ! dir_has_noexec "$tmp_dir"; then
    if run_payload_checked "$tmp_err" "$tmp" "$@"; then
        exit 0
    else
        status=$?
    fi
    if [ "$status" -ne 126 ]; then
        cat "$tmp_err" >&2 2>/dev/null || true
        exit "$status"
    fi
fi

log "[WARN] cannot execute bootstrap binary from $tmp; trying executable staging directories"
if [ -n "${HOME:-}" ]; then
    if stage_and_run_payload "$HOME/.cf-probe/tmp" "$@"; then
        exit 0
    fi
    status=$?
    if [ "$status" -ne 125 ] && [ "$status" -ne 126 ]; then
        exit "$status"
    fi
fi
if [ "$os_name" = "darwin" ]; then
    fallback_dirs="."
else
    fallback_dirs="/usr/local/bin /usr/bin /root ."
fi
for dir in $fallback_dirs; do
    if stage_and_run_payload "$dir" "$@"; then
        exit 0
    fi
    status=$?
    if [ "$status" -ne 125 ] && [ "$status" -ne 126 ]; then
        exit "$status"
    fi
done

die "downloaded binary could not be executed. /tmp may be mounted noexec."
