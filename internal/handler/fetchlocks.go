package handler

import "github.com/finish06/drugs/internal/fetchlock"

// FetchLocks is an alias for fetchlock.Map for backwards compatibility.
type FetchLocks = fetchlock.Map

// NewFetchLocks creates a new FetchLocks instance.
func NewFetchLocks() *FetchLocks {
	return fetchlock.New()
}
