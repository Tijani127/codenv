package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type Style string

const (
	Reset     Style = "\x1b[0m"
	Bold      Style = "\x1b[1m"
	Dim       Style = "\x1b[2m"
	Red       Style = "\x1b[31m"
	Green     Style = "\x1b[32m"
	Yellow    Style = "\x1b[33m"
	Blue      Style = "\x1b[34m"
	Magenta   Style = "\x1b[35m"
	Cyan      Style = "\x1b[36m"
	White     Style = "\x1b[37m"
	Gray      Style = "\x1b[90m"
	LightRed  Style = "\x1b[91m"
	LightGrn  Style = "\x1b[92m"
	LightYel  Style = "\x1b[93m"
	LightBlu  Style = "\x1b[94m"
	LightMag  Style = "\x1b[95m"
	LightCyan Style = "\x1b[96m"
)

var (
	mu       sync.Mutex
	quiet    bool
	colorize           = detectColor()
	out      io.Writer = os.Stdout
	errOut   io.Writer = os.Stderr
)

func SetQuiet(q bool) {
	mu.Lock()
	defer mu.Unlock()
	quiet = q
}

func Quiet() bool {
	mu.Lock()
	defer mu.Unlock()
	return quiet
}

func SetOutput(w io.Writer, e io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	out, errOut = w, e
}

func SetColorEnabled(v bool) {
	mu.Lock()
	defer mu.Unlock()
	colorize = v
}

func ColorEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return colorize
}

func Paint(s Style, text string) string {
	if !ColorEnabled() {
		return text
	}
	return string(s) + text + string(Reset)
}

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CODEENV_FORCE_COLOR") != "" {
		return true
	}
	if !isCharDevice(out) {
		return false
	}
	return enableVirtualTerminal()
}

func isCharDevice(f io.Writer) bool {
	file, ok := f.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func emit(w io.Writer, s string) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprint(w, s)
}

func Out(format string, args ...any) {
	emit(out, fmt.Sprintf(format, args...))
}

func Outln(format string, args ...any) {
	emit(out, fmt.Sprintf(format, args...)+"\n")
}

func Println(format string, args ...any) {
	emit(out, fmt.Sprintf(format, args...)+"\n")
}

func Printf(format string, args ...any) {
	emit(out, fmt.Sprintf(format, args...))
}

func Errln(format string, args ...any) {
	emit(errOut, fmt.Sprintf(format, args...)+"\n")
}

func Print(s string) {
	emit(out, s)
}

func Step(format string, args ...any) {
	if Quiet() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	emit(out, Paint(Yellow, "==>")+" "+msg+"\n")
}

func Info(format string, args ...any) {
	if Quiet() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	emit(out, "    "+Paint(Gray, msg)+"\n")
}

func Detail(format string, args ...any) {
	if Quiet() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	emit(out, "    "+Paint(Gray, "· "+msg)+"\n")
}

func Success(format string, args ...any) {
	if Quiet() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	emit(out, Paint(Green, "✓ ")+msg+"\n")
}

func Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(errOut, Paint(Yellow, "warning: ")+msg+"\n")
}

func Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(errOut, Paint(LightRed, "error: ")+msg+"\n")
}

func Hint(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(out, Paint(Cyan, "hint: ")+msg+"\n")
}

func Label(s Style, name, version string) string {
	if version == "" {
		return Paint(s, name)
	}
	return Paint(s, name) + " " + Paint(Gray, version)
}

func KeyValue(key string, value any) string {
	return Paint(Gray, key+":") + " " + fmt.Sprint(value)
}

func Rule() string {
	return Paint(Gray, strings.Repeat("─", 60))
}
