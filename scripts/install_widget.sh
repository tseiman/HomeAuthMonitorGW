#!/usr/bin/env bash
# Install or update the wago_kbus Zabbix custom widget.
#
# Usage:
#   scripts/install_widget.sh [--zabbix-modules-dir DIR] [--dry-run]
#
# Options:
#   --zabbix-modules-dir DIR   Absolute path to the Zabbix frontend modules directory.
#                              Default: /usr/share/zabbix/modules
#   --dry-run                  Show what would be done without making any changes.
#
# Update behaviour: the destination is replaced via a stage-and-swap.  Running this
# script a second time will never produce a nested wago_kbus/wago_kbus directory and
# stale files from the previous version are always removed.
#
# Safety checks performed before any write:
#   - --zabbix-modules-dir must be an absolute, non-empty path.
#   - The modules directory must exist and must not be a symlink.
#   - If wago_kbus already exists, it must be a plain directory (not a symlink or file).
#
# This script never modifies a live Zabbix database or API session.
# It only copies files on the local filesystem.
set -euo pipefail

ZABBIX_MODULES_DIR="/usr/share/zabbix/modules"
DRY_RUN=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WIDGET_SRC="${REPO_ROOT}/zabbix/modules/wago_kbus"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --zabbix-modules-dir)
            if [[ -z "${2:-}" ]]; then
                echo "ERROR: --zabbix-modules-dir requires a non-empty argument." >&2
                exit 1
            fi
            ZABBIX_MODULES_DIR="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            awk 'NR>1 && /^[^#]/{exit} NR>1{sub(/^# ?/,""); print}' "$0"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Validate: must be an absolute path.
if [[ -z "${ZABBIX_MODULES_DIR}" || "${ZABBIX_MODULES_DIR:0:1}" != "/" ]]; then
    echo "ERROR: --zabbix-modules-dir must be an absolute path; got: '${ZABBIX_MODULES_DIR}'" >&2
    exit 1
fi

WIDGET_DEST="${ZABBIX_MODULES_DIR}/wago_kbus"
WIDGET_STAGE="${WIDGET_DEST}.new.$$"

echo "Source:      ${WIDGET_SRC}"
echo "Destination: ${WIDGET_DEST}"

# --- Source validation -------------------------------------------------------

if [[ ! -d "${WIDGET_SRC}" ]]; then
    echo "ERROR: Widget source directory not found: ${WIDGET_SRC}" >&2
    exit 1
fi

# --- Dry-run -----------------------------------------------------------------

if [[ "${DRY_RUN}" == "true" ]]; then
    echo "Dry run — no changes will be made."
    if [[ -L "${ZABBIX_MODULES_DIR}" ]]; then
        echo "NOTE: ${ZABBIX_MODULES_DIR} is a symlink — would reject and abort."
    elif [[ ! -d "${ZABBIX_MODULES_DIR}" ]]; then
        echo "NOTE: ${ZABBIX_MODULES_DIR} does not exist — would abort."
    fi
    if [[ -L "${WIDGET_DEST}" ]]; then
        echo "NOTE: ${WIDGET_DEST} is a symlink — would reject and abort."
    elif [[ -e "${WIDGET_DEST}" && ! -d "${WIDGET_DEST}" ]]; then
        echo "NOTE: ${WIDGET_DEST} exists but is not a directory — would reject and abort."
    fi
    echo "Would: cp -r '${WIDGET_SRC}' '${WIDGET_STAGE}'"
    if [[ -d "${WIDGET_DEST}" ]]; then
        echo "Would: rm -rf '${WIDGET_DEST}'  (replaces existing installation; removes stale files)"
    fi
    echo "Would: mv '${WIDGET_STAGE}' '${WIDGET_DEST}'"
    if id www-data &>/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
        echo "Would: chown -R www-data:www-data '${WIDGET_DEST}'"
    fi
    exit 0
fi

# --- Destination directory validation ----------------------------------------

if [[ -L "${ZABBIX_MODULES_DIR}" ]]; then
    echo "ERROR: Zabbix modules directory is a symlink: ${ZABBIX_MODULES_DIR}" >&2
    echo "  Refusing to operate on a symlinked destination." >&2
    exit 1
fi

if [[ ! -d "${ZABBIX_MODULES_DIR}" ]]; then
    echo "ERROR: Zabbix modules directory does not exist: ${ZABBIX_MODULES_DIR}" >&2
    echo "  Pass --zabbix-modules-dir to specify the correct path." >&2
    exit 1
fi

if [[ -L "${WIDGET_DEST}" ]]; then
    echo "ERROR: Destination is a symlink: ${WIDGET_DEST}" >&2
    echo "  Remove it manually before installing." >&2
    exit 1
fi

if [[ -e "${WIDGET_DEST}" && ! -d "${WIDGET_DEST}" ]]; then
    echo "ERROR: Destination exists but is not a directory: ${WIDGET_DEST}" >&2
    echo "  Remove it manually before installing." >&2
    exit 1
fi

# --- Stage-and-swap install --------------------------------------------------

cleanup() {
    if [[ -d "${WIDGET_STAGE}" ]]; then
        rm -rf "${WIDGET_STAGE}"
    fi
}
trap cleanup EXIT

echo "Staging widget files..."
cp -r "${WIDGET_SRC}" "${WIDGET_STAGE}"

if id www-data &>/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
    echo "Setting ownership to www-data..."
    chown -R www-data:www-data "${WIDGET_STAGE}"
fi

if [[ -d "${WIDGET_DEST}" ]]; then
    echo "Removing previous installation (${WIDGET_DEST})..."
    rm -rf "${WIDGET_DEST}"
fi

mv "${WIDGET_STAGE}" "${WIDGET_DEST}"

if [[ $EUID -ne 0 ]]; then
    echo "Note: not running as root; ownership not changed. Run with sudo on a live server."
elif ! id www-data &>/dev/null 2>&1; then
    echo "Note: www-data user not found; adjust ownership manually if needed."
fi

echo "Done. Next steps:"
echo "  1. In Zabbix UI → Administration → General → Modules, click 'Scan directory'."
echo "  2. Enable the 'WAGO K-bus Visualizer' module."
echo "  3. Import or update the HomeAuthMonitorGW template (zabbix/template_homeauthmonitorgw.yaml)."
echo "  4. The WAGO 750-880 dashboard will now use the K-bus Visualizer widget."
