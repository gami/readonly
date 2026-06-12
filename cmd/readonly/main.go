// Command readonly runs the readonly analyzer as a standalone tool.
//
// Usage:
//
//	readonly ./...
//	go vet -vettool=$(which readonly) ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/gami/readonly"
)

func main() {
	singlechecker.Main(readonly.Analyzer)
}
