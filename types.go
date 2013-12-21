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
	ProcTm     time.Time    `json:"proctm"`
	StatusTm   time.Time    `json:"statustm"`
	Status     string       `json:"status"`
	Cancelling bool         `json:"cancelling,omitempty"`
}

// ProcResponse is the response for a GET to /proc.
type ProcResponse struct {
	Procs    []ProcDetail `json:"procs"`
	ServerTm time.Time    `json:"servertm"`
}

// JournalDetail encodes one entry from the process' journal.
type JournalDetail struct {
	Ts     time.Time `json:"ts"`
	Status string    `json:"status"`
}

// JournalResponse is the response for a GET to /proc/<id>/journal.
type JournalResponse struct {
	Journal  []JournalDetail `json:"journal"`
	ServerTm time.Time       `json:"servertm"`
}

// CancelRequest is the request body resulting from Kill().
type CancelRequest struct {
	Message string `json:"message"`
}
