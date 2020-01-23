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
	"syscall"
	"time"
)

var version = "0.1.1-untracked-dev"
var displaysFound []display

func main() {
	if ds, err := fetchDisplays(); err == nil {
		displaysFound = ds
	} else {
		log.Fatal("unable to gather screen information")
	}
	validateDeps()
	_, err := startPlank()
	if err != nil {
		log.Fatal(err.Error())
	}
	eventLoop()
}

var (
	versionFlag bool
	interval    = 1
)

func init() {

	flag.BoolVar(&versionFlag, "v", versionFlag, "show version")
	flag.IntVar(&interval, "interval", interval, "mouse poll interval in secs")

	flag.Parse()

	if versionFlag {
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
	// if the cursor is this low on the screen user is going to use plank
	// we start the moving procedure
	yOffset := 100
	return y < d.offset.y+d.axis.y && y > d.offset.y+d.axis.y-yOffset
}

func startPlank() (*os.Process, error) {
	// we set up the process we want to start
	// no error handling needed here because validate deps checks for plank command
	plank, _ := exec.LookPath("plank")
	var cred = &syscall.Credential{Gid: uint32(os.Getuid()), Uid: uint32(os.Getgid()), Groups: []uint32{}, NoSetGroups: true}
	var sysproc = &syscall.SysProcAttr{Credential: cred, Noctty: true}
	var attr = os.ProcAttr{
		Dir: ".",
		Env: os.Environ(),
		Files: []*os.File{
			os.Stdin,
			os.Stdout,
			os.Stderr,
		},
		Sys: sysproc,
	}
	proc, err := os.StartProcess(plank, []string{}, &attr)
	if err != nil {
		return nil, err
	}
	err = proc.Release()
	if err != nil {
		return nil, err
	}
	return proc, nil
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

func fetchDisplays() ([]display, error) {
	cmd := exec.Command("xrandr")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

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

	err = exec.Command("dconf", "write", dconfPlank, value).
		Run()

	if err == nil {
		fmt.Printf("attempting to move plank to %s\n", d.name)
		_ = exec.Command("killall", "plank").Run()
		_, err := startPlank()
		return err
	}
	return err

}

var requiredCommands = []string{
	"xrandr",
	"xdotool",
	"dconf",
	"plank",
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
	var pos axis
	for pos = range pollMouse() {
		for _, d := range displaysFound {
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
