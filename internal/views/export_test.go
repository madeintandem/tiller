package views

// Test hooks for package views_test: unexported engine parts its tests drive
// directly, so they can reuse that package's tree fixtures. Compiled only
// into the test binary.
var (
	CandidatesOf     = candidatesOf
	FrontierFeatures = frontierFeatures
	QueueKindArms    = queueKindArms
)

// Arm exposes a candidate's tie-break arm to tests.
func (c NextCandidate) Arm() int { return c.arm }

// UnplannedReason exposes the no-work-planned hint to tests.
var UnplannedReason = unplannedReason
