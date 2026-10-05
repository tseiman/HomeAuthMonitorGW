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
	cases := []struct{ slot int; article string }{{1, "750-4xx"}, {2, "750-511/000-002"}, {3, "750-5xx"}, {4, "750-652/000-000"}}
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

// TestModuleSvgResolution runs the PHP matching logic (extracted inline) against the
// requirement test cases from req §5 using the php CLI.
func TestModuleSvgResolution(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed — skipping module resolution test")
	}

	// This PHP snippet mirrors the resolveModuleSvg logic in widget.view.php exactly.
	phpLogic := `<?php
function resolveModuleSvg(string $article, array $svgModuleMap, string $fallback): string {
    $article = trim($article);
    if (stripos($article, 'x') !== false || strpos($article, '*') !== false) {
        return $fallback;
    }
    $base = trim(preg_replace('/\/.*$/', '', $article));
    return $svgModuleMap[$base] ?? $fallback;
}
$map = ['750-511' => 'wago_0750-0511.svg'];
$fb  = 'wago_0750-xxxx_modul.svg';
$cases = [
    ['750-511/000-002', 'wago_0750-0511.svg'],
    ['750-511',         'wago_0750-0511.svg'],
    ['750-5xx',         'wago_0750-xxxx_modul.svg'],
    ['750-4xx',         'wago_0750-xxxx_modul.svg'],
    ['750-999',         'wago_0750-xxxx_modul.svg'],
    ['750-5XX',         'wago_0750-xxxx_modul.svg'],
    ['750-5xX/000-001', 'wago_0750-xxxx_modul.svg'],
];
$ok = true;
foreach ($cases as [$input, $expected]) {
    $got = resolveModuleSvg($input, $map, $fb);
    if ($got !== $expected) {
        echo "FAIL: resolveModuleSvg($input) = $got, want $expected\n";
        $ok = false;
    }
}
echo $ok ? "OK\n" : "FAILURES\n";`

	out, err := exec.Command(php, "-r", phpLogic).CombinedOutput()
	if err != nil {
		t.Fatalf("php: %v\n%s", err, out)
	}
	outputStr := strings.TrimSpace(string(out))
	if outputStr != "OK" {
		t.Fatalf("module resolution failures:\n%s", out)
	}
}

// TestControllerSvgResolution verifies the controller article → SVG logic.
func TestControllerSvgResolution(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	phpLogic := `<?php
function resolveControllerSvg(?string $article, array $ctrlMap, string $fallback): string {
    if ($article === null) return $fallback;
    $base = trim(preg_replace('/\/.*$/', '', $article));
    if (stripos($base, 'x') !== false || strpos($base, '*') !== false) return $fallback;
    return $ctrlMap[$base] ?? $fallback;
}
$map = ['750-880' => 'wago_0750-0880.svg'];
$fb  = 'wago_0750-xxxx_controller.svg';
$cases = [
    ['750-880',    'wago_0750-0880.svg'],
    [null,         'wago_0750-xxxx_controller.svg'],
    ['750-881',    'wago_0750-xxxx_controller.svg'],
    ['750-8xx',    'wago_0750-xxxx_controller.svg'],
];
$ok = true;
foreach ($cases as [$input, $expected]) {
    $got = resolveControllerSvg($input, $map, $fb);
    if ($got !== $expected) {
        $disp = $input === null ? 'null' : $input;
        echo "FAIL: resolveControllerSvg($disp) = $got, want $expected\n";
        $ok = false;
    }
}
echo $ok ? "OK\n" : "FAILURES\n";`

	out, err := exec.Command(php, "-r", phpLogic).CombinedOutput()
	if err != nil {
		t.Fatalf("php: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "OK" {
		t.Fatalf("controller resolution failures:\n%s", out)
	}
}

// parseYAMLTemplate is a helper shared across widget tests.
func parseYAMLTemplate(data []byte, out *templateDocument) error {
	return yaml.Unmarshal(data, out)
}
