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
		attrs := make([]AttrDetail, 0, len(p.attrs))
		for name, value := range p.attrs {
			attrs = append(attrs, AttrDetail{
				Name:  name,
				Value: value,
			})
		}
		firstJEntry := p.journal.Front().Value.(*journalEntry)
		lastJEntry := p.journal.Back().Value.(*journalEntry)

		procs = append(procs, ProcDetail{
			Id:         id,
			Attrs:      attrs,
			ProcTm:     firstJEntry.ts,
			StatusTm:   lastJEntry.ts,
			Status:     lastJEntry.status,
			Cancelling: p.cancel.isPending,
		})
		p.mu.RUnlock()
	}

	return procs
}

func httpError(w http.ResponseWriter, httpCode int) {
	http.Error(w, http.StatusText(httpCode), httpCode)
}

func (pl *Proclist) handleProcsReq(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		httpError(w, http.StatusMethodNotAllowed)
		return
	}

	b, err := json.Marshal(ProcResponse{
		Procs:    pl.getProcs(),
		ServerTm: time.Now(),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderContentType, MediaJSON)
	w.Write(b)
}

func (pl *Proclist) getJournal(id string) []JournalDetail {
	pl.mu.RLock()
	p, present := pl.procs[id]
	pl.mu.RUnlock()

	if !present {
		return []JournalDetail{}
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	journal := make([]JournalDetail, 0, p.journal.Len())

	entry := p.journal.Front()
	for entry != nil {
		v := entry.Value.(*journalEntry)
		journal = append(journal, JournalDetail{
			Ts:     v.ts,
			Status: v.status,
		})
		entry = entry.Next()
	}

	return journal
}

func (pl *Proclist) handleJournalReq(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		httpError(w, http.StatusMethodNotAllowed)
		return
	}

	b, err := json.Marshal(JournalResponse{
		Journal:  pl.getJournal(id),
		ServerTm: time.Now(),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderContentType, MediaJSON)
	w.Write(b)
}

func (pl *Proclist) handleCancelReq(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "PUT" {
		httpError(w, http.StatusMethodNotAllowed)
		return
	}

	var message string
	var cancel CancelRequest
	if err := json.NewDecoder(r.Body).Decode(&cancel); err == nil {
		message = cancel.Message
	}
	pl.Kill(id, message)
	w.WriteHeader(http.StatusOK)
}

func (pl *Proclist) handleProcActionReq(w http.ResponseWriter, r *http.Request) {
	// We registered this handle for "/proc/"; splitting by the slash
	// yields the id as the third component
	pathItems := strings.Split(r.URL.Path, "/")
	id := pathItems[2]
	if len(pathItems) != 4 || len(id) == 0 {
		httpError(w, http.StatusNotFound)
		return
	}

	switch pathItems[3] {
	case "journal":
		pl.handleJournalReq(w, r, id)
	case "cancel":
		pl.handleCancelReq(w, r, id)
	default:
		httpError(w, http.StatusNotFound)
		return
	}
}

func (pl *Proclist) ListenAndServe(addr string) error {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/proc", pl.handleProcsReq)
	serveMux.HandleFunc("/proc/", pl.handleProcActionReq)
	return http.ListenAndServe(addr, serveMux)
}
