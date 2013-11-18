// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

/*
Package pm is a process manager with a TCP interface.

A processlist is useful for inspecting and managing what's running in a
program, such as an HTTP server or other server program. Processes within this
program are user-defined tasks, such as an HTTP request.

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
	proc2  init    0.0001  127.0.0.1:3527   GET     /users/5/friends

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

	2013/11/17 15:39:29 http: panic serving 127.0.0.1:55780: process killed: "houston, we have a problem"
	goroutine 6 [running]:
	net/http.func·007()
	    /usr/local/go/src/pkg/net/http/server.go:1022 +0xac
	github.com/VividCortex/pm.func·001()
	    /Users/baron/Documents/go/src/github.com/VividCortex/pm/pm.go:76 +0x4c

This is safe to do with HTTP servers based on net/http as long as the call to
Status() isn't made from a separate goroutine. The goroutine that net/http
starts to serve every incoming request has a deferred recover(), which will
catch and print out such panics without causing the entire server process to
die.

In addition to killing processes, you can control the refresh rate through your
netcat session with the "delay" command, which takes a single argument that is
parsed by time.ParseDuration().
*/
package pm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// Proclist represents a list of processes and the columns associated with them.
// Most people will not use Proclist or its methods directly, but will instead use
// the package-level functions that act on the default package-level variable.
type Proclist struct {
	m     sync.Mutex
	procs map[string]*Proc
	lens  map[string]int // The column headers and their lengths, for aligning
	cols  []string       // The following columns are reserved: id, status, time
}

// SetCols sets the columns expected to be present for every process.
// The id, status, and time columns are built-in.
func (p *Proclist) SetCols(cols ...string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.cols = append([]string{"id", "status", "time"}, cols...)
	for _, col := range p.cols {
		if p.lens[col] < len(col) {
			p.lens[col] = len(col)
		}
	}
}

// Start creates and starts a new process. After this, the process's columns
// do not change, with the exception of the status and time columns, which
// are special.
func (p *Proclist) Start(id string, cols map[string]interface{}) {
	p.m.Lock()
	defer p.m.Unlock()
	p.procs[id] = &Proc{
		profile: map[string]time.Duration{},
		cols:    cols,
		status:  "init",
		start:   time.Now(),
	}
	for name, val := range cols {
		l := len(fmt.Sprint(val))
		if l > p.lens[name] {
			p.lens[name] = l
		}
	}
	if len(id) > p.lens["id"] {
		p.lens["id"] = len(id)
	}
	p.lens["status"] = 6 // len("status")
}

// Done stops and removes a process from the list, and returns it.
func (p *Proclist) Done(id string) *Proc {
	p.m.Lock()
	defer p.m.Unlock()
	proc, present := p.procs[id]
	if present {
		delete(p.procs, id)
		proc.Status("ended", time.Now())
	}
	return proc
}

// Status changes a process's status. Use this to indicate what the process is
// doing. Status() doubles as a status update and a check to see if the
// process has been killed. The return value is a function that will kill the
// process via a runtime panic, if so instructed with Kill(). The correct way
// to use Status() is to execute the function it returns:
//
//    pm.Status("myproc", "authenticating")()
//
// If you don't include the trailing parens, then the process won't be
// killable at this point in its execution.
func (p *Proclist) Status(id, status string) func() {
	p.m.Lock()
	defer p.m.Unlock()
	proc, present := p.procs[id]
	if present {
		proc.Status(status, time.Now())
		if len(status) > p.lens["status"] {
			p.lens["status"] = len(status)
		}
		if proc.kill != "" {
			message := fmt.Sprintf("process killed: %q", proc.kill)
			return func() { panic(message) }
		}
		return func() {}
	}
	return func() { panic("no such process " + id) }
}

// Kill schedules the process for killing at the next Status().
func (p *Proclist) Kill(id, message string) error {
	p.m.Lock()
	defer p.m.Unlock()
	if proc, present := p.procs[id]; present {
		proc.kill = message
		return nil
	}
	return ProcessNotFound
}

// Contents returns an io.Reader containing the formatted processlist.
func (p *Proclist) Contents() io.Reader {
	var (
		b       bytes.Buffer
		cols    = []interface{}{"id", "status", "time"}
		formats = []string{
			fmt.Sprintf("%%-%ds", p.lens["id"]),
			fmt.Sprintf("%%-%ds", p.lens["status"]),
			"%10.4f",
		}
		format string
	)
	p.m.Lock()
	defer p.m.Unlock()

	// Build the header and format, with the id/status/time cols at the left
	for _, col := range p.cols {
		if col != "id" && col != "status" && col != "time" {
			cols = append(cols, col)
			formats = append(formats, fmt.Sprintf("%%-%dv", p.lens[fmt.Sprint(col)]))
		}
	}
	format = strings.Join(formats, "  ") + "\n"

	// Write the header, then the rows
	fmt.Fprintf(&b, strings.Replace(format, "%10.4f", "%10s", 1), cols...)
	for id, proc := range p.procs {
		var vars = []interface{}{id, proc.status, float64(time.Since(proc.start)) / float64(time.Second)}
		for n, val := range proc.cols {
			if n != "id" && n != "status" && n != "time" {
				vars = append(vars, val)
			}
		}
		fmt.Fprintf(&b, format, vars...)
	}
	return &b
}

// ListenAndServe() listens on a TCP socket. Connections to this socket get repeating
// printouts of the processlist at 5-second intervals. Connections can send commands
// as well. The command syntax is:
//
//    kill <id> <message>
//       Schedules the specified process to be killed, with the given message.
//    delay <duration>
//       Changes the refresh interval. The argument is parsed with time.ParseDuration().
func (p *Proclist) ListenAndServe(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}

		// Start a command-listener on this connection.
		go func() {
			defer c.Close()
			delay := time.Second * 3
			go func() {
				buf := make([]byte, 32*1024)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						fields := strings.SplitN(strings.TrimSpace(string(buf[:n])), " ", 3)
						switch fields[0] {
						case "kill":
							if len(fields) == 3 {
								killErr := p.Kill(fields[1], fields[2])
								if killErr != nil {
									c.Write([]byte(ProcessNotFound.Error() + "\n"))
								}
							} else {
								fmt.Fprint(c, "invalid command\n")
							}
						case "delay":
							if len(fields) == 2 {
								d, err := time.ParseDuration(fields[1])
								if err != nil {
									fmt.Fprintf(c, "invalid duration '%s'\n", fields[1])
								} else {
									delay = d
								}
							} else {
								fmt.Fprint(c, "invalid command\n")
							}
						}
					}
					if err != nil { // If we were logging we'd log if ! io.EOF, but...
						c.Close() // will cause the for-loop to close also
						return
					}
				}
			}()
			for {
				_, err := io.Copy(c, p.Contents())
				if err != nil {
					return // the conn will be closed by defer
				}
				time.Sleep(delay)
			}
		}()
	}
}

// Proc represents a process.
type Proc struct {
	status, kill   string
	start, updated time.Time
	profile        map[string]time.Duration
	cols           map[string]interface{}
}

// Status updates the process's status and keeps track of the time spent in each status.
func (p *Proc) Status(status string, t time.Time) {
	p.profile[p.status] = p.profile[p.status] + t.Sub(p.updated)
	p.status = status
	p.updated = t
}

var pl *Proclist // The default proclist

var (
	ProcessNotFound error = errors.New("no such process")
)

func init() {
	pl = &Proclist{
		procs: make(map[string]*Proc),
		lens:  make(map[string]int),
	}
}

// SetCols sets the columns expected to be present for every process.
// The id, status, and time columns are built-in.
func SetCols(cols ...string) {
	pl.SetCols(cols...)
}

// Start creates and starts a new process. After this, the process's columns
// do not change, with the exception of the status and time columns, which
// are special.
func Start(id string, cols map[string]interface{}) {
	pl.Start(id, cols)
}

// Done stops and removes a process from the list, and returns it.
func Done(id string) *Proc {
	return pl.Done(id)
}

// Status changes a process's status. Use this to indicate what the process is
// doing. Status() doubles as a status update and a check to see if the
// process has been killed. The return value is a function that will kill the
// process via a runtime panic, if so instructed with Kill(). The correct way
// to use Status() is to execute the function it returns:
//
//    pm.Status("myproc", "authenticating")()
//
// If you don't include the trailing parens, then the process won't be
// killable at this point in its execution.
func Status(id, status string) func() {
	return pl.Status(id, status)
}

// Kill schedules the process for killing at the next Status().
func Kill(id, message string) error {
	return pl.Kill(id, message)
}

// Contents returns an io.Reader containing the formatted processlist.
func Contents() io.Reader {
	return pl.Contents()
}

// ListenAndServe() listens on a TCP socket. Connections to this socket get repeating
// printouts of the processlist at 5-second intervals. Connections can send commands
// as well. The command syntax is:
//
//    kill <id> <message>
//       Schedules the specified process to be killed, with the given message.
//    delay <duration>
//       Changes the refresh interval. The argument is parsed with time.ParseDuration().
func ListenAndServe(addr string) error {
	return pl.ListenAndServe(addr)
}
