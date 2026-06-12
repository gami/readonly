// Package plugin registers the readonly analyzer as a golangci-lint module
// plugin. It is imported (blank) by the custom golangci-lint binary built
// with `golangci-lint custom`; see the repository README for setup.
package plugin

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/gami/readonly"
)

func init() {
	register.Plugin("readonly", New)
}

// New constructs the plugin instance. The readonly analyzer takes no
// settings, so the raw settings value is ignored.
func New(_ any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

type plugin struct{}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{readonly.Analyzer}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
