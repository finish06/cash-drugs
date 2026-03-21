package model

// Error codes follow the pattern CD-{CATEGORY}{NNN}
// Categories: H=Handler, U=Upstream, M=MongoDB, S=System
const (
	ErrCodeEndpointNotFound     = "CD-H001" // slug not in config
	ErrCodeUpstreamNotFound     = "CD-U002" // upstream returned 404
	ErrCodeUpstreamUnavailable  = "CD-U001" // upstream fetch failed, no cache
	ErrCodeCircuitOpen          = "CD-U003" // circuit breaker is open
	ErrCodeServiceOverloaded    = "CD-S001" // concurrency limit hit
	ErrCodeForceRefreshCooldown = "CD-H002" // force refresh rate limited
	ErrCodeMissingParams        = "CD-H003" // required parameters not provided
	ErrCodeMethodNotAllowed     = "CD-H004" // HTTP method not allowed
	ErrCodeBadRequest           = "CD-H005" // malformed request body or invalid parameters
)
