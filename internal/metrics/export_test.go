package metrics

import "expvar"

// NewUnpublishedExpvar returns an expvar Recorder whose maps are not
// published: tests can create as many as they need (expvar.Publish panics on
// a name published twice).
func NewUnpublishedExpvar() Recorder { return newExpvarRecorder() }

// ExpvarMaps returns the maps of r, a Recorder from Expvar or
// NewUnpublishedExpvar, for the tests to read.
func ExpvarMaps(r Recorder) (requests, seconds *expvar.Map) {
	e := r.(*expvarRecorder) //nolint:forcetypeassert // test-only: a wrong type is a test bug.

	return e.requests, e.seconds
}
