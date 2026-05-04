package handler

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"strings"
)

var protocolPrefixes = []string{
	"vless://",
	"trojan://",
	"vmess://",
	"ss://",
}

// ProcessSubscription processes subscription content and preserves all lines.
// Lines that already have a recognized protocol prefix (vless://, trojan://, etc.)
// are kept as-is. Empty lines are removed. Returns the processed content.
func ProcessSubscription(content []byte) []byte {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func hasProtocolPrefix(s string) bool {
	for _, prefix := range protocolPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// base64DecodeIfApplicable checks if the input looks like base64-encoded content
// (single line, mostly alphanumeric with no newlines) and decodes it.
// Returns the decoded content and nil on success, or the original input on failure.
func base64DecodeIfApplicable(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)

	// If there are newlines, it's likely already plain text
	if bytes.ContainsRune(trimmed, '\n') || bytes.ContainsRune(trimmed, '\r') {
		return data, nil
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(string(trimmed))
	if err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	return data, err
}
