package service

import (
	"strings"
	"testing"
)

func TestExecQuotesEveryArgument(t *testing.T) {
	c := Config{
		Bin:  "/Applications/Beacon Member/tfd",
		Args: []string{"-relay", "wss://relay.example/relay/agent", "-device-secret", "a b'c"},
	}
	got := c.Exec()
	if !strings.HasPrefix(got, "'/Applications/Beacon Member/tfd'") {
		t.Fatalf("binary path not quoted: %s", got)
	}
	if !strings.Contains(got, `wss://relay.example/relay/agent`) {
		t.Fatalf("URL missing: %s", got)
	}
	// A quote inside a value must survive as content, not end the token.
	if !strings.Contains(got, `'a b'\''c'`) {
		t.Fatalf("quote not escaped: %s", got)
	}
}

func TestExecLeavesPlainWordsAlone(t *testing.T) {
	c := Config{Bin: "/usr/local/bin/tfd", Args: []string{"-device-id", "mac-mini-1a2b3c"}}
	want := "/usr/local/bin/tfd -device-id mac-mini-1a2b3c"
	if got := c.Exec(); got != want {
		t.Fatalf("Exec() = %q, want %q", got, want)
	}
}

func TestValidateRejectsRelativeBinary(t *testing.T) {
	if err := (Config{Bin: "tfd"}).Validate(); err == nil {
		t.Fatal("a relative binary path was accepted")
	}
	if err := (Config{Bin: ""}).Validate(); err == nil {
		t.Fatal("an empty binary path was accepted")
	}
	if err := (Config{Bin: "/usr/local/bin/tfd"}).Validate(); err != nil {
		t.Fatalf("absolute path refused: %v", err)
	}
}
