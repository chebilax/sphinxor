package diff

import (
	"fmt"
	"sort"

	"github.com/chebilax/sphinxor/internal/model"
)

// findingKey is a Finding's derived stable cross-run identity: which
// rule produced it, and a stable key for its subject. Finding.SubjectID
// alone isn't safely comparable across runs for every SubjectKind — see
// subjectKey.
type findingKey struct {
	ruleID     string
	subjectKey string
}

func keyOfFinding(f model.Finding, m *model.Model) findingKey {
	return findingKey{ruleID: f.RuleID, subjectKey: subjectKey(f, m)}
}

// subjectKey resolves a Finding's subject to a value that's actually
// stable across two separate extraction runs.
//
//   - SubjectEndpoint: SubjectID already IS Endpoint.ID, deterministically
//     derived from HTTPMethod+Path (model.NewEndpointID) — stable as-is.
//   - SubjectRoleDeclaration: SubjectID is a per-run sequential ID
//     ("role-3"), NOT stable — resolved instead to the declaration's
//     Name ("RoleEnum.admin"), the field ADR 0002 designates as the
//     stable key for this collection.
//   - SubjectAllowMarker: SubjectID is "file:line" — positional by the
//     nature of that finding type (there's nothing else to key a
//     comment's location on); left as-is.
func subjectKey(f model.Finding, m *model.Model) string {
	if f.SubjectKind == model.SubjectRoleDeclaration {
		for _, d := range m.RoleDeclarations {
			if d.ID == f.SubjectID {
				return "role:" + d.Name
			}
		}
		// Declaration not found (shouldn't happen: a finding's subject
		// is always drawn from the same model run) — fall back to the
		// raw ID rather than panic; this can only ever fail to match
		// anything across runs, never falsely match.
		return "role-unresolved:" + string(f.SubjectID)
	}
	return string(f.SubjectKind) + ":" + string(f.SubjectID)
}

// diffRegressions implements ADR 0007 §3: a regression is a head finding
// that is High-confidence and not allowlisted, and either has no match
// in base's High-confidence set at the same (RuleID, subject) key, or
// matches a base finding that WAS allowlisted and no longer is.
//
// base's match set is built from High-confidence findings only — not
// "any base finding regardless of confidence" — deliberately, so a
// finding that transitions from Low in base to High in head at the same
// subject and rule is treated as new (it is: it's the first time
// anything gates on it), not silently matched against its Low ancestor
// and dropped. No rule in the current v0.1 set produces a per-instance
// confidence that could vary this way, but the matching logic has to be
// correct on its own terms — see TestDiffRegressions_LowToHighTransition.
func diffRegressions(base, head Snapshot) []Regression {
	baseHighByKey := make(map[findingKey]model.Finding)
	for _, f := range base.Findings {
		if f.Confidence != model.ConfidenceHigh {
			continue
		}
		baseHighByKey[keyOfFinding(f, base.Model)] = f
	}

	var out []Regression
	for _, f := range head.Findings {
		if f.Confidence != model.ConfidenceHigh || f.Allowlisted {
			continue
		}
		k := keyOfFinding(f, head.Model)
		baseFinding, existed := baseHighByKey[k]
		switch {
		case !existed:
			out = append(out, Regression{Finding: f, Reason: ReasonNew})
		case baseFinding.Allowlisted:
			out = append(out, Regression{Finding: f, Reason: ReasonAllowlistRemoved})
		}
	}

	out = append(out, becamePublicRegressions(base, head)...)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Finding.RuleID != out[j].Finding.RuleID {
			return out[i].Finding.RuleID < out[j].Finding.RuleID
		}
		return out[i].Finding.SubjectID < out[j].Finding.SubjectID
	})
	return out
}

// becamePublicRuleID names ADR 0036 §1's case (c) in the report.
//
// It is NOT a lint rule and `sphinxor lint` never emits it: it is a fact
// about a transition between two snapshots, meaningless in one, absent
// from lint.DefaultRules(), and not among docs/vision.md's three rules.
// Synthesizing a model.Finding is how it reaches the existing report row
// and JSON shape without a parallel structure (ADR 0036 §7).
const becamePublicRuleID = "endpoint-became-public"

// becamePublicRegressions implements ADR 0036 §1 case (c): an endpoint
// present in both snapshots, protected in base, unprotected in head, and
// not allowlisted in head.
//
// It reuses Result.BecamePublic's own computation — via becamePublic —
// rather than recomputing the transition, so the gated set can never
// drift from the set the report prints. The one thing it adds is the
// allowlist filter, and the report deliberately keeps showing an
// allowlisted transition (ADR 0036 §7): excused is not the same as
// invisible.
func becamePublicRegressions(base, head Snapshot) []Regression {
	var out []Regression
	for _, e := range becamePublic(base.Model, head.Model) {
		if head.AllowlistedEndpoints[e.ID] {
			// ADR 0036 §4: the marker is the author's statement about
			// the code as it now stands, which is exactly this case.
			continue
		}
		out = append(out, Regression{
			Finding: model.Finding{
				// Same shape lint.Run assigns, so a JSON consumer sees
				// no empty field where every other finding has an ID.
				// Positional within the run, exactly as lint.Run's is;
				// what is stable across runs is SubjectID, which is
				// derived from method and path (model.NewEndpointID).
				ID:          model.ID(fmt.Sprintf("%s-%d", becamePublicRuleID, len(out)+1)),
				RuleID:      becamePublicRuleID,
				Confidence:  model.ConfidenceHigh,
				SubjectID:   e.ID,
				SubjectKind: model.SubjectEndpoint,
				Message: fmt.Sprintf(
					"%s %s had access control in the base and has none in the head",
					e.HTTPMethod, e.Path),
			},
			Reason: ReasonBecamePublic,
		})
	}
	return out
}
