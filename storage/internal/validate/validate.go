package validate

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"mrsydar/tagbase/storage/internal/models"
)

var collectionNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ValidateCollectionName checks if a collection name is valid.
func ValidateCollectionName(name string) error {
	if !collectionNameRe.MatchString(name) {
		return fmt.Errorf("collection name must be 1-64 chars, lowercase letters, digits, underscore, hyphen, starting with a letter")
	}
	return nil
}

// ValidateDataType checks if data type is in the supported set.
func ValidateDataType(dataType string, supported []string) error {
	for _, s := range supported {
		if s == dataType {
			return nil
		}
	}
	return fmt.Errorf("unsupported data_type: %s", dataType)
}

// ValidateTag checks if a single tag is valid.
func ValidateTag(tag string) error {
	if len(tag) == 0 {
		return fmt.Errorf("tag cannot be empty")
	}
	if !utf8.ValidString(tag) {
		return fmt.Errorf("tag must be valid UTF-8")
	}
	if len(tag) > 128 {
		// byte length check
		return fmt.Errorf("tag exceeds 128 bytes")
	}
	return nil
}

// ValidateTags checks all tags.
func ValidateTags(tags map[string]bool, maxCount int) error {
	if len(tags) > maxCount {
		return fmt.Errorf("too many tags in query: %d > %d", len(tags), maxCount)
	}
	for tag := range tags {
		if err := ValidateTag(tag); err != nil {
			return fmt.Errorf("invalid tag %q: %w", tag, err)
		}
	}
	return nil
}

// ValidateDateFilter checks the date filter shape.
func ValidateDateFilter(df *models.DateFilter) error {
	if df == nil {
		return nil
	}
	if df.EQ != nil {
		if df.GT != nil || df.GTE != nil || df.LT != nil || df.LTE != nil {
			return fmt.Errorf("eq cannot be combined with other date filters")
		}
		return nil
	}
	return nil
}

// ParseDateFilterFromMap parses a date filter from a map (used when parsing JSON with raw values).
func ParseDateFilterFromMap(m map[string]string) (*models.DateFilter, error) {
	if len(m) == 0 {
		return nil, nil
	}
	df := &models.DateFilter{}
	hasEQ := false
	for k, v := range m {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, fmt.Errorf("invalid date format for %s: %w", k, err)
		}
		switch strings.ToLower(k) {
		case "gt":
			df.GT = &t
		case "gte":
			df.GTE = &t
		case "lt":
			df.LT = &t
		case "lte":
			df.LTE = &t
		case "eq":
			df.EQ = &t
			hasEQ = true
		default:
			return nil, fmt.Errorf("unknown date filter key: %s", k)
		}
	}
	if hasEQ && (df.GT != nil || df.GTE != nil || df.LT != nil || df.LTE != nil) {
		return nil, fmt.Errorf("eq cannot be combined with other date filters")
	}
	return df, nil
}
