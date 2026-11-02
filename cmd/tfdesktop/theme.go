package main

import "github.com/egoist/mygo/ui"

// Old settings may still carry an appearance preference; desktop is monochrome.
const (
	themeSystem = ""
	themeLight  = "light"
	themeDark   = "dark"
)

func themeFor(_ string) *ui.Theme        { return darkTheme() }
func applyTheme(c *ui.Context, _ string) { c.SetTheme(darkTheme()) }
func windowGround(_ string) string       { return "#111111" }
