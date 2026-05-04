package handler

import (
	"bufio"
	"bytes"
	"strings"
)

var protocolPrefixes = []string{
	"vless://",
	"trojan://",
	"vmess://",
	"ss://",
}

func ProcessSubscription(content []byte) []byte {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if hasProtocolPrefix(trimmed) {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
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
