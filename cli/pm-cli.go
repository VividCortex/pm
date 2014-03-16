// Program cli is a commandline client for the pm processlist manager.
package main

// Copyright (c) 2014 VividCortex, Inc. All rights reserved.
// Please see the LICENSE file for applicable license terms.

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/VividCortex/multitick"
)

var (
	RefreshInterval = 3 * time.Second
	ScreenHeight    = 40
	ScreenWidth     = 160
	Endpoints       = "" // e.g. "api1:9085,api2:9085,api1:9086,api2:9086"
	Display         = make(chan []Line)
	Trickle         = make(chan Line)

	// These might need to be protected by mutexes
	Columns   = []string{"Host", "Id", "Time", "Status"}
	LengthFor = map[string]int{
		"Host":   len("longhostname:1234"),
		"Id":     len("ae14f5cac98273e8"),
		"Time":   len("300.123s"),
		"Status": len("this one is a long enough status!"),
	}
)

// Message is the response from the pm server, retrieved from /procs/.
type Message struct {
	Procs      []Proc
	ServerTime time.Time
}

// Proc is a single process within the Message.
type Proc struct {
	Id                   string
	Status               string
	Attrs                []Attr
	ProcTime, StatusTime time.Time
}

// Attr is a process's attribute.
type Attr struct {
	Name, Value string
}

// Line is one line of output on the terminal.
type Line struct {
	Host, Id, Status   string
	ProcAge, StatusAge time.Duration
	Cols               map[string]string
}

func main() {
	flag.StringVar(&Endpoints, "endpoints", Endpoints, "Comma-separated host:port list of APIs to poll")
	flag.DurationVar(&RefreshInterval, "interval", RefreshInterval, "Delay between refreshes")
	flag.IntVar(&ScreenHeight, "screen-height", ScreenHeight, "Height of terminal, in lines of text")
	flag.IntVar(&ScreenWidth, "screen-width", ScreenWidth, "Width of terminal, in columns of text")
	flag.Parse()

	// Set global HTTP read timeout (just for the headers of the request)
	http.DefaultTransport.(*http.Transport).ResponseHeaderTimeout = time.Second

	ticker := multitick.NewTicker(RefreshInterval, time.Second)
	endpoints := strings.Split(Endpoints, ",")

	for _, e := range endpoints {
		if !strings.HasPrefix(e, "http://") && !strings.HasPrefix(e, "https://") {
			e = "http://" + e
		}
		go poll(e, ticker.Subscribe())
	}

	go top(ticker.Subscribe())

	for lines := range Display {
		fmt.Print("\033[2J\033[;H") // clear the screen

		// Compute and print column headers
		lineFormat := ""
		for _, c := range Columns {
			l := LengthFor[c]
			colFormat := fmt.Sprintf(" %%-%ds", l)
			fmt.Printf(colFormat, c)
			lineFormat += colFormat
		}
		fmt.Println()
		printed := 1

		// Print as many lines as we have room for
		for _, l := range lines {
			printed++
			args := []interface{}{l.Host, l.Id, fmt.Sprintf("%.4g", l.ProcAge.Seconds()), l.Status}
			for _, c := range Columns[4:] {
				args = append(args, l.Cols[c])
			}
			output := fmt.Sprintf(lineFormat, args...)
			if len(output) > ScreenWidth {
				output = output[:ScreenWidth]
			}
			fmt.Println(output)
			if printed == ScreenHeight {
				break
			}
		}
	}
}

// poll one of the endpoints for its /procs/ data.
func poll(hostPort string, ticker <-chan time.Time) {
	for _ = range ticker {
		res, err := http.Get(hostPort + "/procs/")
		if err != nil {
			log.Println(hostPort, err)
			continue
		}
		if res.StatusCode == 200 {
			msg := Message{}
			dec := json.NewDecoder(res.Body)
			err := dec.Decode(&msg)
			if err != nil {
				log.Println(hostPort, err)
			} else {
				for _, p := range msg.Procs {
					l := Line{
						Host:      strings.Replace(hostPort, "http://", "", -1),
						Id:        p.Id,
						Status:    p.Status,
						ProcAge:   msg.ServerTime.Sub(p.ProcTime),
						StatusAge: msg.ServerTime.Sub(p.StatusTime),
						Cols:      map[string]string{},
					}
					for _, a := range p.Attrs {
						colLen, ok := LengthFor[a.Name]
						if !ok {
							Columns = append(Columns, a.Name)
						}
						if len(a.Name) > colLen {
							LengthFor[a.Name] = len(a.Name)
						}
						l.Cols[a.Name] = a.Value
					}
					Trickle <- l
				}
			}
		} else {
			log.Println(hostPort, res.Status)
		}
	}
}

// aggregate, sort, and batch up the data coming from the pm APIs.
func top(ticker <-chan time.Time) {
	var Lines []Line
	for {
		select {
		case l := <-Trickle:
			Lines = append(Lines, l)
		case <-ticker:
			sort.Sort(ByAge(Lines))
			Display <- Lines
			Lines = Lines[0:0]
		}
	}
}

// ByAge implements sort.Interface for []line based on
// the ProcAge field.
type ByAge []Line

func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].ProcAge > a[j].ProcAge } // Reversed!
