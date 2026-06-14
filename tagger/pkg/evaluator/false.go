package evaluator

import "log/slog"

// FalseEvaluator returns false for every tag.
type FalseEvaluator struct{}

// NewFalseEvaluator creates a new FalseEvaluator.
func NewFalseEvaluator() *FalseEvaluator {
	slog.Debug("NewFalseEvaluator called")
	return &FalseEvaluator{}
}

// Evaluate returns false for all tags regardless of data type.
func (e *FalseEvaluator) Evaluate(dataType DataType, content []byte, tags []string) (map[string]bool, error) {
	slog.Debug("FalseEvaluator.Evaluate", "data_type", dataType, "tags_count", len(tags))
	result := make(map[string]bool, len(tags))
	for _, tag := range tags {
		slog.Debug("evaluating tag", "tag", tag)
		result[tag] = false
	}
	return result, nil
}

// GetSupportedDataTypes returns all supported data types.
func (e *FalseEvaluator) GetSupportedDataTypes() []string {
	slog.Debug("FalseEvaluator.GetSupportedDataTypes called")
	return []string{string(DataTypeTxt), string(DataTypePng)}
}
