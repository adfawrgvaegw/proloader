package utils

import (
	"errors"
	"os"

	"github.com/leaanthony/go-ansi-parser"
	"github.com/natefinch/lumberjack"
	"github.com/pterm/pterm"
)

type Logs struct {
	logs *lumberjack.Logger
}

var (
	DefaultMultiPrinter = pterm.DefaultMultiPrinter
	Log                 = pterm.Logger{
		Formatter: pterm.LogFormatterColorful,
		Level:      pterm.LogLevelInfo,
		MaxWidth:   114514,
		ShowTime:   true,
		TimeFormat: "2006-01-02 15:04:05",
		KeyStyles: map[string]pterm.Style{
			"error":  *pterm.NewStyle(pterm.FgRed, pterm.Bold),
			"err":    *pterm.NewStyle(pterm.FgRed, pterm.Bold),
			"caller": *pterm.NewStyle(pterm.FgGray, pterm.Bold),
		},
	}.WithLevel(pterm.LogLevelTrace)
)

func (l *Logs) Write(p []byte) (int, error) {
	return len(p), errors.Join(func() error { _, err := os.Stdout.Write(p); return err }(), func() error {
		_, err := l.logs.Write([]byte(func() string { s, _ := ansi.Cleanse(string(p)); return s }()))
		return err
	}())
}

func (l *Logs) Close() error {
	return errors.Join(l.logs.Close(), os.Stdout.Close())
}

func init() {
	Log = Log.WithKeyStyles(map[string]pterm.Style{})
}
