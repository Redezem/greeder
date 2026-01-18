package main

import (
	"fmt"
	"io"
	"os"
)

var (
	exitFunc     = os.Exit
	refreshFeeds = func(app *App) error { return app.RefreshFeeds() }
	runTUI       = RunTUI
)

func main() {
	if err := runMain(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		exitFunc(1)
	}
}

func runMain(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintln(stderr, "config error:", err)
		return err
	}
	app, err := NewApp(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "init error:", err)
		return err
	}

	if len(args) >= 2 && args[0] == "--import" {
		if err := app.ImportOPML(args[1]); err != nil {
			fmt.Fprintln(stderr, "import error:", err)
			return err
		}
		fmt.Fprintf(stdout, "Imported feeds from %s\n", args[1])
		return nil
	}
	if len(args) >= 1 && args[0] == "--refresh" {
		if err := refreshFeeds(app); err != nil {
			fmt.Fprintln(stderr, "refresh error:", err)
			return err
		}
		fmt.Fprintf(stdout, "Refreshed %d feeds\n", len(app.feeds))
		return nil
	}

	if !isTerminalReader(stdin) || !isTerminalWriter(stdout) {
		if err := Run(app, stdin, stdout); err != nil {
			fmt.Fprintln(stderr, "run error:", err)
			return err
		}
		return nil
	}

	if err := runTUI(app); err != nil {
		fmt.Fprintln(stderr, "run error:", err)
		return err
	}
	return nil
}

func isTerminalReader(stream io.Reader) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func isTerminalWriter(stream io.Writer) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
