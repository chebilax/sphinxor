package nestjs

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// resolverDecorator marks a class as a GraphQL resolver, and
// graphqlOperationDecorators are the operations one can carry.
//
// Detection is deliberately shallow, exactly as ADR 0020 §3 treats
// reactive Spring chains: establishing *that* a surface exists is what
// converts a silent scope boundary into a stated one, and it is cheap.
// Establishing what that surface requires is the coverage feature
// docs/decisions/0021-graphql-out-of-scope-but-detected.md declines.
const resolverDecorator = "Resolver"

var graphqlOperationDecorators = map[string]bool{
	"Query":        true,
	"Mutation":     true,
	"Subscription": true,
	"ResolveField": true,
}

// detectGraphQL reports whether the project exposes a GraphQL API, and
// how many operations were left unanalyzed.
//
// Operations are counted only inside a class that carries @Resolver.
// @Query in particular is not unique to GraphQL — it is also
// @nestjs/common's query-parameter decorator, used on ordinary REST
// handler parameters — so counting it project-wide would inflate the
// figure on exactly the REST projects that should stay quiet.
func detectGraphQL(files []parsedFile) model.GraphQLStatus {
	var status model.GraphQLStatus
	for _, f := range files {
		for _, group := range groupDecorators(flattenTopLevel(f.tree.RootNode())) {
			if group.decl == nil || group.decl.Type() != "class_declaration" {
				continue
			}
			if _, ok := findDecoratorCall(group.decorators, f.src, resolverDecorator); !ok {
				continue
			}
			status.Present = true
			status.Operations += countOperations(group.decl.ChildByFieldName("body"), f.src)
		}
	}
	return status
}

// countOperations counts the GraphQL operation decorators on a resolver
// class's methods.
func countOperations(body *sitter.Node, src []byte) int {
	if body == nil {
		return 0
	}
	var n int
	for _, methodGroup := range groupDecorators(namedChildren(body)) {
		if methodGroup.decl == nil || methodGroup.decl.Type() != "method_definition" {
			continue
		}
		for _, d := range methodGroup.decorators {
			call, ok := parseDecorator(d, src)
			if ok && graphqlOperationDecorators[call.Name] {
				n++
				break
			}
		}
	}
	return n
}
