package evaluator

import "strings"

// GrepEvaluator matches tags against text content using substring search.
type GrepEvaluator struct{}

// NewGrepEvaluator creates a new GrepEvaluator.
func NewGrepEvaluator() *GrepEvaluator {
	return &GrepEvaluator{}
}

// Evaluate returns true for a tag if the content (when dataType is txt) contains the tag as a substring.
// For non-txt data types all tags evaluate to false.
func (e *GrepEvaluator) Evaluate(dataType DataType, content []byte, tags []string) map[string]bool {
	result := make(map[string]bool, len(tags))
	if dataType != DataTypeTxt {
		for _, tag := range tags {
			result[tag] = false
		}
		return result
	}
	contentStr := string(content)
	for _, tag := range tags {
		result[tag] = strings.Contains(contentStr, tag)
	}
	return result
}

// GetSupportedDataTypes returns the data types supported by this evaluator.
func (e *GrepEvaluator) GetSupportedDataTypes() []string {
	return []string{string(DataTypeTxt)}
}
