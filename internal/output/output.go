// Package output renders command results as tables, plain text, CSV or JSON.
//
// Commands build a Document — a typed JSON payload plus tabular rows with
// semantic tones — and a Printer renders it in the requested Format. Colors
// are applied through a colorprofile.Writer, so they are stripped
// automatically when output is not a terminal, NO_COLOR is set, or the user
// passes --no-color.
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"
)

// SchemaVersion is the version of the JSON output schema. It changes only when
// fields are removed or change meaning; see docs/json-schema.md.
const SchemaVersion = 1

// Format is an output format.
type Format string

// Supported formats.
const (
	Table Format = "table"
	Plain Format = "plain"
	CSV   Format = "csv"
	JSON  Format = "json"
)

// Formats lists the supported formats.
var Formats = []Format{Table, Plain, CSV, JSON}

// ParseFormat validates a format name.
func ParseFormat(s string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimSpace(s)))
	if !slices.Contains(Formats, f) {
		return "", fmt.Errorf("unknown output format %q (want one of: table, plain, csv, json)", s)
	}
	return f, nil
}

// Tone gives a cell semantic meaning; the table format maps it to a color.
type Tone int

// Tones.
const (
	ToneNone Tone = iota
	ToneGood
	ToneBad
	ToneWarn
	ToneMuted
)

// Cell is one table cell.
type Cell struct {
	Text string
	Tone Tone
}

// C is shorthand for an untoned cell.
func C(text string) Cell { return Cell{Text: text} }

// T is shorthand for a toned cell.
func T(text string, tone Tone) Cell { return Cell{Text: text, Tone: tone} }

// Document is a renderable command result.
type Document struct {
	// Kind identifies the payload in JSON output, e.g. "list" or "check".
	Kind string
	// Data is the JSON payload. It should be one of the *JSON types in this package.
	Data any
	// Headers and Rows are used by the table, plain and CSV formats.
	Headers []string
	Rows    [][]Cell
	// Text, if set, replaces the table in table and plain formats.
	Text string
	// Empty is printed in table and plain formats when there are no rows.
	Empty string
}

// Options configure a Printer.
type Options struct {
	// NoColor disables all styling.
	NoColor bool
	// Environ is used to detect color support (NO_COLOR, TERM, …). Usually os.Environ().
	Environ []string
}

// Printer renders Documents to a writer.
type Printer struct {
	w      io.Writer
	format Format
}

// NewPrinter returns a Printer writing to w in format f.
func NewPrinter(w io.Writer, f Format, opts Options) *Printer {
	profile := colorprofile.Detect(w, opts.Environ)
	if opts.NoColor || f != Table || noColorEnv(opts.Environ) {
		profile = colorprofile.NoTTY
	}
	return &Printer{w: &colorprofile.Writer{Forward: w, Profile: profile}, format: f}
}

// noColorEnv reports whether NO_COLOR is set to a non-empty value. Per
// https://no-color.org it takes precedence over CLICOLOR_FORCE.
func noColorEnv(environ []string) bool {
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "NO_COLOR="); ok && v != "" {
			return true
		}
	}
	return false
}

// Format returns the printer's format.
func (p *Printer) Format() Format { return p.format }

// Print renders d.
func (p *Printer) Print(d Document) error {
	switch p.format {
	case JSON:
		return p.printJSON(d)
	case CSV:
		return p.printCSV(d)
	case Plain:
		return p.printPlain(d)
	default:
		return p.printTable(d)
	}
}

type envelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	Data          any    `json:"data"`
}

func (p *Printer) printJSON(d Document) error {
	enc := json.NewEncoder(p.w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(envelope{SchemaVersion: SchemaVersion, Kind: d.Kind, Data: d.Data}); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func (p *Printer) printCSV(d Document) error {
	w := csv.NewWriter(p.w)
	records := make([][]string, 0, len(d.Rows)+1)
	records = append(records, d.Headers)
	for _, row := range d.Rows {
		records = append(records, texts(row))
	}
	if err := w.WriteAll(records); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}

func (p *Printer) printPlain(d Document) error {
	if text, ok := textOrEmpty(d); ok {
		return p.writeLine(text)
	}
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(d.Headers, "\t"))
	for _, row := range d.Rows {
		fmt.Fprintln(tw, strings.Join(texts(row), "\t"))
	}
	_ = tw.Flush() // writes to a strings.Builder cannot fail
	return p.writeLine(trimLines(buf.String()))
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).PaddingRight(2)
	cellStyle   = lipgloss.NewStyle().PaddingRight(2)
	toneStyles  = map[Tone]lipgloss.Style{
		ToneNone:  cellStyle,
		ToneGood:  cellStyle.Foreground(lipgloss.Green),
		ToneBad:   cellStyle.Foreground(lipgloss.Red).Bold(true),
		ToneWarn:  cellStyle.Foreground(lipgloss.Yellow),
		ToneMuted: cellStyle.Foreground(lipgloss.BrightBlack),
	}
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
)

func (p *Printer) printTable(d Document) error {
	if text, ok := textOrEmpty(d); ok {
		return p.writeLine(text)
	}
	rows := make([][]string, len(d.Rows))
	for i, r := range d.Rows {
		rows[i] = texts(r)
	}
	t := table.New().
		Headers(d.Headers...).
		Rows(rows...).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(true).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := headerStyle
			if row != table.HeaderRow {
				st = toneStyles[d.Rows[row][col].Tone]
			}
			if col == len(d.Headers)-1 {
				st = st.UnsetPaddingRight() // no trailing whitespace
			}
			return st
		})
	return p.writeLine(trimLines(t.Render()))
}

// trimLines removes trailing spaces left by column padding.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// textOrEmpty returns the text to print instead of a table, if any.
func textOrEmpty(d Document) (string, bool) {
	switch {
	case d.Text != "":
		return d.Text, true
	case len(d.Rows) == 0 && d.Empty != "":
		return d.Empty, true
	default:
		return "", false
	}
}

func (p *Printer) writeLine(s string) error {
	if _, err := io.WriteString(p.w, strings.TrimRight(s, "\n")+"\n"); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func texts(row []Cell) []string {
	out := make([]string, len(row))
	for i, c := range row {
		out[i] = c.Text
	}
	return out
}
