// Package passhash is the one password-hashing scheme every credential file in
// the relay shares. It exists so the admin account and the registered user
// accounts cannot drift apart — one gets slower, one forgets a salt — and so an
// encoded hash reads the same wherever it is stored.
package passhash

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// iterations is the work factor for an interactive login on a small cloud
// instance: slow enough to make offline guessing of a leaked credential file
// expensive, fast enough to feel instant.
const iterations = 210_000

// Hash returns "pbkdf2_sha256$<iter>$<salt>$<key>", all base64 raw. The salt is
// fresh every call, so two users with the same password store different hashes.
func Hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify parses and checks an encoded hash, in constant time. A malformed hash
// verifies false rather than erroring: a corrupted row must not crash a login.
func Verify(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
