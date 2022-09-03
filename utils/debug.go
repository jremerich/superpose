package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
)

type LogTransport struct {
	RT http.RoundTripper
}

func (t *LogTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var buf bytes.Buffer

	os.Stdout.Write([]byte("\n[request]\n"))
	if req.Body != nil {
		req.Body = ioutil.NopCloser(&readButCopy{req.Body, &buf})
	}
	req.Write(os.Stdout)
	if req.Body != nil {
		req.Body = ioutil.NopCloser(&buf)
	}
	os.Stdout.Write([]byte("\n[/request]\n"))

	res, err := t.RT.RoundTrip(req)

	fmt.Printf("[response]\n")
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	} else {
		body := res.Body
		res.Body = nil
		res.Write(os.Stdout)
		if body != nil {
			res.Body = ioutil.NopCloser(&echoAsRead{body})
		}
	}

	return res, err
}

type echoAsRead struct {
	src io.Reader
}

func (r *echoAsRead) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		os.Stdout.Write(p[:n])
	}
	if err == io.EOF {
		fmt.Printf("\n[/response]\n")
	}
	return n, err
}

type readButCopy struct {
	src io.Reader
	dst io.Writer
}

func (r *readButCopy) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.dst.Write(p[:n])
	}
	return n, err
}

func GetFunctionName() string {
	return GetFunctionNameSkip(1)
}

func GetFunctionNameSkip(skip int) string {
	counter, _, _, success := runtime.Caller(skip + 1)

	if !success {
		println("functionName: runtime.Caller: failed")
		os.Exit(1)
	}

	return runtime.FuncForPC(counter).Name()
}

func getFramesBacktrace() *runtime.Frames {
	// Ask runtime.Callers for up to 10 PCs, including runtime.Callers itself.
	pc := make([]uintptr, 1000)
	n := runtime.Callers(2, pc)
	if n == 0 {
		// No PCs available. This can happen if the first argument to
		// runtime.Callers is large.
		//
		// Return now to avoid processing the zero Frame that would
		// otherwise be returned by frames.Next below.
		return nil
	}

	pc = pc[:n] // pass only valid pcs to runtime.CallersFrames
	return runtime.CallersFrames(pc)
}

func PrintLog(msg, level string) {
	fmt.Printf("level: %v\n", level)
	color := GetLogColor(level == "ERROR")
	colorOff := "\033[0;m"
	log.Printf("%s%s%s", color, msg, colorOff)
}

func GetLogColor(isError bool) string {
	esc := "\033"

	background := ""
	if isError {
		// ⚠️ ×
		// background = esc + "[48;5;11m" + esc + "[38;5;1;1m[ERROR]" + esc + "[0;m"
		background = esc + "[38;5;1;1m[ERROR]" + esc + "[0;m "
		// background = "❌ "
		// background += esc + "[48;5;52m "
	}

	inotifyWatcherColor := esc + "[38;5;4m"
	googleDriveColor := esc + "[38;5;5m"
	googleDriveActivityWatcherColor := esc + "[38;5;6m"

	frames := getFramesBacktrace()
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "InotifyWatcher)") {
			return background + inotifyWatcherColor
		}

		if strings.Contains(frame.Function, "GoogleDrive)") {
			return background + googleDriveColor
		}

		if strings.Contains(frame.Function, "GoogleDriveActivity)") {
			return background + googleDriveActivityWatcherColor
		}

		if !more {
			break
		}
	}
	return ""
}

func GetStacktrace() {
	frames := getFramesBacktrace()
	// Loop to get frames.
	// A fixed number of PCs can expand to an indefinite number of Frames.
	for {
		frame, more := frames.Next()

		// Process this frame.
		//
		// To keep this example's output stable
		// even if there are changes in the testing package,
		// stop unwinding when we leave package runtime.
		if strings.Contains(frame.Function, "runtime.goexit") {
			break
		}
		fmt.Printf("- more:%v | %s [%s:%d]\n", more, frame.Function, frame.File, frame.Line)

		// Check whether there are more frames to process after this one.
		if !more {
			break
		}
	}
}
