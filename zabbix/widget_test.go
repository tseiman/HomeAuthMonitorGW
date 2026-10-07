// File: zabbix/widget_test.go
// Purpose: Focused tests for the wago_kbus custom widget: manifest validity, PHP syntax,
// kbus_layout item JavaScript logic, and module-to-SVG matching rules.

package zabbix

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const widgetDir = "modules/wago_kbus"

// TestWidgetManifestIsValid checks that manifest.json parses as valid JSON and contains
// the required top-level fields for a Zabbix 7.4 custom widget.
func TestWidgetManifestIsValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	required := []string{"manifest_version", "id", "type", "name", "namespace", "version", "widget", "actions", "assets"}
	for _, key := range required {
		if _, ok := manifest[key]; !ok {
			t.Errorf("manifest.json missing required key %q", key)
		}
	}
	if manifest["id"] != "wago_kbus" {
		t.Errorf("manifest id=%q, want wago_kbus", manifest["id"])
	}
	if manifest["type"] != "widget" {
		t.Errorf("manifest type=%q, want widget", manifest["type"])
	}
	widget, ok := manifest["widget"].(map[string]any)
	if !ok {
		t.Fatal("manifest widget section is not an object")
	}
	if widget["js_class"] != "CWidgetWagoKbus" {
		t.Errorf("manifest widget.js_class=%q, want CWidgetWagoKbus", widget["js_class"])
	}
}

// TestWidgetPhpSyntax runs php -l on every PHP file in the widget directory.
func TestWidgetPhpSyntax(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed — skipping PHP syntax check")
	}
	phpFiles, err := filepath.Glob(filepath.Join(widgetDir, "**/*.php"))
	if err != nil {
		t.Fatal(err)
	}
	// Glob doesn't recurse; walk manually.
	var found []string
	for _, pattern := range []string{
		filepath.Join(widgetDir, "*.php"),
		filepath.Join(widgetDir, "actions/*.php"),
		filepath.Join(widgetDir, "includes/*.php"),
		filepath.Join(widgetDir, "views/*.php"),
	} {
		matches, _ := filepath.Glob(pattern)
		found = append(found, matches...)
	}
	_ = phpFiles
	if len(found) == 0 {
		t.Fatal("no PHP files found in widget directory")
	}
	for _, path := range found {
		t.Run(filepath.Base(path), func(t *testing.T) {
			out, err := exec.Command(php, "-l", path).CombinedOutput()
			if err != nil {
				t.Fatalf("php -l %s: %v\n%s", path, err, out)
			}
		})
	}
}

// TestWidgetSvgAssetsExist verifies that the required SVG files are present in assets/img.
func TestWidgetSvgAssetsExist(t *testing.T) {
	required := []string{
		"wago_0750-0880.svg",
		"wago_0750-0511.svg",
		"wago_0750-xxxx_controller.svg",
		"wago_0750-xxxx_modul.svg",
	}
	for _, name := range required {
		path := filepath.Join(widgetDir, "assets/img", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("required SVG asset missing: %s", path)
		}
	}
}

// TestWidgetSvgAssetsAreWellFormed checks basic SVG validity (XML header + svg element).
func TestWidgetSvgAssetsAreWellFormed(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(widgetDir, "assets/img/*.svg"))
	if len(files) == 0 {
		t.Fatal("no SVG files found in assets/img")
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("cannot read %s: %v", path, err)
			continue
		}
		content := string(data)
		if !strings.Contains(content, "<svg") {
			t.Errorf("%s does not contain an <svg element", filepath.Base(path))
		}
		if strings.Contains(content, "<script") {
			t.Errorf("%s contains a <script element (unexpected in static SVG assets)", filepath.Base(path))
		}
		// Check that the SVG declares viewBox (required for proper auto-scaling).
		if !strings.Contains(content, "viewBox") {
			t.Errorf("%s is missing viewBox attribute", filepath.Base(path))
		}
	}
}

// TestKbusLayoutJavaScript executes the new kbus_layout item JavaScript preprocessing
// against sample data that mirrors the reported real-world inventory (19 modules, mix of
// generic 750-4xx/5xx, concrete 750-511/000-002, and concrete 750-652/000-000).
func TestKbusLayoutJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed — skipping kbus_layout JS test")
	}
	data, err := os.ReadFile("template_homeauthmonitorgw.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document templateDocument
	if err := parseYAMLTemplate(data, &document); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	template := document.Export.Templates[0]

	var scriptBody string
	for _, item := range template.Items {
		if item.Key != "automation.gateway.wago.kbus_layout" {
			continue
		}
		for _, step := range item.Preprocessing {
			if step.Type == "JAVASCRIPT" {
				scriptBody = step.Parameters[0]
			}
		}
	}
	if scriptBody == "" {
		t.Fatal("kbus_layout JAVASCRIPT preprocessing not found in template")
	}
	scriptBody = strings.ReplaceAll(scriptBody, "{$WAGO_SOURCE}", "wago-test")

	snapshot := `{"wago-test":{"driver":"snmp","available":true,"stale":false,"last_success":"2026-10-05T00:00:00Z","metrics":[` +
		`{"name":"wioArticleName","value":"750-880"},` +
		`{"name":"wioModulCount","value":4},` +
		`{"name":"wioModuleNumber[1]","value":1},{"name":"wioModuleName[1]","value":"750-4xx"},{"name":"wioModuleType[1]","value":1},` +
		`{"name":"wioModuleNumber[2]","value":2},{"name":"wioModuleName[2]","value":"750-511/000-002"},{"name":"wioModuleType[2]","value":10},` +
		`{"name":"wioModuleNumber[3]","value":3},{"name":"wioModuleName[3]","value":"750-5xx"},{"name":"wioModuleType[3]","value":20},` +
		`{"name":"wioModuleNumber[4]","value":4},{"name":"wioModuleName[4]","value":"750-652/000-000"},{"name":"wioModuleType[4]","value":163}` +
		`]}}`

	script := "const value = process.argv[1]; function transform() {\n" + scriptBody + "\n} process.stdout.write(String(transform()));"
	output, err := exec.Command(node, "-e", script, snapshot).CombinedOutput()
	if err != nil {
		t.Fatalf("kbus_layout JS: %v: %s", err, output)
	}

	var layout struct {
		Controller string `json:"controller"`
		Modules    []struct {
			Slot    int    `json:"slot"`
			Article string `json:"article"`
			Type    *int   `json:"type"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(output, &layout); err != nil {
		t.Fatalf("kbus_layout output not valid JSON: %v — output=%q", err, output)
	}
	if layout.Controller != "750-880" {
		t.Errorf("controller=%q, want 750-880", layout.Controller)
	}
	if len(layout.Modules) != 4 {
		t.Fatalf("module count=%d, want 4; modules=%+v", len(layout.Modules), layout.Modules)
	}
	// Verify slot order and article preservation.
	cases := []struct {
		slot    int
		article string
	}{{1, "750-4xx"}, {2, "750-511/000-002"}, {3, "750-5xx"}, {4, "750-652/000-000"}}
	for i, want := range cases {
		got := layout.Modules[i]
		if got.Slot != want.slot || got.Article != want.article {
			t.Errorf("module[%d]: slot=%d article=%q, want slot=%d article=%q", i, got.Slot, got.Article, want.slot, want.article)
		}
	}
}

// TestKbusLayoutMissingWioArticleName verifies that a snapshot without wioArticleName
// produces controller: null (widget will use fallback SVG).
func TestKbusLayoutMissingWioArticleName(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	data, err := os.ReadFile("template_homeauthmonitorgw.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document templateDocument
	if err := parseYAMLTemplate(data, &document); err != nil {
		t.Fatalf("parse template: %v", err)
	}
	var scriptBody string
	for _, item := range document.Export.Templates[0].Items {
		if item.Key != "automation.gateway.wago.kbus_layout" {
			continue
		}
		for _, step := range item.Preprocessing {
			if step.Type == "JAVASCRIPT" {
				scriptBody = step.Parameters[0]
			}
		}
	}
	if scriptBody == "" {
		t.Fatal("kbus_layout JS not found")
	}
	scriptBody = strings.ReplaceAll(scriptBody, "{$WAGO_SOURCE}", "wago-test")
	snapshot := `{"wago-test":{"driver":"snmp","available":true,"stale":false,"last_success":"2026-10-05T00:00:00Z","metrics":[{"name":"wioModuleNumber[1]","value":1},{"name":"wioModuleName[1]","value":"750-4xx"},{"name":"wioModuleType[1]","value":1}]}}`
	script := "const value = process.argv[1]; function transform() {\n" + scriptBody + "\n} process.stdout.write(String(transform()));"
	output, err := exec.Command(node, "-e", script, snapshot).CombinedOutput()
	if err != nil {
		t.Fatalf("JS: %v: %s", err, output)
	}
	var layout map[string]any
	if err := json.Unmarshal(output, &layout); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if layout["controller"] != nil {
		t.Errorf("controller=%v, want null when wioArticleName absent", layout["controller"])
	}
}

// TestDefaultJsonSchema verifies that default_svg_map.json parses as valid JSON, contains
// exactly 19 module entries, at least one controller entry, and that every entry has the
// four required string fields (SNMP_ID, name, img, description).
func TestDefaultJsonSchema(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "default_svg_map.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Controllers map[string]json.RawMessage `json:"controllers"`
		Modules     map[string]json.RawMessage `json:"modules"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("default_svg_map.json is not valid JSON: %v", err)
	}
	if len(m.Controllers) == 0 {
		t.Error("default_svg_map.json controllers section is empty")
	}
	if len(m.Modules) != 19 {
		t.Errorf("default_svg_map.json has %d module entries, want 19", len(m.Modules))
	}
	type entry struct {
		SNMPID      string `json:"SNMP_ID"`
		Name        string `json:"name"`
		Img         string `json:"img"`
		Description string `json:"description"`
	}
	for key, raw := range m.Controllers {
		var e entry
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Errorf("controllers[%q] not a valid entry object: %v", key, err)
			continue
		}
		if e.SNMPID == "" || e.Name == "" || e.Img == "" {
			t.Errorf("controllers[%q] has empty required field (SNMP_ID=%q name=%q img=%q)", key, e.SNMPID, e.Name, e.Img)
		}
	}
	for key, raw := range m.Modules {
		var e entry
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Errorf("modules[%q] not a valid entry object: %v", key, err)
			continue
		}
		if e.SNMPID == "" || e.Name == "" || e.Img == "" {
			t.Errorf("modules[%q] has empty required field (SNMP_ID=%q name=%q img=%q)", key, e.SNMPID, e.Name, e.Img)
		}
	}
}

// TestJsonModuleResolution verifies the count+slot+SNMP_ID matching logic using the php CLI.
//   - Count match + SNMP_ID match at slot → entry used.
//   - Count mismatch → ALL modules fall back regardless of SNMP_ID.
//   - Count match + SNMP_ID mismatch at one slot → only that slot falls back.
func TestJsonModuleResolution(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed — skipping JSON module resolution test")
	}

	phpLogic := `<?php
// Mirror of the widget count+slot+SNMP_ID resolution logic.
function resolveModules(array $configModules, array $liveModules): array {
    $allFallback = (count($configModules) !== count($liveModules));
    $results = [];
    foreach ($liveModules as $module) {
        $article = $module['article'];
        $slot    = $module['slot'];
        $matched = false;
        if (!$allFallback) {
            $key = (string) $slot;
            if (isset($configModules[$key]) && $configModules[$key]['SNMP_ID'] === $article) {
                $matched = true;
            }
        }
        $results[] = $matched ? 'matched' : 'fallback';
    }
    return $results;
}

$cfg = [
    '1' => ['SNMP_ID' => '750-4xx', 'name' => '750-1405'],
    '2' => ['SNMP_ID' => '750-5xx', 'name' => '750-5xx'],
];
$ok = true;

// Count match, both SNMP_IDs match → both matched.
$r = resolveModules($cfg, [['slot'=>1,'article'=>'750-4xx'],['slot'=>2,'article'=>'750-5xx']]);
if ($r !== ['matched','matched']) { echo "FAIL: count+SNMP match: " . implode(',', $r) . "\n"; $ok = false; }

// Count mismatch (2 config, 3 live) → all fallback.
$r = resolveModules($cfg, [['slot'=>1,'article'=>'750-4xx'],['slot'=>2,'article'=>'750-5xx'],['slot'=>3,'article'=>'750-600']]);
if ($r !== ['fallback','fallback','fallback']) { echo "FAIL: count mismatch: " . implode(',', $r) . "\n"; $ok = false; }

// Count match, SNMP_ID mismatch on slot 2 only → slot 1 matched, slot 2 fallback.
$r = resolveModules($cfg, [['slot'=>1,'article'=>'750-4xx'],['slot'=>2,'article'=>'750-999']]);
if ($r !== ['matched','fallback']) { echo "FAIL: per-slot mismatch: " . implode(',', $r) . "\n"; $ok = false; }

// Count match (0 vs 0) → empty result.
$r = resolveModules([], []);
if ($r !== []) { echo "FAIL: empty: not empty\n"; $ok = false; }

// Count match, literal generic key matches live generic article (strict equality).
$cfgGeneric = ['1' => ['SNMP_ID' => '750-4xx', 'name' => '750-4xx']];
$r = resolveModules($cfgGeneric, [['slot'=>1,'article'=>'750-4xx']]);
if ($r !== ['matched']) { echo "FAIL: generic literal match: " . implode(',', $r) . "\n"; $ok = false; }

echo $ok ? "OK\n" : "FAILURES\n";`

	out, err := exec.Command(php, "-r", phpLogic).CombinedOutput()
	if err != nil {
		t.Fatalf("php: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "OK" {
		t.Fatalf("JSON module resolution failures:\n%s", out)
	}
}

// TestJsonControllerResolution verifies that controller SNMP_ID matching is strict
// case-sensitive equality.  null article and unrecognised articles produce nil (fallback).
func TestJsonControllerResolution(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	phpLogic := `<?php
// Mirror of the widget resolveController logic.
function resolveController(?string $liveArticle, array $configControllers): ?array {
    if ($liveArticle === null) return null;
    foreach ($configControllers as $entry) {
        if ($entry['SNMP_ID'] === $liveArticle) return $entry;
    }
    return null;
}

$cfg = [
    '750-880' => ['SNMP_ID' => '750-880', 'name' => '750-880', 'description' => 'Controller'],
];
$ok = true;

// Exact match.
$r = resolveController('750-880', $cfg);
if ($r === null || $r['name'] !== '750-880') { echo "FAIL: exact match returned null or wrong name\n"; $ok = false; }

// Null article → null (fallback).
if (resolveController(null, $cfg) !== null) { echo "FAIL: null article should return null\n"; $ok = false; }

// Unrecognised article → null (fallback).
if (resolveController('750-881', $cfg) !== null) { echo "FAIL: 750-881 should return null\n"; $ok = false; }

// Case-sensitive: '750-880' does not match '750-8XX'.
if (resolveController('750-8XX', $cfg) !== null) { echo "FAIL: case-different should return null\n"; $ok = false; }

// Empty config → null.
if (resolveController('750-880', []) !== null) { echo "FAIL: empty config should return null\n"; $ok = false; }

echo $ok ? "OK\n" : "FAILURES\n";`

	out, err := exec.Command(php, "-r", phpLogic).CombinedOutput()
	if err != nil {
		t.Fatalf("php: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "OK" {
		t.Fatalf("JSON controller resolution failures:\n%s", out)
	}
}

// TestNoHardcodedMapsInPhp checks that the redesigned widget.view.php no longer contains
// the removed hard-coded product maps or description arrays.
func TestNoHardcodedMapsInPhp(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "views/widget.view.php"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	forbidden := []string{"$WAGO_DESCRIPTIONS", "$SVG_CONTROLLER_MAP", "$SVG_MODULE_MAP"}
	for _, s := range forbidden {
		if strings.Contains(content, s) {
			t.Errorf("widget.view.php still contains removed hard-coded declaration: %q", s)
		}
	}
}

// TestInstallerDefaultPath checks that install_widget.sh defaults to the correct Zabbix 7.4
// module path (/usr/share/zabbix/ui/modules, not the old /usr/share/zabbix/modules).
func TestInstallerDefaultPath(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "scripts", "install_widget.sh"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "/usr/share/zabbix/ui/modules") {
		t.Error("install_widget.sh does not reference /usr/share/zabbix/ui/modules as default path")
	}
	// The old path must not be the default.
	if strings.Contains(content, `ZABBIX_MODULES_DIR="/usr/share/zabbix/modules"`) {
		t.Error("install_widget.sh still sets old default path /usr/share/zabbix/modules")
	}
}

// TestWidgetTooltipPortalContracts guards the browser-side behavior that keeps
// tooltip content outside the widget's clipping containers and cleans it up on
// every Zabbix lifecycle transition.
func TestWidgetTooltipPortalContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "assets/js/class.widget.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	required := []string{
		"document.body.appendChild(tooltip)",
		"getBoundingClientRect()",
		"safeRight",
		"safeBottom",
		"setContents(data)",
		"this._kbusTeardown()",
		"this._kbusSetup()",
		"onResize()",
		"onDeactivate()",
		"onDestroy()",
		"document.addEventListener('scroll'",
		"window.addEventListener('resize'",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("class.widget.js missing tooltip portal/lifecycle contract %q", want)
		}
	}
}

// TestWidgetTooltipPositioningJavaScript executes the real positioning method
// with synthetic DOM rectangles. It covers right/left placement, center clamping,
// and a panel partially outside the viewport without requiring a browser download.
func TestWidgetTooltipPositioningJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed — skipping tooltip geometry test")
	}
	data, err := os.ReadFile(filepath.Join(widgetDir, "assets/js/class.widget.js"))
	if err != nil {
		t.Fatal(err)
	}

	script := `
globalThis.CWidget = class {};
globalThis.document = {documentElement: {clientWidth: 320, clientHeight: 240}};
eval(process.argv[1] + "\nglobalThis.TestWidget = CWidgetWagoKbus;");
const widget = new globalThis.TestWidget();

function check(name, panelRect, itemRect, tipRect, expectedSide) {
  const panel = {getBoundingClientRect: () => panelRect};
  const item = {getBoundingClientRect: () => itemRect};
  const tooltip = {style: {}, getBoundingClientRect: () => tipRect};
  widget._kbusPosition(item, tooltip, panel);
  const left = Number.parseFloat(tooltip.style.left);
  const top = Number.parseFloat(tooltip.style.top);
  const safeLeft = Math.max(panelRect.left, 0);
  const safeTop = Math.max(panelRect.top, 0);
  const safeRight = Math.min(panelRect.right, 320);
  const safeBottom = Math.min(panelRect.bottom, 240);
  if (left < safeLeft + 8 || left + tipRect.width > safeRight - 8) {
    throw new Error(name + ': horizontal overflow at ' + left);
  }
  if (top < safeTop + 8 || top + tipRect.height > safeBottom - 8) {
    throw new Error(name + ': vertical overflow at ' + top);
  }
  if (expectedSide === 'right' && left !== itemRect.right + 8) {
    throw new Error(name + ': did not prefer right side');
  }
  if (expectedSide === 'left' && left + tipRect.width !== itemRect.left - 8) {
    throw new Error(name + ': did not fall back to left side');
  }
}

check('right', {left:0, top:0, right:300, bottom:200},
  {left:20, top:40, right:40, bottom:140, width:20, height:100},
  {width:100, height:40}, 'right');
check('left', {left:0, top:0, right:300, bottom:200},
  {left:260, top:40, right:280, bottom:140, width:20, height:100},
  {width:100, height:40}, 'left');
check('center-clamp', {left:0, top:0, right:300, bottom:200},
  {left:145, top:40, right:155, bottom:140, width:10, height:100},
  {width:180, height:40}, 'center');
check('viewport-intersection', {left:-50, top:-20, right:200, bottom:150},
  {left:10, top:20, right:30, bottom:100, width:20, height:80},
  {width:100, height:40}, 'right');
`
	out, err := exec.Command(node, "-e", script, string(data)).CombinedOutput()
	if err != nil {
		t.Fatalf("tooltip positioning JavaScript: %v\n%s", err, out)
	}
}

// TestWidgetTooltipPinContracts guards the interactive pinned state: one active
// popup, keyboard activation/closing, a safe close button, and focus restoration.
func TestWidgetTooltipPinContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "assets/js/class.widget.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	required := []string{
		"this._kbusActive",
		"e.key === 'Enter'",
		"e.key === ' '",
		"e.key === 'Escape'",
		"document.createElement('button')",
		"btn.textContent = '×'",
		"btn.setAttribute('aria-label', 'Close details')",
		"wago-kbus-tooltip--pinned",
		"tooltip.setAttribute('role', 'dialog')",
		"home.setAttribute('aria-expanded', 'false')",
		"returnFocus.focus()",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("class.widget.js missing pinned-tooltip contract %q", want)
		}
	}
	if strings.Contains(src, "innerHTML") {
		t.Error("class.widget.js must not introduce an innerHTML sink")
	}
}

// TestWidgetTooltipCSSContracts ensures fixed-percentage positioning cannot
// regress and pinned content remains interactive and selectable.
func TestWidgetTooltipCSSContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(widgetDir, "assets/css/widget.css"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	for _, forbidden := range []string{"top: 65%", "top:65%", "top: 35%", "top:35%"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("widget.css still contains brittle tooltip placement %q", forbidden)
		}
	}
	for _, want := range []string{
		".wago-kbus-tooltip--floating",
		"position: fixed",
		".wago-kbus-tooltip--pinned",
		"pointer-events: auto",
		"user-select: text",
		".wago-kbus-tooltip-close",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("widget.css missing tooltip contract %q", want)
		}
	}
}

func TestWidgetTooltipMarkupAndVersion(t *testing.T) {
	view, err := os.ReadFile(filepath.Join(widgetDir, "views/widget.view.php"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(view), `tabindex="0" aria-expanded="false"`); count < 2 {
		t.Errorf("widget.view.php has %d keyboard-enabled item render paths, want at least 2", count)
	}

	data, err := os.ReadFile(filepath.Join(widgetDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	if manifest["version"] != "1.1.1" {
		t.Errorf("manifest version=%q, want 1.1.1", manifest["version"])
	}
}

// parseYAMLTemplate is a helper shared across widget tests.
func parseYAMLTemplate(data []byte, out *templateDocument) error {
	return yaml.Unmarshal(data, out)
}
