package client

import "context"

// Tagger is the interface for a tagging engine client.
type Tagger interface {
	GetSupportedTypes(ctx context.Context) ([]string, error)
	Tag(ctx context.Context, collection, objectID string, tags []string) (map[string]bool, error)
}
