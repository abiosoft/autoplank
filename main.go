package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var version = "0.1-untracked-dev"

func main() {
	validateDeps()
	eventLoop()
}

var (
	versonFlag bool
	interval   = 2
)

func init() {

	flag.BoolVar(&versonFlag, "v", versonFlag, "show version")
	flag.IntVar(&interval, "interval", interval, "mouse poll interval in secs")

	flag.Parse()

	if versonFlag {
		fmt.Println("autoplank v" + version)
		os.Exit(0)
	}
}

type axis struct {
	x, y int
}

type monitor struct {
	axis    axis
	offset  axis
	name    string
	primary bool
}

func (m monitor) Within(x, y int) bool {
	return x > m.offset.x &&
		x < m.offset.x+m.axis.x &&
		y > m.offset.y &&
		y < m.offset.y+m.axis.y
}

func (m monitor) IsBottom(y int) bool {
	return y < m.offset.y+m.axis.y && y > m.offset.y+m.axis.y-20
}

func pollMouse() <-chan axis {
	aChan := make(chan axis)

	go func() {
		for range time.Tick(time.Second * time.Duration(interval)) {
			pos, err := getMouseLocation()
			if err != nil {
				log.Println(err)
				continue
			}
			aChan <- pos
		}
	}()

	return aChan
}

func getMouseLocation() (a axis, err error) {
	cmd := exec.Command("xdotool", "getmouselocation")
	out, err := cmd.Output()
	if err != nil {
		return a, err
	}
	cols := strings.Fields(string(out))
	if len(cols) < 4 {
		return a, errors.New("unexpected output from xdotool")
	}
	a.x, err = strconv.Atoi(cols[0][2:])
	if err != nil {
		return a, err
	}
	a.y, err = strconv.Atoi(cols[1][2:])
	if err != nil {
		return a, err
	}

	return a, nil
}

var mLock sync.RWMutex
var monitors []monitor

func watchMonitors() {
	var err error
	monitors, err = fetchMonitors()
	if err != nil {
		log.Println(err)
	}

	for range time.Tick(time.Second * 5) {
		mLock.Lock()
		monitors, err = fetchMonitors()
		if err != nil {
			log.Println(err)
		}
		mLock.Unlock()
	}
}

func getMonitors(lastUpdate time.Time) ([]monitor, bool) {
	mLock.RLock()
	defer mLock.RUnlock()

	if !monitorUpdate.After(lastUpdate) {
		return nil, false
	}

	if len(monitors) == 0 {
		// this is rare and should never happen
		// may be a one off and can be fixed at the next
		// poll.
		// let's simply log
		log.Println("Error: no monitors are found")
	}

	// create a copy to not worry about
	// race conditions outside this
	copy := make([]monitor, len(monitors))
	for i := range copy {
		copy[i] = monitors[i]
	}

	return copy, true
}

// keep track of previous monitor state
var (
	monitorState  string
	monitorUpdate time.Time
)

func fetchMonitors() ([]monitor, error) {
	cmd := exec.Command("xrandr")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if string(out) == monitorState {
		// ignore
		return monitors, nil
	}
	monitorState = string(out)
	monitorUpdate = time.Now()

	var monitors []monitor
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, " connected") {
			continue
		}
		cols := strings.Fields(line)
		if len(cols) < 3 {
			return nil, errors.New("unexpected response from xrandr")
		}
		var m monitor
		m.name = cols[0]
		xy := cols[2]
		if xy == "primary" {
			m.primary = true
			xy = cols[3]
		}
		vals := strings.FieldsFunc(xy, func(c rune) bool {
			switch c {
			case 'x', '+':
				return true
			}
			return false
		})
		m.axis.x, _ = strconv.Atoi(vals[0])
		m.axis.y, _ = strconv.Atoi(vals[1])
		m.offset.x, _ = strconv.Atoi(vals[2])
		m.offset.y, _ = strconv.Atoi(vals[3])

		monitors = append(monitors, m)
	}

	return monitors, nil
}

const dconfPlank = "/net/launchpad/plank/docks/dock1/monitor"

func movePlankTo(m monitor) error {
	value := fmt.Sprintf(`'%s'`, m.name)
	if m.primary {
		value = `''`
	}

	cmd := exec.Command("dconf", "read", dconfPlank)
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	// no need to set if its same value
	if value == strings.TrimSpace(string(out)) {
		return nil
	}

	var buf bytes.Buffer
	fmt.Fprint(&buf, "attempting to move plank to "+m.name)
	if m.primary {
		fmt.Fprintf(&buf, " - primary")
	}

	log.Println(buf.String())

	return exec.Command("dconf", "write", dconfPlank, value).
		Run()
}

var requiredCommands = []string{
	"xrandr",
	"xdotool",
	"dconf",
}

func validateDeps() {
	errCount := 0
	for _, c := range requiredCommands {
		_, err := exec.LookPath(c)
		if err != nil {
			fmt.Fprintln(os.Stderr, c+" not found in PATH")
			errCount++
		}
	}
	if errCount > 0 {
		os.Exit(1)
	}
}

func eventLoop() {
	var monitors []monitor
	var lastRequest time.Time
	go watchMonitors()
	var pos axis
	for pos = range pollMouse() {
		if ms, ok := getMonitors(lastRequest); ok {
			lastRequest = time.Now()
			monitors = ms
		}
		for _, m := range monitors {
			if m.Within(pos.x, pos.y) && m.IsBottom(pos.y) {
				err := movePlankTo(m)
				if err != nil {
					log.Println(err)
				}
				break
			}
		}
	}

}
