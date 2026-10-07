<?php
// HTML-escape helper.
$e = static fn($v) => htmlspecialchars((string) $v, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');

// Persistent site-local custom SVG data.
// /var/lib is the FHS location for variable state owned by applications — mutable, persistent,
// and outside the package-managed module tree at /usr/share/zabbix/ui/modules/.  Storing custom
// widget data here means normal widget updates (install_widget.sh) never touch it.
define('WAGO_CUSTOM_BASE',   '/var/lib/zabbix/wago_kbus');
define('WAGO_CUSTOM_IMAGES', '/var/lib/zabbix/wago_kbus/images');
define('WAGO_CUSTOM_MAP',    '/var/lib/zabbix/wago_kbus/custom_svg_map.json');

// Fallback SVG paths — built-in assets used when no configured entry covers a slot.
// These are the only product-specific constants that may remain in PHP; all display names,
// expected SNMP_IDs, image filenames, and descriptions come exclusively from JSON at runtime.
$img_dir                 = __DIR__ . '/../assets/img/';
$SVG_FALLBACK_CONTROLLER = $img_dir . 'wago_0750-xxxx_controller.svg';
$SVG_FALLBACK_MODULE     = $img_dir . 'wago_0750-xxxx_modul.svg';
$fallbackCtrlImg         = basename($SVG_FALLBACK_CONTROLLER);
$fallbackModImg          = basename($SVG_FALLBACK_MODULE);

// Valid image filename: basename only, alphanumeric/underscore/hyphen, .svg suffix.
// No path separators — prevents traversal at the filename level.
$validFilename = static fn(string $f): bool =>
    (bool) preg_match('/^[a-zA-Z0-9][a-zA-Z0-9_\-]*\.svg$/', $f);

// Resolve a filename to an absolute path inside WAGO_CUSTOM_IMAGES.
// Is the file a non-symlink readable regular file whose realpath is strictly inside the
// images directory?  Returns the absolute path on success, null otherwise.
$imagesReal = realpath(WAGO_CUSTOM_IMAGES);
$resolveImgPath = static function (string $filename) use ($imagesReal): ?string {
    if ($imagesReal === false) {
        return null; // images dir does not exist yet.
    }
    $path = WAGO_CUSTOM_IMAGES . '/' . $filename;
    if (is_link($path) || !is_file($path) || !is_readable($path)) {
        return null;
    }
    $real = realpath($path);
    return ($real !== false && str_starts_with($real, $imagesReal . '/')) ? $path : null;
};

// Load, validate, and return the configuration from WAGO_CUSTOM_MAP.
//
// Schema (v1.1):
//   controllers  object keyed by SNMP_ID string (starts with digit; alphanumeric/slash/hyphen)
//   modules      object keyed by 1-based slot string ("1", "2", …)
//   Each entry:  {"SNMP_ID":"…","name":"…","img":"filename.svg","description":"…"}
//
// Controller matching: first entry whose SNMP_ID === live layout.controller (strict equality).
// Module matching:     if total configured count ≠ live count → ALL modules fall back.
//                      Otherwise: entry at key=slot whose SNMP_ID === live article.
//
// Any absent/malformed/unsafe entry is silently skipped; falls back to generic built-in SVGs.
$loadConfigMap = static function () use ($validFilename, $resolveImgPath): array {
    $result = ['controllers' => [], 'modules' => []];

    $raw = @file_get_contents(WAGO_CUSTOM_MAP);
    if ($raw === false) {
        return $result; // File absent — normal before custom_svg_map.json is seeded.
    }
    $parsed = json_decode($raw, true);
    if (!is_array($parsed)) {
        return $result; // Malformed JSON — fail safe, use fallback SVGs only.
    }

    // Controller key: starts with digit, then digits/letters/slash/hyphen.
    $validCtrlKey = static fn(string $k): bool =>
        (bool) preg_match('/^[0-9][0-9a-zA-Z\/\-]*$/', $k);
    // Module key: positive integer string (1-based slot number, e.g. "1", "2", "19").
    $validModKey = static fn(string $k): bool =>
        (bool) preg_match('/^[1-9][0-9]*$/', $k);

    // Validate and load a single entry object.  Returns null for any missing/wrong-type field
    // or an invalid image filename.  img_path is null when the file is absent/unsafe.
    $loadEntry = static function ($entry) use ($validFilename, $resolveImgPath): ?array {
        if (!is_array($entry)) {
            return null;
        }
        $name   = $entry['name']        ?? null;
        $snmpId = $entry['SNMP_ID']     ?? null;
        $img    = $entry['img']         ?? null;
        $desc   = $entry['description'] ?? null;
        if (!is_string($name) || !is_string($snmpId) || !is_string($img) || !is_string($desc)) {
            return null;
        }
        if (!$validFilename($img)) {
            return null;
        }
        return [
            'SNMP_ID'     => $snmpId,
            'name'        => $name,
            'img'         => $img,
            'img_path'    => $resolveImgPath($img),
            'description' => $desc,
        ];
    };

    if (isset($parsed['controllers']) && is_array($parsed['controllers'])) {
        foreach ($parsed['controllers'] as $key => $entry) {
            if (!is_string($key) || !$validCtrlKey($key)) {
                continue;
            }
            $loaded = $loadEntry($entry);
            if ($loaded !== null) {
                $result['controllers'][$key] = $loaded;
            }
        }
    }
    if (isset($parsed['modules']) && is_array($parsed['modules'])) {
        foreach ($parsed['modules'] as $key => $entry) {
            $key = (string) $key;
            if (!$validModKey($key)) {
                continue;
            }
            $loaded = $loadEntry($entry);
            if ($loaded !== null) {
                $result['modules'][$key] = $loaded;
            }
        }
    }
    return $result;
};

$configMap         = $loadConfigMap();
$configControllers = $configMap['controllers'];
$configModules     = $configMap['modules'];

// Load and sanitize an SVG from an absolute path.
// All paths entering here were either constructed from $img_dir (built-in allowlist)
// or produced by $resolveImgPath (custom files verified inside WAGO_CUSTOM_IMAGES).
// The is_link / is_file / is_readable guards are defence-in-depth.
$loadSvg = static function (string $abspath): string {
    if (is_link($abspath) || !is_file($abspath) || !is_readable($abspath)) {
        return '';
    }
    $content = @file_get_contents($abspath);
    if ($content === false) {
        return '';
    }
    $content = preg_replace('/<script\b[^>]*>.*?<\/script>/is', '', $content);
    $content = preg_replace('/\bon\w+\s*=/i', 'data-removed=', $content);
    $content = preg_replace('/\bjavascript:/i', 'removed:', $content);
    return $content;
};

// Locate the configured controller entry whose SNMP_ID strictly equals the live article.
// Returns the entry array on match, null when unrecognised or article is absent.
$resolveController = static function (?string $liveArticle) use ($configControllers): ?array {
    if ($liveArticle === null) {
        return null;
    }
    foreach ($configControllers as $entry) {
        if ($entry['SNMP_ID'] === $liveArticle) {
            return $entry;
        }
    }
    return null;
};

// Append a usage hint to a tooltip when no configured entry covers a slot, when the
// entry's img is missing/unsafe, or when it resolves to the generic fallback image.
// Rendered only inside the CSS hover tooltip — not visible on the main rail.
$moduleHint = static function (int $slot, string $liveSnmpId) use ($e): string {
    if ($liveSnmpId === '') {
        return '';
    }
    return '<br>To customise: copy .svg to /var/lib/zabbix/wago_kbus/images/'
        . ' and update ' . $e('modules["' . $slot . '"]')
        . ' in custom_svg_map.json with SNMP_ID ' . $e('"' . $liveSnmpId . '"') . '.';
};

$controllerHint = static function (string $liveSnmpId) use ($e): string {
    if ($liveSnmpId === '') {
        return '';
    }
    return '<br>To customise: copy .svg to /var/lib/zabbix/wago_kbus/images/'
        . ' and add/update a controllers entry with SNMP_ID ' . $e('"' . $liveSnmpId . '"')
        . ' in custom_svg_map.json.';
};

ob_start();
echo '<div class="wago-kbus-panel">';

if ($data['error'] !== null) {
    echo '<div class="wago-kbus-error">' . $e($data['error']) . '</div>';
} else {
    $layout            = $data['layout'];
    $modules           = $layout['modules'] ?? [];
    $controllerArticle = isset($layout['controller']) ? (string) $layout['controller'] : null;

    echo '<div class="wago-kbus-rail">';

    // Controller — always first on the rail.
    $ctrlEntry = $resolveController($controllerArticle);
    if ($ctrlEntry !== null) {
        $ctrlSvgPath = $ctrlEntry['img_path'] ?? $SVG_FALLBACK_CONTROLLER;
        $ctrlLabel   = $ctrlEntry['name'] . ' (Controller)';
        $ctrlTooltip = $e($ctrlLabel);
        if ($ctrlEntry['description'] !== '') {
            $ctrlTooltip .= ': ' . $e($ctrlEntry['description']);
        }
        if ($ctrlEntry['img_path'] === null || $ctrlEntry['img'] === $fallbackCtrlImg) {
            $ctrlTooltip .= $controllerHint($ctrlEntry['SNMP_ID']);
        }
    } else {
        $ctrlSvgPath = $SVG_FALLBACK_CONTROLLER;
        $ctrlLabel   = $controllerArticle !== null
            ? $controllerArticle . ' (Controller)'
            : 'Controller (model unknown — wioArticleName not collected)';
        $ctrlTooltip = $e($ctrlLabel);
        if ($controllerArticle !== null) {
            $ctrlTooltip .= $controllerHint($controllerArticle);
        }
    }
    $ctrlSvg = $loadSvg($ctrlSvgPath);
    if ($ctrlSvg !== '') {
        $b64 = base64_encode($ctrlSvg);
        echo '<div class="wago-kbus-item" role="img" tabindex="0" aria-expanded="false" aria-label="' . $e($ctrlLabel) . '">';
        echo '<img src="data:image/svg+xml;base64,' . $b64 . '" alt="' . $e($ctrlLabel) . '">';
        echo '<div class="wago-kbus-tooltip" role="tooltip">' . $ctrlTooltip . '</div>';
        echo '</div>';
    }

    // K-bus modules in numeric slot order (already sorted by the preprocessing item).
    // If configured module count differs from live count, ALL modules use the generic fallback.
    // The controller is resolved independently of this count check.
    $allFallback = (count($configModules) !== count($modules));

    foreach ($modules as $module) {
        $article   = (string) ($module['article'] ?? '');
        $slot      = (int)    ($module['slot']    ?? 0);
        $type      = $module['type'] !== null ? (int) $module['type'] : null;
        $typeLabel = $type !== null ? ' (type ' . $type . ')' : '';

        $matchedEntry = null;
        if (!$allFallback) {
            $key = (string) $slot;
            if (isset($configModules[$key]) && $configModules[$key]['SNMP_ID'] === $article) {
                $matchedEntry = $configModules[$key];
            }
        }

        if ($matchedEntry !== null) {
            $svgPath   = $matchedEntry['img_path'] ?? $SVG_FALLBACK_MODULE;
            $ariaLabel = 'Slot ' . $slot . ': ' . $matchedEntry['name'];
            $tooltip   = $e('Slot ' . $slot . ': ' . $matchedEntry['name']);
            if ($matchedEntry['description'] !== '') {
                $tooltip .= ' — ' . $e($matchedEntry['description']);
            }
            $tooltip .= $e($typeLabel);
            if ($matchedEntry['img_path'] === null || $matchedEntry['img'] === $fallbackModImg) {
                $tooltip .= $moduleHint($slot, $article);
            }
        } else {
            $svgPath   = $SVG_FALLBACK_MODULE;
            $ariaLabel = 'Slot ' . $slot . ': ' . $article;
            $tooltip   = $e($article) . $e($typeLabel);
            $tooltip  .= $moduleHint($slot, $article);
        }

        $svgContent = $loadSvg($svgPath);
        if ($svgContent !== '') {
            $b64 = base64_encode($svgContent);
            echo '<div class="wago-kbus-item" role="img" tabindex="0" aria-expanded="false" aria-label="' . $e($ariaLabel) . '">';
            echo '<img src="data:image/svg+xml;base64,' . $b64 . '" alt="' . $e($ariaLabel) . '">';
            echo '<div class="wago-kbus-tooltip" role="tooltip">' . $tooltip . '</div>';
            echo '</div>';
        }
    }

    echo '</div>'; // .wago-kbus-rail

    if ($data['clock'] > 0) {
        echo '<div class="wago-kbus-meta">Layout as of ' . $e(date('H:i', $data['clock'])) . '</div>';
    }
}

echo '</div>'; // .wago-kbus-panel
(new CWidgetView($data))->addItem(ob_get_clean())->show();
