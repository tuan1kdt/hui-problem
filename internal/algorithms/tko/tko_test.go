package tko

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleDBPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	candidates := []string{
		filepath.Join(wd, "testdata", "DB_Utility.txt"),
		filepath.Join(wd, "..", "testdata", "DB_Utility.txt"),
	}
	for _, p := range candidates {
		ap, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(ap); err == nil {
			return ap
		}
	}
	t.Fatal("testdata/DB_Utility.txt not found")
	return ""
}

func TestTKOBasic_SampleDB_K8(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := sampleDBPath(t)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	outPath := filepath.Join(dir, "out.txt")
	algo := NewAlgoTKOBasic()
	if err := algo.RunAlgorithm(dbPath, outPath, 8); err != nil {
		t.Fatalf("RunAlgorithm: %v", err)
	}
	if algo.ResultSize() != 8 {
		t.Fatalf("ResultSize = %d, want 8", algo.ResultSize())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 8 {
		t.Fatalf("got %d lines, want 8: %q", len(lines), string(data))
	}
	for _, line := range lines {
		if !strings.Contains(line, " #UTIL: ") {
			t.Fatalf("unexpected line: %q", line)
		}
	}
}
