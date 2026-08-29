package spring

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
)

// parseJava parses a Java snippet for use in a single test, failing the
// test on a parse error.
func parseJava(t *testing.T, src string) (*sitter.Node, []byte) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parsing snippet: %v", err)
	}
	return tree.RootNode(), []byte(src)
}
