// Package pm is a process manager with a TCP interface.
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

type Proclist struct {
	m     sync.Mutex
	procs map[string]*Proc
	lens  map[string]int // The column headers and their lengths, for aligning
	cols  []string       // The following columns are reserved: id, status, time
}

func (p *Proclist) SetCols(cols ...string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.cols = append(p.cols, cols...)
	for _, col := range p.cols {
		if p.lens[col] < len(col) {
			p.lens[col] = len(col)
		}
	}
}

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

func (p *Proclist) Kill(id, message string) error {
	p.m.Lock()
	defer p.m.Unlock()
	if proc, present := p.procs[id]; present {
		proc.kill = message
		return nil
	}
	return ProcessNotFound
}

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

		// Start a command-listener on this connection. It will look for
		// commands and execute them. (kill <id> <msg>, delay <duration>)
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

type Proc struct {
	status, kill   string
	start, updated time.Time
	profile        map[string]time.Duration
	cols           map[string]interface{}
}

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

func SetCols(cols ...string) {
	pl.SetCols(cols...)
}

func Start(id string, cols map[string]interface{}) {
	pl.Start(id, cols)
}

func Done(id string) *Proc {
	return pl.Done(id)
}

func Status(id, status string) func() {
	return pl.Status(id, status)
}

func Kill(id, message string) error {
	return pl.Kill(id, message)
}

func Contents() io.Reader {
	return pl.Contents()
}

func ListenAndServe(addr string) error {
	return pl.ListenAndServe(addr)
}
