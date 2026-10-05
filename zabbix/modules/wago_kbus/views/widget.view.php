<?php
// HTML-escape helper.
$e = static fn($v) => htmlspecialchars((string) $v, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');

// Persistent site-local custom SVG data.
// /var/lib is the FHS location for variable state owned by applications — mutable, persistent,
// and outside the package-managed module tree at /usr/share/zabbix/modules/.  Storing custom
// widget data here means normal widget updates (install_widget.sh) never touch it.
define('WAGO_CUSTOM_BASE',   '/var/lib/zabbix/wago_kbus');
define('WAGO_CUSTOM_IMAGES', '/var/lib/zabbix/wago_kbus/images');
define('WAGO_CUSTOM_MAP',    '/var/lib/zabbix/wago_kbus/custom_svg_map.json');

// Built-in module descriptions.  Users maintain these in the widget source rather than as a
// Zabbix macro so that the full table stays readable without template re-import.
// Minimum preserved: 750-652/000-000 = Serial interface (req §7).
$WAGO_DESCRIPTIONS = [
    '750-652/000-000' => 'Serial interface',
    '750-511'         => 'Serial interface RS232/RS485',
    '750-511/000-002' => 'Serial interface RS232/RS485',
    '750-504'         => '4-channel digital output (0.5 A, 24 V DC)',
    '750-508'         => '4-channel digital input (24 V DC)',
    '750-400'         => '2-channel digital input (24 V DC)',
    '750-402'         => '4-channel digital input (24 V DC)',
    '750-403'         => '8-channel digital input (24 V DC)',
    '750-405'         => '4-channel digital input (5 V DC)',
    '750-410'         => '2-channel digital input (230 V AC)',
    '750-430'         => '8-channel digital input (24 V DC)',
    '750-436'         => '8-channel digital input (24 V DC, 0.2 ms)',
    '750-501'         => '2-channel digital output (24 V DC, 0.5 A)',
    '750-502'         => '2-channel digital output (24 V DC, 2 A)',
    '750-516'         => '2-channel digital output (24 V DC, 0.5 A)',
    '750-530'         => '8-channel digital output (24 V DC, 0.5 A)',
    '750-455'         => '4-channel analog input (0–10 V)',
    '750-467'         => '2-channel PT100/PT1000 input',
    '750-469'         => '2-channel thermocouple input',
    '750-478'         => '4-channel analog input (0/4–20 mA)',
    '750-479'         => '2-channel analog input (±10 V)',
    '750-495'         => '4-channel analog input (0–10 V)',
    '750-562'         => '2-channel analog output (0–20 mA)',
    '750-600'         => 'Bus end module',
    '750-610'         => 'Power supply module (24 V DC)',
    '750-614'         => 'Power supply module (5 V DC)',
    '750-616'         => 'Power supply module (24 V DC, 10 A)',
    '750-638'         => '2-channel absolute encoder input (SSI)',
];

// Built-in SVG maps: article → relative filename in assets/img/.
// Entries are expanded to absolute paths below so $loadSvg takes a single resolved path.
$SVG_CONTROLLER_MAP = [
    '750-880'  => 'wago_0750-0880.svg',
    '750-0880' => 'wago_0750-0880.svg',
];
$SVG_MODULE_MAP = [
    '750-511' => 'wago_0750-0511.svg',
];

$img_dir                 = __DIR__ . '/../assets/img/';
$SVG_FALLBACK_CONTROLLER = $img_dir . 'wago_0750-xxxx_controller.svg';
$SVG_FALLBACK_MODULE     = $img_dir . 'wago_0750-xxxx_modul.svg';

// Expand built-in maps to absolute paths.
foreach ($SVG_CONTROLLER_MAP as $k => $v) {
    $SVG_CONTROLLER_MAP[$k] = $img_dir . $v;
}
foreach ($SVG_MODULE_MAP as $k => $v) {
    $SVG_MODULE_MAP[$k] = $img_dir . $v;
}

// Merge custom SVG entries from WAGO_CUSTOM_MAP into the built-in maps.
// Custom entries override built-in entries with the same key.
// Any absent/malformed/unsafe entry is silently skipped; falls back to built-ins.
$mergeCustomMaps = static function () use (&$SVG_CONTROLLER_MAP, &$SVG_MODULE_MAP): void {
    $raw = @file_get_contents(WAGO_CUSTOM_MAP);
    if ($raw === false) {
        return; // File absent — normal before any custom assets are configured.
    }
    $parsed = json_decode($raw, true);
    if (!is_array($parsed)) {
        return; // Malformed JSON — fail safe, use built-ins only.
    }

    // Valid key: starts with digit, then digits/letters/forward-slash/hyphen only.
    // Covers concrete articles (750-511), variants (750-511/000-002), generics (750-5xx).
    $validKey = static fn(string $k): bool =>
        (bool) preg_match('/^[0-9][0-9a-zA-Z\/\-]*$/', $k);

    // Valid filename: basename only, alphanumeric/underscore/hyphen, .svg suffix.
    // No path separators — prevents any traversal attempt at the filename level.
    $validFilename = static fn(string $f): bool =>
        (bool) preg_match('/^[a-zA-Z0-9][a-zA-Z0-9_\-]*\.svg$/', $f);

    // Is the custom file safe?  Non-symlink, readable regular file whose realpath
    // stays strictly inside WAGO_CUSTOM_IMAGES.
    $imagesReal = realpath(WAGO_CUSTOM_IMAGES);
    $validFile = static function (string $filename) use ($imagesReal): bool {
        if ($imagesReal === false) {
            return false; // WAGO_CUSTOM_IMAGES does not exist.
        }
        $path = WAGO_CUSTOM_IMAGES . '/' . $filename;
        if (is_link($path)) {
            return false; // Reject symlinks — would allow escaping the images dir.
        }
        if (!is_file($path) || !is_readable($path)) {
            return false;
        }
        $real = realpath($path);
        // realpath must be inside images dir (defense-in-depth against bind-mount tricks).
        return $real !== false && str_starts_with($real, $imagesReal . '/');
    };

    foreach (['controllers', 'modules'] as $section) {
        if (!isset($parsed[$section]) || !is_array($parsed[$section])) {
            continue;
        }
        foreach ($parsed[$section] as $key => $filename) {
            if (!is_string($key) || !$validKey($key)) {
                continue;
            }
            if (!is_string($filename) || !$validFilename($filename)) {
                continue;
            }
            if (!$validFile($filename)) {
                continue;
            }
            $absPath = WAGO_CUSTOM_IMAGES . '/' . $filename;
            if ($section === 'controllers') {
                $SVG_CONTROLLER_MAP[$key] = $absPath;
            } else {
                $SVG_MODULE_MAP[$key] = $absPath;
            }
        }
    }
};
$mergeCustomMaps();

// Load and sanitize an SVG from an absolute path.
// All paths entering here were either constructed from $img_dir (built-in allowlist)
// or validated by $mergeCustomMaps (custom files verified inside WAGO_CUSTOM_IMAGES).
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

// Resolve a controller article to an SVG path.
// Returns ['path' => string, 'is_fallback' => bool, 'norm_key' => string].
$resolveControllerSvg = static function (?string $article) use ($SVG_CONTROLLER_MAP, $SVG_FALLBACK_CONTROLLER): array {
    if ($article === null) {
        return ['path' => $SVG_FALLBACK_CONTROLLER, 'is_fallback' => true, 'norm_key' => ''];
    }
    $base = trim(preg_replace('/\/.*$/', '', $article));
    if (stripos($base, 'x') !== false || strpos($base, '*') !== false) {
        return ['path' => $SVG_FALLBACK_CONTROLLER, 'is_fallback' => true, 'norm_key' => $base];
    }
    if (isset($SVG_CONTROLLER_MAP[$base])) {
        return ['path' => $SVG_CONTROLLER_MAP[$base], 'is_fallback' => false, 'norm_key' => $base];
    }
    return ['path' => $SVG_FALLBACK_CONTROLLER, 'is_fallback' => true, 'norm_key' => $base];
};

// Resolve a module article to an SVG path.
// Returns ['path' => string, 'is_fallback' => bool, 'norm_key' => string].
//
// Resolution steps:
//   1. Exact key — allows an explicit custom mapping for generic articles (e.g. '750-5xx').
//   2. Wildcard/family patterns without an explicit entry — always fallback.
//      Guarantees '750-5xx' never accidentally resolves to concrete '750-511'.
//   3. Concrete variant: strip '/...' suffix, look up base article
//      (e.g. '750-511/000-002' → '750-511').
$resolveModuleSvg = static function (string $article) use ($SVG_MODULE_MAP, $SVG_FALLBACK_MODULE): array {
    $article = trim($article);
    // Step 1: exact key match (supports deliberate generic-key custom mappings).
    if (isset($SVG_MODULE_MAP[$article])) {
        return ['path' => $SVG_MODULE_MAP[$article], 'is_fallback' => false, 'norm_key' => $article];
    }
    // Step 2: wildcard/family patterns without explicit entry → always fallback.
    if (stripos($article, 'x') !== false || strpos($article, '*') !== false) {
        return ['path' => $SVG_FALLBACK_MODULE, 'is_fallback' => true, 'norm_key' => $article];
    }
    // Step 3: concrete variant — strip variant suffix and look up base.
    $base = trim(preg_replace('/\/.*$/', '', $article));
    if (isset($SVG_MODULE_MAP[$base])) {
        return ['path' => $SVG_MODULE_MAP[$base], 'is_fallback' => false, 'norm_key' => $base];
    }
    return ['path' => $SVG_FALLBACK_MODULE, 'is_fallback' => true, 'norm_key' => $base];
};

// Append a usage hint to a tooltip when the fallback SVG is used.
// Rendered only inside the CSS hover tooltip — not visible on the main rail.
$fallbackHint = static function (string $normKey, string $section) use ($e): string {
    if ($normKey === '') {
        return ''; // No article known; nothing to hint at.
    }
    return '<br>No specific SVG for ' . $e('"' . $normKey . '"') . '.'
        . ' Copy .svg to /var/lib/zabbix/wago_kbus/images/'
        . ' and add key ' . $e('"' . $normKey . '"')
        . ' to custom_svg_map.json (' . $e($section) . ').';
};

// Look up a human-readable description for a module article.
// Tries the full article first (e.g. 750-511/000-002), then the base (e.g. 750-511).
$moduleDescription = static function (string $article) use ($WAGO_DESCRIPTIONS): string {
    $article = trim($article);
    if (isset($WAGO_DESCRIPTIONS[$article])) {
        return $WAGO_DESCRIPTIONS[$article];
    }
    $base = trim(preg_replace('/\/.*$/', '', $article));
    return $WAGO_DESCRIPTIONS[$base] ?? '';
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
    $ctrlResult  = $resolveControllerSvg($controllerArticle);
    $ctrlSvg     = $loadSvg($ctrlResult['path']);
    $ctrlDisplay = $controllerArticle !== null
        ? $controllerArticle . ' (Controller)'
        : 'Controller (model unknown — wioArticleName not collected)';
    $ctrlHint    = $ctrlResult['is_fallback'] ? $fallbackHint($ctrlResult['norm_key'], 'controllers') : '';
    if ($ctrlSvg !== '') {
        $b64 = base64_encode($ctrlSvg);
        echo '<div class="wago-kbus-item" role="img" aria-label="' . $e($ctrlDisplay) . '">';
        echo '<img src="data:image/svg+xml;base64,' . $b64 . '" alt="' . $e($ctrlDisplay) . '">';
        echo '<div class="wago-kbus-tooltip" role="tooltip">' . $e($ctrlDisplay) . $ctrlHint . '</div>';
        echo '</div>';
    }

    // K-bus modules in numeric slot order (already sorted by the preprocessing item).
    foreach ($modules as $module) {
        $article = (string) ($module['article'] ?? '');
        $slot    = (int)    ($module['slot']    ?? 0);
        $type    = $module['type'] !== null ? (int) $module['type'] : null;

        $desc      = $moduleDescription($article);
        $typeLabel = $type !== null ? ' (type ' . $type . ')' : '';
        $ariaLabel = 'Slot ' . $slot . ': ' . $article;
        $tooltip   = $e($article)
            . ($desc !== '' ? ' — ' . $e($desc) : '')
            . $e($typeLabel);

        $modResult  = $resolveModuleSvg($article);
        $svgContent = $loadSvg($modResult['path']);
        $modHint    = $modResult['is_fallback'] ? $fallbackHint($modResult['norm_key'], 'modules') : '';

        if ($svgContent !== '') {
            $b64 = base64_encode($svgContent);
            echo '<div class="wago-kbus-item" role="img" aria-label="' . $e($ariaLabel) . '">';
            echo '<img src="data:image/svg+xml;base64,' . $b64 . '" alt="' . $e($ariaLabel) . '">';
            echo '<div class="wago-kbus-tooltip" role="tooltip">' . $tooltip . $modHint . '</div>';
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
