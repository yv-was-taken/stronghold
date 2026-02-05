package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// StrongholdMark is the netfilter mark used to identify proxy traffic.
// The proxy sets this mark on its outbound sockets so nftables/iptables
// can skip them, preventing infinite redirect loops.
// 0x2702 = "stronghold" in hex-speak
const StrongholdMark = 0x2702

// TransparentProxy manages transparent proxying via iptables/nftables/pf
type TransparentProxy struct {
	config *CLIConfig
}

// NewTransparentProxy creates a new transparent proxy manager
func NewTransparentProxy(config *CLIConfig) *TransparentProxy {
	return &TransparentProxy{config: config}
}

// IsAvailable checks if transparent proxying is available on this system
func (t *TransparentProxy) IsAvailable() bool {
	switch runtime.GOOS {
	case "linux":
		return t.hasIptables() || t.hasNftables()
	case "darwin":
		return t.hasPfctl()
	default:
		return false
	}
}

// Enable sets up transparent proxying
func (t *TransparentProxy) Enable() error {
	switch runtime.GOOS {
	case "linux":
		return t.enableLinux()
	case "darwin":
		return t.enableDarwin()
	default:
		return fmt.Errorf("transparent proxy not supported on %s", runtime.GOOS)
	}
}

// Disable removes transparent proxying rules
func (t *TransparentProxy) Disable() error {
	switch runtime.GOOS {
	case "linux":
		return t.disableLinux()
	case "darwin":
		return t.disableDarwin()
	default:
		return fmt.Errorf("transparent proxy not supported on %s", runtime.GOOS)
	}
}

// Status returns the current transparent proxy status
func (t *TransparentProxy) Status() (bool, error) {
	switch runtime.GOOS {
	case "linux":
		return t.statusLinux()
	case "darwin":
		return t.statusDarwin()
	default:
		return false, nil
	}
}

// ==================== Linux Implementation ====================

func (t *TransparentProxy) hasIptables() bool {
	_, err := exec.LookPath("iptables")
	return err == nil
}

func (t *TransparentProxy) hasNftables() bool {
	_, err := exec.LookPath("nft")
	return err == nil
}

func (t *TransparentProxy) enableLinux() error {
	// Determine which tool to use
	if t.hasNftables() {
		return t.enableNftables()
	}
	if t.hasIptables() {
		return t.enableIptables()
	}
	return fmt.Errorf("neither iptables nor nftables found")
}

func (t *TransparentProxy) disableLinux() error {
	// Try both, ignore errors
	if t.hasNftables() {
		t.disableNftables()
	}
	if t.hasIptables() {
		t.disableIptables()
	}
	return nil
}

func (t *TransparentProxy) statusLinux() (bool, error) {
	// Check if our rules exist
	if t.hasIptables() {
		cmd := exec.Command("iptables", "-t", "nat", "-L", "OUTPUT", "-n")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "STRONGHOLD") {
			return true, nil
		}
	}
	if t.hasNftables() {
		cmd := exec.Command("nft", "list", "table", "inet", "stronghold")
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
	}
	return false, nil
}

// enableIptables sets up iptables rules for transparent proxying
func (t *TransparentProxy) enableIptables() error {
	proxyPort := strconv.Itoa(t.config.Proxy.Port)
	markHex := fmt.Sprintf("0x%x", StrongholdMark)

	// Create custom chain for stronghold
	// Use packet mark (set by proxy via SO_MARK) instead of UID to identify proxy traffic.
	// This ensures all users' traffic goes through the proxy, including root.
	rules := [][]string{
		// Create chain if doesn't exist
		{"iptables", "-t", "nat", "-N", "STRONGHOLD", "-m", "comment", "--comment", "Stronghold transparent proxy"},
		// Don't redirect traffic from the proxy itself (identified by socket mark)
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-m", "mark", "--mark", markHex, "-j", "RETURN"},
		// Don't redirect localhost traffic (avoid loops)
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-d", "127.0.0.1/8", "-j", "RETURN"},
		// Don't redirect private networks (optional, for local development)
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-d", "10.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-d", "172.16.0.0/12", "-j", "RETURN"},
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-d", "192.168.0.0/16", "-j", "RETURN"},
		// Redirect HTTP traffic to proxy
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-port", proxyPort},
		// Redirect HTTPS traffic to proxy (proxy handles CONNECT)
		{"iptables", "-t", "nat", "-A", "STRONGHOLD", "-p", "tcp", "--dport", "443", "-j", "REDIRECT", "--to-port", proxyPort},
		// Add chain to OUTPUT (for local traffic)
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-j", "STRONGHOLD"},
	}

	for _, rule := range rules {
		cmd := exec.Command(rule[0], rule[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore "chain already exists" errors
			if !strings.Contains(string(output), "Chain already exists") {
				return fmt.Errorf("iptables failed: %s - %s", err, string(output))
			}
		}
	}

	// Enable IP forwarding (needed for some setups)
	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()

	return nil
}

func (t *TransparentProxy) disableIptables() error {
	// Remove rules (ignore errors if they don't exist)
	exec.Command("iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "-j", "STRONGHOLD").Run()
	exec.Command("iptables", "-t", "nat", "-F", "STRONGHOLD").Run()
	exec.Command("iptables", "-t", "nat", "-X", "STRONGHOLD").Run()
	return nil
}

// enableNftables sets up nftables rules for transparent proxying
func (t *TransparentProxy) enableNftables() error {
	proxyPort := strconv.Itoa(t.config.Proxy.Port)

	// Create nftables script
	// Use packet mark (set by proxy via SO_MARK) instead of UID to identify proxy traffic.
	// This ensures all users' traffic goes through the proxy, including root.
	nftScript := fmt.Sprintf(`table inet stronghold {
    chain output {
        type nat hook output priority 0; policy accept;

        # Don't redirect proxy's own traffic (identified by socket mark)
        meta mark 0x%x return

        # Don't redirect localhost
        ip daddr 127.0.0.0/8 return
        ip6 daddr ::1/128 return

        # Don't redirect private networks
        ip daddr 10.0.0.0/8 return
        ip daddr 172.16.0.0/12 return
        ip daddr 192.168.0.0/16 return

        # Redirect HTTP to proxy
        tcp dport 80 redirect to :%s

        # Redirect HTTPS to proxy
        tcp dport 443 redirect to :%s
    }
}`, StrongholdMark, proxyPort, proxyPort)

	// Apply nftables config
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(nftScript)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nftables failed: %s - %s", err, string(output))
	}

	return nil
}

func (t *TransparentProxy) disableNftables() error {
	exec.Command("nft", "delete", "table", "inet", "stronghold").Run()
	return nil
}

// ==================== macOS Implementation ====================

func (t *TransparentProxy) hasPfctl() bool {
	_, err := exec.LookPath("pfctl")
	return err == nil
}

func (t *TransparentProxy) enableDarwin() error {
	proxyPort := strconv.Itoa(t.config.Proxy.Port)

	// Create pf configuration
	// Note: macOS pf doesn't support packet marks like Linux nftables/iptables.
	// We use a tagged approach instead - proxy traffic is tagged and skipped.
	// The proxy must set the "stronghold" tag on its outbound connections.
	pfConf := fmt.Sprintf(`# Stronghold transparent proxy
# Skip proxy's own traffic (tagged by proxy)
pass out quick proto tcp tagged stronghold

# Redirect HTTP to proxy
rdr pass on lo0 inet proto tcp from any to any port 80 -> 127.0.0.1 port %s
rdr pass on lo0 inet proto tcp from any to any port 443 -> 127.0.0.1 port %s

# Allow redirected traffic
pass out quick on lo0 inet proto tcp from any to 127.0.0.1 port %s
`, proxyPort, proxyPort, proxyPort)

	// Write config file
	configPath := "/etc/pf.stronghold.conf"
	if err := os.WriteFile(configPath, []byte(pfConf), 0644); err != nil {
		return fmt.Errorf("failed to write pf config: %w", err)
	}

	// Load pf rules
	cmd := exec.Command("pfctl", "-f", configPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pfctl failed: %s - %s", err, string(output))
	}

	// Enable pf
	exec.Command("pfctl", "-e").Run()

	return nil
}

func (t *TransparentProxy) disableDarwin() error {
	// Remove our rules by reloading default config
	exec.Command("pfctl", "-f", "/etc/pf.conf").Run()

	// Remove our config file
	os.Remove("/etc/pf.stronghold.conf")

	return nil
}

func (t *TransparentProxy) statusDarwin() (bool, error) {
	cmd := exec.Command("pfctl", "-sr")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), "stronghold"), nil
}

// ==================== Proxy Modification ====================

// GetOriginalDst retrieves the original destination for a transparently redirected connection
// This is used by the proxy to know where to forward the request
func GetOriginalDst(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		return "", err
	}
	defer file.Close()

	fd := file.Fd()

	// Use SO_ORIGINAL_DST to get the original destination
	// This requires different syscalls on Linux vs other systems
	if runtime.GOOS == "linux" {
		return getOriginalDstLinux(int(fd))
	}

	// For other systems, return the local address (fallback)
	return conn.LocalAddr().String(), nil
}

// getOriginalDstLinux uses SO_ORIGINAL_DST to get the original destination
func getOriginalDstLinux(fd int) (string, error) {
	// SO_ORIGINAL_DST = 80
	const SO_ORIGINAL_DST = 80

	// This requires syscall.SockaddrInet4/6 parsing
	// For now, return error - would need cgo or unsafe for full implementation
	return "", fmt.Errorf("SO_ORIGINAL_DST requires platform-specific implementation")
}

// IsTransparentProxyEnabled checks if transparent proxying is currently active
func IsTransparentProxyEnabled(config *CLIConfig) bool {
	tp := NewTransparentProxy(config)
	status, _ := tp.Status()
	return status
}
