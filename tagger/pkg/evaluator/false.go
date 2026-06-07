package evaluator

// FalseEvaluator returns false for every tag.
type FalseEvaluator struct{}

// NewFalseEvaluator creates a new FalseEvaluator.
func NewFalseEvaluator() *FalseEvaluator {
	return &FalseEvaluator{}
}

// Evaluate returns false for all tags regardless of data type.
func (e *FalseEvaluator) Evaluate(dataType DataType, content []byte, tags []string) map[string]bool {
	result := make(map[string]bool, len(tags))
	for _, tag := range tags {
		result[tag] = false
	}
	return result
}

// GetSupportedDataTypes returns all supported data types.
func (e *FalseEvaluator) GetSupportedDataTypes() []string {
	return []string{string(DataTypeTxt), string(DataTypePng)}
}
