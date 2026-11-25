package subscription

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mtprotoTestTimeout = 10 * time.Second
	mtprotoDialTimeout = 8 * time.Second
	mtprotoConcurrency = 100

	mtprotoRandomOffset = 11 // TLS record header(5) + handshake type(1) + length(3) + version(2)

	mtprotoTagIntermediate = 0xeeeeeeee
	mtprotoTagPadded       = 0xdddddddd

	mtprotoReqPQMulti = 0xbe7e8ef1
	mtprotoResPQ      = 0x05162463
)

type mtprotoSecret struct {
	fakeTLS bool
	padded  bool
	key     []byte
	domain  string
}

func fillMTProtoResults(results []TestResult, proxies []ParsedProxy, indices []int) {
	if len(proxies) == 0 {
		return
	}

	sem := make(chan struct{}, mtprotoConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, p := range proxies {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, p ParsedProxy) {
			defer func() { <-sem; wg.Done() }()
			ok, lat, errStr := testMTProto(p)
			mu.Lock()
			results[indices[i]] = TestResult{
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
}

func testMTProto(p ParsedProxy) (bool, int64, string) {
	secret, err := decodeMTProtoSecret(p.Secret)
	if err != nil {
		return false, 0, err.Error()
	}
	if secret.fakeTLS {
		return testFakeTLSHandshake(p.Host, p.Port, secret.key, secret.domain)
	}
	return testObfuscatedHandshake(p.Host, p.Port, secret.key, secret.padded)
}

func decodeMTProtoSecret(raw string) (*mtprotoSecret, error) {
	b, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return nil, fmt.Errorf("invalid secret hex")
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("secret too short")
	}

	switch {
	case len(b) >= 17 && b[0] == 0xee:
		domain := string(b[17:])
		if !isPrintableASCII(domain) {
			return nil, fmt.Errorf("fake-tls secret missing hostname")
		}
		return &mtprotoSecret{fakeTLS: true, key: b[1:17], domain: domain}, nil
	case len(b) >= 17 && b[0] == 0xdd:
		return &mtprotoSecret{padded: true, key: b[1:17]}, nil
	default:
		return &mtprotoSecret{key: b[:16]}, nil
	}
}

func isPrintableASCII(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

// testFakeTLSHandshake performs the MTProto FakeTLS handshake (ee secrets) and
// verifies the server digest, which proves the secret is valid.
func testFakeTLSHandshake(host string, port int, key []byte, hostname string) (bool, int64, string) {
	start := time.Now()

	conn, err := dialMTProto(host, port)
	if err != nil {
		return false, 0, err.Error()
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(mtprotoTestTimeout))

	hello, clientRandom := buildFakeTLSClientHello(key, hostname)
	if _, err := conn.Write(hello); err != nil {
		return false, time.Since(start).Milliseconds(), "write hello: " + err.Error()
	}

	// Server reply is exactly: ServerHello, ChangeCipherSpec, ApplicationData.
	packet, err := readTLSRecords(conn, 3)
	if err != nil {
		return false, time.Since(start).Milliseconds(), err.Error()
	}
	if len(packet) < mtprotoRandomOffset+32 {
		return false, time.Since(start).Milliseconds(), "short server hello"
	}
	if packet[0] != 0x16 || packet[5] != 0x02 {
		return false, time.Since(start).Milliseconds(), "unexpected server hello"
	}

	serverRandom := append([]byte(nil), packet[mtprotoRandomOffset:mtprotoRandomOffset+32]...)
	zeroed := append([]byte(nil), packet...)
	for i := mtprotoRandomOffset; i < mtprotoRandomOffset+32; i++ {
		zeroed[i] = 0
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(clientRandom)
	mac.Write(zeroed)
	if !hmac.Equal(mac.Sum(nil), serverRandom) {
		return false, time.Since(start).Milliseconds(), "fake-tls digest mismatch"
	}

	return true, time.Since(start).Milliseconds(), ""
}

func buildFakeTLSClientHello(key []byte, hostname string) ([]byte, []byte) {
	sessionID := make([]byte, 32)
	rand.Read(sessionID) //nolint:errcheck

	cipherSuites := []byte{
		0x13, 0x01, 0x13, 0x02, 0x13, 0x03,
		0xc0, 0x2b, 0xc0, 0x2f, 0xc0, 0x30,
		0xcc, 0xa9, 0xcc, 0xa8, 0xc0, 0x2c,
		0xc0, 0x28, 0xc0, 0x27, 0xc0, 0x24,
		0xc0, 0x23, 0xc0, 0x0a, 0xc0, 0x09,
		0xc0, 0x14, 0xc0, 0x13,
		0x00, 0x9d, 0x00, 0x9c, 0x00, 0x3d,
		0x00, 0x35, 0x00, 0x2f, 0x00, 0x0a,
	}

	var extensions []byte
	addExtension := func(typ uint16, data []byte) {
		extensions = append(extensions,
			byte(typ>>8), byte(typ),
			byte(len(data)>>8), byte(len(data)))
		extensions = append(extensions, data...)
	}

	nameLen := len(hostname)
	sni := []byte{
		byte((nameLen + 3) >> 8), byte(nameLen + 3),
		0x00,
		byte(nameLen >> 8), byte(nameLen),
	}
	sni = append(sni, hostname...)
	addExtension(0x0000, sni) // server_name

	addExtension(0x002b, []byte{0x02, 0x03, 0x04}) // supported_versions (TLS 1.3)

	keyShare := make([]byte, 32)
	rand.Read(keyShare)                                                                   //nolint:errcheck
	addExtension(0x0033, append([]byte{0x00, 0x24, 0x00, 0x1d, 0x00, 0x20}, keyShare...)) // key_share

	addExtension(0x000d, []byte{ // signature_algorithms
		0x00, 0x08,
		0x04, 0x03, 0x08, 0x04, 0x04, 0x01, 0x05, 0x03,
	})
	addExtension(0x000a, []byte{ // supported_groups
		0x00, 0x08,
		0x00, 0x1d, 0x00, 0x17, 0x00, 0x18, 0x00, 0x19,
	})
	addExtension(0x000b, []byte{0x01, 0x00}) // ec_point_formats

	var body []byte
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...) // random, filled with digest below
	body = append(body, byte(len(sessionID)))
	body = append(body, sessionID...)
	body = append(body, byte(len(cipherSuites)>>8), byte(len(cipherSuites)))
	body = append(body, cipherSuites...)
	body = append(body, 0x01, 0x00) // null compression
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)

	handshake := make([]byte, 0, 4+len(body))
	handshake = append(handshake, 0x01, byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	handshake = append(handshake, body...)

	record := make([]byte, 0, 5+len(handshake))
	record = append(record, 0x16, 0x03, 0x01, byte(len(handshake)>>8), byte(len(handshake)))
	record = append(record, handshake...)

	mac := hmac.New(sha256.New, key)
	mac.Write(record)
	random := mac.Sum(nil)

	timestamp := make([]byte, 4)
	binary.LittleEndian.PutUint32(timestamp, uint32(time.Now().Unix()))
	for i := 0; i < 4; i++ {
		random[28+i] ^= timestamp[i]
	}
	copy(record[mtprotoRandomOffset:], random)

	return record, random
}

func readTLSRecords(conn net.Conn, count int) ([]byte, error) {
	var out []byte
	for i := 0; i < count; i++ {
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			return nil, fmt.Errorf("read tls record %d: %w", i, err)
		}
		size := int(binary.BigEndian.Uint16(header[3:5]))
		record := make([]byte, 5+size)
		copy(record, header)
		if _, err := io.ReadFull(conn, record[5:]); err != nil {
			return nil, fmt.Errorf("read tls record %d body: %w", i, err)
		}
		out = append(out, record...)
	}
	return out, nil
}

// testObfuscatedHandshake performs the MTProto obfuscated2 handshake and sends
// req_pq_multi, waiting for a valid resPQ or transport error reply.
func testObfuscatedHandshake(host string, port int, key []byte, padded bool) (bool, int64, string) {
	start := time.Now()

	protocol := uint32(mtprotoTagIntermediate)
	if padded {
		protocol = mtprotoTagPadded
	}

	conn, err := dialMTProto(host, port)
	if err != nil {
		return false, 0, err.Error()
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(mtprotoTestTimeout))

	init := makeMTProtoInit(protocol)
	reversed := reverseBytes(init)
	encryptor := newMTProtoCTR(init[8:40], init[40:56], key)
	decryptor := newMTProtoCTR(reversed[8:40], reversed[40:56], key)

	encryptedInit := make([]byte, len(init))
	encryptor.XORKeyStream(encryptedInit, init)
	packet := append([]byte(nil), init[:56]...)
	packet = append(packet, encryptedInit[56:64]...)
	if _, err := conn.Write(packet); err != nil {
		return false, time.Since(start).Milliseconds(), "write init: " + err.Error()
	}

	frame := makeMTProtoFrame(protocol)
	encryptor.XORKeyStream(frame, frame)
	if _, err := conn.Write(frame); err != nil {
		return false, time.Since(start).Milliseconds(), "write request: " + err.Error()
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return false, time.Since(start).Milliseconds(), "read response: " + err.Error()
	}
	decryptor.XORKeyStream(header, header)
	size := binary.LittleEndian.Uint32(header)
	if size < 4 || size > 1<<20 {
		return false, time.Since(start).Milliseconds(), "invalid response size"
	}

	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return false, time.Since(start).Milliseconds(), "read response body: " + err.Error()
	}
	decryptor.XORKeyStream(body, body)

	constructor := binary.LittleEndian.Uint32(body)
	if constructor != mtprotoResPQ && int32(constructor) >= 0 {
		return false, time.Since(start).Milliseconds(), fmt.Sprintf("unexpected response 0x%08x", constructor)
	}

	return true, time.Since(start).Milliseconds(), ""
}

func makeMTProtoInit(protocol uint32) []byte {
	init := make([]byte, 64)
	for {
		rand.Read(init) //nolint:errcheck
		if init[0] == 0xef {
			continue
		}
		switch binary.LittleEndian.Uint32(init[0:4]) {
		case 0, 0x44414548, 0x54534f50, 0x20544547, 0x4954504f, 0x02010316, mtprotoTagPadded, mtprotoTagIntermediate:
			continue
		}
		if binary.LittleEndian.Uint32(init[4:8]) == 0 {
			continue
		}
		break
	}
	binary.LittleEndian.PutUint32(init[56:60], protocol)
	binary.LittleEndian.PutUint16(init[60:62], 2) // DC 2
	return init
}

func makeMTProtoFrame(protocol uint32) []byte {
	payload := make([]byte, 20) // req_pq_multi + 16-byte nonce
	binary.LittleEndian.PutUint32(payload, mtprotoReqPQMulti)
	rand.Read(payload[4:]) //nolint:errcheck

	if protocol == mtprotoTagPadded {
		padding := make([]byte, 8)
		rand.Read(padding) //nolint:errcheck
		frame := make([]byte, 4+len(payload)+len(padding))
		binary.LittleEndian.PutUint32(frame, uint32(len(payload)+len(padding)))
		copy(frame[4:], payload)
		copy(frame[4+len(payload):], padding)
		return frame
	}

	frame := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	return frame
}

func newMTProtoCTR(seed, iv, secret []byte) cipher.Stream {
	h := sha256.New()
	h.Write(seed)
	h.Write(secret)
	block, _ := aes.NewCipher(h.Sum(nil))
	return cipher.NewCTR(block, iv)
}

func reverseBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func dialMTProto(host string, port int) (net.Conn, error) {
	dialer := net.Dialer{Timeout: mtprotoDialTimeout}
	return dialer.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}
