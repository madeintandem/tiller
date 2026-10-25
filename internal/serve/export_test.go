package serve

// MaxServedFileBytes exposes the evidence and prose read cap to the external
// test package, so the 413 tests size their fixtures from the real limit.
const MaxServedFileBytes = maxServedFileBytes
