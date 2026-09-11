package clientip

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxyParsing(t *testing.T) {
	for _, value := range []string{"not-a-network", "127.0.0.1,", " , ", "0.0.0.0/0", "::/0", "::ffff:0.0.0.0/96", "fe80::1%eth0"} {
		if _, err := ParseTrustedProxies(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	prefixes, err := ParseTrustedProxies("192.0.2.1,2001:db8::1,::ffff:192.0.2.2/128")
	if err != nil {
		t.Fatal(err)
	}
	if prefixes[0].Bits() != 32 || prefixes[1].Bits() != 128 || prefixes[2].String() != "192.0.2.2/32" {
		t.Fatalf("wrong masks: %v", prefixes)
	}
}

func TestProxyAttributionRejectsMalformedAndNormalizesIPv6(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8,2001:db8::1")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ remote, xff, want string }{
		{"198.51.100.1:9", "192.0.2.1", "198.51.100.1"},
		{"10.0.0.1:9", "192.0.2.1, 198.51.100.7, 10.0.0.2", "198.51.100.7"},
		{"10.0.0.1:9", "192.0.2.1, secret-not-an-IP", "10.0.0.1"},
		{"10.0.0.1:9", "::ffff:192.0.2.1", "192.0.2.1"},
		{"[2001:db8::1]:9", "2001:0DB8:0000:0000:0000:0000:0000:0002", "2001:db8::2"},
		{"[2001:db8::99]:9", "192.0.2.1", "2001:db8::99"},
		{"10.0.0.1:9", strings.Repeat("a", 1025), "10.0.0.1"},
		{"malformed-peer", "192.0.2.1", "unknown"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", tc.xff)
		if got := Address(r, proxies); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}
