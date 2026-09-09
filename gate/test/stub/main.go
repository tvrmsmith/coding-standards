// Command metric-gate-csharp is a stub extractor for the gate's black-box
// suite. It answers the ADR 0006 wire contract from a canned config file
// named by METRIC_GATE_STUB, so a case can pin capabilities, the exit code,
// and the JSON body without a .NET SDK present.
//
// It installs under the same name and into the same directory as the real
// dotnet tool, so a full-stack case can replace it with
// `dotnet tool install --tool-path <dir>` and change nothing else.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// config is the canned behaviour of one stub run.
type config struct {
	// Language is what --capabilities reports, defaulting to the language the
	// gate's table located this binary under.
	Language   string   `json:"language"`
	Extensions []string `json:"extensions"`
	// CapabilitiesStdout replaces the rendered --capabilities response
	// verbatim, so a case can hand the gate a body that is not JSON.
	CapabilitiesStdout string `json:"capabilitiesStdout"`
	ExitCode           int    `json:"exitCode"`
	Stdout             string `json:"stdout"`
	// StdinLog is a file the stub copies its stdin to, so a case can assert
	// the file list the gate handed it rather than only the document that
	// came out. Unset, the stub discards stdin as before.
	StdinLog string `json:"stdinLog"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	if len(os.Args) > 1 && os.Args[1] == "--capabilities" {
		writeCapabilities(cfg)
		return
	}
	// The real extractor reads every path before it answers, so the stub
	// drains stdin too and the two agree on when the gate's write completes.
	if err := drainStdin(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	if cfg.ExitCode != 0 {
		os.Exit(cfg.ExitCode)
	}
	fmt.Print(cfg.Stdout)
}

// drainStdin reads the path list the gate wrote, recording it when the case
// asked for it. The read happens either way and a failed read fails the stub
// either way, so a case that logs nothing runs against the same stub behaviour
// as one that does. Dropped on the discarding side, a broken pipe from the
// gate's write would come back as a successful extraction in the suite built to
// catch it. The input is a short path list, so buffering it even when no case
// asked for the log costs nothing and leaves one read and one wrapped error.
func drainStdin(cfg config) error {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("stub extractor: reading stdin: %w", err)
	}
	if cfg.StdinLog == "" {
		return nil
	}
	if err := os.WriteFile(cfg.StdinLog, body, 0o644); err != nil {
		return fmt.Errorf("stub extractor: %w", err)
	}
	return nil
}

// loadConfig reads the config file named by METRIC_GATE_STUB.
func loadConfig() (config, error) {
	path := os.Getenv("METRIC_GATE_STUB")
	if path == "" {
		return config{}, fmt.Errorf("stub extractor: METRIC_GATE_STUB is unset")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("stub extractor: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return config{}, fmt.Errorf("stub extractor: %s: %w", path, err)
	}
	return cfg, nil
}

// writeCapabilities answers --capabilities in the ADR 0006 wire format.
func writeCapabilities(cfg config) {
	if cfg.CapabilitiesStdout != "" {
		fmt.Print(cfg.CapabilitiesStdout)
		return
	}
	language := cfg.Language
	if language == "" {
		language = "csharp"
	}
	body, err := json.Marshal(map[string]any{
		"language":   language,
		"extensions": cfg.Extensions,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	fmt.Println(string(body))
}
