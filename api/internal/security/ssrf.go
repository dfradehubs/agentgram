package security

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// blockedHeaders contains headers that must never be forwarded to outbound requests.
// These can be used to bypass cloud metadata API protections or leak internal state.
var blockedHeaders = map[string]bool{
	"metadata-flavor":           true, // GCP metadata API
	"x-google-metadata-request": true, // GCP metadata API (legacy)
	"x-aws-ec2-metadata-token":  true, // AWS IMDSv2
	"x-forwarded-for":           true,
	"x-forwarded-host":          true,
	"x-real-ip":                 true,
}

// blockedHeaderPrefixes contains header prefixes that should be blocked.
var blockedHeaderPrefixes = []string{
	"x-goog-",  // GCP internal headers
	"x-gfe-",   // Google Front End headers
	"x-amzn-",  // AWS internal headers
	"x-azure-", // Azure internal headers
}

// metadataNetworks contains CIDR ranges used by cloud metadata services.
// Only these are blocked — legitimate cluster-internal IPs (10.x, 172.x) are allowed
// since agent endpoints are configured by admins.
var metadataNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"169.254.0.0/16",    // Link-local (cloud metadata: 169.254.169.254)
		"fd00:ec2::254/128", // AWS IPv6 metadata
	}
	for _, cidr := range cidrs {
		_, network, _ := net.ParseCIDR(cidr)
		metadataNetworks = append(metadataNetworks, network)
	}
}

// blockedHostnames contains hostnames that must never be targeted.
var blockedHostnames = map[string]bool{
	"metadata.google.internal":             true,
	"metadata.goog":                        true,
	"169.254.169.254":                      true,
	"[fd00:ec2::254]":                      true,
	"metadata.azure.internal":              true,
	"kubernetes.default":                   true,
	"kubernetes.default.svc":               true,
	"kubernetes.default.svc.cluster.local": true,
}

// ValidateEndpointURL validates that a URL is safe to use as an outbound endpoint.
// It blocks cloud metadata endpoints and dangerous schemes. Internal cluster IPs
// (10.x, 172.x, etc.) are allowed since endpoints are configured by admins.
func ValidateEndpointURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Enforce HTTPS or HTTP scheme only
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("only http and https schemes are allowed, got: %s", scheme)
	}

	hostname := strings.ToLower(parsed.Hostname())

	// Block known cloud metadata hostnames
	if blockedHostnames[hostname] {
		return fmt.Errorf("endpoint targets a blocked internal hostname: %s", hostname)
	}

	// Resolve hostname and check if it points to a metadata IP
	ips, err := net.ResolveIPAddr("ip", hostname)
	if err != nil {
		// Allow unresolvable hostnames — they may resolve inside the cluster at runtime
		return nil
	}

	if isMetadataIP(ips.IP) {
		return fmt.Errorf("endpoint resolves to a cloud metadata IP address: %s -> %s", hostname, ips.IP)
	}

	return nil
}

// ValidateHeaders checks that no blocked headers are present in the map.
func ValidateHeaders(headers map[string]string) error {
	for key := range headers {
		lower := strings.ToLower(key)

		if blockedHeaders[lower] {
			return fmt.Errorf("header %q is not allowed", key)
		}

		for _, prefix := range blockedHeaderPrefixes {
			if strings.HasPrefix(lower, prefix) {
				return fmt.Errorf("header %q is not allowed (blocked prefix: %s)", key, prefix)
			}
		}
	}
	return nil
}

// FilterHeaders returns a copy of headers with blocked entries removed.
// Use this at the HTTP client level as a defense-in-depth measure.
func FilterHeaders(headers map[string]string) map[string]string {
	filtered := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)

		if blockedHeaders[lower] {
			continue
		}

		blocked := false
		for _, prefix := range blockedHeaderPrefixes {
			if strings.HasPrefix(lower, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered[key] = value
		}
	}
	return filtered
}

// NewSafeTransport creates an http.Transport that blocks connections to cloud
// metadata IPs (169.254.x.x) while allowing legitimate cluster-internal traffic.
// Header filtering (FilterHeaders) provides the primary SSRF defense.
func NewSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			// Block metadata hostnames at transport level too
			if blockedHostnames[strings.ToLower(host)] {
				return nil, fmt.Errorf("connection to metadata endpoint %s is blocked", host)
			}

			// Resolve the hostname
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
			}

			// Block only metadata IPs — allow cluster-internal IPs
			for _, ip := range ips {
				if isMetadataIP(ip.IP) {
					return nil, fmt.Errorf("connection to metadata IP %s (%s) is blocked", ip.IP, host)
				}
			}

			// Connect to the first resolved IP
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 10 * time.Second,
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
		// 5m: fan-out agents can take >150s before emitting the first byte.
		// Callers with stricter needs cap it via Client.Timeout or context.
		ResponseHeaderTimeout: 5 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
	}
}

func isMetadataIP(ip net.IP) bool {
	for _, network := range metadataNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
