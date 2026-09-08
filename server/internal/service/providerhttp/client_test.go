package providerhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSafeErrors(test *testing.T) {
	secret := "synthetic-secret"
	for _, cause := range []error{errors.New(secret), context.Canceled, context.DeadlineExceeded, Failure("provider: HTTP 401")} {
		err := SafeError("provider", &url.Error{Op: "Post", URL: "https://example.invalid/" + secret, Err: cause})
		if strings.Contains(err.Error(), secret) {
			test.Fatal("credential leaked")
		}
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			if !errors.Is(err, cause) {
				test.Fatal("lost context error identity")
			}
		}
	}
}

func TestResponseContract(test *testing.T) {
	for _, scenario := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"accepted", 201, `{}`, false}, {"rejected", 401, "synthetic-secret", true}, {"wrong-success", 200, `{}`, true}, {"oversized", 201, strings.Repeat("x", int(ResponseLimit)+1), true},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.WriteHeader(scenario.status)
				_, _ = response.Write([]byte(scenario.body))
			}))
			defer server.Close()
			request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
			if err != nil {
				test.Fatal(err)
			}
			_, err = Response(NewClient(), request, "provider", 201)
			if (err != nil) != scenario.wantError {
				test.Fatalf("error = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				test.Fatal("response body leaked")
			}
		})
	}
}

func TestClientRefusesRedirectsAndBoundsIO(test *testing.T) {
	client := NewClient()
	if client.Timeout != 30*time.Second || client.Jar != nil || client.Transport.(*http.Transport).Proxy != nil {
		test.Fatal("unsafe client defaults")
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(response, request, "/target", http.StatusFound)
			return
		}
		if request.URL.Path == "/target" {
			test.Error("redirect followed")
			return
		}
		<-request.Context().Done()
	}))
	defer server.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/redirect", nil)
	if _, err := Response(client, request, "provider", 0); err == nil {
		test.Fatal("redirect counted as success")
	}
	client.Timeout = 30 * time.Millisecond
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/stall", nil)
	if _, err := Response(client, request, "provider", 0); !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("timeout error = %v", err)
	}
}
