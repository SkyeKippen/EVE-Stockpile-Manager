package esi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvSkipsByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	env := "\xef\xbb\xbfSTOCKPILE_TEST_FIRST=one\nSTOCKPILE_TEST_SECOND=two\n"
	if err := os.WriteFile(filepath.Join(dir, EnvFile), []byte(env), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Cleanup(func() {
		os.Unsetenv("STOCKPILE_TEST_FIRST")
		os.Unsetenv("STOCKPILE_TEST_SECOND")
	})

	if err := LoadEnv(); err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if got := os.Getenv("STOCKPILE_TEST_FIRST"); got != "one" {
		t.Errorf("STOCKPILE_TEST_FIRST = %q, want %q", got, "one")
	}
	if got := os.Getenv("STOCKPILE_TEST_SECOND"); got != "two" {
		t.Errorf("STOCKPILE_TEST_SECOND = %q, want %q", got, "two")
	}
}
