#!/bin/sh
set -e

INSTALL_DIR="{{ .InstallDir }}"
INSTALL_FILENAME="{{ .InstallFilename }}"
INSTALL_PATH="$INSTALL_DIR/$INSTALL_FILENAME"
PREFER_DOWNLOAD="{{ .PreferDownload }}"
CHMOD_PATH="{{ .ChmodPath }}"

command_exists() {
    command -v "$@" > /dev/null 2>&1
}

is_arm() {
    case "$(uname -a)" in
        *arm* | *arm64* | *aarch* | *aarch64*) true ;;
        *) false ;;
    esac
}

check_noexec() {
    mount_path="$(df "${INSTALL_DIR}" | tail -n +2 | rev | cut -d' ' -f1 | rev)"
    if mount | grep "on ${mount_path} " | grep -q noexec; then
        >&2 echo "Error: Installation directory $INSTALL_DIR is mounted with noexec flag"
        return 1
    fi
    return 0
}

handshake() {
    echo "ping"
    read -r DEVPOD_PING
    if [ "$DEVPOD_PING" != "pong" ]; then
        >&2 echo "Error: Handshake failed - received '$DEVPOD_PING' instead of 'pong'"
        exit 1
    fi
}

inject_binary() {
    echo "ARM-$(is_arm && echo -n 'true' || echo -n 'false')"

    temp_file="$(mktemp "$INSTALL_PATH.XXXXXX" 2> /dev/null || echo "$INSTALL_PATH.$$")"

    if ! $sh_c "cat > \"$temp_file\""; then
        >&2 echo "Error: Failed to write binary to $temp_file"
        $sh_c "rm -f \"$temp_file\""
        return 1
    fi

    if ! $sh_c "mv \"$temp_file\" \"$INSTALL_PATH\""; then
        >&2 echo "Error: Failed to move binary to $INSTALL_PATH"
        $sh_c "rm -f \"$temp_file\""
        return 1
    fi

    if [ "$CHMOD_PATH" = "true" ]; then
        $sh_c "chmod +x \"$INSTALL_PATH\"" || {
            >&2 echo "Error: Failed to chmod $INSTALL_PATH"
            return 1
        }
    fi

    return 0
}

download_binary() {
    DOWNLOAD_URL="{{ .DownloadAmd }}"
    if is_arm; then
        DOWNLOAD_URL="{{ .DownloadArm }}"
    fi

    iteration=1
    max_iteration=3

    while [ "$iteration" -le "$max_iteration" ]; do
        temp_file="$(mktemp "$INSTALL_PATH.XXXXXX" 2> /dev/null || echo "$INSTALL_PATH.$$")"

        if command_exists curl; then
            if $sh_c "curl -fsSL \"$DOWNLOAD_URL\" -o \"$temp_file\""; then
                break
            fi
            cmd_status=$?
        elif command_exists wget; then
            if $sh_c "wget -q \"$DOWNLOAD_URL\" -O \"$temp_file\""; then
                break
            fi
            cmd_status=$?
        else
            >&2 echo "Error: No download tool found (curl or wget required)"
            return 127
        fi

        $sh_c "rm -f \"$temp_file\""

        >&2 echo "Error: Download attempt $iteration failed (exit code: ${cmd_status})"
        >&2 echo "       URL: $DOWNLOAD_URL"

        if [ "$iteration" -lt "$max_iteration" ]; then
            >&2 echo "       Retrying in 10 seconds"
            sleep 10
        fi
        iteration=$((iteration + 1))
    done

    if [ "$iteration" -gt "$max_iteration" ]; then
        >&2 echo "Error: Failed to download devpod after $max_iteration attempts"
        return 1
    fi

    if ! $sh_c "mv \"$temp_file\" \"$INSTALL_PATH\""; then
        >&2 echo "Error: Failed to move downloaded binary to $INSTALL_PATH"
        $sh_c "rm -f \"$temp_file\""
        return 1
    fi

    return 0
}

setup_sudo() {
    if ! mkdir -p "$INSTALL_DIR" 2> /dev/null \
        || ! touch "$INSTALL_PATH" 2> /dev/null \
        || ! chmod +x "$INSTALL_PATH" 2> /dev/null \
        || ! rm -f "$INSTALL_PATH" 2> /dev/null; then

        if command_exists sudo; then
            if ! sudo -nl > /dev/null 2>&1; then
                >&2 echo "Error: sudo requires a password. Please configure NOPASSWD"
                exit 1
            fi
            sh_c='sudo -E sh -c'
        elif command_exists su; then
            sh_c='su -c'
        else
            >&2 echo "Error: Need root access but sudo/su not available"
            exit 1
        fi

        $sh_c "mkdir -p \"$INSTALL_DIR\""
    fi
}

install_agent() {
    if [ "$PREFER_DOWNLOAD" = "true" ]; then
        if ! download_binary; then
            >&2 echo "Download failed, attempting stdin injection"
            inject_binary || {
                >&2 echo "Error: Both download and injection failed"
                exit 1
            }
        fi
    else
        if ! inject_binary; then
            >&2 echo "Injection failed, attempting download"
            download_binary || {
                >&2 echo "Error: Both injection and download failed"
                exit 1
            }
        fi
    fi

    if [ "$CHMOD_PATH" = "true" ]; then
        $sh_c "chmod +x \"$INSTALL_PATH\"" || {
            >&2 echo "Error: Failed to make $INSTALL_PATH executable"
            exit 1
        }
    fi
}

execute_command() {
    echo "done"
    export DEVPOD_AGENT_URL={{ .DownloadBase }}
    {{ .Command }}
}

main() {
    handshake

    if {{ .ExistsCheck }}; then
        sh_c='sh -c'
        setup_sudo
        check_noexec || exit 1
        $sh_c "rm -f \"$INSTALL_PATH\" 2>/dev/null || true"
        install_agent
    fi

    execute_command
}

main
