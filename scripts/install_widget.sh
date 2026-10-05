#!/usr/bin/env bash
# Install or update the wago_kbus Zabbix custom widget.
#
# Usage:
#   scripts/install_widget.sh [--zabbix-modules-dir DIR] [--dry-run]
#
# Options:
#   --zabbix-modules-dir DIR   Path to the Zabbix frontend modules directory.
#                              Default: /usr/share/zabbix/modules
#   --dry-run                  Show what would be done without making any changes.
#
# After installation:
#   1. Log in to the Zabbix web UI as Admin.
#   2. Go to Administration → General → Modules.
#   3. Click "Scan directory" if wago_kbus does not appear, then enable it.
#   4. The WAGO 750-880 dashboard will now contain the K-bus Visualizer widget.
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
            ZABBIX_MODULES_DIR="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

WIDGET_DEST="${ZABBIX_MODULES_DIR}/wago_kbus"

echo "Source:      ${WIDGET_SRC}"
echo "Destination: ${WIDGET_DEST}"

if [[ ! -d "${WIDGET_SRC}" ]]; then
    echo "ERROR: Widget source directory not found: ${WIDGET_SRC}" >&2
    exit 1
fi

if [[ "${DRY_RUN}" == "true" ]]; then
    echo "Dry run — no changes will be made."
    echo "Would run: cp -r '${WIDGET_SRC}' '${ZABBIX_MODULES_DIR}/'"
    if id www-data &>/dev/null 2>&1; then
        echo "Would run: chown -R www-data:www-data '${WIDGET_DEST}'"
    fi
    exit 0
fi

if [[ ! -d "${ZABBIX_MODULES_DIR}" ]]; then
    echo "ERROR: Zabbix modules directory does not exist: ${ZABBIX_MODULES_DIR}" >&2
    echo "  Pass --zabbix-modules-dir to specify the correct path." >&2
    exit 1
fi

echo "Copying widget files..."
cp -r "${WIDGET_SRC}" "${ZABBIX_MODULES_DIR}/"

if id www-data &>/dev/null 2>&1; then
    echo "Setting ownership to www-data..."
    chown -R www-data:www-data "${WIDGET_DEST}"
else
    echo "Note: www-data user not found; adjust ownership manually if your web server runs as a different user."
fi

echo "Done. Next steps:"
echo "  1. In Zabbix UI → Administration → General → Modules, click 'Scan directory'."
echo "  2. Enable the 'WAGO K-bus Visualizer' module."
echo "  3. Import or update the HomeAuthMonitorGW template (zabbix/template_homeauthmonitorgw.yaml)."
echo "  4. The WAGO 750-880 dashboard will now use the K-bus Visualizer widget."
