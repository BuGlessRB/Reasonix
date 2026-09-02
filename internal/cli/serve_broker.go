package cli

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reasonix/internal/provider"
	"reasonix/internal/providerbroker"
)

// brokerFlags are what a bootstrapped serve is told about the machine holding
// the model credentials. Both are set by the launch command, never by a person.
type brokerFlags struct {
	addr      *string
	tokenFile *string
}

func registerBrokerFlags(fs *flag.FlagSet) brokerFlags {
	return brokerFlags{
		addr: fs.String("provider-broker", "",
			"resolve providers from the machine that bootstrapped this serve, over this loopback base URL (set by `reasonix remote`)"),
		tokenFile: fs.String("provider-broker-token-file", "",
			"read the provider broker's pre-shared token from this file (keeps the secret out of argv)"),
	}
}

// resolver builds the broker-backed provider.Resolver, or (nil, nil) when this
// serve was not launched with one — in which case boot keeps reading providers
// out of this machine's own config.
func (b brokerFlags) resolver() (provider.Resolver, error) {
	if b.addr == nil || strings.TrimSpace(*b.addr) == "" {
		return nil, nil
	}
	base, err := loopbackBrokerURL(*b.addr)
	if err != nil {
		return nil, err
	}
	if b.tokenFile == nil || strings.TrimSpace(*b.tokenFile) == "" {
		return nil, fmt.Errorf("--provider-broker needs --provider-broker-token-file")
	}
	token, err := readServeTokenFile(strings.TrimSpace(*b.tokenFile))
	if err != nil {
		return nil, fmt.Errorf("provider broker token: %w", err)
	}
	// No overall timeout: a completion streams for as long as the model takes,
	// and a client deadline would cut the long turns first. The tunnel closing
	// is what ends a dead one.
	return providerbroker.NewClient(base, token, &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}), nil
}

// loopbackBrokerURL refuses a broker that is not on this machine's loopback.
// The address arrives from the forward that published it, so a non-loopback one
// means the tunnel is not what is carrying the conversation — and the whole
// conversation, with the workspace in it, is what these requests send.
func loopbackBrokerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("--provider-broker %q: %w", raw, err)
	}
	if u.Scheme != "http" {
		return "", fmt.Errorf("--provider-broker must be http on loopback, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "localhost" {
		return strings.TrimRight(u.String(), "/"), nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("--provider-broker must be on loopback, got %q", host)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
