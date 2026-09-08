package http

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/arcaptcha/kaftar/server/internal/service/providerhttp"
)

type destinationPolicy struct {
	allowed map[string]bool
	private []netip.Prefix
	resolve func(context.Context, string) ([]netip.Addr, error)
	dial    func(context.Context, string, string) (net.Conn, error)
}

var specialNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func newDestinationPolicy(config *Config) (*destinationPolicy, error) {
	policy := &destinationPolicy{allowed: make(map[string]bool), resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}, dial: (&net.Dialer{Timeout: config.Timeout}).DialContext}
	for _, destination := range config.AllowedDestinations {
		host, port, err := net.SplitHostPort(destination)
		number, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || number < 1 || number > 65535 || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") {
			return nil, errors.New("HTTP destinations must be exact host:port entries")
		}
		policy.allowed[net.JoinHostPort(strings.ToLower(strings.TrimSuffix(host, ".")), strconv.Itoa(number))] = true
	}
	for _, value := range config.AllowedPrivateCIDRs {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New("invalid HTTP private destination CIDR")
		}
		policy.private = append(policy.private, prefix.Masked())
	}
	return policy, nil
}

func (policy *destinationPolicy) validate(destination *url.URL) error {
	if destination == nil || destination.User != nil || destination.Fragment != "" || (destination.Scheme != "http" && destination.Scheme != "https") {
		return providerhttp.Failure("http: destination is not permitted")
	}
	port := destination.Port()
	if port == "" {
		port = "80"
		if destination.Scheme == "https" {
			port = "443"
		}
	}
	host := strings.ToLower(strings.TrimSuffix(destination.Hostname(), "."))
	if !policy.allowed[net.JoinHostPort(host, port)] {
		return providerhttp.Failure("http: destination is not allowlisted")
	}
	return nil
}

func (policy *destinationPolicy) permits(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" || address.IsUnspecified() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return false
	}
	if address == netip.MustParseAddr("168.63.129.16") || address == netip.MustParseAddr("fd00:ec2::254") {
		return false
	}
	for _, prefix := range specialNetworks {
		if prefix.Contains(address) {
			return false
		}
	}
	if address.IsPrivate() || address.IsLoopback() {
		for _, prefix := range policy.private {
			if prefix.Contains(address) {
				return true
			}
		}
		return false
	}
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	return address.IsGlobalUnicast()
}

func (policy *destinationPolicy) dialContext(ctx context.Context, network, target string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, providerhttp.Failure("http: invalid destination")
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if !policy.allowed[net.JoinHostPort(host, port)] {
		return nil, providerhttp.Failure("http: destination is not allowlisted")
	}
	var addresses []netip.Addr
	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = []netip.Addr{address}
	} else {
		addresses, err = policy.resolve(ctx, host)
	}
	if err != nil {
		return nil, providerhttp.SafeError("http DNS", err)
	}
	if len(addresses) == 0 {
		return nil, providerhttp.Failure("http: destination has no addresses")
	}
	for _, address := range addresses {
		if !policy.permits(address) {
			return nil, providerhttp.Failure("http: destination address is blocked")
		}
	}
	for _, address := range addresses {
		var connection net.Conn
		connection, err = policy.dial(ctx, network, net.JoinHostPort(address.Unmap().String(), port))
		if err == nil {
			return connection, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, providerhttp.SafeError("http dial", err)
}
