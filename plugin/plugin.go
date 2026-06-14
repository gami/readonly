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

// Settings mirrors the linter's settings block in .golangci.yml.
type Settings struct {
	AllowAllTestFiles bool `json:"allow-all-test-files"`
}

// New constructs the plugin instance from its golangci-lint settings.
func New(settings any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](settings)
	if err != nil {
		return nil, err
	}
	return plugin{settings: s}, nil
}

type plugin struct {
	settings Settings
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	if p.settings.AllowAllTestFiles {
		if err := readonly.Analyzer.Flags.Set("allow-all-test-files", "true"); err != nil {
			return nil, err
		}
	}
	return []*analysis.Analyzer{readonly.Analyzer}, nil
}

func (p plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
