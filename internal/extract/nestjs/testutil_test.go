package nestjs

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// parseTS parses a TypeScript snippet for use in a single test, failing
// the test on a parse error.
func parseTS(t *testing.T, src string) (*sitter.Node, []byte) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parsing snippet: %v", err)
	}
	return tree.RootNode(), []byte(src)
}
