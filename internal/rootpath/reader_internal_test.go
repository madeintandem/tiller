package rootpath

// RetainedCount and RetainedDirectoryLimit let the external test package
// observe the Reader's descriptor bound.
func (r *Reader) RetainedCount() int { return len(r.dirs) }

const RetainedDirectoryLimit = retainedDirectoryLimit
