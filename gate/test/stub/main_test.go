package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadConfigReadsTheFileTheVariableNames pins the stub's one input: it
// opens whatever METRIC_GATE_STUB points at and decodes the canned run from
// it, which is what lets a case pin capabilities and an exit code with no .NET
// SDK present.
func TestLoadConfigReadsTheFileTheVariableNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stub.json")
	body := `{"language":"csharp","extensions":[".cs"],"exitCode":3,"stdout":"{}"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METRIC_GATE_STUB", path)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.Language != "csharp" {
		t.Errorf("Language = %q, want %q", cfg.Language, "csharp")
	}
	if len(cfg.Extensions) != 1 || cfg.Extensions[0] != ".cs" {
		t.Errorf("Extensions = %q, want [.cs]", cfg.Extensions)
	}
	if cfg.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", cfg.ExitCode)
	}
	if cfg.Stdout != "{}" {
		t.Errorf("Stdout = %q, want %q", cfg.Stdout, "{}")
	}
}

// TestLoadConfigFailsWhenTheFileIsMissing pins that an unreadable config is a
// stub error naming the stub, because main turns it into exit 64 and a case
// reading that has to be able to tell it from the extractor's own output.
func TestLoadConfigFailsWhenTheFileIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	t.Setenv("METRIC_GATE_STUB", path)

	_, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig succeeded, want an error for a config that is not there")
	}
	if !strings.HasPrefix(err.Error(), "stub extractor: ") {
		t.Errorf("error = %q, want it to name the stub", err)
	}
}
