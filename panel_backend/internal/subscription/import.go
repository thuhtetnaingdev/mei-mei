package subscription

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ParsedProxy struct {
	Protocol    string `json:"protocol"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	UUID        string `json:"uuid,omitempty"`
	Password    string `json:"password,omitempty"`
	Method      string `json:"method,omitempty"`
	Flow        string `json:"flow,omitempty"`
	Security    string `json:"security,omitempty"`
	Network     string `json:"network,omitempty"`
	SNI         string `json:"sni,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	ShortID     string `json:"shortId,omitempty"`
	Path        string `json:"path,omitempty"`
	Remark      string `json:"remark,omitempty"`
	RawURI      string `json:"rawUri"`
}

type TestResult struct {
	URI       string `json:"uri"`
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Working   bool   `json:"working"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

type ImportResult struct {
	Parsed    []ParsedProxy          `json:"parsed"`
	Tested    []TestResult           `json:"tested"`
	Working   []SingboxOutbound      `json:"working"`
	FailCount int                    `json:"failCount"`
	TotalURLs int                    `json:"totalUrls"`
}

type SingboxOutbound struct {
	Tag       string                 `json:"tag"`
	Config    map[string]interface{} `json:"config"`
	LatencyMs int64                  `json:"latencyMs"`
	Remark    string                 `json:"remark"`
	RawURI    string                 `json:"rawUri"`
}

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
}

func FetchSubscription(urlStr string) ([]string, error) {
	resp, err := httpClient.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return nil, fmt.Errorf("empty subscription data")
	}

	decoded := raw
	if decodedBytes, err := base64.StdEncoding.DecodeString(raw); err == nil {
		decoded = string(decodedBytes)
	} else if decodedBytes, err := base64.RawStdEncoding.DecodeString(raw); err == nil {
		decoded = string(decodedBytes)
	} else if decodedBytes, err := base64.URLEncoding.DecodeString(raw); err == nil {
		decoded = string(decodedBytes)
	} else if decodedBytes, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		decoded = string(decodedBytes)
	}

	lines := strings.Split(strings.TrimSpace(decoded), "\n")
	uris := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		uris = append(uris, line)
	}
	return uris, nil
}

func ParseAll(uris []string) []ParsedProxy {
	parsed := make([]ParsedProxy, 0, len(uris))
	for _, uri := range uris {
		if p := ParseURI(uri); p != nil {
			p.RawURI = uri
			parsed = append(parsed, *p)
		}
	}
	return parsed
}

func ParseURI(raw string) *ParsedProxy {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	protocol := strings.ToLower(u.Scheme)
	remark := strings.TrimSpace(u.Fragment)

	switch protocol {
	case "vless":
		return parseVLESS(u, raw, remark)
	case "vmess":
		return parseVMess(u, raw, remark)
	case "trojan":
		return parseTrojan(u, raw, remark)
	case "ss":
		return parseShadowsocks(u, raw, remark)
	case "hysteria2", "hy2":
		return parseHysteria2(u, raw, remark)
	case "tuic":
		return parseTUIC(u, raw, remark)
	default:
		return nil
	}
}

func parseVLESS(u *url.URL, raw, remark string) *ParsedProxy {
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil
	}

	q := u.Query()
	p := &ParsedProxy{
		Protocol:  "vless",
		Host:      host,
		Port:      port,
		UUID:      strings.TrimPrefix(u.User.String(), ":"),
		Flow:      q.Get("flow"),
		Security:  q.Get("security"),
		Network:   q.Get("type"),
		SNI:       q.Get("sni"),
		PublicKey: q.Get("pbk"),
		ShortID:   q.Get("sid"),
		Path:      q.Get("path"),
		Remark:    remark,
	}
	if p.Security == "" {
		p.Security = "none"
	}
	if p.Network == "" {
		p.Network = "tcp"
	}
	return p
}

func parseVMess(u *url.URL, raw, remark string) *ParsedProxy {
	var vmessConfig struct {
		Add  string `json:"add"`
		Host string `json:"host"`
		Port int    `json:"port"`
		Ps   string `json:"ps"`
		ID   string `json:"id"`
		Aid  int    `json:"aid"`
		Net  string `json:"net"`
		Type string `json:"type"`
		TLS  string `json:"tls"`
		Path string `json:"path"`
		Scy  string `json:"scy"`
	}

	rawConfig := raw
	after := strings.TrimPrefix(raw, "vmess://")
	if decoded, err := base64.StdEncoding.DecodeString(after); err == nil {
		rawConfig = string(decoded)
	} else if decoded, err := base64.RawStdEncoding.DecodeString(after); err == nil {
		rawConfig = string(decoded)
	}

	if err := json.Unmarshal([]byte(rawConfig), &vmessConfig); err != nil {
		return nil
	}

	host := vmessConfig.Add
	port := vmessConfig.Port
	remark2 := vmessConfig.Ps
	if remark2 == "" {
		remark2 = remark
	}

	security := "none"
	if vmessConfig.TLS == "tls" {
		security = "tls"
	}

	p := &ParsedProxy{
		Protocol: "vmess",
		Host:     host,
		Port:     port,
		UUID:     vmessConfig.ID,
		Security: security,
		Network:  vmessConfig.Net,
		SNI:      vmessConfig.Host,
		Path:     vmessConfig.Path,
		Remark:   remark2,
	}
	if p.Network == "" {
		p.Network = "tcp"
	}
	return p
}

func parseTrojan(u *url.URL, raw, remark string) *ParsedProxy {
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil
	}

	q := u.Query()
	password, _ := u.User.Password()
	if password == "" {
		password = u.User.String()
	}

	p := &ParsedProxy{
		Protocol: "trojan",
		Host:     host,
		Port:     port,
		Password: password,
		Security: "tls",
		SNI:      q.Get("sni"),
		Remark:   remark,
	}
	if p.SNI == "" {
		p.SNI = host
	}
	return p
}

func parseShadowsocks(u *url.URL, raw, remark string) *ParsedProxy {
	userInfo := u.User.String()

	var method, password string
	if idx := strings.Index(userInfo, ":"); idx != -1 {
		methodEncoded := userInfo[:idx]
		password = userInfo[idx+1:]

		methodDecoded, err := base64.StdEncoding.DecodeString(methodEncoded)
		if err == nil {
			method = string(methodDecoded)
		} else {
			method = methodEncoded
		}
	} else {
		decoded, err := base64.StdEncoding.DecodeString(userInfo)
		if err == nil {
			decodedStr := string(decoded)
			if idx2 := strings.Index(decodedStr, ":"); idx2 != -1 {
				method = decodedStr[:idx2]
				password = decodedStr[idx2+1:]
			}
		}
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil
	}

	return &ParsedProxy{
		Protocol: "shadowsocks",
		Host:     host,
		Port:     port,
		Method:   method,
		Password: password,
		Remark:   remark,
	}
}

func parseHysteria2(u *url.URL, raw, remark string) *ParsedProxy {
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil
	}

	q := u.Query()
	password, _ := u.User.Password()
	if password == "" {
		password = u.User.String()
	}

	p := &ParsedProxy{
		Protocol: "hysteria2",
		Host:     host,
		Port:     port,
		Password: password,
		Security: "tls",
		SNI:      q.Get("sni"),
		Remark:   remark,
	}
	if p.SNI == "" {
		p.SNI = host
	}
	return p
}

func parseTUIC(u *url.URL, raw, remark string) *ParsedProxy {
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil
	}

	q := u.Query()
	userInfo := u.User.String()
	var uuid, password string
	if idx := strings.Index(userInfo, ":"); idx != -1 {
		uuid = userInfo[:idx]
		password = userInfo[idx+1:]
	} else {
		uuid = userInfo
		password = userInfo
	}

	p := &ParsedProxy{
		Protocol: "tuic",
		Host:     host,
		Port:     port,
		UUID:     uuid,
		Password: password,
		Security: "tls",
		SNI:      q.Get("sni"),
		Remark:   remark,
	}
	if p.SNI == "" {
		p.SNI = host
	}
	return p
}

func TestAll(proxies []ParsedProxy) []TestResult {
	return TestAllWithConcurrency(proxies, 100)
}

func TestAllWithConcurrency(proxies []ParsedProxy, concurrency int) []TestResult {
	if len(proxies) == 0 {
		return nil
	}

	results := make([]TestResult, len(proxies))
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, p := range proxies {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, p ParsedProxy) {
			defer func() { <-sem; wg.Done() }()
			ok, lat := basicTest(p)
			errStr := ""
			if !ok {
				errStr = "tcp/tls handshake failed"
			}
			mu.Lock()
			results[i] = TestResult{
				URI:       p.RawURI,
				Protocol:  p.Protocol,
				Host:      p.Host,
				Port:      p.Port,
				Working:   ok,
				LatencyMs: lat,
				Error:     errStr,
			}
			mu.Unlock()
		}(i, p)
	}
	wg.Wait()

	return results
}

func basicTest(p ParsedProxy) (bool, int64) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", p.Host, p.Port), 10*time.Second)
	if err != nil {
		return false, time.Since(start).Milliseconds()
	}
	defer conn.Close()
	latency := time.Since(start).Milliseconds()

	if p.Security == "tls" || p.Security == "reality" {
		sni := p.SNI
		if sni == "" {
			sni = p.Host
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		})
		if err := tlsConn.Handshake(); err != nil {
			return false, time.Since(start).Milliseconds()
		}
		tlsConn.Close()
	}
	return true, latency
}

func ConvertWorking(proxies []ParsedProxy, results []TestResult) []SingboxOutbound {
	working := make([]SingboxOutbound, 0, len(results))
	resultMap := make(map[string]TestResult)
	for _, r := range results {
		resultMap[r.URI] = r
	}

	for _, p := range proxies {
		r, ok := resultMap[p.RawURI]
		if !ok || !r.Working {
			continue
		}
		outbound := proxyToSingbox(p)
		if outbound == nil {
			continue
		}
		tag := fmt.Sprintf("import-%s-%s-%d", p.Protocol, sanitizeTag(p.Host), p.Port)
		working = append(working, SingboxOutbound{
			Tag:       tag,
			Config:    outbound,
			LatencyMs: r.LatencyMs,
			Remark:    p.Remark,
			RawURI:    p.RawURI,
		})
	}
	return working
}

func proxyToSingbox(p ParsedProxy) map[string]interface{} {
	switch p.Protocol {
	case "vless":
		return vlessToSingbox(p)
	case "vmess":
		return vmessToSingbox(p)
	case "trojan":
		return trojanToSingbox(p)
	case "shadowsocks":
		return shadowsocksToSingbox(p)
	case "hysteria2":
		return hysteria2ToSingbox(p)
	case "tuic":
		return tuicToSingbox(p)
	default:
		return nil
	}
}

func vlessToSingbox(p ParsedProxy) map[string]interface{} {
	outbound := map[string]interface{}{
		"type":        "vless",
		"server":      p.Host,
		"server_port": p.Port,
		"uuid":        p.UUID,
		"flow":        p.Flow,
		"network":     p.Network,
	}

	if p.Security == "tls" || p.Security == "reality" {
		tlsConfig := map[string]interface{}{
			"enabled":     true,
			"insecure":    true,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		}
		if p.SNI != "" {
			tlsConfig["server_name"] = p.SNI
		} else {
			tlsConfig["server_name"] = p.Host
		}
		if p.Security == "reality" && p.PublicKey != "" {
			tlsConfig["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": p.PublicKey,
				"short_id":   p.ShortID,
			}
		}
		outbound["tls"] = tlsConfig
	}

	if p.Network == "ws" {
		outbound["transport"] = map[string]interface{}{
			"type":    "ws",
			"path":    p.Path,
			"headers": map[string]interface{}{},
		}
	}

	return outbound
}

func vmessToSingbox(p ParsedProxy) map[string]interface{} {
	outbound := map[string]interface{}{
		"type":        "vmess",
		"server":      p.Host,
		"server_port": p.Port,
		"uuid":        p.UUID,
		"network":     p.Network,
	}

	if p.Security == "tls" {
		tlsConfig := map[string]interface{}{
			"enabled":  true,
			"insecure": true,
		}
		if p.SNI != "" {
			tlsConfig["server_name"] = p.SNI
		} else {
			tlsConfig["server_name"] = p.Host
		}
		outbound["tls"] = tlsConfig
	}

	if p.Network == "ws" {
		outbound["transport"] = map[string]interface{}{
			"type":    "ws",
			"path":    p.Path,
			"headers": map[string]interface{}{},
		}
	}

	return outbound
}

func trojanToSingbox(p ParsedProxy) map[string]interface{} {
	tlsConfig := map[string]interface{}{
		"enabled":  true,
		"insecure": true,
	}
	if p.SNI != "" {
		tlsConfig["server_name"] = p.SNI
	} else {
		tlsConfig["server_name"] = p.Host
	}

	return map[string]interface{}{
		"type":        "trojan",
		"server":      p.Host,
		"server_port": p.Port,
		"password":    p.Password,
		"tls":         tlsConfig,
	}
}

func shadowsocksToSingbox(p ParsedProxy) map[string]interface{} {
	method := p.Method
	if method == "" {
		method = "aes-256-gcm"
	}
	return map[string]interface{}{
		"type":        "shadowsocks",
		"server":      p.Host,
		"server_port": p.Port,
		"method":      method,
		"password":    p.Password,
		"network":     "tcp",
	}
}

func hysteria2ToSingbox(p ParsedProxy) map[string]interface{} {
	tlsConfig := map[string]interface{}{
		"enabled":     true,
		"insecure":    true,
	}
	if p.SNI != "" {
		tlsConfig["server_name"] = p.SNI
	} else {
		tlsConfig["server_name"] = p.Host
	}

	return map[string]interface{}{
		"type":        "hysteria2",
		"server":      p.Host,
		"server_port": p.Port,
		"password":    p.Password,
		"tls":         tlsConfig,
	}
}

func tuicToSingbox(p ParsedProxy) map[string]interface{} {
	tlsConfig := map[string]interface{}{
		"enabled":     true,
		"insecure":    true,
	}
	if p.SNI != "" {
		tlsConfig["server_name"] = p.SNI
	} else {
		tlsConfig["server_name"] = p.Host
	}

	return map[string]interface{}{
		"type":               "tuic",
		"server":             p.Host,
		"server_port":        p.Port,
		"uuid":               p.UUID,
		"password":           p.Password,
		"congestion_control": "bbr",
		"tls":                tlsConfig,
	}
}

func splitHostPort(hostport string) (string, int, error) {
	host := hostport
	port := 0

	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host = h
		if n, err := fmt.Sscanf(p, "%d", &port); err != nil || n != 1 {
			return "", 0, fmt.Errorf("invalid port: %s", p)
		}
	} else {
		if strings.HasPrefix(hostport, "[") {
			return "", 0, fmt.Errorf("missing port in IPv6 address")
		}
	}

	if host == "" {
		return "", 0, fmt.Errorf("empty host")
	}
	if port == 0 {
		return host, 443, nil
	}

	return host, port, nil
}

func sanitizeTag(s string) string {
	r := strings.NewReplacer(
		".", "-",
		":", "-",
		"/", "-",
		"@", "-",
		" ", "-",
		"_", "-",
	)
	return strings.Trim(r.Replace(strings.ToLower(s)), "-")
}

func ImportSubscription(subURL string) (*ImportResult, error) {
	uris, err := FetchSubscription(subURL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	if len(uris) == 0 {
		return nil, fmt.Errorf("no valid URIs found in subscription")
	}

	parsed := ParseAll(uris)
	tested := TestAll(parsed)
	working := ConvertWorking(parsed, tested)

	failCount := 0
	for _, t := range tested {
		if !t.Working {
			failCount++
		}
	}

	return &ImportResult{
		Parsed:    parsed,
		Tested:    tested,
		Working:   working,
		FailCount: failCount,
		TotalURLs: len(uris),
	}, nil
}
