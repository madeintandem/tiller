package store

// Bug severity values, stored lowercase, most severe first. Severity is a
// bug-only field; it never feeds routing or ordering.
const (
	SeverityBlocker   = "blocker"
	SeverityImportant = "important"
	SeverityMedium    = "medium"
	SeverityMinor     = "minor"
)

// Severities returns the legal severities, most severe first.
func Severities() []string {
	return []string{SeverityBlocker, SeverityImportant, SeverityMedium, SeverityMinor}
}

// ValidSeverity reports whether s is a persisted severity value.
func ValidSeverity(s string) bool {
	return SeverityRank(s) < len(Severities())
}

// SeverityRank orders severities: blocker 0 through minor 3. The empty
// string and any unknown value rank 4, after every rated bug.
func SeverityRank(s string) int {
	for i, v := range Severities() {
		if v == s {
			return i
		}
	}
	return len(Severities())
}
