package tenantcheck_test

import (
	"testing"

	"github.com/juanfont/headscale/tools/tenantcheck"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), tenantcheck.Analyzer, "a")
}
