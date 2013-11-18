pm
==

![build status](https://circleci.com/gh/VividCortex/pm.png?circle-token=d37ec652ea117165cd1b342400a801438f575209)

pm is a process manager with a TCP interface. We use it at
[VividCortex](https://vividcortex.com/) to inspect and manage API server
programs.

A processlist is useful for inspecting and managing what's running in a
program, such as an HTTP server or other server program. Processes within this
program are user-defined tasks, such as HTTP requests.

Documentation
=============

Please read the generated [package documentation](http://godoc.org/github.com/VividCortex/pm).

Getting Started
===============

To use pm, you should make one call to SetCols() when you start your server
running. This specifies the columns you want to see in printouts of the
processlist. To continue the HTTP example:

	pm.SetCols("host", "method", "uri")

Follow this with a call to ListenAndServe():

	go func() {
		for {
			log.Println(pm.ListenAndServe(":8081"))
			time.Sleep(time.Second)
		}
	}()

Then, when a process/task/request begins, you will call Start() with the
process's ID and values for its columns, and defer a call to Done():

	pm.Start(requestID, map[string]interface{}{
		"host":   req.RemoteAddr,
		"method": req.Method,
		"uri":    req.RequestURI,
	})
	defer pm.Done(requestID)

This is all you'll need to view a table of running requests and how long they've
been executing. You can now connect to port 8081 and view the processlist with a
tool such as netcat:

	$ nc localhost 8081
	id     status    time  host             method  uri
	proc1  init    0.0000  127.0.0.1:18364  GET     /users/
	proc2  init    0.0001  127.0.0.1:55780  GET     /users/5/friends

If you add calls to pm.Status() throughout your code, you'll be able to see the
status change, which can be helpful for troubleshooting:

	pm.Status(requestId, "authenticating")

Status changes also serve as an opportunity to optionally make processes
killable. If you execute the return value of calls to Status(), then you can
introduce a runtime panic into that goroutine as desired, with a specified
message:

	pm.Status(requestId, "authenticating")()

The kill command and message can be specified through your netcat connection:

	kill proc2 houston, we have a problem

This will cause a panic with a stacktrace such as the following:

	2013/11/17 15:39:29 http: panic serving 127.0.0.1:55780:
		process killed: "houston, we have a problem"
	goroutine 6 [running]:
	net/http.func·007()
	    /usr/local/go/src/pkg/net/http/server.go:1022 +0xac
	github.com/VividCortex/pm.func·001()
	    github.com/VividCortex/pm/pm.go:76 +0x4c

This is safe to do with HTTP servers based on net/http as long as the call to
Status() isn't made from a separate goroutine. The goroutine that net/http
starts to serve every incoming request has a deferred recover(), which will
catch and print out such panics without causing the entire server process to
die.

In addition to killing processes, you can control the refresh rate through your
netcat session with the "delay" command, which takes a single argument that is
parsed by time.ParseDuration().

Contributing
============

Pull requests (with tests, ideally) are welcome! We're especially interested
in ways to improve performance or correctness, or to make the code more
idiomatic Go.

License
=======

Copyright (c) 2013 VividCortex, licensed under the MIT license.
Please see the LICENSE file for details.

Cat Picture
===========

![mechanic cat](http://heidicullinan.files.wordpress.com/2012/03/funny-cat-pictures-lolcats-mechanic-cat-is-on-the-job.jpg)