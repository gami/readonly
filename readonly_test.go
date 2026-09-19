package readonly_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/gami/readonly"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "model", "service")
}

func TestAllowAllTestFiles(t *testing.T) {
	t.Parallel()
	a := readonly.NewAnalyzer(readonly.Options{AllowAllTestFiles: true})
	analysistest.Run(t, analysistest.TestData(), a, "repo")
}

func TestReportAddressOf(t *testing.T) {
	t.Parallel()
	a := readonly.NewAnalyzer(readonly.Options{ReportAddressOf: true})
	analysistest.Run(t, analysistest.TestData(), a, "addr")
}

// 既定では address-of は報告されない: service パッケージの期待値は変わらない。
func TestReportAddressOfOffByDefault(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "service")
}

// フラグは NewAnalyzer が返したインスタンスにだけ効き、他のインスタンスや
// 既定の Analyzer には影響しない。
func TestFlagsBindToInstance(t *testing.T) {
	t.Parallel()
	flagged := readonly.NewAnalyzer(readonly.Options{})
	if err := flagged.Flags.Set("report-address-of", "true"); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), flagged, "addr")

	// service には address-of の期待値がないので、既定の Analyzer が
	// 上のフラグに影響されていれば余分な診断で失敗する。
	analysistest.Run(t, analysistest.TestData(), readonly.Analyzer, "service")
}

// Options の値はフラグの既定値になり、フラグで上書きできる。
func TestFlagDefaultsFromOptions(t *testing.T) {
	t.Parallel()
	a := readonly.NewAnalyzer(readonly.Options{ReportAddressOf: true})
	if got := a.Flags.Lookup("report-address-of").DefValue; got != "true" {
		t.Fatalf("report-address-of default = %q, want %q", got, "true")
	}
	if err := a.Flags.Set("report-address-of", "false"); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), a, "service")
}
