package main

import (
	"github.com/VividCortex/multitick"
	"github.com/VividCortex/pm"
	"github.com/VividCortex/pm/client"

	"flag"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

var (
	Endpoints       = "" // e.g. "api1:9085,api2:9085,api1:9086,api2:9086"
	KeepHist        = true
	RefreshInterval = time.Second
	clients         = map[string]*client.Client{}

	ScreenHeight = 40
	ScreenWidth  = 160

	Display = make(chan []Line)
	Trickle = make(chan Line)

	// These might need to be protected by mutexes
	Columns   = []string{"Host", "Id", "Time", "Status"}
	LengthFor = map[string]int{
		"Host":   len("longhostname:1234"),
		"Id":     len("ae14f5cac98273e8"),
		"Time":   len("300.123s"),
		"Status": len("this one is a long enough status!"),
	}

	paused = false
)

type Line struct {
	Host, Id, Status   string
	ProcAge, StatusAge time.Duration
	Cols               map[string]string
}

func init() {
	// disable input buffering
	exec.Command("stty", "-f", "/dev/tty", "cbreak").Run()
	checkTermSize()
}

func main() {
	flag.StringVar(&Endpoints, "endpoints", Endpoints, "Comma-separated host:port list of APIs to poll")
	flag.BoolVar(&KeepHist, "keep-hist", KeepHist, "Keep output history on refreshes")
	flag.DurationVar(&RefreshInterval, "refresh", RefreshInterval, "Time interval between refreshes")
	flag.Parse()

	ticker := multitick.NewTicker(RefreshInterval, RefreshInterval)

	endpoints := strings.Split(Endpoints, ",")
	for _, e := range endpoints {
		if !strings.HasPrefix(e, "http://") && !strings.HasPrefix(e, "https://") {
			e = "http://" + e
		}
		clients[e] = client.NewClient(e)

		go poll(e, ticker.Subscribe())
	}

	go top(ticker.Subscribe())
	go inputLoop()

	clearScreen(true)
	for lines := range Display {
		if paused {
			continue
		}

		checkTermSize()
		clearScreen(KeepHist)

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
			if printed == ScreenHeight-1 {
				break
			}
		}
	}
}

// poll one of the endpoints for its /procs/ data.
func poll(hostPort string, ticker <-chan time.Time) {
	for _ = range ticker {
		msg, err := clients[hostPort].Processes()
		if err == nil {
			msgToLines(hostPort, msg)
		}
	}
}

func msgToLines(hostPort string, msg *pm.ProcResponse) {
	for _, p := range msg.Procs {
		l := Line{
			Host:      strings.Replace(hostPort, "http://", "", -1),
			Id:        p.Id,
			Status:    p.Status,
			ProcAge:   msg.ServerTime.Sub(p.ProcTime),
			StatusAge: msg.ServerTime.Sub(p.StatusTime),
			Cols:      map[string]string{},
		}
		for name, value := range p.Attrs {
			colLen, ok := LengthFor[name]
			if !ok {
				Columns = append(Columns, name)
			}
			if len(name) > colLen {
				LengthFor[name] = len(name)
			}
			l.Cols[name] = value.(string)
		}
		Trickle <- l
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
