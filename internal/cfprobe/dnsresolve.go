package cfprobe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// updateDNSServers 用于更新检查的公共 DNS（UDP 53），规避系统 DNS 被污染
// 导致 api.github.com / raw.githubusercontent.com 无法解析的问题。
var updateDNSServers = []string{
	"223.5.5.5:53",              // 阿里 DNS
	"119.29.29.29:53",           // DNSPod
	"114.114.114.114:53",        // 114 DNS
	"1.1.1.1:53",                // Cloudflare
	"8.8.8.8:53",                // Google
	"8.8.4.4:53",                // Google 备用
	"[2606:4700:4700::1111]:53", // Cloudflare IPv6
	"[2001:4860:4860::8888]:53", // Google IPv6
}

const updateDNSServerEnv = "CF_PROBE_UPDATE_DNS"

var (
	updatePreferV4Once sync.Once
	updateHasIPv4      bool

	sharedClientsMu sync.Mutex
	sharedClients   = map[sharedClientKey]*http.Client{}
)

type sharedClientKey struct {
	timeout      time.Duration
	usePublicDNS bool
}

// usePublicDNSResolver 决定是否启用内置公共 DNS 轮询解析。
// 仅在配置了 --install-ghproxy（UpdateProxy 非空，通常为国内服务器）或
// 显式设置 CF_PROBE_UPDATE_DNS 时启用；海外服务器直接使用系统原生 DNS，
// 避免公共 DNS 出口差异导致解析到非最优地址甚至解析失败。
func usePublicDNSResolver(cfg Config) bool {
	if strings.TrimSpace(os.Getenv(updateDNSServerEnv)) != "" {
		return true
	}
	return strings.TrimSpace(cfg.UpdateProxy) != ""
}

// sharedReportHTTPClient 返回按 timeout 和 DNS 模式缓存的共用客户端，
// 供上报等高频调用复用 TCP 连接，避免每次重新解析和握手。
func sharedReportHTTPClient(timeout time.Duration, usePublicDNS bool) *http.Client {
	sharedClientsMu.Lock()
	defer sharedClientsMu.Unlock()
	key := sharedClientKey{timeout: timeout, usePublicDNS: usePublicDNS}
	if client := sharedClients[key]; client != nil {
		return client
	}
	client := newUpdateHTTPClient(timeout, usePublicDNS)
	sharedClients[key] = client
	return client
}

// normalizeUpdateDNSServer 将 DNS 服务器字符串规范化为 host:port 形式。
func normalizeUpdateDNSServer(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "[") && strings.Contains(s, "]:")) || (strings.Count(s, ":") == 1 && !strings.Contains(s, "]")) {
		return s
	}
	if strings.Count(s, ":") >= 2 && !strings.Contains(s, "]") {
		return "[" + s + "]:53"
	}
	if !strings.Contains(s, ":") {
		return s + ":53"
	}
	return s
}

func updateResolverServers() []string {
	if custom := strings.TrimSpace(os.Getenv(updateDNSServerEnv)); custom != "" {
		return append([]string{normalizeUpdateDNSServer(custom)}, updateDNSServers...)
	}
	return updateDNSServers
}

// newUpdateResolver 返回使用内置公共 DNS 的解析器，不依赖系统 DNS。
func newUpdateResolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 5 * time.Second}
			for _, server := range updateResolverServers() {
				if conn, err := dialer.DialContext(ctx, "udp", server); err == nil {
					return conn, nil
				}
			}
			return nil, fmt.Errorf("no available DNS server")
		},
	}
}

// newUpdateHTTPClient 返回更新检查/下载专用的 HTTP 客户端。
// usePublicDNS 为 true 时优先使用内置公共 DNS 解析，失败时回退系统 DNS，
// 并逐个尝试解析到的 IP；为 false 时使用系统原生 DNS 的默认拨号行为。
func newUpdateHTTPClient(timeout time.Duration, usePublicDNS bool) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	if usePublicDNS {
		resolver := newUpdateResolver()
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolver.LookupHost(ctx, host)
			if err != nil {
				ips, err = net.DefaultResolver.LookupHost(ctx, host)
				if err != nil {
					return nil, err
				}
			}
			sortUpdateIPsByPreference(ips)
			dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				return nil, fmt.Errorf("no addresses resolved for %s", host)
			}
			return nil, fmt.Errorf("failed to dial %s: %w", host, lastErr)
		}
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func sortUpdateIPsByPreference(ips []string) {
	preferV4 := preferUpdateIPv4First()
	preferred := make([]string, 0, len(ips))
	others := make([]string, 0, len(ips))
	for _, ip := range ips {
		isV4 := net.ParseIP(ip) != nil && net.ParseIP(ip).To4() != nil
		if isV4 == preferV4 {
			preferred = append(preferred, ip)
		} else {
			others = append(others, ip)
		}
	}
	n := copy(ips, preferred)
	copy(ips[n:], others)
}

// preferUpdateIPv4First 检测本机是否存在可用的 IPv4 地址。
func preferUpdateIPv4First() bool {
	updatePreferV4Once.Do(func() {
		ifaces, _ := net.Interfaces()
		for _, iface := range ifaces {
			if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() {
					continue
				}
				if ip.To4() != nil {
					updateHasIPv4 = true
					return
				}
			}
		}
	})
	return updateHasIPv4
}
