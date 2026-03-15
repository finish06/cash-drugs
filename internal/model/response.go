package model

// APIResponse is the envelope returned to consumers.
type APIResponse struct {
	Data interface{}  `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

// ResponseMeta contains metadata about the cached response.
type ResponseMeta struct {
	Slug         string `json:"slug"`
	SourceURL    string `json:"source_url"`
	FetchedAt    string `json:"fetched_at"`
	PageCount    int    `json:"page_count"`
	ResultsCount int    `json:"results_count"`
	Stale        bool   `json:"stale"`
	StaleReason  string `json:"stale_reason,omitempty"`
}

// ErrorResponse is returned for error conditions.
type ErrorResponse struct {
	Error          string `json:"error"`
	Slug           string `json:"slug,omitempty"`
	UpstreamStatus int    `json:"upstream_status,omitempty"`
	Message        string `json:"message,omitempty"`
}
