package hub

import "testing"

func TestPasteNameCannotEscapeThePasteDirectory(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":     "passwd",
		`C:\Users\me\shot.png`: "shot.png",
		"/tmp/.hidden.txt":     "hidden.txt",
		"a b.png":              "a_b.png",
		"":                     "paste",
		"...":                  "paste",
		"name.with.dots.png":   "name.with.dots.png",
	}
	for in, want := range cases {
		if got := pasteName(in); got != want {
			t.Errorf("pasteName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPasteNameIsBounded(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	got := pasteName(long + ".png")
	if len(got) > 52 {
		t.Fatalf("pasteName kept %d characters: %q", len(got), got)
	}
}
