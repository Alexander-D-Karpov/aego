package services

// ConsoleMessage is a value snapshot of one row. Paths and line numbers are
// reported only when supplied by the producer. Text includes all log fields.
type ConsoleMessage struct {
	Time    float64 `json:"time"`
	Level   string  `json:"level"`
	Channel string  `json:"channel"`
	Text    string  `json:"text"`
	Code    string  `json:"code,omitempty"`
	Cause   string  `json:"causeCode,omitempty"`
	Path    string  `json:"path,omitempty"`
	Asset   string  `json:"asset,omitempty"`
	Line    int     `json:"line"`
	Count   int     `json:"count"`
}
