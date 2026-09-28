package cerbos

import (
	"strings"
	"testing"

	"github.com/chebilax/sphinxor/internal/model"
)

// Unit tests for docs/decisions/0041-permission-export.md's translation.
// Synthetic by necessity (docs/testing.md): most branches have no corpus
// instance — RuoYi-Vue's calls are single-literal hasPermi, and the rest
// is one hasRole — and each test says which rule it pins.

// permModel builds one controller whose endpoints each carry one
// @PreAuthorize bean call, given as Via and literals.
func permModel(calls ...[]string) *model.Model {
	m := &model.Model{Controllers: []model.Controller{{ID: "c1", Name: "SysUserController"}}}
	verbs := []model.HTTPMethod{model.MethodGet, model.MethodPost, model.MethodPut, model.MethodDelete, model.MethodPatch}
	for i, call := range calls {
		ep := model.ID("e" + string(rune('1'+i)))
		g := model.ID("g" + string(rune('1'+i)))
		m.Endpoints = append(m.Endpoints, model.Endpoint{ID: ep, ControllerID: "c1", HTTPMethod: verbs[i], Path: "/u"})
		m.GuardApplications = append(m.GuardApplications, model.GuardApplication{
			ID: g, EndpointID: ep, GuardName: "PreAuthorize", AppliedAt: model.ScopeMethod, DeclaresRoles: true,
		})
		for j, lit := range call[1:] {
			m.PermissionReferences = append(m.PermissionReferences, model.PermissionReference{
				ID: model.ID(string(g) + "p" + string(rune('0'+j))), GuardApplicationID: g, RawLiteral: lit, Via: call[0],
			})
		}
	}
	return m
}

func ruleFor(t *testing.T, r Result, action string) (Rule, bool) {
	t.Helper()
	for _, rule := range r.Rules {
		if rule.Action == action {
			return rule, true
		}
	}
	return Rule{}, false
}

func omissionFor(r Result, action string) (Omission, bool) {
	for _, o := range r.Omissions {
		if strings.ToLower(string(o.Endpoint.HTTPMethod)) == action {
			return o, true
		}
	}
	return Omission{}, false
}

func TestPermissionExport_Shapes(t *testing.T) {
	m := permModel(
		[]string{"@ss.hasPermi", "system:user:list"},   // get: single permission
		[]string{"@el.check", "job:list", "user:list"}, // post: several, any-of declared
		[]string{"@ss.hasRole", "admin"},               // put: a role callee
		[]string{"@ss.lacksPermi", "system:user:list"}, // delete: undeclared negation
		[]string{"@ss.hasAnyPermi", "a:b", "c:d"},      // patch: several, combination unstated
	)
	d := Declarations{
		PermissionCallees: map[string]Combine{"@ss.hasPermi": CombineUnstated, "@el.check": CombineAnyOf, "@ss.hasAnyPermi": CombineUnstated},
		RoleCallees:       map[string]Combine{"@ss.hasRole": CombineUnstated},
		NoSuperuserEscape: true,
	}
	r := TranslateWith(m, d)

	if rule, ok := ruleFor(t, r, "get"); !ok || rule.Condition.key() != `"system:user:list" in P.attr.permissions` || strings.Join(rule.Roles, ",") != "*" {
		t.Errorf("get: want roles [*] with the permission condition, got %+v", rule)
	}
	if rule, ok := ruleFor(t, r, "post"); !ok || rule.Condition.key() != `any("job:list" in P.attr.permissions,"user:list" in P.attr.permissions)` {
		t.Errorf("post: want an any-of condition over both literals, got %+v", rule)
	}
	if rule, ok := ruleFor(t, r, "put"); !ok || rule.Condition != nil || strings.Join(rule.Roles, ",") != "admin" {
		t.Errorf("put: a declared role callee is a plain roles list, got %+v", rule)
	}
	// ADR 0041 §2: an undeclared callee is never interpreted — a
	// lacksPermi read by name would grant the principals it denies.
	if o, ok := omissionFor(r, "delete"); !ok || o.Reason != ReasonCalleeNotDeclared {
		t.Errorf("delete: want callee-not-declared, got %+v", o)
	}
	if o, ok := omissionFor(r, "patch"); !ok || o.Reason != ReasonCombinationNotDeclared {
		t.Errorf("patch: want combination-not-declared, got %+v", o)
	}
	if len(r.Declarations) == 0 || r.Contract == "" {
		t.Error("an export with declarations must carry them and the integration contract into the report")
	}
}

// TestPermissionExport_EscapesAreAlternatives pins §3/§4: the declared
// superuser satisfies every rule on its own, and a role callee gains a
// condition only when a superuser permission must be admitted too.
func TestPermissionExport_EscapesAreAlternatives(t *testing.T) {
	m := permModel([]string{"@ss.hasPermi", "system:user:edit"}, []string{"@ss.hasRole", "admin"})
	d := Declarations{
		PermissionCallees:    map[string]Combine{"@ss.hasPermi": CombineUnstated},
		RoleCallees:          map[string]Combine{"@ss.hasRole": CombineUnstated},
		SuperuserPermissions: []string{"*:*:*"},
		SuperuserRoles:       []string{"admin"},
	}
	r := TranslateWith(m, d)
	get, _ := ruleFor(t, r, "get")
	if want := `any("*:*:*" in P.attr.permissions,"admin" in P.roles,"system:user:edit" in P.attr.permissions)`; get.Condition.key() != want {
		t.Errorf("permission rule condition = %s, want %s", get.Condition.key(), want)
	}
	post, _ := ruleFor(t, r, "post")
	// "admin" is both the literal and the superuser role: one term, not two.
	if want := `any("*:*:*" in P.attr.permissions,"admin" in P.roles)`; post.Condition.key() != want {
		t.Errorf("role rule condition = %s, want %s", post.Condition.key(), want)
	}
}

// TestPermissionExport_MixedEvidenceIsOmitted: an endpoint whose roles and
// permission come from separate annotations keeps no partial grant, since
// exporting the roles alone would drop the permission — an over-grant.
func TestPermissionExport_MixedEvidenceIsOmitted(t *testing.T) {
	m := permModel([]string{"@ss.hasPermi", "system:user:edit"})
	m.GuardApplications = append(m.GuardApplications, model.GuardApplication{
		ID: "gc", EndpointID: "e1", GuardName: "PreAuthorize", AppliedAt: model.ScopeClass, DeclaresRoles: true,
	})
	m.RoleReferences = append(m.RoleReferences, model.RoleReference{ID: "r1", GuardApplicationID: "gc", RawLiteral: "USER"})
	r := TranslateWith(m, Declarations{PermissionCallees: map[string]Combine{"@ss.hasPermi": CombineUnstated}, NoSuperuserEscape: true})
	if len(r.Rules) != 0 {
		t.Fatalf("want no rule, got %+v", r.Rules)
	}
	if o, ok := omissionFor(r, "get"); !ok || o.Reason != ReasonPermissionNotExportable {
		t.Errorf("want permission-not-exportable, got %+v", o)
	}
}

// TestPermissionExport_ValidateNeverRefusesTheZeroValue is §1's guarantee
// at the library level; the CLI test pins it end to end.
func TestPermissionExport_ValidateNeverRefusesTheZeroValue(t *testing.T) {
	if err := (Declarations{}).Validate(); err != nil {
		t.Errorf("the zero Declarations must validate, got %v", err)
	}
}
