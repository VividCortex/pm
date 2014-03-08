package pm

import (
	"time"
)

// AttrDetail encodes a single, user-defined name/value pair.
type AttrDetail struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// Type ProcDetail encodes a full process list from the server, including an
// attributes array with application-defined names/values.
type ProcDetail struct {
	Id         string       `json:"id"`
	Attrs      []AttrDetail `json:"attrs,omitempty"`
	ProcTime   time.Time    `json:"procTime"`
	StatusTime time.Time    `json:"statusTime"`
	Status     string       `json:"status"`
	Cancelling bool         `json:"cancelling,omitempty"`
}

// ProcResponse is the response for a GET to /proc.
type ProcResponse struct {
	Procs      []ProcDetail `json:"procs"`
	ServerTime time.Time    `json:"serverTime"`
}

// HistoryDetail encodes one entry from the process' history.
type HistoryDetail struct {
	Ts     time.Duration `json:"cumulativeTime"`
	Status string    `json:"status"`
}

// HistoryResponse is the response for a GET to /proc/<id>/history.
type HistoryResponse struct {
	History    []HistoryDetail `json:"history"`
	ServerTime time.Time       `json:"serverTime"`
}

// CancelRequest is the request body resulting from Kill().
type CancelRequest struct {
	Message string `json:"message"`
}
