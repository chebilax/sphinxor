package diff

import (
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

	sort.Slice(out, func(i, j int) bool {
		if out[i].Finding.RuleID != out[j].Finding.RuleID {
			return out[i].Finding.RuleID < out[j].Finding.RuleID
		}
		return out[i].Finding.SubjectID < out[j].Finding.SubjectID
	})
	return out
}
