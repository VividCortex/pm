pm
==

![build status](https://circleci.com/gh/VividCortex/pm.png?circle-token=f30450460e330fd0e9253c899c6a379e085989e7)

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

Package pm is a process manager with an HTTP monitoring/control interface.

Processes or tasks are user-defined routines within a running Go program. Think
of the routines handling client requests in a web server, for instance. This
package is designed to keep track of them, making information available through
an HTTP interface. Client tools connecting to the later can thus monitor active
tasks, having access to the full status history with timing data. Also,
application-specific attributes may be attached to tasks (method/URI for the web
server case, for example), that will be integrated with status/timing
information.


Using pm starts by opening a server port to handle requests for task information
through HTTP. That goes like this (although you probably want to add error
checking/handling code):

```go
go pm.ListenAndServe(":8081")
```

Processes to be tracked must call `Start()` with a process identifier and,
optionally, a set of attributes. Even though the id is arbitrary, it's up to the
application to choose one not in use by any other running task. A deferred call
to `Done()` with the same id should follow:

```go
pm.Start(requestID, nil, map[string]interface{}{
	"host":   req.RemoteAddr,
	"method": req.Method,
	"uri":    req.RequestURI,
})
defer pm.Done(requestID)
```

Finally, each task can change its status as often as required with a `Status()`
call. Status strings are completely arbitrary and never inspected by the
package. Now you're all set to try something like this:

```
curl http://localhost:8081/procs/
curl http://localhost:8081/procs/<id>/history
```

where `<id>` stands for an actual process id in your application. You'll get
JSON responses including, respectively, the set of processes currently running
and the full history for your chosen id.

Tasks can also be cancelled from the HTTP interface. In order to do that, you
should call the DELETE method on `/procs/<id>`. Given the lack of support in Go
to cancel a running routine, cancellation requests are implemented in this
package as panics. Please refer to the full package documentation to learn how
to properly deal with this. If you're **not** interested in this feature, you
can disable cancellation completely by running the following *before* you
`Start()` any task:

```go
pm.SetOptions(ProclistOptions{
	ForbidCancel: true
})
```

See package `pm/client` for an HTTP client implementation you can readily use
from Go applications.

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
