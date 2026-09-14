// Package fixtures holds one deliberate violation per linter the curated config enables.
//
// cases_test.go beside it reads the enable list out of golangci.yml, runs the personal binary
// over this package and fails if any enabled linter reported nothing. That is what stops a
// linter from being enabled in name only: a typo'd setting, a linter that needs a language
// version the config does not ask for, or an exclusion preset creeping back in all show up as
// silence here.
//
// The functions are exported so `unused` reports only the one symbol that is its own fixture,
// and none of them is called.
package fixtures

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// MayFail stands in for any operation that reports failure.
func MayFail() error { return nil }

// Errcheck ignores a returned error outright.
func Errcheck() {
	MayFail()
}

// Errorlint formats the cause in rather than wrapping it, so errors.Is cannot see through it.
func Errorlint(err error) error {
	return fmt.Errorf("loading the page: %v", err)
}

// Nilerr checks the error and then reports success anyway.
func Nilerr() error {
	if err := MayFail(); err != nil {
		return nil
	}
	return nil
}

// Nilnesserr checks the second error and returns the first, which this branch already proved
// is nil, so the caller sees success.
func Nilnesserr() error {
	first := MayFail()
	if first != nil {
		return first
	}
	if second := MayFail(); second != nil {
		return first
	}
	return nil
}

// Bodyclose leaks the response body.
func Bodyclose(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	_ = resp.StatusCode
	return nil
}

// SQLClose leaks the rows handle and never asks it whether iteration failed, which is both the
// sqlclosecheck and the rowserrcheck fixture.
func SQLClose(db *sql.DB) error {
	rows, err := db.Query("select 1")
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	return nil
}

// Loggercheck logs a key with no value, so the record silently drops it.
func Loggercheck() {
	slog.Info("page loaded", "duration")
}

// Durationcheck multiplies a duration by a duration, which is a unit error the types allow.
func Durationcheck(timeout time.Duration) time.Duration {
	return timeout * time.Second
}

// Makezero appends to a slice made with a length, so the first ten elements stay zero.
func Makezero() []int {
	pages := make([]int, 10)
	pages = append(pages, 1)
	return pages
}

// Predeclared shadows a builtin, so `len` means something else for the rest of the body.
func Predeclared(len int) int {
	return len
}

// Govet passes a string to a %d verb.
func Govet() string {
	return fmt.Sprintf("%d pages", "three")
}

// Staticcheck compares a bool against a bool literal.
func Staticcheck(ready bool) bool {
	if ready == true {
		return true
	}
	return false
}

// Ineffassign assigns a value nothing can read.
func Ineffassign() int {
	count := 1
	count = 2
	return count
}

// Gocritic appends to one slice and assigns the result to another.
func Gocritic(left, right []int) []int {
	left = append(right, 1)
	return left
}

// Gosec hashes with MD5.
func Gosec(payload []byte) [16]byte {
	return md5.Sum(payload)
}

// unusedHelper is the `unused` fixture: unexported, and nothing calls it.
func unusedHelper() {}
