//go:build race

package api_test

// raceEnabled reports whether the tests run with -race, which adds
// allocations of its own.
const raceEnabled = true
