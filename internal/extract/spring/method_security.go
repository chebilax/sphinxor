package spring

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chebilax/sphinxor/internal/model"
)

// methodSecurityDefaults are each enabling annotation's own real default
// values for its three independent attributes, verified against Spring's
// own documentation (docs/decisions/0015-inert-method-security-guard.md) —
// easy to get backwards, so stated explicitly rather than left to a
// zero-value Go bool to imply the wrong thing silently.
type methodSecurityDefaults struct {
	prePostEnabled bool
	securedEnabled bool
	jsr250Enabled  bool
}

var (
	// @EnableMethodSecurity: prePostEnabled defaults true; secured/jsr250 default false.
	enableMethodSecurityDefaults = methodSecurityDefaults{prePostEnabled: true, securedEnabled: false, jsr250Enabled: false}
	// @EnableGlobalMethodSecurity (deprecated): all three default false.
	enableGlobalMethodSecurityDefaults = methodSecurityDefaults{prePostEnabled: false, securedEnabled: false, jsr250Enabled: false}
)

// scanMethodSecurityStatus walks root for @EnableMethodSecurity or
// @EnableGlobalMethodSecurity on any class, OR-combining every located
// annotation's effective flags into status — Spring only needs one such
// configuration to activate a family application-context-wide, so any one
// enabling occurrence is enough to count that family as confirmed live.
func scanMethodSecurityStatus(root *sitter.Node, src []byte, status *model.MethodSecurityStatus) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "class_declaration" {
			for _, ann := range annotationsOf(n, src) {
				defaults, ok := methodSecurityDefaultsFor(ann.Name)
				if !ok {
					continue
				}
				status.Found = true
				status.PrePostEnabled = status.PrePostEnabled || boolAttribute(ann.Args, src, "prePostEnabled", defaults.prePostEnabled)
				status.SecuredEnabled = status.SecuredEnabled || boolAttribute(ann.Args, src, "securedEnabled", defaults.securedEnabled)
				status.Jsr250Enabled = status.Jsr250Enabled || boolAttribute(ann.Args, src, "jsr250Enabled", defaults.jsr250Enabled)
			}
		}
		for _, c := range namedChildren(n) {
			walk(c)
		}
	}
	walk(root)
}

func methodSecurityDefaultsFor(annotationName string) (methodSecurityDefaults, bool) {
	switch annotationName {
	case "EnableMethodSecurity":
		return enableMethodSecurityDefaults, true
	case "EnableGlobalMethodSecurity":
		return enableGlobalMethodSecurityDefaults, true
	default:
		return methodSecurityDefaults{}, false
	}
}

// boolAttribute reads a boolean element_value_pair named key from a
// marker or annotation's arguments, falling back to fallback when args is
// nil (a bare marker annotation, e.g. plain @EnableMethodSecurity) or key
// isn't explicitly set.
func boolAttribute(args *sitter.Node, src []byte, key string, fallback bool) bool {
	if args == nil {
		return fallback
	}
	for _, arg := range namedChildren(args) {
		if arg.Type() != "element_value_pair" {
			continue
		}
		k := arg.ChildByFieldName("key")
		if k == nil || k.Content(src) != key {
			continue
		}
		v := arg.ChildByFieldName("value")
		if v == nil {
			continue
		}
		switch v.Type() {
		case "true":
			return true
		case "false":
			return false
		default:
			return fallback
		}
	}
	return fallback
}
