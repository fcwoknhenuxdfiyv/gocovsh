// Package program wraps the initial interaction with the end user, before (if
// ever) the control is passed to the tea.Program.
package program

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/orlangure/gocovsh/internal/model"
	"github.com/waigani/diffparser"
)

const (
	defaultProfileFilename = "coverage.out"
	usageHeader            = `gocovsh: Go Coverage in your terminal

Usage: %s [options]

If provided, stdin is expected to be a list of files to be processed, for example:

	git diff --name-only | %s

Supported options:

`
)

// New return a new Program instance. Optional configuration is available using
// `With...` functions.
func New(opts ...Option) *Program {
	p := &Program{
		input:   os.Stdin,
		output:  os.Stdout,
		flagSet: flag.CommandLine,
		args:    os.Args[1:],
	}

	for _, opt := range opts {
		opt(p)
	}

	p.flagSet.BoolVar(&p.showVersion, "version", false, "show version")
	p.flagSet.BoolVar(&p.sortByCoverage, "sort-by-coverage", false, "sort files by coverage instead of alphabetically")
	p.flagSet.StringVar(
		&p.profileFilename, "profile", defaultProfileFilename,
		"File name of coverage profile generated by go test -coverprofile coverage.out",
	)

	p.flagSet.Usage = func() {
		fmt.Fprintf(p.output, usageHeader, p.flagSet.Name(), p.flagSet.Name())
		p.flagSet.PrintDefaults()
	}

	return p
}

// Program holds the program configuration.
type Program struct {
	version string
	commit  string
	date    string

	modVersion string
	modSum     string

	showVersion     bool
	profileFilename string
	sortByCoverage  bool

	flagSet *flag.FlagSet
	args    []string
	input   fs.File
	output  io.Writer
	logFile string

	requestedFiles []string
	diffLines      map[string][]int
}

// Run parses the command line arguments and runs the program.
func (p *Program) Run() error {
	if err := p.flagSet.Parse(p.args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if p.showVersion {
		out := fmt.Sprintf(
			"Version: %s\nCommit: %s\nDate: %s\n",
			p.version, p.commit, p.date,
		)
		if p.modVersion != "" {
			out += fmt.Sprintf(
				"Module Version: %s\nModule Checksum: %s\n",
				p.modVersion, p.modSum,
			)
		}

		_, err := fmt.Fprint(p.output, out)

		return err
	}

	if err := p.parseInput(); err != nil {
		return fmt.Errorf("failed to parse input: %w", err)
	}

	m := model.New(
		model.WithProfileFilename(p.profileFilename),
		model.WithRequestedFiles(p.requestedFiles),
		model.WithCoverageSorting(p.sortByCoverage),
		model.WithFilteredLines(p.diffLines),
	)

	if p.logFile != "" {
		f, err := tea.LogToFile(p.logFile, "gocovsh")
		if err != nil {
			return fmt.Errorf("failed to setup logger: %w", err)
		}

		defer func() { _ = f.Close() }()

		log.Println("logging to", p.logFile)
	} else {
		log.SetOutput(io.Discard)
	}

	if err := tea.NewProgram(m, tea.WithAltScreen()).Start(); err != nil {
		return fmt.Errorf("failed to start program: %w", err)
	}

	return nil
}

func (p *Program) parseInput() error {
	if p.isInputStreamAvailable() {
		bs, err := io.ReadAll(p.input)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		if diff, err := diffparser.Parse(string(bs)); err != nil {
			// fall back to file list mode - this is not an error
			p.requestedFiles = p.splitLines(string(bs))
		} else {
			p.diffLines = diff.Changed()

			for file := range p.diffLines {
				if !strings.HasSuffix(file, ".go") {
					delete(p.diffLines, file)
				}
			}

			for _, file := range diff.Files {
				p.requestedFiles = append(p.requestedFiles, file.NewName)
			}
		}
	}

	return nil
}

func (p *Program) isInputStreamAvailable() bool {
	fi, err := p.input.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeNamedPipe != 0
}

func (p *Program) splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
