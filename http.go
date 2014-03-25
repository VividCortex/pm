package pm

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderContentType = "Content-Type"
	MediaJSON         = "application/json"
)

func (pl *Proclist) getProcs() []ProcDetail {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	procs := make([]ProcDetail, 0, len(pl.procs))
	for id, p := range pl.procs {
		p.mu.RLock()
		attrs := make(map[string]interface{})
		for name, value := range p.attrs {
			attrs[name] = value
		}
		procs = append(procs, ProcDetail{
			Id:         id,
			Attrs:      attrs,
			ProcTime:   p.initialUpdate,
			StatusTime: p.latestUpdate,
			Status:     p.currentStatus,
			Cancelling: p.cancel.isPending,
		})
		p.mu.RUnlock()
	}
	return procs
}

func httpError(w http.ResponseWriter, httpCode int) {
	http.Error(w, http.StatusText(httpCode), httpCode)
}

func (pl *Proclist) handleProclistReq(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(ProcResponse{
		Procs:      pl.getProcs(),
		ServerTime: time.Now(),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderContentType, MediaJSON)
	w.Write(b)
}

func (pl *Proclist) getHistory(id string) ([]HistoryDetail, error) {
	pl.mu.RLock()
	p, present := pl.procs[id]
	pl.mu.RUnlock()

	if !present {
		return []HistoryDetail{}, ErrNoSuchProcess
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	p.history[p.currentStatus] += time.Since(p.latestUpdate)
	p.latestUpdate = time.Now()
	history := make([]HistoryDetail, 0, len(p.history))

	for entry, value := range p.history {
		history = append(history, HistoryDetail{
			Ts:     value.String(),
			Status: entry,
		})
	}
	return history, nil
}

func (pl *Proclist) handleHistoryReq(w http.ResponseWriter, r *http.Request, id string) {

	history, err := pl.getHistory(id)

	if err != nil {
		httpError(w, http.StatusNotFound)
	}
	b, err := json.Marshal(HistoryResponse{
		History:    history,
		ServerTime: time.Now(),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderContentType, MediaJSON)
	w.Write(b)
}

func (pl *Proclist) handleCancelReq(w http.ResponseWriter, r *http.Request, id string) {
	var message string
	var cancel CancelRequest
	if err := json.NewDecoder(r.Body).Decode(&cancel); err == nil {
		message = cancel.Message
	}
	if err := pl.Kill(id, message); err != nil {
		httpCode := http.StatusNotFound
		if err == ErrForbidden {
			httpCode = http.StatusForbidden
		}
		httpError(w, httpCode)
	}
}

func (pl *Proclist) handleProcsReq(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	path := r.URL.Path
	if path == "/procs/" {
		if r.Method == "GET" {
			pl.handleProclistReq(w, r)
		} else {
			httpError(w, http.StatusMethodNotAllowed)
		}
		return
	}

	// Path should start with "/procs/<id>"
	subdir := path[len("/procs/"):]
	sep := strings.Index(subdir, "/")
	if sep < 0 {
		sep = len(subdir)
	}
	if sep == 0 {
		httpError(w, http.StatusNotFound)
		return
	}
	id := subdir[:sep]
	subdir = subdir[sep:]

	switch {
	case subdir == "" || subdir == "/":
		if r.Method == "DELETE" {
			pl.handleCancelReq(w, r, id)
		} else if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "DELETE")
		} else {
			httpError(w, http.StatusMethodNotAllowed)
		}
	case subdir == "/history":
		if r.Method == "GET" {
			pl.handleHistoryReq(w, r, id)
		} else {
			httpError(w, http.StatusMethodNotAllowed)
		}
	default:
		httpError(w, http.StatusNotFound)
	}
}

// ListenAndServe starts an HTTP server at the given address (localhost:80
// by default, as results from the underlying net/http implementation).
func (pl *Proclist) ListenAndServe(addr string) error {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/procs/", pl.handleProcsReq)
	return http.ListenAndServe(addr, serveMux)
}

// ListenAndServe starts an HTTP server at the given address (localhost:80
// by default, as results from the underlying net/http implementation).
func ListenAndServe(addr string) error {
	return DefaultProclist.ListenAndServe(addr)
}
