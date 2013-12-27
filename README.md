pm
==

![build status](https://circleci.com/gh/VividCortex/pm.png?circle-token=d37ec652ea117165cd1b342400a801438f575209)

pm is a process manager with an HTTP interface. We use it at
[VividCortex](https://vividcortex.com/) to inspect and manage API server
programs. It replaces an internal-only project that was similar.

pm is in beta and will change rapidly. Please see the issues list for what's
planned, or to suggest changes.

A Processlist is useful for inspecting and managing what's running in a
program, such as an HTTP server or other server program. Processes within this
program are user-defined tasks, such as HTTP requests.

Documentation
=============

Please read the generated package documentation for both
[server](http://godoc.org/github.com/VividCortex/pm) and
[client](http://godoc.org/github.com/VividCortex/pm/client).

Getting Started
===============

To this package, processes or tasks are user-defined tasks within a running
program. This model is particularly suitable to server programs, with
independent client generated requests for action. The library would keep track
of all such tasks and make the information available through HTTP, thus
providing valuable insight into the running application. The client can also ask
for a particular task to be canceled.

Using pm starts by calling ListenAndServe() in a separate routine, like so:

```go
go pm.ListenAndServe(":8081")
```

Note that pm's ListenAndServe() will not return by itself. Nevertheless, it will
if the underlying net/http's ListenAndServe() does. So it's probably a good idea
to wrap that call with some error checking and retrying for production code.

Tasks to be tracked must be declared with Start(); that's when the identifier
gets linked to them. An optional set of (arbitrary) attributes may be provided,
that will get attached to the running task and be reported to the HTTP client
tool as well. (Think of host, method and URI for a web server, for instance.)
This could go like this:

```go
pm.Start(requestID, map[string]interface{}{
	"host":   req.RemoteAddr,
	"method": req.Method,
	"uri":    req.RequestURI,
})
defer pm.Done(requestID)
```

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

```go
pm.SetOptions(ProclistOptions{
	StopCancelPanic: true
})
```

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

```go
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
```

Contributing
============

Pull requests (with tests, ideally) are welcome! We're especially interested
in things such as ways to improve performance or correctness, or to make the code more
idiomatic Go.

License
=======

Copyright (c) 2013 VividCortex, licensed under the MIT license.
Please see the LICENSE file for details.

Cat Picture
===========

![mechanic cat](http://heidicullinan.files.wordpress.com/2012/03/funny-cat-pictures-lolcats-mechanic-cat-is-on-the-job.jpg)
