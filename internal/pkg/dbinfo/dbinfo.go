// Package dbinfo scans SPMF utility transaction files for TKU-family algorithms
// (items : transactionUtility : itemUtilities).
package dbinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// DatabaseMetadata holds statistics computed from a utility transaction database.
type DatabaseMetadata struct {
	MaxID     int
	DBSize    int
	InputPath string
}

// New opens the file and fills MaxID and DBSize (line count, including blanks per TKU semantics).
func New(inputPath string) (*DatabaseMetadata, error) {
	d := &DatabaseMetadata{InputPath: inputPath}
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

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
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

// GetMaxID returns the largest item id seen in the database.
func (d *DatabaseMetadata) GetMaxID() int {
	return d.MaxID
}

// GetDBSize returns the number of lines scanned (transaction count including empty lines).
func (d *DatabaseMetadata) GetDBSize() int {
	return d.DBSize
}

func Scanner(filePath string) (*bufio.Scanner, func()) {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}

	return bufio.NewScanner(f), func() {
		_ = f.Close()
	}
}
