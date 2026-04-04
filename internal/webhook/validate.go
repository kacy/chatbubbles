package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

type Validator struct {
	lookupIP LookupFunc
}

func NewValidator() *Validator {
	return &Validator{
		lookupIP: func(ctx context.Context, host string) ([]netip.Addr, error) {
			addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			out := make([]netip.Addr, 0, len(addrs))
			for _, addr := range addrs {
				out = append(out, addr.Unmap())
			}
			return out, nil
		},
	}
}

func (v *Validator) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, errors.New("webhook url must be valid")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("webhook url must use https")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("webhook url must include a host")
	}

	addrs, err := v.lookupIP(ctx, parsed.Hostname())
	if err != nil {
		return nil, fmt.Errorf("resolve webhook host: %w", err)
	}
	if len(addrs) == 0 {
		return nil, errors.New("webhook host did not resolve")
	}

	for _, addr := range addrs {
		if blockedAddr(addr) {
			return nil, errors.New("webhook host resolves to a blocked address")
		}
	}

	return parsed, nil
}

func blockedAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}
	if addr.Is6() {
		if addr.IsMulticast() || addr.IsInterfaceLocalMulticast() || addr.IsLinkLocalUnicast() {
			return true
		}
		return false
	}

	if addr.String() == "169.254.169.254" {
		return true
	}
	return false
}
