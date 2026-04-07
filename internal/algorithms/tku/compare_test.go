package tku

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestTKU_CompareWithJava_K8(t *testing.T) {
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

	algo.PrintStats()

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	goLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	sort.Strings(goLines)

	// Java reference output (from: java -jar spmf.jar run TKU DB_utility.txt output.txt 8)
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

	t.Logf("=== Go output (%d patterns) ===", len(goLines))
	for _, l := range goLines {
		t.Log(l)
	}
	t.Logf("=== Java output (%d patterns) ===", len(javaLines))
	for _, l := range javaLines {
		t.Log(l)
	}

	if len(goLines) != len(javaLines) {
		t.Fatalf("Pattern count mismatch: Go=%d, Java=%d", len(goLines), len(javaLines))
	}

	for i := range goLines {
		if strings.TrimSpace(goLines[i]) != strings.TrimSpace(javaLines[i]) {
			t.Errorf("Mismatch at line %d:\n  Go:   %q\n  Java: %q", i, goLines[i], javaLines[i])
		}
	}
}
