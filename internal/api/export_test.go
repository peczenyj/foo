package api

import "slices"

// Routes returns the patterns registered by New, for the contract test.
func (a *API) Routes() []string { return slices.Clone(a.routes) }
