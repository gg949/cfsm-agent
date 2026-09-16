package cfprobe

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// 公网 IP 查询接口列表，参考 komari-agent 的多源回退策略：
// 任意一个源失败时依次尝试下一个，优先国内可达性较好的源。
var ipv4LookupEndpoints = []string{
	"https://www.visa.cn/cdn-cgi/trace",
	"https://www.qualcomm.cn/cdn-cgi/trace",
	"https://edge-ip.html.zone/geo",
	"https://vercel-ip.html.zone/geo",
	"http://ipv4.ip.sb",
	"https://api.ipify.org?format=json",
	"https://cloudflare.com/cdn-cgi/trace",
}

var ipv6LookupEndpoints = []string{
	"https://v6.ip.zxinc.org/info.php?type=json",
	"https://api6.ipify.org?format=json",
	"https://ipv6.icanhazip.com",
	"http://api-ipv6.ip.sb/geoip",
	"https://cloudflare.com/cdn-cgi/trace",
}

const (
	ipLookupUserAgent = "curl/8.0.1"
	ipLookupTimeout   = 8 * time.Second
	ipLookupMaxBody   = 4096
)

var (
	ipv4BodyRE = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	ipv6BodyRE = regexp.MustCompile(`([0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}|([0-9A-Fa-f]{1,4}:){1,6}:([0-9A-Fa-f]{1,4}:){0,4}[0-9A-Fa-f]{0,4}`)
)

// lookupPublicIP 通过多个公共接口查询本机公网 IP。
// network 为 "tcp4" 或 "tcp6"，锁定协议族避免取到错误版本的地址。
// usePublicDNS 为 true 时优先使用内置公共 DNS 解析，否则使用系统原生 DNS。
// 全部源失败时返回空字符串，由调用方决定保留旧值或使用占位符。
func lookupPublicIP(network string, log logger, usePublicDNS bool) string {
	endpoints := ipv4LookupEndpoints
	wantV4 := true
	if network == "tcp6" {
		endpoints = ipv6LookupEndpoints
		wantV4 = false
	}
	client := newIPLookupClient(network, ipLookupTimeout, usePublicDNS)
	defer client.CloseIdleConnections()
	for _, endpoint := range endpoints {
		ip, err := fetchIPFromEndpoint(client, endpoint, wantV4)
		if err != nil {
			log.debugf("lookup %s via %s failed: %v", network, endpoint, err)
			continue
		}
		if ip == "" {
			log.debugf("lookup %s via %s: no valid address in response", network, endpoint)
			continue
		}
		log.debugf("lookup %s via %s success: %s", network, endpoint, ip)
		return ip
	}
	return ""
}

func fetchIPFromEndpoint(client *http.Client, endpoint string, wantV4 bool) (string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ipLookupUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, ipLookupMaxBody))
	if err != nil {
		return "", err
	}
	return extractIPFromBody(string(body), wantV4), nil
}

// extractIPFromBody 从响应体中提取 IP：
// 先解析 cdn-cgi/trace 的 ip= 行，再用正则兜底并用 net.ParseIP 校验，
// 同时校验协议族与期望一致，避免 IPv6 源返回 IPv4 映射地址等情况。
func extractIPFromBody(body string, wantV4 bool) string {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "ip=") {
			continue
		}
		if ip := net.ParseIP(strings.TrimSpace(strings.TrimPrefix(line, "ip="))); ip != nil && (ip.To4() != nil) == wantV4 {
			return ip.String()
		}
	}
	candidateRE := ipv4BodyRE
	if !wantV4 {
		candidateRE = ipv6BodyRE
	}
	for _, candidate := range candidateRE.FindAllString(body, -1) {
		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		if (ip.To4() != nil) == wantV4 {
			return ip.String()
		}
	}
	return ""
}

// newIPLookupClient 返回锁定协议族（tcp4/tcp6）的 HTTP 客户端。
// usePublicDNS 为 true 时解析优先走内置公共 DNS（规避系统 DNS 污染），
// 失败时回退系统 DNS；为 false 时直接使用系统原生 DNS。
func newIPLookupClient(network string, timeout time.Duration, usePublicDNS bool) *http.Client {
	if timeout <= 0 {
		timeout = ipLookupTimeout
	}
	lookupNetwork := "ip"
	if network == "tcp4" {
		lookupNetwork = "ip4"
	} else if network == "tcp6" {
		lookupNetwork = "ip6"
	}
	resolver := net.DefaultResolver
	if usePublicDNS {
		resolver = newUpdateResolver()
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolver.LookupIP(ctx, lookupNetwork, host)
			if usePublicDNS && (err != nil || len(ips) == 0) {
				ips, err = net.DefaultResolver.LookupIP(ctx, lookupNetwork, host)
				if err != nil {
					return nil, err
				}
			}
			dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				return nil, fmt.Errorf("no %s addresses resolved for %s", lookupNetwork, host)
			}
			return nil, fmt.Errorf("failed to dial %s: %w", host, lastErr)
		},
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}
