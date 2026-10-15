package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// selfSignedFor builds a throwaway certificate the way a private relay's would
// look: self-issued, so nothing but its own copy can vouch for it.
func selfSignedFor(t *testing.T, cn string, hosts ...string) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pemBytes
}

// serveTLS starts a TLS listener that presents the given certificate and
// accepts one connection, so FetchCACert has something real to talk to.
func serveTLS(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Complete the handshake, then let the caller close.
			if tc, ok := c.(*tls.Conn); ok {
				_ = tc.Handshake()
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}

func TestFetchCACertReturnsPresentedChain(t *testing.T) {
	cert, wantPEM := selfSignedFor(t, "cc-admin", "127.0.0.1")
	addr := serveTLS(t, cert)

	got, err := FetchCACert("wss://"+addr+"/relay/agent", 5*time.Second)
	if err != nil {
		t.Fatalf("FetchCACert: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FetchCACert returned nothing")
	}
	info, err := DescribeCert(got)
	if err != nil {
		t.Fatalf("DescribeCert(fetched): %v", err)
	}
	if info.Subject != "cc-admin" {
		t.Errorf("subject = %q, want %q", info.Subject, "cc-admin")
	}
	// The fetched leaf must be the very certificate the server showed, so that
	// pinning it reproduces the manual "download from the panel" flow.
	wantInfo, err := DescribeCert(wantPEM)
	if err != nil {
		t.Fatal(err)
	}
	if info.Fingerprint != wantInfo.Fingerprint {
		t.Errorf("fingerprint = %s, want %s", info.Fingerprint, wantInfo.Fingerprint)
	}
}

func TestFetchCACertRejectsNonTLSAndBadURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"plain ws", "ws://example.com/relay/agent", "not TLS"},
		{"no scheme", "example.com/relay/agent", "must start with ws:// or wss://"},
		{"http", "https://example.com/relay/agent", "must start with ws:// or wss://"},
		{"no host", "wss:///relay/agent", "has no host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FetchCACert(tc.url, time.Second)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestFetchCACertReportsUnreachableHost(t *testing.T) {
	// Nothing listens on this port; the point is that the failure is an error,
	// not a silent empty result.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if _, err := FetchCACert("wss://"+addr+"/relay/agent", 2*time.Second); err == nil {
		t.Fatal("want an error for an unreachable relay, got nil")
	}
}

func TestDescribeCertRejectsNonCertificates(t *testing.T) {
	cases := []struct {
		name string
		pem  string
	}{
		{"empty", ""},
		{"not pem", "hello"},
		{"wrong block type", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"},
		{"bad der", "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DescribeCert([]byte(tc.pem)); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}

func TestDescribeCertSkipsLeadingNonCertificateBlock(t *testing.T) {
	_, certPEM := selfSignedFor(t, "later", "127.0.0.1")
	mixed := append([]byte("-----BEGIN EC PRIVATE KEY-----\nAAAA\n-----END EC PRIVATE KEY-----\n"), certPEM...)

	info, err := DescribeCert(mixed)
	if err != nil {
		t.Fatalf("DescribeCert: %v", err)
	}
	if info.Subject != "later" {
		t.Errorf("subject = %q, want %q", info.Subject, "later")
	}
	if len(info.Fingerprint) != 64 {
		t.Errorf("fingerprint = %q, want 64 hex characters", info.Fingerprint)
	}
}

// A certificate the relay could plausibly present for a bare IP: the fetch must
// work even though there is no DNS name to match.
func TestFetchCACertByIPAddress(t *testing.T) {
	cert, _ := selfSignedFor(t, "cc-admin", "127.0.0.1")
	addr := serveTLS(t, cert)

	got, err := FetchCACert("wss://"+addr, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchCACert: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FetchCACert returned nothing")
	}
}
