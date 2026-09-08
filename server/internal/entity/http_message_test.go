package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHTTPMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		message *HTTPMessage
		wantErr string
	}{
		{
			name:    "missing URL",
			message: &HTTPMessage{},
			wantErr: "http: URL must use http or https",
		},
		{
			name:    "unsupported scheme",
			message: &HTTPMessage{URL: "ftp://example.com"},
			wantErr: "http: URL must use http or https",
		},
		{
			name:    "missing host",
			message: &HTTPMessage{URL: "https:///path"},
			wantErr: "http: URL must include a host",
		},
		{
			name:    "invalid header value",
			message: &HTTPMessage{URL: "https://example.com", Headers: map[string]string{"X-Test": "bad\nvalue"}},
			wantErr: "http: invalid value for header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			t.Log(err)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPMessageRoundTrip(t *testing.T) {
	original := &HTTPMessage{
		URL:     "https://example.com/hook",
		Method:  "PATCH",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"ok":true}`,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	message, err := UnmarshalMessage(ChannelHTTP, data)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ConcreteHttpMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if actual.URL != original.URL || actual.Method != original.Method || actual.Body != original.Body || actual.Headers["Content-Type"] != original.Headers["Content-Type"] {
		t.Fatalf("round trip mismatch: got %#v, want %#v", actual, original)
	}
}

func TestHTTPMessageEffectiveMethod(t *testing.T) {
	if got := (&HTTPMessage{URL: "https://example.com"}).EffectiveMethod(); got != "POST" {
		t.Fatalf("EffectiveMethod() = %q, want POST", got)
	}
	if got := (&HTTPMessage{URL: "https://example.com", Method: " patch "}).EffectiveMethod(); got != "PATCH" {
		t.Fatalf("EffectiveMethod() = %q, want PATCH", got)
	}
}
