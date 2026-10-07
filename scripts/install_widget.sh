#!/usr/bin/env bash
# Install or update the wago_kbus Zabbix custom widget.
#
# Usage:
#   scripts/install_widget.sh [--zabbix-modules-dir DIR] [--web-group GROUP] [--dry-run]
#
# Options:
#   --zabbix-modules-dir DIR   Absolute path to the Zabbix frontend modules directory.
#                              Default: /usr/share/zabbix/ui/modules
#   --web-group GROUP          Unix group that runs the Zabbix web frontend (e.g. www-data,
#                              apache, nginx, zabbix).  Used to set group-read permissions
#                              on custom asset directories.  Default: www-data
#   --dry-run                  Show what would be done without making any changes.
#
# Environment variables:
#   WAGO_KBUS_DATA_DIR         Override the persistent custom data directory.
#                              Default: /var/lib/zabbix/wago_kbus
#                              Set this in tests to avoid writing to /var/lib.
#
# Widget module directory (/usr/share/zabbix/ui/modules/wago_kbus):
#   Replaced atomically on each run via stage-and-swap; stale files are removed.
#   Running this script a second time is safe and idempotent.
#
# Persistent custom SVG data ($WAGO_KBUS_DATA_DIR, default /var/lib/zabbix/wago_kbus):
#   Created only if absent; NEVER overwritten or deleted on install/update.
#   /var/lib is the FHS location for persistent application state — outside the
#   package-managed module tree so normal widget updates never touch custom assets.
#   Permissions: root:<web-group> 750 (dirs) and 640 (files) — readable by the web
#   server but not writable, so a compromised frontend cannot modify its own config.
#
#   On first install, default_svg_map.json (reusable module catalog) is copied to
#   custom_svg_map.json ONLY when the file does not yet exist.  If an existing
#   custom_svg_map.json is found (including an old slot-keyed schema),
#   its contents are left untouched.  See README for manual schema migration guidance.
#
#   Every regular, non-symlink *.svg from assets/img and catalog_images is copied to the
#   images directory ONLY when its destination is absent. Existing files are never
#   overwritten, preserving site-local modifications.
#
# Safety checks performed before any write:
#   - --zabbix-modules-dir must be an absolute, non-empty path.
#   - Module and persistent-data paths must not contain symlink components.
#   - If wago_kbus already exists, it must be a plain directory (not a symlink or file).
#
# This script never modifies a live Zabbix database or API session.
# It only copies files on the local filesystem.
set -euo pipefail

ZABBIX_MODULES_DIR="/usr/share/zabbix/ui/modules"
WEB_GROUP="www-data"
DRY_RUN=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WIDGET_SRC="${REPO_ROOT}/zabbix/modules/wago_kbus"

# Persistent custom data directory (overridable via env for testing).
CUSTOM_DATA_DIR="${WAGO_KBUS_DATA_DIR:-/var/lib/zabbix/wago_kbus}"

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
        --web-group)
            if [[ -z "${2:-}" ]]; then
                echo "ERROR: --web-group requires a non-empty argument." >&2
                exit 1
            fi
            WEB_GROUP="$2"
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
if [[ -z "${CUSTOM_DATA_DIR}" || "${CUSTOM_DATA_DIR:0:1}" != "/" ]]; then
    echo "ERROR: WAGO_KBUS_DATA_DIR must be an absolute path; got: '${CUSTOM_DATA_DIR}'" >&2
    exit 1
fi

WIDGET_DEST="${ZABBIX_MODULES_DIR}/wago_kbus"
WIDGET_STAGE="${WIDGET_DEST}.new.$$"

CUSTOM_IMAGES_DIR="${CUSTOM_DATA_DIR}/images"
CUSTOM_MAP_FILE="${CUSTOM_DATA_DIR}/custom_svg_map.json"
DEFAULT_MAP_FILE="${WIDGET_SRC}/default_svg_map.json"
CATALOG_IMAGES_DIR="${WIDGET_SRC}/catalog_images"

# Persistent state must be disjoint from the atomically replaced widget tree. Otherwise an
# update could delete site-local catalog/SVG files before the only-if-absent checks run.
WIDGET_DEST_NORMALIZED="$(realpath -m -- "${WIDGET_DEST}")"
CUSTOM_DATA_NORMALIZED="$(realpath -m -- "${CUSTOM_DATA_DIR}")"
if [[ "${CUSTOM_DATA_NORMALIZED}" == "${WIDGET_DEST_NORMALIZED}" ||
      "${CUSTOM_DATA_NORMALIZED}" == "${WIDGET_DEST_NORMALIZED}/"* ||
      "${WIDGET_DEST_NORMALIZED}" == "${CUSTOM_DATA_NORMALIZED}/"* ]]; then
    echo "ERROR: Persistent data and widget destination paths overlap." >&2
    echo "  Widget destination: ${WIDGET_DEST_NORMALIZED}" >&2
    echo "  Persistent data:   ${CUSTOM_DATA_NORMALIZED}" >&2
    echo "  Use disjoint paths so widget replacement cannot delete persistent files." >&2
    exit 1
fi

# Build a deterministic source list without following symlinks. assets/img wins if a
# catalog_images file has the same basename.
declare -a SHIPPED_IMAGE_SOURCES=()
declare -A SHIPPED_IMAGE_NAMES=()
shopt -s nullglob
for source_dir in "${WIDGET_SRC}/assets/img" "${CATALOG_IMAGES_DIR}"; do
    for src in "${source_dir}"/*.svg; do
        [[ -f "${src}" && ! -L "${src}" ]] || continue
        img="${src##*/}"
        if [[ -z "${SHIPPED_IMAGE_NAMES[${img}]+x}" ]]; then
            SHIPPED_IMAGE_NAMES["${img}"]=1
            SHIPPED_IMAGE_SOURCES+=("${src}")
        fi
    done
done
shopt -u nullglob

echo "Source:           ${WIDGET_SRC}"
echo "Widget dest:      ${WIDGET_DEST}"
echo "Custom data dir:  ${CUSTOM_DATA_DIR}"

# --- Source validation -------------------------------------------------------

if [[ ! -d "${WIDGET_SRC}" ]]; then
    echo "ERROR: Widget source directory not found: ${WIDGET_SRC}" >&2
    exit 1
fi

reject_symlink_components() {
    local path="$1"
    local current="/"
    local component
    local -a components=()

    IFS='/' read -r -a components <<< "${path#/}"
    for component in "${components[@]}"; do
        [[ -n "${component}" ]] || continue
        if [[ "${component}" == "." || "${component}" == ".." ]]; then
            echo "ERROR: Path contains forbidden component '${component}': ${path}" >&2
            exit 1
        fi
        current="${current%/}/${component}"
        if [[ -L "${current}" ]]; then
            echo "ERROR: Path contains a symlink component: ${current}" >&2
            echo "  Refusing privileged writes through symlinks." >&2
            exit 1
        fi
    done
}

validate_root_owned_ancestors() {
    [[ $EUID -eq 0 ]] || return 0

    local path="$1"
    local current="/"
    local component owner mode permissions group_digit other_digit
    local -a components=()

    IFS='/' read -r -a components <<< "${path#/}"
    for component in "${components[@]}"; do
        [[ -n "${component}" ]] || continue
        current="${current%/}/${component}"
        [[ -e "${current}" ]] || break
        read -r owner mode < <(stat -c '%u %a' -- "${current}")
        permissions="${mode: -3}"
        group_digit="${permissions:1:1}"
        other_digit="${permissions:2:1}"
        if [[ "${owner}" != "0" ]] || (( (group_digit & 2) != 0 || (other_digit & 2) != 0 )); then
            echo "ERROR: Unsafe privileged destination component: ${current}" >&2
            echo "  Expected uid 0 and no group/other write bits; found uid ${owner}, mode ${mode}." >&2
            echo "  Refusing writes because an unprivileged account could swap this path." >&2
            exit 1
        fi
    done
}

validate_persistent_paths() {
    reject_symlink_components "${CUSTOM_DATA_DIR}"
    reject_symlink_components "${CUSTOM_IMAGES_DIR}"
    reject_symlink_components "${CUSTOM_MAP_FILE}"
    validate_root_owned_ancestors "${CUSTOM_DATA_DIR}"
    validate_root_owned_ancestors "${CUSTOM_IMAGES_DIR}"
    validate_root_owned_ancestors "${CUSTOM_MAP_FILE}"

    if [[ -e "${CUSTOM_DATA_DIR}" && ! -d "${CUSTOM_DATA_DIR}" ]]; then
        echo "ERROR: Persistent data path is not a directory: ${CUSTOM_DATA_DIR}" >&2
        exit 1
    fi
    if [[ -e "${CUSTOM_IMAGES_DIR}" && ! -d "${CUSTOM_IMAGES_DIR}" ]]; then
        echo "ERROR: Persistent image path is not a directory: ${CUSTOM_IMAGES_DIR}" >&2
        exit 1
    fi
    if [[ -e "${CUSTOM_MAP_FILE}" && ! -f "${CUSTOM_MAP_FILE}" ]]; then
        echo "ERROR: Persistent catalog path is not a regular file: ${CUSTOM_MAP_FILE}" >&2
        exit 1
    fi

    local src dst
    for src in "${SHIPPED_IMAGE_SOURCES[@]}"; do
        dst="${CUSTOM_IMAGES_DIR}/${src##*/}"
        reject_symlink_components "${dst}"
        validate_root_owned_ancestors "${dst}"
        if [[ -e "${dst}" && ! -f "${dst}" ]]; then
            echo "ERROR: Persistent SVG destination is not a regular file: ${dst}" >&2
            exit 1
        fi
    done
}

# Validate all privileged destinations even for --dry-run so previews do not accept unsafe targets.
reject_symlink_components "${ZABBIX_MODULES_DIR}"
reject_symlink_components "${WIDGET_DEST}"
validate_root_owned_ancestors "${ZABBIX_MODULES_DIR}"
validate_root_owned_ancestors "${WIDGET_DEST}"
validate_persistent_paths

# --- Dry-run -----------------------------------------------------------------

if [[ "${DRY_RUN}" == "true" ]]; then
    echo "Dry run — no changes will be made."

    echo ""
    echo "Widget module (stage-and-swap):"
    if [[ -L "${ZABBIX_MODULES_DIR}" ]]; then
        echo "  NOTE: ${ZABBIX_MODULES_DIR} is a symlink — would reject and abort."
    elif [[ ! -d "${ZABBIX_MODULES_DIR}" ]]; then
        echo "  NOTE: ${ZABBIX_MODULES_DIR} does not exist — would abort."
    fi
    if [[ -L "${WIDGET_DEST}" ]]; then
        echo "  NOTE: ${WIDGET_DEST} is a symlink — would reject and abort."
    elif [[ -e "${WIDGET_DEST}" && ! -d "${WIDGET_DEST}" ]]; then
        echo "  NOTE: ${WIDGET_DEST} exists but is not a directory — would reject and abort."
    fi
    echo "  Would: cp -r '${WIDGET_SRC}' '${WIDGET_STAGE}'"
    if [[ -d "${WIDGET_DEST}" ]]; then
        echo "  Would: rm -rf '${WIDGET_DEST}'  (replaces existing; removes stale files)"
    fi
    echo "  Would: mv '${WIDGET_STAGE}' '${WIDGET_DEST}'"
    if id "${WEB_GROUP}" &>/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
        echo "  Would: chown -R root:${WEB_GROUP} '${WIDGET_DEST}' && chmod ..."
    fi

    echo ""
    echo "Persistent custom data (only-if-absent, never overwritten):"
    for path in "${CUSTOM_DATA_DIR}" "${CUSTOM_IMAGES_DIR}"; do
        if [[ -d "${path}" ]]; then
            echo "  Exists (skip): ${path}"
        else
            echo "  Would create:  ${path}"
        fi
    done
    if [[ -f "${CUSTOM_MAP_FILE}" ]]; then
        echo "  Exists (skip): ${CUSTOM_MAP_FILE}"
    else
        echo "  Would copy:    ${CUSTOM_MAP_FILE}  (from ${DEFAULT_MAP_FILE})"
    fi
    echo ""
    echo "  Shipped/catalog images (copy only when destination absent):"
    for src in "${SHIPPED_IMAGE_SOURCES[@]}"; do
        img="${src##*/}"
        dst="${CUSTOM_IMAGES_DIR}/${img}"
        if [[ -f "${dst}" ]]; then
            echo "  Exists (skip): ${dst}"
        else
            echo "  Would copy:    ${dst}  (from ${src})"
        fi
    done
    if [[ $EUID -eq 0 ]]; then
        echo "  Would set: root:${WEB_GROUP} 750 on dirs, 640 on files (created paths only)"
    else
        echo "  Note: not root — permissions would not be set."
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

# --- Stage-and-swap widget install -------------------------------------------

cleanup() {
    if [[ -d "${WIDGET_STAGE}" ]]; then
        rm -rf "${WIDGET_STAGE}"
    fi
}
trap cleanup EXIT

echo "Staging widget files..."
cp -r "${WIDGET_SRC}" "${WIDGET_STAGE}"

if id "${WEB_GROUP}" &>/dev/null 2>&1 && [[ $EUID -eq 0 ]]; then
    echo "Setting widget ownership to root:${WEB_GROUP}..."
    chown -R root:"${WEB_GROUP}" "${WIDGET_STAGE}"
fi

if [[ -d "${WIDGET_DEST}" ]]; then
    echo "Removing previous widget installation (${WIDGET_DEST})..."
    rm -rf "${WIDGET_DEST}"
fi

mv "${WIDGET_STAGE}" "${WIDGET_DEST}"

if [[ $EUID -ne 0 ]]; then
    echo "Note: not running as root; widget ownership not changed. Run with sudo on a live server."
elif ! id "${WEB_GROUP}" &>/dev/null 2>&1; then
    echo "Note: group '${WEB_GROUP}' not found; adjust ownership manually (--web-group to specify)."
fi

# --- Persistent custom data (only-if-absent) ---------------------------------
# These paths are NEVER deleted or overwritten; they survive every widget update.

echo ""
echo "Checking persistent custom data..."

if [[ ! -d "${CUSTOM_DATA_DIR}" ]]; then
    mkdir -p "${CUSTOM_DATA_DIR}"
    echo "  Created: ${CUSTOM_DATA_DIR}"
else
    echo "  Exists (preserved): ${CUSTOM_DATA_DIR}"
fi

if [[ ! -d "${CUSTOM_IMAGES_DIR}" ]]; then
    mkdir -p "${CUSTOM_IMAGES_DIR}"
    echo "  Created: ${CUSTOM_IMAGES_DIR}"
else
    echo "  Exists (preserved): ${CUSTOM_IMAGES_DIR}"
fi

# Revalidate after directory creation to narrow the check/use window.
validate_persistent_paths

# Seed custom_svg_map.json from the versioned default ONLY when absent.
# If an existing file is found (including old slot-keyed JSON), it is left untouched.
# See README for manual schema migration guidance.
if [[ ! -f "${CUSTOM_MAP_FILE}" ]]; then
    if [[ -f "${DEFAULT_MAP_FILE}" ]]; then
        cp --no-clobber --no-dereference "${DEFAULT_MAP_FILE}" "${CUSTOM_MAP_FILE}"
        echo "  Created: ${CUSTOM_MAP_FILE}  (seeded from default_svg_map.json)"
    else
        starter="${CUSTOM_DATA_DIR}/.custom_svg_map.json.$$"
        printf '%s\n' '{"controllers":{},"modules":{}}' > "${starter}"
        mv --no-clobber "${starter}" "${CUSTOM_MAP_FILE}"
        rm -f "${starter}"
        echo "  Created: ${CUSTOM_MAP_FILE}  (empty starter — default_svg_map.json not found)"
    fi
else
    echo "  Exists (preserved): ${CUSTOM_MAP_FILE}"
fi
validate_persistent_paths

# Copy shipped/catalog SVG images ONLY when each destination file is absent.
# Existing files (including site-local replacements) are never overwritten.
echo ""
echo "Checking shipped/catalog SVG images..."
for src in "${SHIPPED_IMAGE_SOURCES[@]}"; do
    img="${src##*/}"
    dst="${CUSTOM_IMAGES_DIR}/${img}"
    if [[ ! -f "${dst}" ]]; then
        if [[ -f "${src}" && ! -L "${src}" ]]; then
            cp --no-clobber --no-dereference "${src}" "${dst}"
            echo "  Copied: ${dst}"
        fi
    else
        echo "  Exists (preserved): ${dst}"
    fi
done

# Refuse a path swap before applying ownership or mode changes.
validate_persistent_paths

# Set permissions on custom data.
# root:<web-group> 750/640: readable by web server, not writable.
if [[ $EUID -eq 0 ]] && id "${WEB_GROUP}" &>/dev/null 2>&1; then
    chown root:"${WEB_GROUP}" "${CUSTOM_DATA_DIR}" "${CUSTOM_IMAGES_DIR}" 2>/dev/null || true
    chmod 750 "${CUSTOM_DATA_DIR}" "${CUSTOM_IMAGES_DIR}" 2>/dev/null || true
    chown root:"${WEB_GROUP}" "${CUSTOM_MAP_FILE}" 2>/dev/null || true
    chmod 640 "${CUSTOM_MAP_FILE}" 2>/dev/null || true
    for src in "${SHIPPED_IMAGE_SOURCES[@]}"; do
        img="${src##*/}"
        dst="${CUSTOM_IMAGES_DIR}/${img}"
        if [[ -f "${dst}" ]]; then
            chown root:"${WEB_GROUP}" "${dst}" 2>/dev/null || true
            chmod 640 "${dst}" 2>/dev/null || true
        fi
    done
elif [[ $EUID -ne 0 ]]; then
    echo ""
    echo "  Note: not running as root; permissions on custom data not set."
    echo "        Run with sudo on a live server, or set manually:"
    echo "          sudo chown root:${WEB_GROUP} '${CUSTOM_DATA_DIR}' '${CUSTOM_IMAGES_DIR}'"
    echo "          sudo chmod 750 '${CUSTOM_DATA_DIR}' '${CUSTOM_IMAGES_DIR}'"
    echo "          sudo chmod 640 '${CUSTOM_MAP_FILE}'"
    echo "          sudo chown root:${WEB_GROUP} '${CUSTOM_IMAGES_DIR}'/*.svg"
    echo "          sudo chmod 640 '${CUSTOM_IMAGES_DIR}'/*.svg"
fi

echo ""
echo "Done. Next steps:"
echo "  1. In Zabbix UI → Administration → General → Modules, click 'Scan directory'."
echo "  2. Enable the 'WAGO K-bus Visualizer' module."
echo "  3. Import or update the HomeAuthMonitorGW template (zabbix/template_homeauthmonitorgw.yaml)."
echo "  4. After template import, open the WAGO 750-880 dashboard and select the host"
echo "     manually in the K-bus Visualizer widget (hostid is not persisted in the template)."
echo ""
echo "  To add a custom module/controller SVG (no restart needed):"
echo "    sudo cp your-module.svg '${CUSTOM_IMAGES_DIR}/'"
echo "    sudo chown root:${WEB_GROUP} '${CUSTOM_IMAGES_DIR}/your-module.svg'"
echo "    sudo chmod 640 '${CUSTOM_IMAGES_DIR}/your-module.svg'"
echo "    Then add or update the entry in '${CUSTOM_MAP_FILE}' — see README for JSON schema."
echo ""
echo "  If upgrading from a slot-keyed catalog, custom_svg_map.json was preserved unchanged."
echo "    Migrate it manually to the v2.0 article catalog described in the README."
