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
	withFlag(t, "allow-all-test-files")
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "repo")
}

func TestReportAddressOf(t *testing.T) {
	withFlag(t, "report-address-of")
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "addr")
}

// 既定では address-of は報告されない: service パッケージの期待値は変わらない。
func TestReportAddressOfOffByDefault(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "service")
}

// withFlag enables a boolean analyzer flag for the duration of the test.
// The flag is package-global state, so tests that use it must not be
// parallel.
func withFlag(t *testing.T, name string) {
	t.Helper()
	if err := readonly.Analyzer.Flags.Set(name, "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := readonly.Analyzer.Flags.Set(name, "false"); err != nil {
			t.Fatal(err)
		}
	})
}
