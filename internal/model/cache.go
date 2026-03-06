package model

import "time"

// CachedResponse represents a cached upstream API response stored in MongoDB.
type CachedResponse struct {
	Slug        string            `bson:"slug" json:"slug"`
	Params      map[string]string `bson:"params,omitempty" json:"params,omitempty"`
	CacheKey    string            `bson:"cache_key" json:"cache_key"`
	Data        interface{}       `bson:"data" json:"data"`
	ContentType string            `bson:"content_type" json:"content_type"`
	FetchedAt   time.Time         `bson:"fetched_at" json:"fetched_at"`
	SourceURL   string            `bson:"source_url" json:"source_url"`
	HTTPStatus  int               `bson:"http_status" json:"http_status"`
	PageCount   int               `bson:"page_count" json:"page_count"`
	CreatedAt   time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time         `bson:"updated_at" json:"updated_at"`
}
