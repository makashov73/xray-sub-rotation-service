package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

func TestSubscriptionHandler(t *testing.T) {
	s := store.NewStore("fastest")

	// Mock response from 3x-ui
	mockResponse := "vless://test1\ntrojan://test2\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	s.AddEndpoint(srv.URL+"/sub/abc123", "abc123", "test-server")

	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	// Request subscription for abc123
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != strings.TrimSpace(mockResponse) {
		t.Errorf("Body = %q, want %q", body, mockResponse)
	}
}

func TestSubscriptionNotFound(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/subrouter/nonexistent-uuid", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	s := store.NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	// Record some health data
	s.RecordHealth("https://xray1.example.com/sub/abc", store.HealthInfo{
		Healthy:     true,
		LatencyMS:   42,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", store.HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})

	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"abc123"`) {
		t.Errorf("Body missing subId: %s", body)
	}
	if !strings.Contains(body, `"US-East"`) {
		t.Errorf("Body missing server name: %s", body)
	}
}

func TestHealthCheckEndpointEmpty(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// json.Encoder adds a trailing newline, so we compare the trimmed body
	body := strings.TrimSpace(w.Body.String())
	if body != "{}" {
		t.Errorf("Body = %q, want '{}'", body)
	}
}

func TestLivezEndpoint(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/livez", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if strings.TrimSpace(w.Body.String()) != "ok" {
		t.Errorf("Body = %q, want 'ok'", w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":        "nosniff",
		"X-Frame-Options":               "DENY",
		"X-XSS-Protection":              "1; mode=block",
		"Strict-Transport-Security":     "max-age=31536000; includeSubDomains",
		"Cache-Control":                 "no-store",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		if got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}
}

func TestRequestID(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-Id")
	if requestID == "" {
		t.Error("X-Request-Id header is empty")
	}

	if len(requestID) != 32 {
		t.Errorf("X-Request-Id = %q (len %d), want 32 hex characters", requestID, len(requestID))
	}
}

func TestProcessSubscription(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "vless prefix preserved",
			input: "vless://uuid@host:port#tag\n",
			want:  "vless://uuid@host:port#tag\n",
		},
		{
			name:  "multiple protocols preserved",
			input: "vless://uuid@host:port\n# comment\ntrojan://pass@host:443\n",
			want:  "vless://uuid@host:port\n# comment\ntrojan://pass@host:443\n",
		},
		{
			name:  "empty lines removed",
			input: "vless://uuid@host:port\n\n\n",
			want:  "vless://uuid@host:port\n",
		},
		{
			name:  "vmess prefix preserved",
			input: "vmess://base64@host:443\n",
			want:  "vmess://base64@host:443\n",
		},
		{
			name:  "ss prefix preserved",
			input: "ss://YWVzLTEyOC1nY206cGFzcw==@host:443#name\n",
			want:  "ss://YWVzLTEyOC1nY206cGFzcw==@host:443#name\n",
		},
		{
			name:  "all lines preserved including comments",
			input: "# generated by 3x-ui\nvless://uuid@host:port\n# end\n",
			want:  "# generated by 3x-ui\nvless://uuid@host:port\n# end\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProcessSubscription([]byte(tt.input))
			if string(got) != tt.want {
				t.Errorf("ProcessSubscription() = %q, want %q", got, tt.want)
			}
		})
	}
}
