#!/bin/sh
set -e

setopt SH_WORD_SPLIT 2>/dev/null || :

INSTALL_DIR="{{ .InstallDir }}"
INSTALL_FILENAME="{{ .InstallFilename }}"
INSTALL_PATH="$INSTALL_DIR/$INSTALL_FILENAME"
PREFER_DOWNLOAD="{{ .PreferDownload }}"
CHMOD_PATH="{{ .ChmodPath }}"
DOWNLOAD_AMD="{{ .DownloadAmd }}"
DOWNLOAD_ARM="{{ .DownloadArm }}"
DOWNLOAD_BASE="{{ .DownloadBase }}"
COMMAND="{{ .Command }}"
# EXISTS_CHECK returns 0 (true) when installation is NEEDED
# i.e., when agent does not exist or has wrong version
EXISTS_CHECK="{{ .ExistsCheck }}"
TEMP_PATH="$INSTALL_PATH.$$"

command_exists() { command -v "$@" >/dev/null 2>&1; }
fail() {
    echo >&2 "Error: $1"
    return 1
}

is_arm() {
    case "$(uname -a)" in
        *arm* | *aarch*) return 0 ;;
        *) return 1 ;;
    esac
}

detect_architecture() { is_arm && echo "true" || echo "false"; }
select_download_url() { is_arm && echo "$DOWNLOAD_ARM" || echo "$DOWNLOAD_AMD"; }

find_download_tool() {
    command_exists curl && echo "curl" && return 0
    command_exists wget && echo "wget" && return 0
    return 1
}

handshake() {
    echo "ping"
    # shellcheck disable=SC3045
    if ! IFS= read -r -t 30 response 2>/dev/null; then
        # Fallback for shells without -t support
        if command_exists timeout; then
            # shellcheck disable=SC2016
            response="$(timeout 30 sh -c 'IFS= read -r line && printf %s "$line"')" || fail "handshake timeout"
        else
            IFS= read -r response || fail "failed to read handshake response"
        fi
    fi
    [ "$response" = "pong" ] || fail "received wrong answer for ping request: $response"
}

check_noexec() {
    mount_path="$(df -P "$INSTALL_DIR" 2>/dev/null | tail -n 1 | awk '{print $NF}')"
    [ -z "$mount_path" ] && return 0
    mount | grep -F "on ${mount_path} " | grep -q noexec &&
        fail "installation directory $INSTALL_DIR is noexec, please choose another location"
    return 0
}

can_write_without_privilege() {
    mkdir -p "$INSTALL_DIR" 2>/dev/null &&
        touch "$INSTALL_PATH" 2>/dev/null &&
        chmod +x "$INSTALL_PATH" 2>/dev/null &&
        rm -f "$INSTALL_PATH" 2>/dev/null
}

detect_privilege_command() {
    if can_write_without_privilege; then
        echo "sh -c"
    elif command_exists sudo; then
        sudo -nl >/dev/null 2>&1 ||
            fail "sudo requires a password and no password is available. Please ensure your user account is configured with NOPASSWD"
        echo "sudo -E sh -c"
    else
        fail "this installer needs the ability to run commands as root. No passwordless sudo or writable install directory found"
    fi
}

setup_install_directory() {
    $1 "mkdir -p '$INSTALL_DIR'" || fail "failed to create install directory"
}

receive_binary() {
    echo "ARM-$(detect_architecture)"
    $1 "cat > '$TEMP_PATH'" || return 1
    $1 "mv '$TEMP_PATH' '$INSTALL_PATH'" || return 1
    [ "$CHMOD_PATH" = "true" ] && { $1 "chmod +x '$INSTALL_PATH'" || return 1; }
    return 0
}

download_file() {
    case "$3" in
        curl) $1 "curl -fsSL '$2' -o '$TEMP_PATH'" ;;
        wget) $1 "wget -q '$2' -O '$TEMP_PATH'" ;;
        *) return 1 ;;
    esac
}

download_with_retry() {
    attempt=1
    while [ "$attempt" -le 3 ]; do
        download_file "$1" "$2" "$3" && return 0
        if [ "$attempt" -lt 3 ]; then
            echo >&2 "error: download failed (attempt $attempt/3)"
            echo >&2 "trying again in 10 seconds"
            sleep 10
        fi
        attempt=$((attempt + 1))
    done
    return 1
}

download_binary() {
    url="$(select_download_url)"
    tool="$(find_download_tool)" || fail "no download tool found, please install curl or wget"
    download_with_retry "$1" "$url" "$tool" || return 1
    $1 "mv '$TEMP_PATH' '$INSTALL_PATH'" || return 1
    [ "$CHMOD_PATH" = "true" ] && { $1 "chmod +x '$INSTALL_PATH'" || return 1; }
    return 0
}

install_binary() {
    $1 "rm -f '$INSTALL_PATH' 2>/dev/null || true"
    if [ "$PREFER_DOWNLOAD" = "true" ]; then
        download_binary "$1" || receive_binary "$1" || return 1
    else
        receive_binary "$1" || download_binary "$1" || return 1
    fi
}

main() {
    handshake || return 1

    if ! eval "$EXISTS_CHECK"; then
        echo "done"
        export DEVPOD_AGENT_URL="$DOWNLOAD_BASE"
        eval "$COMMAND"
        return 0
    fi

    sh_c="$(detect_privilege_command)" || return 1
    setup_install_directory "$sh_c" || return 1
    check_noexec || return 1
    install_binary "$sh_c" || return 1
    eval "$EXISTS_CHECK" && fail "failed to install devpod"

    echo "done"
    export DEVPOD_AGENT_URL="$DOWNLOAD_BASE"
    eval "$COMMAND"
}

main
exit $?
