package evaluator

// DataType is the supported data type enum.
type DataType string

const (
	DataTypeTxt DataType = "txt"
	DataTypePng DataType = "png"
)

// Evaluator evaluates tags against object content.
type Evaluator interface {
	Evaluate(dataType DataType, content []byte, tags []string) (map[string]bool, error)
	GetSupportedDataTypes() []string
}
