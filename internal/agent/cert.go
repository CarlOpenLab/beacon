package agent

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// CertInfo describes a certificate in the terms a person needs in order to
// decide whether to trust it: who it claims to be, its fingerprint, and when it
// stops being valid.
type CertInfo struct {
	Subject     string
	Fingerprint string
	NotAfter    time.Time
}

// FetchCACert dials the relay's TLS endpoint and returns the chain it presents,
// PEM-encoded.
//
// It exists so the settings window can offer "get the certificate from the
// server" instead of asking someone to find a file they downloaded from a
// browser. The handshake deliberately does not verify the certificate: there is
// nothing to verify it against until it has been fetched. What comes back is
// therefore a trust-on-first-use decision — the same one a browser asks a person
// to make when it warns about a self-signed certificate — and not proof of
// identity. Callers should show DescribeCert's result so the person can see what
// they are about to trust.
func FetchCACert(relayURL string, timeout time.Duration) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(relayURL))
	if err != nil {
		return nil, fmt.Errorf("agent: %q is not a valid URL: %w", relayURL, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("agent: relay URL must start with ws:// or wss://, got %q", relayURL)
	}
	if u.Scheme != "wss" {
		return nil, fmt.Errorf("agent: %s is not TLS, so there is no certificate to fetch", relayURL)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("agent: %q has no host", relayURL)
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "443")
	}

	dialer := &net.Dialer{Timeout: timeout}
	// The skip is the point of this call, not an oversight: see the doc comment.
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         u.Hostname(),
	})
	if err != nil {
		return nil, fmt.Errorf("agent: %s: %w", addr, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("agent: %s presented no certificate", addr)
	}
	var buf bytes.Buffer
	for _, c := range certs {
		if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DescribeCert reads the first certificate in a PEM blob. It is what the
// settings window shows next to the CA field, so a wrong or expired file is
// visible before anyone wonders why the machine stays offline.
func DescribeCert(pemBytes []byte) (CertInfo, error) {
	var block *pem.Block
	rest := pemBytes
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			return CertInfo{}, fmt.Errorf("agent: no PEM certificate found")
		}
		if block.Type == "CERTIFICATE" {
			break
		}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return CertInfo{}, fmt.Errorf("agent: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	subject := cert.Subject.CommonName
	if subject == "" {
		subject = cert.Subject.String()
	}
	return CertInfo{
		Subject:     subject,
		Fingerprint: hex.EncodeToString(sum[:]),
		NotAfter:    cert.NotAfter,
	}, nil
}
