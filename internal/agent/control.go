package agent

// Asking the relay for a credential instead of being handed the fleet's secret.
//
// A desktop that wants to drive the *other* machines has to be a client of the
// relay, and the relay's client endpoint wants a credential. The shared one is
// what a browser or a phone is given — the fleet's password — and typing it into
// every desktop is exactly the friction this file removes: a machine that is
// already enrolled proves it is itself with the device credential it dials the
// member link with, and the relay hands back a credential of its own, good for a
// few minutes. Reach is then something that expires, that revocation takes away,
// and that never involves a person copying a secret between machines.

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ControlTokenTTL is how long a credential asked for this way is expected to
// last when the relay does not say. It is only a hint for a caller deciding when
// to ask again: the relay's answer carries the real expiry.
const ControlTokenTTL = 15 * time.Minute

// ControlToken is a credential the relay minted for this machine.
type ControlToken struct {
	Token     string
	ExpiresAt time.Time
}

// RequestControlToken asks the relay for a short-lived client credential.
//
// memberURL is the address this machine already joins the relay with, such as
// wss://relay.example/relay/agent; the control endpoint lives beside it. roots,
// when set, is the certificate pool a private or self-signed relay is verified
// against — the same pin the member link uses.
func RequestControlToken(memberURL, deviceID, secret string, roots *x509.CertPool, timeout time.Duration) (ControlToken, error) {
	if strings.TrimSpace(deviceID) == "" || secret == "" {
		return ControlToken{}, errors.New("agent: a device id and secret are needed to ask for a control token")
	}
	endpoint, err := ControlTokenURL(memberURL)
	if err != nil {
		return ControlToken{}, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return ControlToken{}, err
	}
	// The member credential, in the same "id:secret" form the member link
	// presents: the relay checks it against the registry, so a revoked machine
	// cannot ask.
	req.Header.Set("Authorization", "Bearer "+deviceID+":"+secret)

	client := &http.Client{Timeout: timeout}
	if roots != nil {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ControlToken{}, fmt.Errorf("agent: asking %s for a control token: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		reason := strings.TrimSpace(string(body))
		if reason == "" {
			reason = resp.Status
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return ControlToken{}, fmt.Errorf("agent: the relay does not know this machine (%s); enroll it there first", reason)
		}
		return ControlToken{}, fmt.Errorf("agent: %s refused a control token: %s", endpoint, reason)
	}

	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return ControlToken{}, fmt.Errorf("agent: %s answered with something that is not a token: %w", endpoint, err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return ControlToken{}, fmt.Errorf("agent: %s minted an empty token", endpoint)
	}
	expires, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil {
		// A token whose lifetime cannot be read is taken as short-lived rather
		// than eternal: the caller asks again, which costs one request.
		expires = time.Now().Add(ControlTokenTTL)
	}
	return ControlToken{Token: out.Token, ExpiresAt: expires}, nil
}

// ControlTokenURL is where a member asks its relay for a control credential,
// from the member endpoint it already knows: the two endpoints live beside each
// other on the same host, as the member and client ones do.
func ControlTokenURL(memberURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(memberURL))
	if err != nil {
		return "", fmt.Errorf("agent: %q is not a valid URL: %w", memberURL, err)
	}
	var scheme string
	switch u.Scheme {
	case "wss":
		scheme = "https"
	case "ws":
		scheme = "http"
	default:
		return "", fmt.Errorf("agent: relay URL must start with ws:// or wss://, got %q", memberURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("agent: %q has no host", memberURL)
	}
	// The path names the member endpoint: .../relay/agent. The control endpoint
	// is its sibling, so the last segment is replaced rather than appended — a
	// URL with no path at all cannot say where the relay's endpoints are.
	i := strings.LastIndex(u.Path, "/")
	if i <= 0 {
		return "", fmt.Errorf("agent: %q does not name an endpoint; it should look like wss://relay.example/relay/agent", memberURL)
	}
	u.Scheme = scheme
	u.Path = u.Path[:i+1] + "control-token"
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}
