package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// runAdopted prints "adopted" or "not-adopted" and exits 0, or exits 1 when
// the registry exists but cannot be read. The third answer is the point: a
// registry the shell could not match read as "not wired", which --staged
// skips in silence, so the commit passed unlinted.
func runAdopted(args AdoptedArgs, stdout, stderr io.Writer) int {
	ok, err := adopted(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lint-changed:", err)
		return 1
	}
	answer := "not-adopted"
	if ok {
		answer = "adopted"
	}
	if _, err := fmt.Fprintln(stdout, answer); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// adopted reports whether the registry names the key. A missing registry is
// a machine where nothing was bootstrapped for the language, so it answers
// not adopted rather than broken.
func adopted(args AdoptedArgs) (bool, error) {
	data, err := os.ReadFile(args.Registry)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if args.Language == lintfind.LanguageGo {
		return goRegistryNames(string(data), args.Key), nil
	}
	return propsScopes(data, args.Key, args.Registry)
}

// goRegistryNames reads the Go registry, one repository path per line. Each
// line is cleaned before comparing, so a CRLF line ending or a trailing slash
// from a hand edit still names its repository.
func goRegistryNames(registry, key string) bool {
	want := filepath.Clean(key)
	for _, line := range strings.Split(registry, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" && filepath.Clean(line) == want {
			return true
		}
	}
	return false
}

// propsScopes reports whether the props file carries an Import whose
// Condition scopes the key, the condition bootstrap writes and MSBuild tests.
// It parses the XML rather than searching the text: a condition in a comment
// or on another element scopes nothing, an entity-encoded one still does, and
// a file MSBuild cannot load is broken rather than silently unwired.
func propsScopes(data []byte, key, path string) (bool, error) {
	want := "$(MSBuildProjectDirectory.StartsWith('" + key + "/'))"
	decoder := xml.NewDecoder(bytes.NewReader(data))
	rooted, found := false, false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) && !rooted {
			err = errors.New("no root element")
		}
		if errors.Is(err, io.EOF) {
			return found, nil
		}
		if err != nil {
			return false, fmt.Errorf("%s is not well-formed XML, so MSBuild cannot load it either: %w", path, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		rooted = true
		if start.Name.Local != "Import" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Space == "" && attr.Name.Local == "Condition" && strings.Contains(attr.Value, want) {
				found = true
			}
		}
	}
}
