package sublist

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Entry represents one subscription line in sublist.md.
type Entry struct {
	SubId string
	URL   string
	Name  string
}

// Parse reads a sublist.md file and returns subscription entries.
func Parse(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		subId := strings.TrimSpace(parts[0])
		urlStr := strings.TrimSpace(parts[1])
		if _, err := url.Parse(urlStr); err != nil {
			return entries, fmt.Errorf("line %d: invalid URL %q: %w", lineNum, urlStr, err)
		}
		name := ""
		if len(parts) >= 3 {
			name = strings.TrimSpace(parts[2])
		}

		entries = append(entries, Entry{
			SubId: subId,
			URL:   urlStr,
			Name:  name,
		})
	}

	return entries, scanner.Err()
}
