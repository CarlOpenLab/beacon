package agent

import (
	"errors"
	"strings"
	"testing"
)

// The point of these strings is that someone staring at a settings window can
// tell a wrong certificate from a wrong secret from an unreachable host. Those
// three failures look identical if the pane only shows "offline".
func TestShortReasonNamesTheLikelyCause(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "certificate",
			err:  errors.New(`tls: failed to verify certificate: x509: "cc-admin" certificate is not standards compliant`),
			want: "证书校验失败",
		},
		{
			name: "rejected credentials",
			err:  errors.New(`relay refused the link: unknown device "mac-mini"`),
			want: "拒绝",
		},
		{
			name: "unreachable",
			err:  errors.New(`dial tcp 106.14.250.196:443: connect: connection refused`),
			want: "网络不可达",
		},
		{
			name: "something else",
			err:  errors.New("unexpected end of JSON input"),
			want: "连不上：unexpected end of JSON input",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shortReason(tc.err)
			if !strings.Contains(got, tc.want) {
				t.Errorf("shortReason(%v) = %q, want it to contain %q", tc.err, got, tc.want)
			}
		})
	}
}
