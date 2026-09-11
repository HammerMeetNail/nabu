// Package clientip validates proxy trust and extracts a bounded, canonical IP.
package clientip

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies rejects malformed or universal trust. A bare IP trusts
// that one address (including /128 for IPv6), never an inferred IPv6 /32.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for i, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		var prefix netip.Prefix
		var err error
		if strings.Contains(raw, "/") {
			prefix, err = netip.ParsePrefix(raw)
		} else {
			var addr netip.Addr
			addr, err = netip.ParseAddr(raw)
			if err == nil && addr.Zone() != "" {
				err = fmt.Errorf("zone not supported")
			}
			if err == nil {
				addr = addr.Unmap()
				prefix = netip.PrefixFrom(addr, addr.BitLen())
			}
		}
		if err != nil || !prefix.IsValid() || prefix.Bits() == 0 {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS entry %d must be a specific IP or CIDR", i+1)
		}
		if prefix.Addr().Is4In6() {
			if prefix.Bits() < 96 {
				return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS entry %d has an invalid mapped IPv4 prefix", i+1)
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			if prefix.Bits() == 0 {
				return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must not trust every address")
			}
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

func trusted(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func remote(r *http.Request) netip.Addr {
	if pair, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return pair.Addr().Unmap()
	}
	if addr, err := netip.ParseAddr(r.RemoteAddr); err == nil && addr.Zone() == "" {
		return addr.Unmap()
	}
	return netip.Addr{}
}

// Address walks the trusted chain right to left. Malformed untrusted data never
// becomes a key, and never lets a caller select an arbitrary leftmost address.
func Address(r *http.Request, prefixes []netip.Prefix) string {
	peer := remote(r)
	if !peer.IsValid() {
		return "unknown"
	}
	if !trusted(peer, prefixes) {
		return peer.String()
	}
	header := r.Header.Get("X-Forwarded-For")
	if len(header) > 1024 {
		return peer.String()
	}
	values := strings.Split(header, ",")
	if len(values) > 32 {
		return peer.String()
	}
	for i := len(values) - 1; i >= 0; i-- {
		value := strings.TrimSpace(values[i])
		if value == "" {
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil || addr.Zone() != "" {
			return peer.String()
		}
		addr = addr.Unmap()
		if !trusted(addr, prefixes) {
			return addr.String()
		}
	}
	return peer.String()
}
