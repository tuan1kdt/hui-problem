package ptku

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"hui-problem/internal/pkg/datastructure"
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
	t.Fatal("testdata/DB_Utility.txt not found (run tests from module or ptku package dir)")
	return ""
}

func TestPTKU_SampleDB_K3(t *testing.T) {
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

	algo := NewPTKU()
	algo.Workers = 4
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

func TestPTKU_SampleDB_K8(t *testing.T) {
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

	algo := NewPTKU()
	algo.Workers = 4
	if err := algo.RunAlgorithm(dbPath, outPath, 8); err != nil {
		t.Fatalf("RunAlgorithm: %v", err)
	}
	if algo.patternCount != 8 {
		t.Fatalf("patternCount = %d, want 8", algo.patternCount)
	}
}

func TestPTKU_CompareWithJava_K8(t *testing.T) {
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

	algo := NewPTKU()
	algo.Workers = 4
	if err := algo.RunAlgorithm(dbPath, outPath, 8); err != nil {
		t.Fatalf("RunAlgorithm: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	goLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(goLines)

	javaOutput := `4 2 #UTIL: 30
2 5 #UTIL: 31
4 5 3 2 #UTIL: 40
4 5 2 #UTIL: 36
2 5 3 #UTIL: 37
4 2 3 #UTIL: 34
1 5 3 #UTIL: 31
6 5 4 3 2 1 #UTIL: 30`
	javaLines := strings.Split(strings.TrimSpace(javaOutput), "\n")
	sort.Strings(javaLines)

	if len(goLines) != len(javaLines) {
		t.Fatalf("Pattern count mismatch: Go=%d, Java=%d", len(goLines), len(javaLines))
	}
	for i := range goLines {
		if strings.TrimSpace(goLines[i]) != strings.TrimSpace(javaLines[i]) {
			t.Errorf("Mismatch at line %d:\n  Go:   %q\n  Java: %q", i, goLines[i], javaLines[i])
		}
	}
}

func TestTriangularMatrixIncrement(t *testing.T) {
	m := datastructure.NewTriangularMatrix(5)
	m.IncrementCount(1, 2, 1)
	m.IncrementCount(1, 2, 1)
	if m.GetSupportForItems(1, 2) != 2 {
		t.Fatalf("support 1,2 = %d", m.GetSupportForItems(1, 2))
	}
}
