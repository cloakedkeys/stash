package scraper

import (
	"net"
	"net/url"
	"testing"
)

func TestSetCDPHost(t *testing.T) {
	for _, tc := range []struct{ input, ip, want string }{
		{"http://chrome:9222/json/version", "2001:db8::1", "http://[2001:db8::1]:9222/json/version"},
		{"https://chrome/json/version", "2001:db8::1", "https://[2001:db8::1]/json/version"},
		{"ws://user:pass@chrome:9222/devtools/browser?q=1", "192.0.2.1", "ws://user:pass@192.0.2.1:9222/devtools/browser?q=1"},
		{"https://chrome/json/version", "192.0.2.1", "https://192.0.2.1/json/version"},
	} {
		t.Run(tc.input+tc.ip, func(t *testing.T) {
			u, err := url.Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			setCDPHost(u, net.ParseIP(tc.ip))
			if got := u.String(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
