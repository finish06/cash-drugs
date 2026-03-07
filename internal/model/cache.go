package model

import "time"

// PageData holds the items for a single page of a paginated response.
type PageData struct {
	Page int           `bson:"page" json:"page"`
	Data []interface{} `bson:"data" json:"data"`
}

// CachedResponse represents a cached upstream API response stored in MongoDB.
// For multi-page responses, each page is stored as a separate document with Page set.
type CachedResponse struct {
	Slug        string            `bson:"slug" json:"slug"`
	Params      map[string]string `bson:"params,omitempty" json:"params,omitempty"`
	CacheKey    string            `bson:"cache_key" json:"cache_key"`
	Page        int               `bson:"page" json:"page"`
	PageCount   int               `bson:"page_count" json:"page_count"`
	Data        interface{}       `bson:"data" json:"data"`
	Pages       []PageData        `bson:"-" json:"-"`
	ContentType string            `bson:"content_type" json:"content_type"`
	FetchedAt   time.Time         `bson:"fetched_at" json:"fetched_at"`
	SourceURL   string            `bson:"source_url" json:"source_url"`
	HTTPStatus  int               `bson:"http_status" json:"http_status"`
	CreatedAt   time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time         `bson:"updated_at" json:"updated_at"`
}
