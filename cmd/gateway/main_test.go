package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	old := version
	version = "1.2.3-test"
	defer func() { version = old }()
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "1.2.3-test") {
		t.Fatalf("output=%q", out.String())
	}
}
func TestCheckReportsMissingConfiguration(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--config", "/does/not/exist", "--check"}, &out, &errOut); code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(errOut.String(), "configuration invalid") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}
