package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/arcaptcha/kaftar/server/internal/entity"
)

func TestDestinationAddressPolicy(test *testing.T) {
	policy, err := newDestinationPolicy((&Config{AllowedDestinations: []string{"example.invalid:443"}, AllowedPrivateCIDRs: []string{"10.0.0.0/8", "fd00::/8", "127.0.0.0/8"}}).OrDefault())
	if err != nil {
		test.Fatal(err)
	}
	for _, scenario := range []struct {
		address string
		allowed bool
	}{
		{"8.8.8.8", true}, {"2606:4700:4700::1111", true}, {"10.1.2.3", true}, {"127.0.0.1", true}, {"fd00::1", true}, {"192.168.1.1", false},
		{"0.0.0.0", false}, {"::", false}, {"169.254.169.254", false}, {"fe80::1", false}, {"ff02::1", false}, {"224.0.0.1", false}, {"100.100.100.200", false}, {"168.63.129.16", false}, {"fd00:ec2::254", false}, {"64:ff9b::a00:1", false}, {"::ffff:169.254.169.254", false}, {"192.0.2.1", false},
	} {
		test.Run(scenario.address, func(test *testing.T) {
			if got := policy.permits(netip.MustParseAddr(scenario.address)); got != scenario.allowed {
				test.Fatalf("allowed = %v", got)
			}
		})
	}
}

func TestDNSValidationPinsDialAddress(test *testing.T) {
	policy, err := newDestinationPolicy((&Config{AllowedDestinations: []string{"example.invalid:443"}}).OrDefault())
	if err != nil {
		test.Fatal(err)
	}
	lookups, dials := 0, 0
	policy.resolve = func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	}
	policy.dial = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return nil, errors.New("synthetic dial failure")
	}
	if _, err := policy.dialContext(context.Background(), "tcp", "example.invalid:443"); err == nil || dials != 0 {
		test.Fatal("mixed DNS response reached dialer")
	}
	policy.resolve = func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	policy.dial = func(_ context.Context, _ string, address string) (net.Conn, error) {
		dials++
		if address != "8.8.8.8:443" {
			test.Error("dialer received hostname instead of validated IP")
		}
		return nil, errors.New("synthetic dial failure")
	}
	_, _ = policy.dialContext(context.Background(), "tcp", "example.invalid:443")
	if lookups != 2 || dials != 1 {
		test.Fatalf("lookups=%d dials=%d", lookups, dials)
	}
}

func TestWebhookSecurityDefaults(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(response, request, "/target", http.StatusFound)
			return
		}
		test.Error("blocked request reached destination")
	}))
	defer server.Close()
	defaultSender, err := New(nil)
	if err != nil {
		test.Fatal(err)
	}
	if err := defaultSender.Send(context.Background(), &entity.HTTPMessage{URL: server.URL}); err == nil {
		test.Fatal("default allowed destination")
	}
	localSender, err := newLocalSender(test, server.URL, nil)
	if err != nil {
		test.Fatal(err)
	}
	for _, message := range []*entity.HTTPMessage{{URL: server.URL + "/redirect"}, {URL: server.URL, Headers: map[string]string{"Connection": "keep-alive"}}, {URL: "http://user:synthetic-secret@" + server.Listener.Addr().String()}} {
		if err := localSender.Send(context.Background(), message); err == nil {
			test.Fatal("unsafe request accepted")
		}
	}
}
