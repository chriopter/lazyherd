package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", config)
	for _, dir := range []string{"git", "code", "arg"} {
		if err := os.Mkdir(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := rootDir(""); err != nil || got != filepath.Join(home, "git") {
		t.Fatalf("default: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(config, configFile), []byte("root: ~/code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := rootDir(""); err != nil || got != filepath.Join(home, "code") {
		t.Fatalf("config: %q %v", got, err)
	}
	if got, err := rootDir(filepath.Join(home, "arg")); err != nil || got != filepath.Join(home, "arg") {
		t.Fatalf("argument: %q %v", got, err)
	}
	if _, err := rootDir(filepath.Join(home, "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
	if err := os.WriteFile(filepath.Join(config, configFile), []byte("root: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rootDir(""); err == nil {
		t.Fatal("broken config accepted")
	}
}
