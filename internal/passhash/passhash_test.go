package passhash

import "testing"

// TestHashRoundTrips covers the primitives directly, including the refusal of a
// malformed record.
func TestHashRoundTrips(t *testing.T) {
	h, err := Hash("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(h, "hunter2hunter2") {
		t.Fatal("a correct password did not verify")
	}
	if Verify(h, "hunter2hunter3") {
		t.Fatal("a wrong password verified")
	}
	for _, bad := range []string{"", "nonsense", "pbkdf2_sha256$0$AA$AA", "md5$1$AA$AA", "pbkdf2_sha256$x$AA$AA"} {
		if Verify(bad, "hunter2hunter2") {
			t.Fatalf("malformed record %q verified", bad)
		}
	}
	// Two hashes of the same password must differ: that is the salt doing work.
	h2, err := Hash("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if h == h2 {
		t.Fatal("hashing the same password twice produced the same record; the salt is fixed")
	}
}
