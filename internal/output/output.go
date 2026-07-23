package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
)

var (
	JSON     bool
	NoColor  bool
	Quiet    bool
	Verbose  bool
	Out      io.Writer = os.Stdout
	ErrOut   io.Writer = os.Stderr
	red      = color.New(color.FgRed, color.Bold)
	green    = color.New(color.FgGreen, color.Bold)
	yellow   = color.New(color.FgYellow)
	cyan     = color.New(color.FgCyan)
	dim      = color.New(color.Faint)
)

func initColors() {
	if NoColor {
		color.NoColor = true
	}
}

func Success(msg string) {
	initColors()
	if Quiet {
		return
	}
	fmt.Fprintf(Out, "%s %s\n", green.Sprint("✓"), msg)
}

func Info(msg string) {
	initColors()
	if Quiet {
		return
	}
	fmt.Fprintln(Out, msg)
}

func Warn(msg string) {
	initColors()
	fmt.Fprintf(ErrOut, "%s %s\n", yellow.Sprint("!"), msg)
}

// Fail prints a human error with optional fix hint to stderr.
func Fail(msg string, fix string) {
	initColors()
	fmt.Fprintf(ErrOut, "%s %s\n", red.Sprint("✗"), msg)
	if fix != "" {
		fmt.Fprintf(ErrOut, "  %s %s\n", dim.Sprint("Fix:"), fix)
	}
}

// FailJSON writes a structured error for --json mode.
func FailJSON(code, message, hint string) {
	enc := json.NewEncoder(ErrOut)
	_ = enc.Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
			"hint":    hint,
		},
	})
}

func PrintJSON(v any) error {
	enc := json.NewEncoder(Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func Table(headers []string, rows [][]string) {
	initColors()
	w := tabwriter.NewWriter(Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
}

func Cyan(s string) string {
	initColors()
	return cyan.Sprint(s)
}

func Green(s string) string {
	initColors()
	return green.Sprint(s)
}

func Red(s string) string {
	initColors()
	return red.Sprint(s)
}

func Dim(s string) string {
	initColors()
	return dim.Sprint(s)
}

func HealthDot(ok bool) string {
	initColors()
	if ok {
		return green.Sprint("●")
	}
	return red.Sprint("●")
}

func HealthDotBroken() string {
	initColors()
	return yellow.Sprint("●")
}
