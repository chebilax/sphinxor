package cerbos

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Declarations are the owner's statements about what a @PreAuthorize bean
// call means — docs/decisions/0041-permission-export.md. Sphinxor never
// infers them: RuoYi's own bean defines lacksPermi, a negation, beside
// hasPermi, so a reading by name would turn a denial into a grant.
//
// The zero value declares nothing, and Translate behaves exactly as it
// did before permissions could be exported (ADR 0041 §1).
type Declarations struct {
	// PermissionCallees: the literal is a permission the principal must
	// hold. Keyed by PermissionReference.Via, matched exactly.
	PermissionCallees map[string]Combine
	// RoleCallees: the literal is a role the principal must hold.
	RoleCallees map[string]Combine
	// SuperuserPermissions and SuperuserRoles are the escape the beans
	// honour (ADR 0035 Amendment 1), added to every exported permission
	// and role rule.
	SuperuserPermissions []string
	SuperuserRoles       []string
	// NoSuperuserEscape states explicitly that the beans have none.
	NoSuperuserEscape bool
}

// Combine is how a call naming several literals combines them. A call
// naming one literal needs no suffix; one naming several is exported only
// when its callee says how (ADR 0041 §2).
type Combine int

const (
	CombineUnstated Combine = iota
	CombineAnyOf
	CombineAllOf
)

// Active reports whether any callee was declared — the only case in which
// permissions are exported, and the only case in which the escape must be
// addressed.
func (d Declarations) Active() bool {
	return len(d.PermissionCallees)+len(d.RoleCallees) > 0
}

// ErrEscapeUndeclared is ADR 0041 §3's refusal.
var ErrEscapeUndeclared = errors.New("permission export requires the superuser escape to be declared or waived: " +
	"pass --superuser-permission and/or --superuser-role naming the escape the bean honours, or --no-superuser-escape " +
	"to state it has none. Every bean behind a permission in Sphinxor's corpus admits a superuser the call does not " +
	"name (docs/decisions/0035-permissions-in-the-model.md Amendment 1), so a policy built from the literal alone " +
	"would deny exactly the most privileged users")

// Validate applies ADR 0041 §3. It never fails for the zero value, so an
// export run with none of the new flags can never be refused (§1).
func (d Declarations) Validate() error {
	escape := len(d.SuperuserPermissions)+len(d.SuperuserRoles) > 0
	switch {
	case !d.Active() && (escape || d.NoSuperuserEscape):
		return errors.New("--superuser-permission, --superuser-role and --no-superuser-escape apply only to declared " +
			"callees: pass --permission-callee or --role-callee as well")
	case !d.Active():
		return nil
	case escape && d.NoSuperuserEscape:
		return errors.New("--no-superuser-escape contradicts --superuser-permission/--superuser-role: pass one or the other")
	case !escape && !d.NoSuperuserEscape:
		return ErrEscapeUndeclared
	}
	for callee := range d.PermissionCallees {
		if _, both := d.RoleCallees[callee]; both {
			return fmt.Errorf("%s is declared both a permission callee and a role callee", callee)
		}
	}
	return nil
}

// ParseCallee splits a flag value such as "@el.check:any-of" into the
// callee and how several literals combine.
func ParseCallee(value string) (string, Combine, error) {
	callee, combine := value, CombineUnstated
	switch {
	case strings.HasSuffix(value, ":any-of"):
		callee, combine = strings.TrimSuffix(value, ":any-of"), CombineAnyOf
	case strings.HasSuffix(value, ":all-of"):
		callee, combine = strings.TrimSuffix(value, ":all-of"), CombineAllOf
	}
	if !strings.HasPrefix(callee, "@") || len(callee) < 2 {
		return "", 0, fmt.Errorf("callee %q: expected the call as the matrix shows it, such as @ss.hasPermi", value)
	}
	return callee, combine, nil
}

// summary lists the declarations verbatim, for the report: a reviewer of
// the policy sees whose statements its semantics are.
func (d Declarations) summary() []string {
	if !d.Active() {
		return nil
	}
	var out []string
	name := func(c Combine) string {
		switch c {
		case CombineAnyOf:
			return " (several literals: any of them)"
		case CombineAllOf:
			return " (several literals: all of them)"
		}
		return ""
	}
	for _, c := range sortedKeys(d.PermissionCallees) {
		out = append(out, c+" names a permission the principal must hold"+name(d.PermissionCallees[c]))
	}
	for _, c := range sortedKeys(d.RoleCallees) {
		out = append(out, c+" names a role the principal must hold"+name(d.RoleCallees[c]))
	}
	switch {
	case d.NoSuperuserEscape:
		out = append(out, "the beans admit no superuser beyond what each call names (--no-superuser-escape)")
	default:
		for _, p := range d.SuperuserPermissions {
			out = append(out, "a principal holding permission "+strconv.Quote(p)+" passes every declared check")
		}
		for _, r := range d.SuperuserRoles {
			out = append(out, "a principal with role "+strconv.Quote(r)+" passes every declared check")
		}
	}
	return out
}

func sortedKeys(m map[string]Combine) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// integrationContract is what the calling service must supply for a
// permission rule to mean what it says (ADR 0041 §4).
const integrationContract = "Permission rules test the principal attribute `permissions` (`P.attr.permissions`): the calling " +
	"service must pass the principal's permission strings there, as granted and before any bean logic. The superuser " +
	"escape is written into the policy, so it must not also be expanded in the attribute. A principal without the " +
	"attribute is denied."

// Condition is a Cerbos match expression: one CEL expression, or any/all
// of nested conditions.
type Condition struct {
	Expr string      `json:",omitempty"`
	Any  []Condition `json:",omitempty"`
	All  []Condition `json:",omitempty"`
}

// key renders a condition canonically, so two endpoints' requirements can
// be compared when deciding whether they share one rule.
func (c *Condition) key() string {
	if c == nil {
		return ""
	}
	switch {
	case c.Expr != "":
		return c.Expr
	case len(c.Any) > 0:
		return "any(" + joinKeys(c.Any) + ")"
	default:
		return "all(" + joinKeys(c.All) + ")"
	}
}

func joinKeys(cs []Condition) string {
	parts := make([]string, len(cs))
	for i := range cs {
		parts[i] = cs[i].key()
	}
	return strings.Join(parts, ",")
}

// permissionTerm and roleTerm are the CEL tests ADR 0041 verified against
// the real engine. strconv.Quote yields a double-quoted literal whose
// escapes CEL reads the same way.
func permissionTerm(p string) Condition {
	return Condition{Expr: strconv.Quote(p) + " in P.attr.permissions"}
}

func roleTerm(r string) Condition {
	return Condition{Expr: strconv.Quote(r) + " in P.roles"}
}
