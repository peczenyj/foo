package metrics

import "expvar"

// NewUnpublishedExpvar returns an ExpvarRecorder whose maps are not
// published: tests can create as many as they need (expvar.Publish panics on
// a name published twice).
func NewUnpublishedExpvar() *ExpvarRecorder { return newExpvarRecorder() }

// Maps returns the recorder's maps, for the tests to read.
func (r *ExpvarRecorder) Maps() (requests, seconds *expvar.Map) { return r.requests, r.seconds }
