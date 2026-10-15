package picker

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/nyactl/approvekit/sanitize"
)

// ErrCanceled is returned when the person leaves without choosing.
var ErrCanceled = errors.New("nothing chosen")

// ErrNoTerminal is returned when there is no terminal to show the picker on.
var ErrNoTerminal = errors.New("this needs an interactive terminal")

// Action is a key that chooses the line under the cursor for something
// other than Enter does, such as "ctrl-o" to open it.
type Action struct {
	Key  string // "ctrl-a" to "ctrl-z", except those the picker uses
	Name string // shown in the help line, e.g. "open in browser"
}

// Options configure a choice.
type Options struct {
	Title string
	Items []string
	// Enter names what Enter does, for the help line; default "choose".
	Enter   string
	Actions []Action
	// Cursor is the item to start on, Query the text to start with, and
	// Status a line shown below the list.
	Cursor int
	Query  string
	Status string
}

// Result is a choice.
type Result struct {
	Index  int    // into Options.Items
	Action string // the Action's key, or "" for Enter
	Query  string // the query at the time, to start the next choice with
}

// Model is the picker's state, apart from the terminal.
type Model struct {
	opts    Options
	query   []rune
	matches []int
	cur     int // index into matches
}

// NewModel starts a choice.
func NewModel(opts Options) *Model {
	m := &Model{opts: opts, query: []rune(opts.Query)}
	m.refilter()
	for k, i := range m.matches {
		if i == opts.Cursor {
			m.cur = k
		}
	}
	return m
}

func (m *Model) refilter() {
	m.matches = Filter(string(m.query), m.opts.Items)
	m.cur = 0
}

// Query returns the current query.
func (m *Model) Query() string { return string(m.query) }

// Matches returns the indexes of the matching items, best first.
func (m *Model) Matches() []int { return m.matches }

// Current returns the item under the cursor, or -1 when nothing matches.
func (m *Model) Current() int {
	if len(m.matches) == 0 {
		return -1
	}
	return m.matches[m.cur]
}

// keys the picker uses itself; the rest of ctrl-a to ctrl-z can be actions.
var own = map[string]bool{"ctrl-c": true, "ctrl-n": true, "ctrl-p": true, "ctrl-j": true, "ctrl-k": true, "ctrl-u": true, "ctrl-w": true, "ctrl-h": true, "ctrl-m": true, "ctrl-i": true}

// Key handles one key. It reports whether the choice is over, with the
// result, or an error for a canceled choice.
func (m *Model) Key(k string, page int) (done bool, r Result, err error) {
	page = max(page, 1)
	switch k {
	case "up", "ctrl-p", "ctrl-k":
		m.cur = max(m.cur-1, 0)
	case "down", "ctrl-n", "ctrl-j":
		m.cur = min(m.cur+1, max(len(m.matches)-1, 0))
	case "pgup":
		m.cur = max(m.cur-page, 0)
	case "pgdn":
		m.cur = min(m.cur+page, max(len(m.matches)-1, 0))
	case "home":
		m.cur = 0
	case "end":
		m.cur = max(len(m.matches)-1, 0)
	case "backspace":
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.refilter()
		}
	case "ctrl-u":
		m.query = nil
		m.refilter()
	case "ctrl-w":
		q := strings.TrimRight(string(m.query), " ")
		if i := strings.LastIndex(q, " "); i >= 0 {
			m.query = []rune(q[:i+1])
		} else {
			m.query = nil
		}
		m.refilter()
	case "esc", "ctrl-c":
		return true, Result{Index: -1, Query: m.Query()}, ErrCanceled
	case "enter":
		if i := m.Current(); i >= 0 {
			return true, Result{Index: i, Query: m.Query()}, nil
		}
	default:
		for _, a := range m.opts.Actions {
			if a.Key == k && !own[k] {
				if i := m.Current(); i >= 0 {
					return true, Result{Index: i, Action: a.Key, Query: m.Query()}, nil
				}
			}
		}
		if r, size := utf8.DecodeRuneInString(k); size == len(k) && r >= ' ' && r != utf8.RuneError {
			m.query = append(m.query, r)
			m.refilter()
		}
	}
	return false, Result{}, nil
}

func (m *Model) help() string {
	enter := m.opts.Enter
	if enter == "" {
		enter = "choose"
	}
	parts := []string{"type to filter", "↑/↓ move", "Enter " + enter}
	for _, a := range m.opts.Actions {
		parts = append(parts, keyName(a.Key)+" "+a.Name)
	}
	return strings.Join(append(parts, "Esc quit"), " · ")
}

func keyName(k string) string {
	if rest, ok := strings.CutPrefix(k, "ctrl-"); ok {
		return "Ctrl-" + strings.ToUpper(rest)
	}
	return k
}

// Render draws the picker for a terminal of width columns and rows lines.
func (m *Model) Render(width, rows int) string {
	var b strings.Builder
	b.WriteString("\x1b[?25l\x1b[2J\x1b[H")
	b.WriteString(clip(sanitize.String(m.opts.Title), width) + "\r\n")
	b.WriteString(clip(m.help(), width) + "\r\n")
	count := fmt.Sprintf("  %d/%d", len(m.matches), len(m.opts.Items))
	b.WriteString(clip("> "+sanitize.String(string(m.query))+count, width) + "\r\n")
	list := max(rows-4, 1)
	if m.opts.Status != "" {
		list = max(list-2, 1)
	}
	start := max(0, min(m.cur-list/2, len(m.matches)-list))
	for k := start; k < len(m.matches) && k < start+list; k++ {
		line := clip("  "+sanitize.String(m.opts.Items[m.matches[k]]), width)
		if k == m.cur {
			line = "\x1b[7m" + clip("> "+sanitize.String(m.opts.Items[m.matches[k]]), width) + "\x1b[0m"
		}
		b.WriteString(line + "\r\n")
	}
	if m.opts.Status != "" {
		b.WriteString("\r\n" + clip(sanitize.String(m.opts.Status), width) + "\r\n")
	}
	return b.String()
}

// Run shows the picker on the controlling terminal.
func Run(opts Options) (Result, error) {
	if len(opts.Items) == 0 {
		return Result{Index: -1}, errors.New("nothing to choose from")
	}
	f, width, height, restore, err := rawTerminal()
	if err != nil {
		return Result{Index: -1}, err
	}
	defer restore()
	m := NewModel(opts)
	r := bufio.NewReader(f)
	for {
		_, _ = io.WriteString(f, m.Render(width, height-1))
		k, err := ReadKey(r)
		if err != nil {
			return Result{Index: -1}, err
		}
		if done, res, err := m.Key(k, max(height-6, 1)); done {
			return res, err
		}
	}
}

// ReadKey reads one key press from a terminal in raw mode: a printable
// rune as itself, or a name such as "enter", "up" or "ctrl-o".
func ReadKey(r *bufio.Reader) (string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	switch {
	case b == '\r' || b == '\n':
		return "enter", nil
	case b == 0x7f || b == 0x08:
		return "backspace", nil
	case b == '\t':
		return "tab", nil
	case b == 0x1b:
		return readEscape(r)
	case b < 0x20:
		return "ctrl-" + string(rune('a'+b-1)), nil
	case b < 0x80:
		return string(rune(b)), nil
	}
	// The rest of a UTF-8 sequence.
	buf := []byte{b}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		next, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		buf = append(buf, next)
	}
	return string(buf), nil
}

func readEscape(r *bufio.Reader) (string, error) {
	if r.Buffered() == 0 {
		return "esc", nil
	}
	next, err := r.ReadByte()
	if err != nil || (next != '[' && next != 'O') {
		return "esc", nil
	}
	code, err := r.ReadByte()
	if err != nil {
		return "esc", nil
	}
	switch code {
	case 'A':
		return "up", nil
	case 'B':
		return "down", nil
	case 'C':
		return "right", nil
	case 'D':
		return "left", nil
	case 'H':
		return "home", nil
	case 'F':
		return "end", nil
	case '5', '6', '1', '4':
		// Page Up, Page Down, Home and End in their other forms end with ~.
		if t, err := r.ReadByte(); err == nil && t == '~' {
			return map[byte]string{'5': "pgup", '6': "pgdn", '1': "home", '4': "end"}[code], nil
		}
	}
	return "", nil
}

// rawTerminal opens the controlling terminal in raw mode on its alternate
// screen, as fzf does; restore puts it back, with the screen as it was.
func rawTerminal() (f *os.File, width, height int, restore func(), err error) {
	f, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, 0, 0, nil, fmt.Errorf("%w (%w)", ErrNoTerminal, err)
	}
	state, err := term.MakeRaw(int(f.Fd()))
	if err != nil {
		_ = f.Close()
		return nil, 0, 0, nil, fmt.Errorf("%w (%w)", ErrNoTerminal, err)
	}
	width, height, err = term.GetSize(int(f.Fd()))
	if err != nil || width < 20 || height < 6 {
		width, height = 80, 24
	}
	_, _ = io.WriteString(f, "\x1b[?1049h")
	return f, width, height, func() {
		_, _ = io.WriteString(f, "\x1b[2J\x1b[H\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(f.Fd()), state)
		_ = f.Close()
	}, nil
}

// View shows text full screen, scrollable, until the person goes back.
func View(title, text string) error {
	f, width, height, restore, err := rawTerminal()
	if err != nil {
		return err
	}
	defer restore()
	lines := Wrap(sanitize.String(text), width)
	v := &Viewer{n: len(lines), rows: max(height-3, 1)}
	r := bufio.NewReader(f)
	for {
		_, _ = io.WriteString(f, v.Render(title, lines, width))
		k, err := ReadKey(r)
		if err != nil {
			return err
		}
		if v.Key(k) {
			return nil
		}
	}
}

// Viewer scrolls text of n lines in a window of rows lines.
type Viewer struct {
	n, rows, top int
}

// NewViewer starts a view of n lines in rows lines.
func NewViewer(n, rows int) *Viewer { return &Viewer{n: n, rows: max(rows, 1)} }

const viewHelp = "↑/↓ PgUp/PgDn Space scroll · Enter/q/Esc/← back"

// Key handles one key and reports whether to go back.
func (v *Viewer) Key(k string) bool {
	last := max(v.n-v.rows, 0)
	switch k {
	case "up", "k":
		v.top--
	case "down", "j":
		v.top++
	case "pgup", "b":
		v.top -= v.rows
	case "pgdn", " ":
		v.top += v.rows
	case "home", "g":
		v.top = 0
	case "end", "G":
		v.top = last
	case "enter", "q", "esc", "left", "ctrl-c":
		return true
	}
	v.top = min(max(v.top, 0), last)
	return false
}

// Top returns the first line shown.
func (v *Viewer) Top() int { return v.top }

// Render draws the view.
func (v *Viewer) Render(title string, lines []string, width int) string {
	var b strings.Builder
	b.WriteString("\x1b[?25l\x1b[2J\x1b[H")
	b.WriteString(clip(sanitize.String(title), width) + "\r\n")
	end := min(v.top+v.rows, len(lines))
	pos := fmt.Sprintf("%s  (lines %d-%d of %d)", viewHelp, min(v.top+1, end), end, len(lines))
	b.WriteString(clip(pos, width) + "\r\n")
	for _, l := range lines[v.top:end] {
		b.WriteString(l + "\r\n")
	}
	return b.String()
}

// Wrap breaks text into lines of at most width runes.
func Wrap(text string, width int) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		r := []rune(l)
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = r[width:]
		}
		out = append(out, string(r))
	}
	return out
}

// clip shortens s to width runes, with an ellipsis.
func clip(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	r := []rune(s)
	return string(r[:max(width-1, 0)]) + "…"
}
