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

type display struct {
	axis    axis
	offset  axis
	name    string
	primary bool
}

func (d display) Within(x, y int) bool {
	return x > d.offset.x &&
		x < d.offset.x+d.axis.x &&
		y > d.offset.y &&
		y < d.offset.y+d.axis.y
}

func (d display) IsBottom(y int) bool {
	return y < d.offset.y+d.axis.y && y > d.offset.y+d.axis.y-20
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

var dLock sync.RWMutex
var displaysFound []display

func watchDisplays() {
	var err error
	displaysFound, err = fetchDisplays()
	if err != nil {
		log.Println(err)
	}

	for range time.Tick(time.Second * 5) {
		dLock.Lock()
		displaysFound, err = fetchDisplays()
		if err != nil {
			log.Println(err)
		}
		dLock.Unlock()
	}
}

func getDisplays(lastUpdate time.Time) ([]display, bool) {
	dLock.RLock()
	defer dLock.RUnlock()

	if !displaysUpdateTime.After(lastUpdate) {
		return nil, false
	}

	if len(displaysFound) == 0 {
		// this is rare and should never happen
		// may be a one off and can be fixed at the next
		// poll.
		// let's simply log
		log.Println("Error: no displays are found")
	}

	// create a copy to not worry about
	// race conditions outside this
	displaysCopy := make([]display, len(displaysFound))
	copy(displaysCopy, displaysFound)

	return displaysCopy, true
}

// keep track of previous displays state
var (
	displaysConf       string
	displaysUpdateTime time.Time
)

func fetchDisplays() ([]display, error) {
	cmd := exec.Command("xrandr")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if string(out) == displaysConf {
		// ignore
		return displaysFound, nil
	}
	displaysConf = string(out)
	displaysUpdateTime = time.Now()

	var displays []display
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
		var m display
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

		displays = append(displays, m)
	}

	return displays, nil
}

const dconfPlank = "/net/launchpad/plank/docks/dock1/monitor"

func movePlankTo(d display) error {
	value := fmt.Sprintf(`'%s'`, d.name)
	if d.primary {
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
	fmt.Fprint(&buf, "attempting to move plank to "+d.name)
	if d.primary {
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
	var displays []display
	var lastRequestTime time.Time
	go watchDisplays()
	var pos axis
	for pos = range pollMouse() {
		if ds, ok := getDisplays(lastRequestTime); ok {
			lastRequestTime = time.Now()
			displays = ds
		}
		for _, d := range displays {
			if d.Within(pos.x, pos.y) && d.IsBottom(pos.y) {
				err := movePlankTo(d)
				if err != nil {
					log.Println(err)
				}
				break
			}
		}
	}
}
