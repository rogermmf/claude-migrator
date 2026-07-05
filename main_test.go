package main

import "testing"

// go test ./...  runs the built-in self-test (token rewriting, exclusions,
// merge-safe import, vault placement).
func TestSelftest(t *testing.T) {
	if selftest() != 0 {
		t.Fatal("selftest reported failures")
	}
}
