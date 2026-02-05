package proxy

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"time"
)

// StrongholdMark is the netfilter mark used to identify proxy traffic.
// This must match the value in internal/cli/transparent.go
const StrongholdMark = 0x2702

// MarkedDialer creates TCP connections with SO_MARK set to identify proxy traffic.
// This allows nftables/iptables rules to skip the proxy's own outbound connections,
// preventing infinite redirect loops while still proxying all other traffic (including root).
type MarkedDialer struct {
	Timeout time.Duration
}

// DialContext creates a marked TCP connection
func (d *MarkedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 1 * time.Second
	}

	// Create a raw socket with control function to set SO_MARK
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// Set SO_MARK on the socket so nftables/iptables can identify our traffic
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, StrongholdMark)
			})
		},
	}

	return dialer.DialContext(ctx, network, addr)
}

// Dial creates a marked TCP connection (non-context version)
func (d *MarkedDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// NewMarkedTransport creates an http.Transport that marks all outbound connections.
func NewMarkedTransport() *http.Transport {
	dialer := &MarkedDialer{Timeout: 1 * time.Second}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewMarkedClient creates an http.Client that marks all outbound connections.
// No overall timeout is set - the dial and TLS timeouts protect against stuck connections,
// but we don't want to cut off legitimate long-running transfers (large downloads, etc.).
func NewMarkedClient() *http.Client {
	return &http.Client{
		Transport: NewMarkedTransport(),
		// Don't follow redirects to prevent payment headers from being sent
		// to attacker-controlled URLs via redirect chains
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
