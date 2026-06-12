package readonly_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/gami/readonly"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "model", "service")
}
