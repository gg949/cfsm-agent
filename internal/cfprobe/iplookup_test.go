package cfprobe

import "testing"

func TestExtractIPFromBody(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantV4 bool
		want   string
	}{
		{name: "trace ipv4", body: "fl=123f45\nh=cloudflare.com\nip=203.0.113.10\nts=1700000000.1\n", wantV4: true, want: "203.0.113.10"},
		{name: "trace ipv6", body: "fl=123f45\nh=cloudflare.com\nip=2001:db8::1\nts=1700000000.1\n", wantV4: false, want: "2001:db8::1"},
		{name: "trace ipv4 wants v6", body: "ip=203.0.113.10\n", wantV4: false, want: ""},
		{name: "trace ipv6 wants v4", body: "ip=2001:db8::1\n", wantV4: true, want: ""},
		{name: "json ipv4", body: `{"ip":"198.51.100.7"}`, wantV4: true, want: "198.51.100.7"},
		{name: "plain ipv4", body: "192.0.2.1\n", wantV4: true, want: "192.0.2.1"},
		{name: "json ipv6", body: `{"ip":"2001:db8:85a3::8a2e:370:7334"}`, wantV4: false, want: "2001:db8:85a3::8a2e:370:7334"},
		{name: "plain ipv6", body: "2001:db8::ff00:42:8329", wantV4: false, want: "2001:db8::ff00:42:8329"},
		{name: "invalid ipv4 octets", body: "999.999.999.999", wantV4: true, want: ""},
		{name: "empty body", body: "", wantV4: true, want: ""},
		{name: "geo ipv4 with extra text", body: `{"country":"CN","ip":"203.0.113.99","asn":4134}`, wantV4: true, want: "203.0.113.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractIPFromBody(tt.body, tt.wantV4); got != tt.want {
				t.Fatalf("extractIPFromBody(%q, %v) = %q, want %q", tt.body, tt.wantV4, got, tt.want)
			}
		})
	}
}

func TestLookupPublicIPEndpointsConfigured(t *testing.T) {
	if len(ipv4LookupEndpoints) == 0 {
		t.Fatal("ipv4LookupEndpoints is empty")
	}
	if len(ipv6LookupEndpoints) == 0 {
		t.Fatal("ipv6LookupEndpoints is empty")
	}
}
