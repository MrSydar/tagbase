package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the storage service public API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a new storage client.
func New(baseURL string) *Client {
	slog.Debug("New", "baseURL", baseURL)
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Collection represents a collection from the storage service.
type Collection struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// Object represents object metadata from the storage service.
type Object struct {
	ID          string          `json:"id"`
	Collection  string          `json:"collection"`
	DataType    string          `json:"data_type"`
	Date        time.Time       `json:"date"`
	SizeBytes   int64           `json:"size_bytes"`
	ContentHash string          `json:"content_hash"`
	CreatedAt   time.Time       `json:"created_at"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	PayloadKey  string          `json:"payload_key,omitempty"`
	Tags        map[string]bool `json:"tags,omitempty"`
}

// ObjectUploadResponse is returned after uploading an object.
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
	Tags   map[string]bool `json:"tags,omitempty"`
	Date   *DateFilter     `json:"date,omitempty"`
	Limit  int             `json:"limit"`
	Cursor string          `json:"cursor,omitempty"`
}

// DateFilter defines date constraints.
type DateFilter struct {
	GT  *time.Time `json:"gt,omitempty"`
	GTE *time.Time `json:"gte,omitempty"`
	LT  *time.Time `json:"lt,omitempty"`
	LTE *time.Time `json:"lte,omitempty"`
	EQ  *time.Time `json:"eq,omitempty"`
}

// TagsQueryResponse is the response for tag queries.
type TagsQueryResponse struct {
	Objects []Object `json:"objects"`
	Next    string   `json:"next,omitempty"`
}

// TagResponse is returned when getting tags for an object.
type TagResponse struct {
	ID   string          `json:"id"`
	Tags map[string]bool `json:"tags"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

// ListCollections returns all collections.
func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	slog.Debug("ListCollections: called")
	url := c.baseURL + "/v1/collections"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list collections: status %d", resp.StatusCode)
	}
	var result struct {
		Collections []Collection `json:"collections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode collections: %w", err)
	}
	return result.Collections, nil
}

// CreateCollection creates a new collection.
func (c *Client) CreateCollection(ctx context.Context, name, dataType string) (*Collection, error) {
	slog.Debug("CreateCollection", "name", name, "dataType", dataType)
	reqBody, err := json.Marshal(map[string]string{"name": name, "data_type": dataType})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/collections", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create collection: status %d", resp.StatusCode)
	}
	var coll Collection
	if err := json.NewDecoder(resp.Body).Decode(&coll); err != nil {
		return nil, fmt.Errorf("decode collection: %w", err)
	}
	return &coll, nil
}

// GetObjectMetadata fetches object metadata by collection and ID.
func (c *Client) GetObjectMetadata(ctx context.Context, collection, id string) (*Object, error) {
	slog.Debug("GetObjectMetadata", "collection", collection, "id", id)
	url := fmt.Sprintf("%s/v1/collections/%s/objects/%s", c.baseURL, collection, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch metadata: status %d", resp.StatusCode)
	}
	var obj Object
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	return &obj, nil
}

// GetObjectData downloads object payload by collection and ID.
func (c *Client) GetObjectData(ctx context.Context, collection, id string) ([]byte, error) {
	slog.Debug("GetObjectData", "collection", collection, "id", id)
	url := fmt.Sprintf("%s/v1/collections/%s/objects/%s/data", c.baseURL, collection, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch data: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch data: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}
	return data, nil
}

// GetObjectTags fetches tags for an object.
func (c *Client) GetObjectTags(ctx context.Context, collection, id string, tags []string) (*TagResponse, error) {
	slog.Debug("GetObjectTags", "collection", collection, "id", id, "tags", tags)
	url := fmt.Sprintf("%s/v1/collections/%s/objects/%s/tags?tags=%s", c.baseURL, collection, id, strings.Join(tags, ","))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tags: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tags: status %d", resp.StatusCode)
	}
	var result TagResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	return &result, nil
}

// QueryObjects queries objects by tags.
func (c *Client) QueryObjects(ctx context.Context, collection string, req TagsQueryRequest) (*TagsQueryResponse, error) {
	slog.Debug("QueryObjects", "collection", collection)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v1/collections/%s/objects/query", c.baseURL, collection)
	hreq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("query objects: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query objects: status %d", resp.StatusCode)
	}
	var result TagsQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode query response: %w", err)
	}
	return &result, nil
}

// UploadObject uploads an object body to the storage service.
func (c *Client) UploadObject(ctx context.Context, collection, dataType string, data []byte, date time.Time, ttlSeconds int) (*ObjectUploadResponse, error) {
	slog.Debug("UploadObject", "collection", collection, "dataType", dataType, "dataLen", len(data), "ttl", ttlSeconds)
	q := fmt.Sprintf("%s/v1/collections/%s/objects?data_type=%s", c.baseURL, collection, dataType)
	if !date.IsZero() {
		q += fmt.Sprintf("&date=%s", date.Format(time.RFC3339))
	}
	if ttlSeconds > 0 {
		q += fmt.Sprintf("&ttl_seconds=%d", ttlSeconds)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", q, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload object: status %d", resp.StatusCode)
	}
	var result ObjectUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode upload response: %w", err)
	}
	return &result, nil
}

// DeleteCollection deletes a collection by name.
func (c *Client) DeleteCollection(ctx context.Context, collection string) error {
	slog.Debug("DeleteCollection", "collection", collection)
	url := fmt.Sprintf("%s/v1/collections/%s", c.baseURL, collection)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete collection: status %d", resp.StatusCode)
	}
	return nil
}

// DeleteObject deletes an object by collection and ID.
func (c *Client) DeleteObject(ctx context.Context, collection, id string) error {
	slog.Debug("DeleteObject", "collection", collection, "id", id)
	url := fmt.Sprintf("%s/v1/collections/%s/objects/%s", c.baseURL, collection, id)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete object: status %d", resp.StatusCode)
	}
	return nil
}
