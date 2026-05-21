package subscription

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

const (
	testHost       = "www.gstatic.com"
	testPath       = "/generate_204"
	testPort       = 80
	testTimeout    = 15 * time.Second
	xrayStartWait  = 800 * time.Millisecond
	xrayStartLimit  = 50
	xraySocksLimit  = 100
)

var xrayPaths = []string{
	"/usr/local/bin/xray",
	"/usr/local/xray/xray",
	"/opt/homebrew/bin/xray",
}

var socksPortCounter uint32 = 19000

func findXrayBinary() string {
	for _, p := range xrayPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("xray"); err == nil {
		return path
	}
	return ""
}

func nextSocksPort() int {
	return int(atomic.AddUint32(&socksPortCounter, 1))
}

func isXraySupported(p ParsedProxy) bool {
	switch p.Protocol {
	case "vless", "vmess", "trojan", "shadowsocks":
		return true
	}
	return false
}

func buildXrayConfig(p ParsedProxy, socksPort int) map[string]interface{} {
	inbound := map[string]interface{}{
		"port":     socksPort,
		"listen":   "127.0.0.1",
		"protocol": "socks",
		"settings": map[string]interface{}{
			"auth": "noauth",
			"udp":  false,
		},
	}

	return map[string]interface{}{
		"inbounds":  []map[string]interface{}{inbound},
		"outbounds": []map[string]interface{}{buildXrayOutbound(p)},
		"log":       map[string]interface{}{"loglevel": "error"},
	}
}

func buildXrayOutbound(p ParsedProxy) map[string]interface{} {
	streamSettings := buildXrayStreamSettings(p)

	switch p.Protocol {
	case "vless":
		return map[string]interface{}{
			"protocol": "vless",
			"settings": map[string]interface{}{
				"vnext": []map[string]interface{}{
					{
						"address": p.Host,
						"port":    p.Port,
						"users": []map[string]interface{}{
							{
								"id":         p.UUID,
								"flow":       p.Flow,
								"encryption": "none",
							},
						},
					},
				},
			},
			"streamSettings": streamSettings,
		}
	case "vmess":
		return map[string]interface{}{
			"protocol": "vmess",
			"settings": map[string]interface{}{
				"vnext": []map[string]interface{}{
					{
						"address": p.Host,
						"port":    p.Port,
						"users": []map[string]interface{}{
							{
								"id": p.UUID,
							},
						},
					},
				},
			},
			"streamSettings": streamSettings,
		}
	case "trojan":
		return map[string]interface{}{
			"protocol": "trojan",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{
					{
						"address":  p.Host,
						"port":     p.Port,
						"password": p.Password,
					},
				},
			},
			"streamSettings": streamSettings,
		}
	case "shadowsocks":
		method := p.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		return map[string]interface{}{
			"protocol": "shadowsocks",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{
					{
						"address":  p.Host,
						"port":     p.Port,
						"method":   method,
						"password": p.Password,
					},
				},
			},
			"streamSettings": streamSettings,
		}
	}
	return nil
}

func buildXrayStreamSettings(p ParsedProxy) map[string]interface{} {
	settings := map[string]interface{}{
		"network":  p.Network,
		"security": p.Security,
	}

	if p.Network == "ws" {
		wsSettings := map[string]interface{}{
			"path": p.Path,
		}
		if p.SNI != "" {
			wsSettings["headers"] = map[string]interface{}{
				"Host": p.SNI,
			}
		}
		settings["wsSettings"] = wsSettings
	}

	if p.Security == "tls" || p.Security == "reality" {
		sni := p.SNI
		if sni == "" {
			sni = p.Host
		}
		if p.Security == "tls" {
			settings["tlsSettings"] = map[string]interface{}{
				"serverName": sni,
			}
		} else if p.Security == "reality" {
			realitySettings := map[string]interface{}{
				"serverName":  sni,
				"fingerprint": "chrome",
			}
			if p.PublicKey != "" {
				realitySettings["publicKey"] = p.PublicKey
			}
			if p.ShortID != "" {
				realitySettings["shortId"] = p.ShortID
			}
			settings["realitySettings"] = realitySettings
		}
	}

	return settings
}

func socks5Connect(proxyHost string, proxyPort int, targetHost string, targetPort int, timeout time.Duration) (bool, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", proxyHost, proxyPort), timeout)
	if err != nil {
		return false, fmt.Errorf("socks5 dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false, fmt.Errorf("socks5 handshake write: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return false, fmt.Errorf("socks5 handshake read: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return false, fmt.Errorf("socks5 handshake rejected: %v", resp)
	}

	domainLen := len(targetHost)
	req := make([]byte, 0, 4+1+domainLen+2)
	req = append(req, 0x05, 0x01, 0x00, 0x03, byte(domainLen))
	req = append(req, []byte(targetHost)...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		return false, fmt.Errorf("socks5 connect write: %w", err)
	}

	resp = make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return false, fmt.Errorf("socks5 connect read: %w", err)
	}
	if resp[1] != 0x00 {
		return false, fmt.Errorf("socks5 connect rejected: code %d", resp[1])
	}
	// drain the remaining SOCKS5 response (bind address + port)
	switch resp[3] {
	case 0x01: // IPv4 - 4 bytes addr + 2 bytes port
		io.ReadFull(conn, make([]byte, 6))
	case 0x03: // domain - 1 byte len + len bytes addr + 2 bytes port
		domainLenResp := make([]byte, 1)
		io.ReadFull(conn, domainLenResp)
		io.ReadFull(conn, make([]byte, int(domainLenResp[0])+2))
	case 0x04: // IPv6 - 16 bytes addr + 2 bytes port
		io.ReadFull(conn, make([]byte, 18))
	}

	httpReq := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0 (compatible; Proxy-Test/1.0)\r\nConnection: close\r\n\r\n", testPath, testHost)
	if _, err := conn.Write([]byte(httpReq)); err != nil {
		return false, fmt.Errorf("http write: %w", err)
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("http read: %w", err)
	}

	if len(response) < 9 {
		return false, fmt.Errorf("invalid http response: %s", response)
	}
	statusCode := strings.TrimSpace(response[9:12])
	code, _ := strconv.Atoi(statusCode)
	if code >= 200 && code < 400 {
		return true, nil
	}
	return false, fmt.Errorf("http status %s", statusCode)
}

type xrayInstance struct {
	cmd        *exec.Cmd
	socksPort  int
	configPath string
}

func testXrayBatch(proxies []ParsedProxy, indices []int, results []TestResult) {
	xrayBin := findXrayBinary()
	if xrayBin == "" {
		for i, p := range proxies {
			ok, lat := basicTest(p)
			results[indices[i]] = TestResult{
				URI:       p.RawURI,
				Protocol:  p.Protocol,
				Host:      p.Host,
				Port:      p.Port,
				Working:   ok,
				LatencyMs: lat,
			}
		}
		return
	}

	instances := make([]*xrayInstance, len(proxies))
	var startWg sync.WaitGroup
	startSem := make(chan struct{}, xrayStartLimit)

	for i, p := range proxies {
		startSem <- struct{}{}
		startWg.Add(1)
		go func(i int, p ParsedProxy) {
			defer func() { <-startSem; startWg.Done() }()
			socksPort := nextSocksPort()
			config := buildXrayConfig(p, socksPort)
			configJSON, _ := json.Marshal(config)
			configPath := filepath.Join(os.TempDir(), fmt.Sprintf("xray_%d_%d.json", time.Now().UnixNano(), socksPort))
			os.WriteFile(configPath, configJSON, 0644)

			cmd := exec.Command(xrayBin, "run", "-config", configPath)
			cmd.Start()
			instances[i] = &xrayInstance{cmd: cmd, socksPort: socksPort, configPath: configPath}
		}(i, p)
	}
	startWg.Wait()

	time.Sleep(xrayStartWait)

	var testWg sync.WaitGroup
	testSem := make(chan struct{}, xraySocksLimit)
	var mu sync.Mutex

	for i, inst := range instances {
		if inst == nil {
			continue
		}
		testSem <- struct{}{}
		testWg.Add(1)
		go func(idx int, inst *xrayInstance, p ParsedProxy) {
			defer func() { <-testSem; testWg.Done() }()
			start := time.Now()
			ok, _ := socks5Connect("127.0.0.1", inst.socksPort, testHost, testPort, testTimeout)
			lat := time.Since(start).Milliseconds()
			errStr := ""
			if !ok {
				errStr = "proxy test failed"
			}
			mu.Lock()
			results[idx] = TestResult{
				URI:       p.RawURI,
				Protocol:  p.Protocol,
				Host:      p.Host,
				Port:      p.Port,
				Working:   ok,
				LatencyMs: lat,
				Error:     errStr,
			}
			mu.Unlock()
		}(indices[i], inst, proxies[i])
	}
	testWg.Wait()

	for _, inst := range instances {
		if inst != nil && inst.cmd != nil && inst.cmd.Process != nil {
			inst.cmd.Process.Kill()
			os.Remove(inst.configPath)
		}
	}
}

func TestAll(proxies []ParsedProxy) []TestResult {
	return TestAllWithConcurrency(proxies, 1000)
}

func TestAllWithConcurrency(proxies []ParsedProxy, batchSize int) []TestResult {
	if len(proxies) == 0 {
		return nil
	}

	results := make([]TestResult, len(proxies))

	var xrayIdxs, basicIdxs []int
	var xrayProxies, basicProxies []ParsedProxy

	for i, p := range proxies {
		if isXraySupported(p) {
			xrayProxies = append(xrayProxies, p)
			xrayIdxs = append(xrayIdxs, i)
		} else {
			basicProxies = append(basicProxies, p)
			basicIdxs = append(basicIdxs, i)
		}
	}

	for i := 0; i < len(xrayProxies); i += batchSize {
		end := i + batchSize
		if end > len(xrayProxies) {
			end = len(xrayProxies)
		}
		testXrayBatch(xrayProxies[i:end], xrayIdxs[i:end], results)
	}

	fillResults(results, basicProxies, basicIdxs)

	return results
}

func fillResults(results []TestResult, proxies []ParsedProxy, indices []int) {
	if len(proxies) == 0 {
		return
	}
	sem := make(chan struct{}, 100)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, p := range proxies {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, p ParsedProxy) {
			defer func() { <-sem; wg.Done() }()
			ok, lat := basicTest(p)
			mu.Lock()
			results[indices[i]] = TestResult{
				URI:       p.RawURI,
				Protocol:  p.Protocol,
				Host:      p.Host,
				Port:      p.Port,
				Working:   ok,
				LatencyMs: lat,
			}
			mu.Unlock()
		}(i, p)
	}
	wg.Wait()
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
