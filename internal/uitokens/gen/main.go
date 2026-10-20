// Command gen writes the browser client's token sheet from package uitokens,
// which is the single source of the two clients' colors.
//
//	go run ./internal/uitokens/gen
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

func main() {
	path := filepath.Join("web", "src", "tokens.css")
	if err := os.WriteFile(path, []byte(uitokens.CSS()), 0o644); err != nil {
		log.Fatalf("uitokens/gen: %v", err)
	}
	log.Printf("uitokens/gen: wrote %s", path)
}
