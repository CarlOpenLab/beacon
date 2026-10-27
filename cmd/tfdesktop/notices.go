package main

import _ "embed"

// The notices travel inside the binary: the release package has to carry the
// licences of what it draws, and an app bundle cannot be relied on to keep a
// file next to the executable.
//
//go:embed assets/third-party-notices.md
var thirdPartyNotices string
