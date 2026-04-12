package analytics

type Event map[string]map[string]any

type Client interface {
	RecordEvent(Event)
	Flush()
}

type Request struct {
	Data []Event `json:"data,omitempty"`
}
