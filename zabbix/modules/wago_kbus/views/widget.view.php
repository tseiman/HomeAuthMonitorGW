<?php
// HTML-escape helper.
$e = static fn($v) => htmlspecialchars((string) $v, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');

// Persistent site-local catalog and SVG data. Widget updates never overwrite these paths.
define('WAGO_CUSTOM_BASE', '/var/lib/zabbix/wago_kbus');
define('WAGO_CUSTOM_IMAGES', '/var/lib/zabbix/wago_kbus/images');
define('WAGO_CUSTOM_MAP', '/var/lib/zabbix/wago_kbus/custom_svg_map.json');

$imgDir = __DIR__ . '/../assets/img/';
$SVG_FALLBACK_CONTROLLER = $imgDir . 'wago_0750-xxxx_controller.svg';
$SVG_FALLBACK_MODULE = $imgDir . 'wago_0750-xxxx_modul.svg';
$SVG_END_MODULE = $imgDir . 'wago_0750-0600.svg';
$fallbackCtrlImg = basename($SVG_FALLBACK_CONTROLLER);
$fallbackModImg = basename($SVG_FALLBACK_MODULE);

$validFilename = static fn(string $filename): bool =>
    (bool) preg_match('/^[a-zA-Z0-9][a-zA-Z0-9_\-]*\.svg$/', $filename);
$validCatalogKey = static fn(string $key): bool =>
    (bool) preg_match('/^[0-9][0-9a-zA-Z\/\-]*$/', $key);

$imagesReal = realpath(WAGO_CUSTOM_IMAGES);
$resolveImgPath = static function (string $filename) use ($imagesReal): ?string {
    if ($imagesReal === false) {
        return null;
    }
    $path = WAGO_CUSTOM_IMAGES . '/' . $filename;
    if (is_link($path) || !is_file($path) || !is_readable($path)) {
        return null;
    }
    $real = realpath($path);
    return ($real !== false && str_starts_with($real, $imagesReal . '/')) ? $path : null;
};

// Schema v2.0:
// - controllers are keyed by exact controller article;
// - modules are a reusable article catalog keyed by entry name, never by physical slot;
// - module entries require type and a complete four-field process-image bit signature.
$loadConfigMap = static function () use ($validFilename, $validCatalogKey, $resolveImgPath): array {
    $result = ['controllers' => [], 'modules' => []];
    $raw = @file_get_contents(WAGO_CUSTOM_MAP);
    if ($raw === false) {
        return $result;
    }
    $parsed = json_decode($raw, true);
    if (!is_array($parsed)) {
        return $result;
    }

    $loadCommon = static function ($entry) use ($validFilename, $resolveImgPath): ?array {
        if (!is_array($entry)) {
            return null;
        }
        $name = $entry['name'] ?? null;
        $snmpId = $entry['SNMP_ID'] ?? null;
        $img = $entry['img'] ?? null;
        $description = $entry['description'] ?? null;
        if (!is_string($name) || !is_string($snmpId) || !is_string($img) || !is_string($description)) {
            return null;
        }
        if ($name === '' || $snmpId === '' || !$validFilename($img)) {
            return null;
        }
        return [
            'SNMP_ID' => $snmpId,
            'name' => $name,
            'img' => $img,
            'img_path' => $resolveImgPath($img),
            'description' => $description
        ];
    };

    if (isset($parsed['controllers']) && is_array($parsed['controllers'])) {
        foreach ($parsed['controllers'] as $key => $entry) {
            if (!is_string($key) || !$validCatalogKey($key)) {
                continue;
            }
            $loaded = $loadCommon($entry);
            if ($loaded !== null) {
                $result['controllers'][$key] = $loaded;
            }
        }
    }

    if (isset($parsed['modules']) && is_array($parsed['modules'])) {
        foreach ($parsed['modules'] as $key => $entry) {
            $key = (string) $key;
            if (!$validCatalogKey($key)) {
                continue;
            }
            $loaded = $loadCommon($entry);
            if ($loaded === null || $key !== $loaded['name']) {
                continue; // Reject legacy slot keys and misleading aliases.
            }
            $type = $entry['type'] ?? null;
            $processImage = $entry['process_image'] ?? null;
            if (!is_int($type) || $type < 0 || !is_array($processImage)) {
                continue;
            }
            $signature = [];
            $validSignature = true;
            foreach (['analog_in', 'analog_out', 'digital_in', 'digital_out'] as $field) {
                $bits = $processImage[$field] ?? null;
                if (!is_int($bits) || $bits < 0) {
                    $validSignature = false;
                    break;
                }
                $signature[$field] = $bits;
            }
            if (!$validSignature) {
                continue;
            }
            $loaded['type'] = $type;
            $loaded['process_image'] = $signature;
            $result['modules'][$key] = $loaded;
        }
    }
    return $result;
};

$configMap = $loadConfigMap();
$configControllers = $configMap['controllers'];
$moduleCatalog = $configMap['modules'];

$loadSvg = static function (string $path): string {
    if (is_link($path) || !is_file($path) || !is_readable($path)) {
        return '';
    }
    $content = @file_get_contents($path);
    if ($content === false) {
        return '';
    }
    $content = preg_replace('/<script\b[^>]*>.*?<\/script>/is', '', $content);
    $content = preg_replace('/\bon\w+\s*=/i', 'data-removed=', $content);
    return preg_replace('/\bjavascript:/i', 'removed:', $content);
};

$resolveController = static function (?string $liveArticle) use ($configControllers): ?array {
    if ($liveArticle === null) {
        return null;
    }
    $matches = [];
    foreach ($configControllers as $entry) {
        if ($entry['SNMP_ID'] === $liveArticle) {
            $matches[] = $entry;
        }
    }
    if (count($matches) !== 1) {
        return null;
    }
    return reset($matches);
};

// Filter candidates by every available live attribute. Missing process-image fields are
// unknown and are never converted to zero. An exact article is selected only when one
// candidate remains. Structural inventory defects take precedence over identification.
$identifyModule = static function (array $module, bool $inventoryValid) use ($moduleCatalog): array {
    if (!$inventoryValid || empty($module['inventory_complete'])) {
        return ['state' => 'data-incomplete', 'entry' => null, 'candidates' => []];
    }
    $article = (string) $module['article'];
    $type = (int) $module['type'];
    $candidates = array_filter(
        $moduleCatalog,
        static fn(array $entry): bool => $entry['SNMP_ID'] === $article && $entry['type'] === $type
    );
    if (!$candidates) {
        return ['state' => 'unknown', 'entry' => null, 'candidates' => []];
    }

    $liveProcessImage = is_array($module['process_image'] ?? null) ? $module['process_image'] : [];
    foreach (['analog_in', 'analog_out', 'digital_in', 'digital_out'] as $field) {
        $liveBits = $liveProcessImage[$field] ?? null;
        if ($liveBits === null) {
            continue;
        }
        $candidates = array_filter(
            $candidates,
            static fn(array $entry): bool => $entry['process_image'][$field] === (int) $liveBits
        );
        if (!$candidates) {
            return ['state' => 'unknown', 'entry' => null, 'candidates' => []];
        }
    }

    if (count($candidates) === 1) {
        return ['state' => 'identified', 'entry' => reset($candidates), 'candidates' => array_keys($candidates)];
    }
    $names = array_keys($candidates);
    sort($names, SORT_NATURAL);
    return ['state' => 'ambiguous', 'entry' => null, 'candidates' => $names];
};

$moduleCatalogHint = static function (string $liveSnmpId) use ($e): string {
    if ($liveSnmpId === '') {
        return '';
    }
    return '<br>To add support: copy the SVG to /var/lib/zabbix/wago_kbus/images/'
        . ' and add one article entry to custom_svg_map.json for SNMP_ID '
        . $e('"' . $liveSnmpId . '"') . '.';
};
$controllerHint = static function (string $liveSnmpId) use ($e): string {
    if ($liveSnmpId === '') {
        return '';
    }
    return '<br>To add support: copy the SVG to /var/lib/zabbix/wago_kbus/images/'
        . ' and add a controllers entry to custom_svg_map.json for SNMP_ID '
        . $e('"' . $liveSnmpId . '"') . '.';
};
$formatProcessImage = static function ($processImage) use ($e): string {
    if (!is_array($processImage)) {
        return 'Process image: unavailable';
    }
    $labels = ['digital_in' => 'DI', 'digital_out' => 'DO', 'analog_in' => 'AI', 'analog_out' => 'AO'];
    $available = false;
    $parts = [];
    foreach ($labels as $field => $label) {
        $value = $processImage[$field] ?? null;
        if ($value !== null) {
            $available = true;
        }
        $parts[] = $label . ' ' . ($value === null ? 'unknown' : $e((int) $value) . ' bit');
    }
    return $available ? 'Process image: ' . implode(', ', $parts) : 'Process image: unavailable';
};

ob_start();
echo '<div class="wago-kbus-panel">';

if ($data['error'] !== null) {
    echo '<div class="wago-kbus-error">' . $e($data['error']) . '</div>';
} else {
    $layout = $data['layout'];
    $modules = $layout['modules'] ?? [];
    $controllerArticle = isset($layout['controller']) ? (string) $layout['controller'] : null;
    $inventoryValid = ($layout['inventory_valid'] ?? false) === true;
    $errorGroup = isset($layout['error_group']) ? (int) $layout['error_group'] : null;
    $errorCode = isset($layout['error_code']) ? (int) $layout['error_code'] : null;
    $globalFault = ($errorGroup !== null && $errorGroup !== 0) || ($errorCode !== null && $errorCode !== 0);

    if ($globalFault) {
        $faultText = 'K-bus/controller fault';
        if (!empty($layout['error_description'])) {
            $faultText .= ': ' . (string) $layout['error_description'];
        }
        echo '<div class="wago-kbus-global-fault">' . $e($faultText) . '</div>';
    } elseif (!$inventoryValid) {
        $reasons = [];
        if (!empty($layout['count_mismatch'])) $reasons[] = 'reported module count does not match inventory entries';
        if (!empty($layout['duplicate_slots'])) $reasons[] = 'duplicate physical slot numbers';
        if (($layout['inventory_contiguous'] ?? false) !== true) $reasons[] = 'physical slots do not form 1..count';
        if (!$reasons) $reasons[] = 'required module number, name, or type is missing';
        echo '<div class="wago-kbus-inventory-warning">Inventory incomplete: ' . $e(implode('; ', $reasons)) . '</div>';
    }

    echo '<div class="wago-kbus-rail">';

    $ctrlEntry = $resolveController($controllerArticle);
    $controllerIdentified = $ctrlEntry !== null;
    if ($controllerIdentified) {
        $ctrlSvgPath = $ctrlEntry['img_path'] ?? $SVG_FALLBACK_CONTROLLER;
        $ctrlLabel = $ctrlEntry['name'] . ' (Controller)';
        $ctrlTooltip = $e($ctrlLabel);
        if ($ctrlEntry['description'] !== '') {
            $ctrlTooltip .= ': ' . $e($ctrlEntry['description']);
        }
        if ($ctrlEntry['img_path'] === null || $ctrlEntry['img'] === $fallbackCtrlImg) {
            $ctrlTooltip .= $controllerHint($ctrlEntry['SNMP_ID']);
        }
    } else {
        $ctrlSvgPath = $SVG_FALLBACK_CONTROLLER;
        $ctrlLabel = $controllerArticle !== null
            ? $controllerArticle . ' (Controller)'
            : 'Controller (model unavailable)';
        $ctrlTooltip = $e($ctrlLabel);
        if ($controllerArticle !== null) {
            $ctrlTooltip .= $controllerHint($controllerArticle);
        }
    }
    if ($globalFault) {
        $ctrlTooltip .= '<br>' . $e(
            'K-bus diagnostic: group ' . ($errorGroup ?? 'unknown')
            . ', code ' . ($errorCode ?? 'unknown')
            . (!empty($layout['error_description']) ? ' — ' . $layout['error_description'] : '')
        );
        $ctrlTooltip .= '<br>Error argument is shown globally and is not interpreted as a slot.';
    }
    $ctrlSvg = $loadSvg($ctrlSvgPath);
    if ($ctrlSvg !== '') {
        $ctrlClass = 'wago-kbus-item wago-kbus-item--identified' . ($globalFault ? ' wago-kbus-item--fault' : '');
        echo '<div class="' . $ctrlClass . '" role="img" tabindex="0" aria-expanded="false" aria-label="' . $e($ctrlLabel) . '">';
        echo '<img src="data:image/svg+xml;base64,' . base64_encode($ctrlSvg) . '" alt="' . $e($ctrlLabel) . '">';
        echo '<div class="wago-kbus-tooltip" role="tooltip">' . $ctrlTooltip . '</div>';
        echo '</div>';
    }

    foreach ($modules as $module) {
        if (!is_array($module)) {
            continue;
        }
        $slot = isset($module['slot']) ? (int) $module['slot'] : 0;
        $slotLabel = $slot > 0 ? (string) $slot : '?';
        $article = isset($module['article']) ? (string) $module['article'] : '';
        $type = isset($module['type']) ? (int) $module['type'] : null;
        $resolution = $identifyModule($module, $inventoryValid);
        $state = $resolution['state'];
        $entry = $resolution['entry'];
        $svgPath = $SVG_FALLBACK_MODULE;
        $ariaLabel = 'Slot ' . $slotLabel . ': ' . ($article !== '' ? $article : 'inventory incomplete');

        if ($state === 'identified' && $entry !== null) {
            $svgPath = $entry['img_path'] ?? $SVG_FALLBACK_MODULE;
            $ariaLabel = 'Slot ' . $slotLabel . ': ' . $entry['name'];
            $tooltip = $e('Slot ' . $slotLabel . ': ' . $entry['name']);
            if ($entry['description'] !== '') {
                $tooltip .= ' — ' . $e($entry['description']);
            }
            $tooltip .= '<br>' . $e('SNMP: ' . $article . ' (type ' . $type . ')');
            $tooltip .= '<br>' . $formatProcessImage($module['process_image'] ?? null);
            $tooltip .= '<br>Identification: automatic / unique';
            if ($entry['img_path'] === null || $entry['img'] === $fallbackModImg) {
                $tooltip .= '<br>Catalog image unavailable; generic image shown.';
            }
        } else {
            $tooltip = $e('Slot ' . $slotLabel);
            $tooltip .= '<br>' . $e('SNMP: ' . ($article !== '' ? $article : 'unavailable')
                . ($type !== null ? ' (type ' . $type . ')' : ' (type unavailable)'));
            $tooltip .= '<br>' . $formatProcessImage($module['process_image'] ?? null);
            if ($state === 'ambiguous') {
                $tooltip .= '<br>Exact module: ambiguous';
                $tooltip .= '<br>' . $e('Candidates: ' . implode(', ', $resolution['candidates']));
            } elseif ($state === 'unknown') {
                $tooltip .= '<br>Module: unknown';
                $tooltip .= '<br>No matching module in catalog';
                $tooltip .= $moduleCatalogHint($article);
            } else {
                $tooltip .= '<br>Inventory data incomplete; exact module not evaluated';
            }
        }

        $svgContent = $loadSvg($svgPath);
        if ($svgContent === '') {
            continue;
        }
        $warningClass = $state === 'identified' ? '' : ' wago-kbus-item--warning';
        echo '<div class="wago-kbus-item wago-kbus-item--' . $e($state) . $warningClass
            . '" role="img" tabindex="0" aria-expanded="false" aria-label="' . $e($ariaLabel) . '">';
        echo '<img src="data:image/svg+xml;base64,' . base64_encode($svgContent) . '" alt="' . $e($ariaLabel) . '">';
        echo '<div class="wago-kbus-tooltip" role="tooltip">' . $tooltip . '</div>';
        echo '</div>';
    }

    // Append the passive 750-600 after the runtime inventory. It has no SNMP identity and is
    // deliberately excluded from physical slot counting and article matching.
    if ($controllerIdentified) {
        $endLabel = '750-600 (End module)';
        $endSvg = $loadSvg($SVG_END_MODULE);
        if ($endSvg === '') {
            $endSvg = $loadSvg($SVG_FALLBACK_MODULE);
        }
        if ($endSvg !== '') {
            echo '<div class="wago-kbus-item wago-kbus-item--identified wago-kbus-item--end-module"'
                . ' role="img" aria-label="' . $e($endLabel) . '">';
            echo '<img src="data:image/svg+xml;base64,' . base64_encode($endSvg) . '" alt="">';
            echo '</div>';
        }
    }

    echo '</div>';
    if ($data['clock'] > 0) {
        echo '<div class="wago-kbus-meta">Layout as of ' . $e(date('H:i', $data['clock'])) . '</div>';
    }
}

echo '</div>';
(new CWidgetView($data))->addItem(ob_get_clean())->show();
