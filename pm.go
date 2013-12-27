// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

/*
Package pm is a process manager with an HTTP monitoring/control interface.

To this package, processes or tasks are user-defined tasks within a running
program. This model is particularly suitable to server programs, with
independent client generated requests for action. The library would keep track
of all such tasks and make the information available through HTTP, thus
providing valuable insight into the running application. The client can also ask
for a particular task to be canceled.

Using pm starts by calling ListenAndServe() in a separate routine, like so:

	go pm.ListenAndServe(":8081")

Note that pm's ListenAndServe() will not return by itself. Nevertheless, it will
if the underlying net/http's ListenAndServe() does. So it's probably a good idea
to wrap that call with some error checking and retrying for production code.

Tasks to be tracked must be declared with Start(); that's when the identifier
gets linked to them. An optional set of (arbitrary) attributes may be provided,
that will get attached to the running task and be reported to the HTTP client
tool as well. (Think of host, method and URI for a web server, for instance.)
This could go like this:

	pm.Start(requestID, map[string]interface{}{
		"host":   req.RemoteAddr,
		"method": req.Method,
		"uri":    req.RequestURI,
	})
	defer pm.Done(requestID)

Note the deferred pm.Done() call. That's essential to mark the task as finished
and release resources, and has to be called no matter if the task succeeded or
not; that's why it's a good idea to use Go's defer mechanism. In fact, the
protection from cancel-related panics (see StopCancelPanic below) does NOT work
if Done() is called in-line (i.e., not in a defer) due to properties of
recover().

An HTTP client issuing a GET call to /proc would receive a JSON response with a
snapshot of all currently running tasks. Each one will include the time when it
was started (as per the server clock) as well as the complete set of attributes
provided to Start(). Note that to avoid issues with clock skews among servers,
the current time for the server is returned as well.

The reply to the /proc GET also includes a status. When tasks start they are set
to "init", but they may change their status using pm's Status() function. Each
call will record the change and the HTTP client will receive the last status
available, together with the time it was set. Furthermore, the client may GET
/proc/<id>/journal instead (where <id> is the task identifier) and get a
complete journal for the given task, including all status changes with their
time information. However, note that task information is completely recycled as
soon as they are Done(). Hence, client applications should be prepared to
receive an empty reply even if they've just seen the task in a /proc GET result.

Given the lack of a statement in Go to kill a routine, cancellations are
implemented as panics. A call to Kill() will mark the task with the given
identifier as cancellation-pending. Nevertheless, the task will never notice
until it reaches another Status() call, which is by definition a cancellation
point. Calls to status either return having set the new status, or panic with an
error of type CancelErr. Needless to say, the application should handle this
gracefully or will otherwise crash. Programs serving multiple requests will
probably be already protected, with a recover() call at the level where the
routine was started. But if that's not the case, or if you simply want the panic
to be handled transparently, you may use this call:

	pm.SetOptions(ProclistOptions{
		StopCancelPanic: true
	})

When the StopCancelPanic option is set (which is NOT the default) Done() will
recover a panic due to a Kill() operation. In such a case, the routine running
that code will jump to the next statement after the invocation to the function
that called Start(). (Read it again.) In other words, stack unfolding stops at
the level where Done() is deferred. Notice, though, that this behavior is
specifically tailored to panics raising from pending cancellation requests.
Should any other panic arise for any reason, it will continue past Done() as
usual. So will panics due to Kill() requests if StopCancelPanic is not set.
(Although you'd probably do it as part of your server initialization, it IS
legal to change StopCancelPanic while your pm-enabled program is running.
Changes take effect immediately.)

HTTP clients can learn about pending cancellation requests. Furthermore, if a
client request happens to be handled between the task is done/canceled and
resource recycling (a VERY tiny time window), then the result would include one
of these as the status: "killed", "aborted" or "ended", if it was respectively
canceled, died out of another panic (not pm-related) or finished successfully.
The "killed" status may optionally add a user-defined message, provided through
the HTTP /proc/<id>/cancel PUT method.

For the cancellation feature to be useful, applications should collaborate. Go
lacks a mechanism to cancel arbitrary routines (it even lacks identifiers for
them), so programs willing to provide the feature must be willing to help. It's
a good practice to add cancellation points every once in a while, particularly
when lengthy operations are run. However, applications are not expected to
change status that frequently. This package provides the function CheckCancel()
for that. It works as a cancellation point by definition, without messing with
the task status, nor leaving a trace in the journal.

Finally, please note that cancellation requests yield panics in the same routine
that called Start() with that given identifier. However, it's not unusual for
servers to spawn additional Go routines to handle the same request. The
application is responsible of cleaning up, if there are additional resources
that should be recycled. The proper way to do this is by catching CancelErr type
panics, cleaning-up and then re-panic, i.e.:

	func handleRequest(requestId string) {
		pm.Start(requestId, map[string]interface{}{})
		defer pm.Done(requestId)
		defer func() {
			if e := recover(); e != nil {
				if c, canceled := e.(CancelErr); canceled {
					// do your cleanup here
				}
				panic(e) // re-panic with same error (cancel or not)
			}
		}()
		// your application code goes here
	}
*/
package pm

import (
	"container/list"
	"sync"
	"time"
)

// Type Proclist is the main type for the process-list. You may have as many as
// you wish, each with it's own HTTP server, but that's probably useful only in
// a handful of cases. The typical use of this package is through the default
// Proclist object (DefaultProclist) and package-level functions. The zero value
// for the type is a Proclist ready to be used.
type Proclist struct {
	mu    sync.RWMutex
	procs map[string]*proc
	opts  ProclistOpts
}

// Type ProclistOpts provides all options to be set for a Proclist.
type ProclistOpts struct {
	StopCancelPanic bool // Stop cancel-related panics at Done()
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

// Options returns the options set for this Proclist.
func (pl *Proclist) Options() ProclistOpts {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.opts
}

// SetOptions sets the options to be used for this Proclist. Some options like
// StopCancelPanic take effect immediately, but don't rely on that. You should
// set all your options prior to start using the Proclist.
func (pl *Proclist) SetOptions(opts ProclistOpts) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.opts = opts
}

// Start marks the beginning of a task at this Proclist. All attributes are
// recorded but not handled internally by the package. They will be mapped to
// JSON and provided to HTTP clients for this task. It's the caller's
// responsibility to provide different identifiers for separate tasks. An id can
// only be reused after the task previously using it is over.
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

// Type CancelErr is the type used for cancellation-induced panics.
type CancelErr struct {
	message string
}

// Error returns the error message for a CancelErr.
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

// Status changes the status for a task in a Proclist, adding an item to the
// task's journal. Note that Status() is a cancellation point, thus the routine
// calling it is subject to a panic due to a pending Kill().
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

// CheckCancel introduces a cancellation point just like Status() does, but
// without changing the task status, nor adding an entry to the journal.
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

// Kill sets a cancellation request to the task with the given identifier, that
// will be effective as soon as the routine running that task hits a
// cancellation point. The (optional) message will be included in the CancelErr
// object used for panic.
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

// Done marks the end of a task, writing the journal depending on the outcome
// (i.e., aborted, killed or finished successfully). This function releases
// resources associated with the process, thus making the id available for use
// by another task. It also stops panics raising from cancellation requests, but
// only when the StopCancelPanic option is set AND Done is called with a defer
// statement.
func (pl *Proclist) Done(id string) {
	pl.done(id, recover())
}

// This is the default Proclist set for the package. Package-level operations
// use this list.
var DefaultProclist Proclist

// Options returns the options set for the default Proclist.
func Options() ProclistOpts {
	return DefaultProclist.Options()
}

// SetOptions sets the options to be used for the default Proclist. Some options
// like StopCancelPanic take effect immediately, but don't rely on that. You
// should set all your options prior to start using the Proclist.
func SetOptions(opts ProclistOpts) {
	DefaultProclist.SetOptions(opts)
}

// Start marks the beginning of a task at the default Proclist. All attributes
// are recorded but not handled internally by the package. They will be mapped
// to JSON and provided to HTTP clients for this task. It's the caller's
// responsibility to provide different identifiers for separate tasks. An id can
// only be reused after the task previously using it is over.
func Start(id string, attrs map[string]interface{}) {
	DefaultProclist.Start(id, attrs)
}

// Status changes the status for a task in the default Proclist, adding an item
// to the task's journal. Note that Status() is a cancellation point, thus the
// routine calling it is subject to a panic due to a pending Kill().
func Status(id, status string) {
	DefaultProclist.Status(id, status)
}

// CheckCancel introduces a cancellation point just like Status() does, but
// without changing the task status, nor adding an entry to the journal.
func CheckCancel(id string) {
	DefaultProclist.CheckCancel(id)
}

// Kill sets a cancellation request to the task with the given identifier, that
// will be effective as soon as the routine running that task hits a
// cancellation point. The (optional) message will be included in the CancelErr
// object used for panic.
func Kill(id, message string) {
	DefaultProclist.Kill(id, message)
}

// Done marks the end of a task, writing the journal depending on the outcome
// (i.e., aborted, killed or finished successfully). This function releases
// resources associated with the process, thus making the id available for use
// by another task. It also stops panics raising from cancellation requests, but
// only when the StopCancelPanic option is set AND Done is called with a defer
// statement.
func Done(id string) {
	DefaultProclist.done(id, recover())
}
