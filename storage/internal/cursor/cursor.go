package cursor

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Encode creates a cursor from date and id.
func Encode(date time.Time, id string) string {
	unixMillis := date.UnixNano() / 1_000_000
	raw := fmt.Sprintf("%d|%s", unixMillis, id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses a cursor into date millis and id.
func Decode(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", fmt.Errorf("empty cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	unixMillis, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor date: %w", err)
	}
	date := time.Unix(0, unixMillis*1_000_000).UTC()
	return date, parts[1], nil
}
