package tku

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
	t.Fatal("testdata/DB_Utility.txt not found (run tests from module or tku package dir)")
	return ""
}

func TestTKU_SampleDB_K3(t *testing.T) {
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

	algo := NewTKU()
	if err := algo.RunAlgorithm(dbPath, outPath, 3); err != nil {
		t.Fatalf("RunAlgorithm: %v", err)
	}
	if algo.patternCount != 3 {
		t.Fatalf("patternCount = %d, want 3", algo.patternCount)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d output lines, want 3: %q", len(lines), string(data))
	}
	for _, line := range lines {
		if !strings.Contains(line, " #UTIL: ") {
			t.Fatalf("unexpected line format: %q", line)
		}
	}
}

func TestTKU_SampleDB_K8(t *testing.T) {
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

	algo := NewTKU()
	if err := algo.RunAlgorithm(dbPath, outPath, 8); err != nil {
		t.Fatalf("RunAlgorithm: %v", err)
	}
	if algo.patternCount != 8 {
		t.Fatalf("patternCount = %d, want 8", algo.patternCount)
	}
}

func TestTriangularMatrixIncrement(t *testing.T) {
	m := NewTriangularMatrix(5)
	m.IncrementCount(1, 2, 1)
	m.IncrementCount(1, 2, 1)
	if m.GetSupportForItems(1, 2) != 2 {
		t.Fatalf("support 1,2 = %d", m.GetSupportForItems(1, 2))
	}
}
