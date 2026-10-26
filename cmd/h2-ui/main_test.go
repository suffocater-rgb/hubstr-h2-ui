package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Smoke: make sure the binary's helper routines stay sane when invoked from
// Go's test runner (we cannot call main() itself).
func TestEnvOr(t *testing.T) {
	if got := envOr("H2UI_TEST_MISSING", "default"); got != "default" {
		t.Fatalf("envOr default: %q", got)
	}
	os.Setenv("H2UI_TEST_PRESENT", "x")
	defer os.Unsetenv("H2UI_TEST_PRESENT")
	if got := envOr("H2UI_TEST_PRESENT", "default"); got != "x" {
		t.Fatalf("envOr from env: %q", got)
	}
}

func TestDetectOutboundIPReturnsIP(t *testing.T) {
	ip := detectOutboundIP()
	if ip == "" {
		t.Fatal("empty outbound IP")
	}
	// must at least look like an IP literal
	if got := filepath.Base(ip); got != ip {
		t.Fatalf("not an ip literal: %q", ip)
	}
}
