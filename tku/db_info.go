package tku

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// DatabaseInfo holds statistics computed from a utility transaction database
// (SPMF format: items : transactionUtility : itemUtilities).
type DatabaseInfo struct {
	MaxID        int
	DBSize       int
	InputPath    string
	totalUtility int64
}

// NewDatabaseInfo creates a scanner for the given input path.
func NewDatabaseInfo(inputPath string) *DatabaseInfo {
	return &DatabaseInfo{InputPath: inputPath}
}

// RunCalculate scans the file and fills MaxID and DBSize (transaction count).
func (d *DatabaseInfo) RunCalculate() error {
	f, err := os.Open(d.InputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	d.MaxID = 0
	d.DBSize = 0
	d.totalUtility = 0

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		d.DBSize++
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		tu, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err == nil {
			d.totalUtility += tu
		}
		items := strings.Fields(strings.TrimSpace(parts[0]))
		for _, it := range items {
			num, err := strconv.Atoi(it)
			if err != nil {
				continue
			}
			if num > d.MaxID {
				d.MaxID = num
			}
		}
	}
	return sc.Err()
}

// GetMaxID returns the largest item id seen in the database.
func (d *DatabaseInfo) GetMaxID() int {
	return d.MaxID
}

// GetDBSize returns the number of transactions (non-empty lines).
func (d *DatabaseInfo) GetDBSize() int {
	return d.DBSize
}
