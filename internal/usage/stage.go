package usage

// Stage is the part of the workflow a model call is credited to.
type Stage string

const (
	SpecWriting     Stage = "spec_writing"
	Planning        Stage = "planning"
	Implementation  Stage = "implementation"
	Checks          Stage = "checks"
	AcceptanceEvals Stage = "acceptance_evals"
	CodeReview      Stage = "code_review"
	Publishing      Stage = "publishing"
	Closeout        Stage = "closeout"
	Other           Stage = "other"
)

// Stages lists every stage in display order.
var Stages = []Stage{SpecWriting, Planning, Implementation, Checks, AcceptanceEvals, CodeReview, Publishing, Closeout, Other}

// Label is the stage's name on the Report page.
func (s Stage) Label() string {
	switch s {
	case SpecWriting:
		return "Spec writing"
	case Planning:
		return "Planning"
	case Implementation:
		return "Implementation"
	case Checks:
		return "Checks"
	case AcceptanceEvals:
		return "Acceptance evals"
	case CodeReview:
		return "Code review"
	case Publishing:
		return "Publishing"
	case Closeout:
		return "Closeout"
	default:
		return "Other"
	}
}
