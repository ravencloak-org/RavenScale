// Command tenantcheck runs the tenantcheck analyzer standalone (and in CI).
//
// Usage:
//
//	go run ./tools/tenantcheck/cmd/tenantcheck ./hscontrol/...
package main

import (
	"github.com/juanfont/headscale/tools/tenantcheck"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(tenantcheck.Analyzer)
}
