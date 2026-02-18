package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

// Printer handles structured, colored CLI output
type Printer struct {
	out   io.Writer
	color bool
}

// NewPrinter creates a new Printer. Colors are enabled when writing to a terminal.
func NewPrinter() *Printer {
	p := &Printer{out: os.Stdout}
	if f, ok := p.out.(*os.File); ok {
		p.color = term.IsTerminal(int(f.Fd()))
	}
	return p
}

// NewPrinterWithWriter creates a Printer with a custom writer (colors disabled).
func NewPrinterWithWriter(w io.Writer) *Printer {
	return &Printer{out: w, color: false}
}

// --- Color helpers ---

func (p *Printer) colorize(color, text string) string {
	if !p.color {
		return text
	}
	return color + text + colorReset
}

func (p *Printer) green(text string) string  { return p.colorize(colorGreen, text) }
func (p *Printer) red(text string) string    { return p.colorize(colorRed, text) }
func (p *Printer) yellow(text string) string { return p.colorize(colorYellow, text) }
func (p *Printer) cyan(text string) string   { return p.colorize(colorCyan, text) }
func (p *Printer) dim(text string) string    { return p.colorize(colorDim, text) }
func (p *Printer) bold(text string) string   { return p.colorize(colorBold, text) }

// --- Output methods ---

// Println prints a line
func (p *Printer) Println(a ...interface{}) {
	fmt.Fprintln(p.out, a...)
}

// Printf prints formatted text
func (p *Printer) Printf(format string, a ...interface{}) {
	fmt.Fprintf(p.out, format, a...)
}

// Blank prints an empty line
func (p *Printer) Blank() {
	fmt.Fprintln(p.out)
}

// Header prints a section header with decorative line
func (p *Printer) Header(title string) {
	p.Blank()
	p.Println(p.bold(title))
	p.Println(p.dim(strings.Repeat("─", len(title)+2)))
	p.Blank()
}

// PhaseHeader prints a phase header with line decorations
func (p *Printer) PhaseHeader(title string) {
	line := strings.Repeat("━", 3)
	p.Blank()
	p.Printf("%s %s %s\n", p.cyan(line), p.bold(title), p.cyan(line))
	p.Blank()
}

// Success prints a success message with green checkmark
func (p *Printer) Success(text string) {
	p.Printf("  %s %s\n", p.green("✓"), text)
}

// Failure prints a failure message with red cross
func (p *Printer) Failure(text string) {
	p.Printf("  %s %s\n", p.red("✗"), text)
}

// FailureWithOutput prints a failure with the captured step output
func (p *Printer) FailureWithOutput(text string, output string) {
	p.Printf("  %s %s\n", p.red("✗"), text)
	if output != "" {
		p.Blank()
		for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
			p.Printf("    %s\n", p.dim(line))
		}
		p.Blank()
	}
}

// Skip prints a skipped item with yellow dash
func (p *Printer) Skip(text string) {
	p.Printf("  %s %s\n", p.yellow("–"), p.dim(text))
}

// Item prints an indented item
func (p *Printer) Item(text string) {
	p.Printf("  %s\n", text)
}

// Detail prints a detail line under an item
func (p *Printer) Detail(label, value string) {
	p.Printf("    %s %s\n", p.dim(label), value)
}

// Progress prints a progress indicator like [2/5]
func (p *Printer) Progress(current, total int, text string) {
	p.Printf("  %s %s\n", p.dim(fmt.Sprintf("[%d/%d]", current, total)), text)
}

// BearHeader prints the Bear branding header
func (p *Printer) BearHeader(command string) {
	p.Blank()
	p.Println(p.bold(fmt.Sprintf("Bear %s", command)))
	p.Println(p.dim(strings.Repeat("─", len(command)+5)))
	p.Blank()
}

// Summary prints a summary line at the bottom
func (p *Printer) Summary(parts ...string) {
	p.Blank()
	p.Println(p.dim(strings.Repeat("─", 40)))
	p.Printf("  %s\n", strings.Join(parts, "  "))
}

// SummaryValidated returns a formatted validated count
func (p *Printer) SummaryValidated(n int) string {
	return p.green(fmt.Sprintf("✓ %d validated", n))
}

// SummaryDeploy returns a formatted deploy count
func (p *Printer) SummaryDeploy(n int) string {
	return p.cyan(fmt.Sprintf("~ %d to deploy", n))
}

// SummaryDeployed returns a formatted deployed count
func (p *Printer) SummaryDeployed(n int) string {
	return p.green(fmt.Sprintf("✓ %d deployed", n))
}

// SummarySkipped returns a formatted skipped count
func (p *Printer) SummarySkipped(n int) string {
	return p.dim(fmt.Sprintf("– %d skipped", n))
}

// SummaryFailed returns a formatted failed count
func (p *Printer) SummaryFailed(n int) string {
	return p.red(fmt.Sprintf("✗ %d failed", n))
}

// ErrorBox prints captured error output in an indented, dimmed block
func (p *Printer) ErrorBox(output string) {
	if output == "" {
		return
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	for _, line := range lines {
		p.Printf("      %s\n", p.dim(line))
	}
}

// Hint prints a hint/instruction at the bottom
func (p *Printer) Hint(text string) {
	p.Blank()
	p.Printf("  %s\n", p.dim(text))
}

// Warning prints a warning message
func (p *Printer) Warning(text string) {
	p.Printf("  %s %s\n", p.yellow("⚠"), text)
}
