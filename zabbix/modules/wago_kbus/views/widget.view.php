<?php
// HTML-escape helper.
$e = static fn($v) => htmlspecialchars((string) $v, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');

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

// Server-side allowlists — the ONLY way a filename reaches file_get_contents().
// Never derived from user input; immune to path traversal.
$SVG_CONTROLLER_MAP = [
    '750-880'  => 'wago_0750-0880.svg',
    '750-0880' => 'wago_0750-0880.svg',
];
$SVG_MODULE_MAP = [
    '750-511' => 'wago_0750-0511.svg',
];
$SVG_FALLBACK_CONTROLLER = 'wago_0750-xxxx_controller.svg';
$SVG_FALLBACK_MODULE     = 'wago_0750-xxxx_modul.svg';

$img_dir = __DIR__ . '/../assets/img/';

// Load an SVG from the img directory.  Strips script elements and inline event handlers
// (defence-in-depth; all files are from our own controlled allowlist).
$loadSvg = static function (string $filename) use ($img_dir): string {
    $content = @file_get_contents($img_dir . $filename);
    if ($content === false) {
        return '';
    }
    $content = preg_replace('/<script\b[^>]*>.*?<\/script>/is', '', $content);
    $content = preg_replace('/\bon\w+\s*=/i', 'data-removed=', $content);
    $content = preg_replace('/\bjavascript:/i', 'removed:', $content);
    return $content;
};

// Resolve a controller article (e.g. "750-880") to an SVG filename.
// Falls back to the generic controller if no exact match exists.
$resolveControllerSvg = static function (?string $article) use ($SVG_CONTROLLER_MAP, $SVG_FALLBACK_CONTROLLER): string {
    if ($article === null) {
        return $SVG_FALLBACK_CONTROLLER;
    }
    $base = trim(preg_replace('/\/.*$/', '', $article));
    // Wildcard patterns ('x', '*') are not valid concrete article numbers.
    if (stripos($base, 'x') !== false || strpos($base, '*') !== false) {
        return $SVG_FALLBACK_CONTROLLER;
    }
    return $SVG_CONTROLLER_MAP[$base] ?? $SVG_FALLBACK_CONTROLLER;
};

// Resolve a module article to an SVG filename.
// Critical rules (req §5):
//   - Wildcard/family identifiers containing 'x' (e.g. 750-5xx, 750-4xx) MUST use fallback.
//   - Concrete identifiers (e.g. 750-511/000-002) strip the variant suffix before lookup.
//   - Only filenames explicitly present in $SVG_MODULE_MAP are served.
$resolveModuleSvg = static function (string $article) use ($SVG_MODULE_MAP, $SVG_FALLBACK_MODULE): string {
    $article = trim($article);
    // Wildcard/family patterns must never resolve to a specific product SVG.
    if (stripos($article, 'x') !== false || strpos($article, '*') !== false) {
        return $SVG_FALLBACK_MODULE;
    }
    $base = trim(preg_replace('/\/.*$/', '', $article));
    return $SVG_MODULE_MAP[$base] ?? $SVG_FALLBACK_MODULE;
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
    $layout  = $data['layout'];
    $modules = $layout['modules'] ?? [];
    $controllerArticle = isset($layout['controller']) ? (string) $layout['controller'] : null;

    echo '<div class="wago-kbus-rail">';

    // Controller — always first on the rail.
    $ctrlFile    = $resolveControllerSvg($controllerArticle);
    $ctrlSvg     = $loadSvg($ctrlFile);
    $ctrlDisplay = $controllerArticle !== null
        ? $controllerArticle . ' (Controller)'
        : 'Controller (model unknown — wioArticleName not collected)';
    if ($ctrlSvg !== '') {
        $b64 = base64_encode($ctrlSvg);
        echo '<div class="wago-kbus-item" role="img" aria-label="' . $e($ctrlDisplay) . '">';
        echo '<img src="data:image/svg+xml;base64,' . $b64 . '" alt="' . $e($ctrlDisplay) . '">';
        echo '<div class="wago-kbus-tooltip" role="tooltip">' . $e($ctrlDisplay) . '</div>';
        echo '</div>';
    }

    // K-bus modules in numeric slot order (already sorted by the preprocessing item).
    foreach ($modules as $module) {
        $article = (string) ($module['article'] ?? '');
        $slot    = (int)    ($module['slot']    ?? 0);
        $type    = $module['type'] !== null ? (int) $module['type'] : null;

        $desc       = $moduleDescription($article);
        $typeLabel  = $type !== null ? ' (type ' . $type . ')' : '';
        $ariaLabel  = 'Slot ' . $slot . ': ' . $article;
        $tooltip    = $e($article)
            . ($desc !== '' ? ' — ' . $e($desc) : '')
            . $e($typeLabel);

        $svgFile    = $resolveModuleSvg($article);
        $svgContent = $loadSvg($svgFile);
        if ($svgContent !== '') {
            $b64 = base64_encode($svgContent);
            echo '<div class="wago-kbus-item" role="img" aria-label="' . $e($ariaLabel) . '">';
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
