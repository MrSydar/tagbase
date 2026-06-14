package evaluator

import (
	"log/slog"
	"strings"
)

// GrepEvaluator matches tags against text content using substring search.
type GrepEvaluator struct{}

// NewGrepEvaluator creates a new GrepEvaluator.
func NewGrepEvaluator() *GrepEvaluator {
	slog.Debug("NewGrepEvaluator called")
	return &GrepEvaluator{}
}

// Evaluate returns true for a tag if the content (when dataType is txt) contains the tag as a substring.
// For non-txt data types all tags evaluate to false.
func (e *GrepEvaluator) Evaluate(dataType DataType, content []byte, tags []string) (map[string]bool, error) {
	slog.Debug("GrepEvaluator.Evaluate", "data_type", dataType, "tags_count", len(tags))
	result := make(map[string]bool, len(tags))
	if dataType != DataTypeTxt {
		slog.Debug("non-txt data type, returning false for all tags")
		for _, tag := range tags {
			result[tag] = false
		}
		return result, nil
	}
	contentStr := string(content)
	for _, tag := range tags {
		result[tag] = strings.Contains(contentStr, tag)
		slog.Debug("evaluating tag", "tag", tag, "result", result[tag])
	}
	return result, nil
}

// GetSupportedDataTypes returns the data types supported by this evaluator.
func (e *GrepEvaluator) GetSupportedDataTypes() []string {
	slog.Debug("GrepEvaluator.GetSupportedDataTypes called")
	return []string{string(DataTypeTxt)}
}
