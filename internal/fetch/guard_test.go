package fetch

import (
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"0.0.0.0", false}, {"::", false},
		{"127.0.0.1", false}, {"::1", false}, {"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
		{"10.1.2.3", false}, {"172.16.0.1", false}, {"192.168.1.1", false}, {"fc00::1", false}, {"fd12::1", false},
		{"169.254.169.254", false}, {"169.254.0.1", false}, {"fe80::1", false},
		{"224.0.0.1", false}, {"ff02::1", false},
		{"0.1.2.3", false}, {"100.64.0.1", false}, {"100.127.255.255", false},
		{"192.0.0.8", false}, {"198.18.0.1", false}, {"198.19.255.255", false},
		{"240.0.0.1", false}, {"255.255.255.255", false},
		{"64:ff9b::7f00:1", false}, {"2001:db8::1", false},
		{"64:ff9b:1::1", false}, {"2002:7f00:1::1", false}, {"2001::1", false},
		{"::127.0.0.1", false}, {"::8.8.8.8", false}, {"::ffff:0:7f00:1", false},
		{"fec0::1", false}, {"100::1", false}, {"192.88.99.1", false},
		{"::ffff:8.8.8.8", true},
		{"8.8.8.8", true}, {"2606:4700::1111", true}, {"100.63.255.255", true},
	}
	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			if got := IsPublic(netip.MustParseAddr(tc.ip)); got != tc.want {
				t.Fatalf("IsPublic(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
	if IsPublic(netip.Addr{}) {
		t.Fatal("zero Addr must not be public")
	}
}

func TestDefaultDialPolicyPorts(t *testing.T) {
	for _, tc := range []struct {
		ap string
		ok bool
	}{
		{"8.8.8.8:80", true}, {"8.8.8.8:443", true}, {"8.8.8.8:22", false}, {"8.8.8.8:8080", false}, {"127.0.0.1:80", false},
	} {
		err := defaultDialPolicy(netip.MustParseAddrPort(tc.ap))
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v, want ok=%v", tc.ap, err, tc.ok)
		}
	}
}
