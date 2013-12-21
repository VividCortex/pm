// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

package pm

import (
	"container/list"
	"sync"
	"time"
)

type Proclist struct {
	mu    sync.RWMutex
	procs map[string]*proc
	opts  ProclistOpts
}

type ProclistOpts struct {
	StopCancelPanic bool
}

type proc struct {
	mu      sync.RWMutex
	id      string
	attrs   map[string]interface{}
	journal list.List
	cancel  struct {
		isPending bool
		message   string
	}
}

type journalEntry struct {
	ts     time.Time
	status string
}

func (pl *Proclist) Options() ProclistOpts {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.opts
}

func (pl *Proclist) SetOptions(opts ProclistOpts) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.opts = opts
}

func (pl *Proclist) Start(id string, attrs map[string]interface{}) {
	p := &proc{
		id:    id,
		attrs: attrs,
	}
	p.addJournalEntry(time.Now(), "init")

	pl.mu.Lock()
	if pl.procs == nil {
		pl.procs = make(map[string]*proc)
	}
	pl.procs[id] = p
	pl.mu.Unlock()
}

type CancelErr struct {
	message string
}

func (e CancelErr) Error() string {
	return e.message
}

func (p *proc) doCancel() {
	message := "killed"
	if len(p.cancel.message) > 0 {
		message += ": " + p.cancel.message
	}

	panic(CancelErr{message: message})
}

// addJournalEntry pushes a new entry to the processes' journal, assuming the
// lock is already held.
func (p *proc) addJournalEntry(ts time.Time, status string) {
	p.journal.PushBack(&journalEntry{
		ts:     ts,
		status: status,
	})
}

func (pl *Proclist) Status(id, status string) {
	ts := time.Now()
	pl.mu.RLock()
	p, present := pl.procs[id]
	pl.mu.RUnlock()

	if present {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.addJournalEntry(ts, status)

		if p.cancel.isPending {
			p.doCancel()
		}
	}
}

func (pl *Proclist) CheckCancel(id string) {
	pl.mu.RLock()
	p, present := pl.procs[id]
	pl.mu.RUnlock()

	if present {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.cancel.isPending {
			p.doCancel()
		}
	}
}

func (pl *Proclist) Kill(id, message string) {
	ts := time.Now()
	pl.mu.RLock()
	p, present := pl.procs[id]
	pl.mu.RUnlock()

	if present {
		p.mu.Lock()
		defer p.mu.Unlock()

		if !p.cancel.isPending {
			p.cancel.isPending = true
			p.cancel.message = message

			var jentry string
			if len(message) > 0 {
				jentry = "[cancel request: " + message + "]"
			} else {
				jentry = "[cancel request]"
			}
			p.addJournalEntry(ts, jentry)
		}
	}
}

// done marks the end of a process, registering it depending on the outcome.
// Parameter e is supposed to be the result of recover(), so that we know
// whether processing ended normally, was canceled or aborted due to any other
// panic.
func (pl *Proclist) done(id string, e interface{}) {
	pl.mu.Lock()
	p, present := pl.procs[id]
	if present {
		delete(pl.procs, id)
	}
	stopPanic := pl.opts.StopCancelPanic
	pl.mu.Unlock()

	if present {
		ts := time.Now()
		p.mu.Lock()
		defer p.mu.Unlock()

		if e != nil {
			if c, canceled := e.(CancelErr); canceled {
				p.addJournalEntry(ts, c.message)
				if !stopPanic {
					panic(e)
				}
			} else {
				p.addJournalEntry(ts, "aborted")
				panic(e)
			}
		} else {
			p.addJournalEntry(ts, "ended")
		}
	} else if e != nil {
		_, canceled := e.(CancelErr)
		if !canceled || !stopPanic {
			panic(e)
		}
	}
}

func (pl *Proclist) Done(id string) {
	pl.done(id, recover())
}

var DefaultProclist Proclist

func Options() ProclistOpts {
	return DefaultProclist.Options()
}

func SetOptions(opts ProclistOpts) {
	DefaultProclist.SetOptions(opts)
}

func Start(id string, attrs map[string]interface{}) {
	DefaultProclist.Start(id, attrs)
}

func Status(id, status string) {
	DefaultProclist.Status(id, status)
}

func CheckCancel(id string) {
	DefaultProclist.CheckCancel(id)
}

func Kill(id, message string) {
	DefaultProclist.Kill(id, message)
}

func Done(id string) {
	DefaultProclist.done(id, recover())
}

func ListenAndServe(addr string) error {
	return DefaultProclist.ListenAndServe(addr)
}
