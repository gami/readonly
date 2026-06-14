package readonly_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/gami/readonly"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "model", "service")
}

func TestAllowAllTestFiles(t *testing.T) {
	if err := readonly.Analyzer.Flags.Set("allow-all-test-files", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := readonly.Analyzer.Flags.Set("allow-all-test-files", "false"); err != nil {
			t.Fatal(err)
		}
	})
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "repo")
}
