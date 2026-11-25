package subscription

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
)

func TestParseTelegramProxyLinks(t *testing.T) {
	secret := "EE19F35317DF4DFF3CAB0202FA96EE51DE676F6F676C652E636F6D"
	lowerSecret := "ee19f35317df4dff3cab0202fa96ee51de676f6f676c652e636f6d"

	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantPort int
		wantRem  string
	}{
		{
			name:     "tg scheme",
			raw:      "tg://proxy?server=proxy.example.com&port=8443&secret=" + secret + "#fast",
			wantHost: "proxy.example.com",
			wantPort: 8443,
			wantRem:  "fast",
		},
		{
			name:     "t.me link",
			raw:      "https://t.me/proxy?server=1.2.3.4&port=443&secret=" + lowerSecret,
			wantHost: "1.2.3.4",
			wantPort: 443,
			wantRem:  "MTProto 1.2.3.4:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseURI(tt.raw)
			if p == nil {
				t.Fatalf("ParseURI(%q) returned nil", tt.raw)
			}
			if p.Protocol != "mtproto" {
				t.Fatalf("protocol = %q, want mtproto", p.Protocol)
			}
			if p.Host != tt.wantHost || p.Port != tt.wantPort {
				t.Fatalf("host:port = %s:%d, want %s:%d", p.Host, p.Port, tt.wantHost, tt.wantPort)
			}
			if p.Secret != lowerSecret {
				t.Fatalf("secret = %q, want %q", p.Secret, lowerSecret)
			}
			if p.Remark != tt.wantRem {
				t.Fatalf("remark = %q, want %q", p.Remark, tt.wantRem)
			}
		})
	}
}

func TestParseURIRejectsInvalidTelegramLinks(t *testing.T) {
	invalid := []string{
		"tg://socks?server=1.2.3.4&port=443",
		"tg://proxy?server=1.2.3.4&port=443",
		"tg://proxy?server=1.2.3.4&secret=zz",
		"tg://proxy?server=&port=443&secret=ee19f35317df4dff3cab0202fa96ee51",
		"https://t.me/somechannel?server=1.2.3.4&port=443&secret=ee19f35317df4dff3cab0202fa96ee51",
	}
	for _, raw := range invalid {
		if p := ParseURI(raw); p != nil {
			t.Fatalf("ParseURI(%q) = %+v, want nil", raw, p)
		}
	}
}

func TestDecodeMTProtoSecret(t *testing.T) {
	keyHex := "19f35317df4dff3cab0202fa96ee51de"

	t.Run("fake tls", func(t *testing.T) {
		s, err := decodeMTProtoSecret("ee" + keyHex + "676f6f676c652e636f6d")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !s.fakeTLS || s.padded {
			t.Fatalf("expected fakeTLS mode, got %+v", s)
		}
		if len(s.key) != 16 {
			t.Fatalf("key length = %d, want 16", len(s.key))
		}
		if s.domain != "google.com" {
			t.Fatalf("domain = %q, want google.com", s.domain)
		}
	})

	t.Run("padded intermediate", func(t *testing.T) {
		s, err := decodeMTProtoSecret("dd" + keyHex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.fakeTLS || !s.padded {
			t.Fatalf("expected padded mode, got %+v", s)
		}
	})

	t.Run("classic", func(t *testing.T) {
		s, err := decodeMTProtoSecret(keyHex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.fakeTLS || s.padded {
			t.Fatalf("expected classic mode, got %+v", s)
		}
	})

	t.Run("fake tls without hostname", func(t *testing.T) {
		if _, err := decodeMTProtoSecret("ee" + keyHex); err == nil {
			t.Fatal("expected error for fake-tls secret without hostname")
		}
	})

	t.Run("too short", func(t *testing.T) {
		if _, err := decodeMTProtoSecret("00112233"); err == nil {
			t.Fatal("expected error for short secret")
		}
	})
}

func TestObfuscatedHandshakeAgainstMockServer(t *testing.T) {
	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(i + 1)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serveMTProtoMock(ln, key)
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	ok, latency, errStr := testObfuscatedHandshake(host, port, key, false)
	if err := <-serverErr; err != nil {
		t.Fatalf("mock server: %v", err)
	}
	if !ok {
		t.Fatalf("handshake failed: %s", errStr)
	}
	if latency < 0 {
		t.Fatalf("negative latency")
	}
}

func serveMTProtoMock(ln net.Listener, key []byte) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	wire := make([]byte, 64)
	if _, err := io.ReadFull(conn, wire); err != nil {
		return fmt.Errorf("read init: %w", err)
	}

	// Only bytes 56..63 are encrypted; the server rebuilds the full init from
	// the plaintext prefix and the decrypted tail.
	decryptedTail := make([]byte, 64)
	decryptor := newMTProtoCTR(wire[8:40], wire[40:56], key)
	decryptor.XORKeyStream(decryptedTail, wire)
	init := make([]byte, 64)
	copy(init, wire[:56])
	copy(init[56:], decryptedTail[56:])

	if tag := binary.LittleEndian.Uint32(init[56:60]); tag != mtprotoTagIntermediate {
		return fmt.Errorf("unexpected protocol tag 0x%08x", tag)
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read frame header: %w", err)
	}
	decryptor.XORKeyStream(header, header)
	size := binary.LittleEndian.Uint32(header)
	if size < 4 || size > 1<<20 {
		return fmt.Errorf("invalid frame size %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return fmt.Errorf("read frame body: %w", err)
	}
	decryptor.XORKeyStream(body, body)
	if ctor := binary.LittleEndian.Uint32(body); ctor != mtprotoReqPQMulti {
		return fmt.Errorf("unexpected constructor 0x%08x", ctor)
	}

	reversed := reverseBytes(init)
	encryptor := newMTProtoCTR(reversed[8:40], reversed[40:56], key)
	response := make([]byte, 4+20)
	binary.LittleEndian.PutUint32(response, 20)
	binary.LittleEndian.PutUint32(response[4:], mtprotoResPQ)
	encryptor.XORKeyStream(response, response)
	_, err = conn.Write(response)
	return err
}

func TestFakeTLSHandshakeAgainstMockServer(t *testing.T) {
	key := []byte("0123456789abcdef")

	run := func(t *testing.T, clientKey []byte, want bool) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer ln.Close()

		serverErr := make(chan error, 1)
		go func() {
			serverErr <- serveFakeTLSMock(ln, key)
		}()

		host, portStr, _ := net.SplitHostPort(ln.Addr().String())
		port, _ := strconv.Atoi(portStr)
		ok, _, errStr := testFakeTLSHandshake(host, port, clientKey, "example.com")
		if err := <-serverErr; err != nil {
			t.Fatalf("mock server: %v", err)
		}
		if ok != want {
			t.Fatalf("handshake ok = %v (err %q), want %v", ok, errStr, want)
		}
	}

	t.Run("valid secret", func(t *testing.T) { run(t, key, true) })
	t.Run("wrong secret", func(t *testing.T) { run(t, []byte("fedcba9876543210"), false) })
}

// serveFakeTLSMock reads the client hello, replies with the three-record
// ServerHello/CCS/ApplicationData packet and a digest computed exactly as a
// real MTProxy does.
func serveFakeTLSMock(ln net.Listener, key []byte) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read hello header: %w", err)
	}
	record := make([]byte, 5+int(binary.BigEndian.Uint16(header[3:5])))
	copy(record, header)
	if _, err := io.ReadFull(conn, record[5:]); err != nil {
		return fmt.Errorf("read hello body: %w", err)
	}
	if record[0] != 0x16 || len(record) < mtprotoRandomOffset+32 {
		return fmt.Errorf("not a client hello")
	}

	clientRandom := append([]byte(nil), record[mtprotoRandomOffset:mtprotoRandomOffset+32]...)

	hello := []byte{0x02, 0x00, 0x00, 0x28, 0x03, 0x03}
	hello = append(hello, make([]byte, 32)...) // server random placeholder
	hello = append(hello, 0x00, 0x13, 0x01, 0x00, 0x00, 0x00)

	reply := []byte{0x16, 0x03, 0x03, 0x00, byte(len(hello))}
	reply = append(reply, hello...)
	reply = append(reply, 0x14, 0x03, 0x03, 0x00, 0x01, 0x01)
	reply = append(reply, 0x17, 0x03, 0x03, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00)

	mac := hmac.New(sha256.New, key)
	mac.Write(clientRandom)
	mac.Write(reply)
	copy(reply[mtprotoRandomOffset:], mac.Sum(nil))

	_, err = conn.Write(reply)
	return err
}

func TestConvertWorkingKeepsMTProtoWithoutSingboxConfig(t *testing.T) {
	p := ParsedProxy{
		Protocol: "mtproto",
		Host:     "proxy.example.com",
		Port:     8443,
		Secret:   "ee19f35317df4dff3cab0202fa96ee51de676f6f676c652e636f6d",
		Remark:   "fast",
		RawURI:   "tg://proxy?server=proxy.example.com&port=8443&secret=ee19f35317df4dff3cab0202fa96ee51de676f6f676c652e636f6d#fast",
	}

	working := ConvertWorking([]ParsedProxy{p}, []TestResult{{
		URI:       p.RawURI,
		Protocol:  p.Protocol,
		Host:      p.Host,
		Port:      p.Port,
		Working:   true,
		LatencyMs: 42,
	}})

	if len(working) != 1 {
		t.Fatalf("working count = %d, want 1", len(working))
	}
	if working[0].Config != nil {
		t.Fatalf("expected nil sing-box config for mtproto, got %+v", working[0].Config)
	}
	if working[0].RawURI != p.RawURI || working[0].Remark != p.Remark {
		t.Fatalf("metadata not preserved: %+v", working[0])
	}
}
