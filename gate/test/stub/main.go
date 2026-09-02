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
	Extensions []string `json:"extensions"`
	ExitCode   int      `json:"exitCode"`
	Stdout     string   `json:"stdout"`
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
	io.Copy(io.Discard, os.Stdin)
	if cfg.ExitCode != 0 {
		os.Exit(cfg.ExitCode)
	}
	fmt.Print(cfg.Stdout)
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
	body, err := json.Marshal(map[string]any{
		"language":   "csharp",
		"extensions": cfg.Extensions,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(64)
	}
	fmt.Println(string(body))
}
