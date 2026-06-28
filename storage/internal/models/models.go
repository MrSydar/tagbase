package models

import (
	"time"
)

// Collection represents a collection in the system.
type Collection struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	DataType  string    `json:"data_type"`
	CreatedAt time.Time `json:"created_at"`
}

// Object represents object metadata.
type Object struct {
	ID           string          `json:"id"`
	Collection   string          `json:"collection"`
	CollectionID string          `json:"-"`
	DataType     string          `json:"data_type"`
	Date         time.Time       `json:"date"`
	SizeBytes    int64           `json:"size_bytes"`
	ContentHash  string          `json:"content_hash"`
	CreatedAt    time.Time       `json:"created_at"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	PayloadKey   string          `json:"payload_key,omitempty"`
	Tags         map[string]bool `json:"tags,omitempty"`
}

// TagResult represents a tag evaluation result.
type TagResult struct {
	Tag   string `json:"tag"`
	Value bool   `json:"value"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

// CollectionCreateRequest is the request to create a collection.
type CollectionCreateRequest struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// CollectionCreateResponse is the response after creating a collection.
type CollectionCreateResponse struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// CollectionsListResponse is the response for listing collections.
type CollectionsListResponse struct {
	Collections []Collection `json:"collections"`
}

// ObjectUploadResponse is the response after uploading an object.
type ObjectUploadResponse struct {
	ID          string    `json:"id"`
	Collection  string    `json:"collection"`
	DataType    string    `json:"data_type"`
	Date        time.Time `json:"date"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentHash string    `json:"content_hash"`
}

// TagsQueryRequest is the request for querying objects by tags.
type TagsQueryRequest struct {
	Tags      map[string]bool `json:"tags,omitempty"`
	Date      *DateFilter     `json:"date,omitempty"`
	Limit     int             `json:"limit"`
	Cursor    string          `json:"cursor,omitempty"`
	TimeoutMs int             `json:"timeout_ms,omitempty"`
}

// TagsQueryResponse is the response for tag queries.
type TagsQueryResponse struct {
	Objects []Object `json:"objects"`
	Next    string   `json:"next,omitempty"`
}

// DateFilter defines date constraints.
type DateFilter struct {
	GT  *time.Time `json:"gt,omitempty"`
	GTE *time.Time `json:"gte,omitempty"`
	LT  *time.Time `json:"lt,omitempty"`
	LTE *time.Time `json:"lte,omitempty"`
	EQ  *time.Time `json:"eq,omitempty"`
}

// TaggingSupportedTypesResponse is the response from supported-types endpoint.
type TaggingSupportedTypesResponse struct {
	Types []string `json:"types"`
}

// TaggingRequest is the request to the tagging engine.
type TaggingRequest struct {
	Collection string   `json:"collection"`
	ObjectID   string   `json:"object_id"`
	Tags       []string `json:"tags"`
}

// TaggingResponse is the response from the tagging engine.
type TaggingResponse struct {
	Tags map[string]bool `json:"tags"`
}
