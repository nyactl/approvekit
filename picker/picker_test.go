package picker

import (
	"bufio"
	"errors"
	"reflect"
	"strings"
	"testing"
)

var tickets = []string{
	"CMP-154  Login times out on slow networks",
	"CMP-155  Add dark mode to the settings page",
	"CMP-156  Write the release checklist",
	"KAN-12   Release notes for 0.3",
}

func TestFilter(t *testing.T) {
	t.Parallel()
	for q, want := range map[string][]int{
		"":              {0, 1, 2, 3},
		"release":       {3, 2},
		"rel chk":       {2},
		"CMP-15":        {0, 1, 2},
		"dkmd":          {1},
		"login slow":    {0},
		"kan release":   {3},
		"nothing like":  {},
		"156":           {2},
		"  release   ":  {3, 2},
		"WRITE release": {2},
	} {
		if got := Filter(q, tickets); !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("Filter(%q) = %v, want %v", q, got, want)
		}
	}
	// Matches at the start of a word, in a row and early in the line rank
	// first.
	items := []string{"the settings page", "set up the tests", "Settings", "subset"}
	if got := Filter("set", items); len(got) != 4 || got[2] != 0 || got[3] != 3 {
		t.Errorf("ranking = %v", got)
	}
}

// press sends keys that do not end the choice.
func press(t *testing.T, m *Model, ks ...string) {
	t.Helper()
	if done, _, _ := keys(t, m, ks...); done {
		t.Fatal("the choice ended")
	}
}

func keys(t *testing.T, m *Model, ks ...string) (bool, Result, error) {
	t.Helper()
	for i, k := range ks {
		done, r, err := m.Key(k, 2)
		if done {
			if i != len(ks)-1 {
				t.Fatalf("done after %q, with keys left", k)
			}
			return done, r, err
		}
	}
	return false, Result{}, nil
}

func TestModelKeys(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Items: tickets, Actions: []Action{{Key: "ctrl-o", Name: "open"}}})
	if _, r, err := keys(t, m, "r", "e", "l", "down", "enter"); err != nil || r.Index != 2 || r.Query != "rel" {
		t.Errorf("result %+v, %v", r, err)
	}
	m = NewModel(Options{Items: tickets, Actions: []Action{{Key: "ctrl-o", Name: "open"}}, Cursor: 2})
	if m.Current() != 2 {
		t.Errorf("starts on %d", m.Current())
	}
	if _, r, _ := keys(t, m, "ctrl-o"); r.Index != 2 || r.Action != "ctrl-o" {
		t.Errorf("action: %+v", r)
	}
	m = NewModel(Options{Items: tickets})
	press(t, m, "x", "y", "z")
	if m.Current() != -1 {
		t.Error("something matches xyz")
	}
	if done, _, _ := m.Key("enter", 2); done {
		t.Error("Enter chose with no match")
	}
	press(t, m, "backspace", "ctrl-u", "end")
	if m.Query() != "" || m.Current() != 3 {
		t.Errorf("query %q, current %d", m.Query(), m.Current())
	}
	press(t, m, "pgup", "up", "up")
	if m.Current() != 0 {
		t.Errorf("current %d", m.Current())
	}
	press(t, m, "a", "b", " ", "c", "ctrl-w")
	if m.Query() != "ab " {
		t.Errorf("ctrl-w left %q", m.Query())
	}
	if _, _, err := keys(t, m, "esc"); !errors.Is(err, ErrCanceled) {
		t.Errorf("esc: %v", err)
	}
	m = NewModel(Options{Items: tickets})
	press(t, m, "é", "ctrl-o")
	if m.Query() != "é" {
		t.Errorf("unknown ctrl key typed: %q", m.Query())
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{Title: "Pick \x1b[31mone", Items: append([]string{"evil \x1b]0;title\x07 line"}, tickets...), Enter: "show", Actions: []Action{{Key: "ctrl-o", Name: "open in browser"}}, Status: "opened CMP-154"})
	out := m.Render(120, 10)
	for _, want := range []string{"Enter show", "Ctrl-O open in browser", "Esc quit", "5/5", "opened CMP-154", "\x1b[7m> evil"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%q", want, out)
		}
	}
	for _, bad := range []string{"\x1b[31m", "\x1b]0;"} {
		if strings.Contains(out, bad) {
			t.Errorf("render passes %q through", bad)
		}
	}
	// A list longer than the screen scrolls with the cursor.
	many := make([]string, 50)
	for i := range many {
		many[i] = "item " + strings.Repeat("x", i%3)
	}
	m = NewModel(Options{Items: many, Cursor: 40})
	if out := m.Render(40, 10); strings.Count(out, "\r\n") > 10 || !strings.Contains(out, "\x1b[7m") {
		t.Errorf("render of a long list:\n%q", out)
	}
}

func TestReadKey(t *testing.T) {
	t.Parallel()
	in := "a\r\x7f\x0f\x1b[A\x1b[B\x1b[5~\x1b[6~\x1bOH\x1b[4~é\t"
	r := bufio.NewReader(strings.NewReader(in))
	var got []string
	for {
		k, err := ReadKey(r)
		if err != nil {
			break
		}
		got = append(got, k)
	}
	want := []string{"a", "enter", "backspace", "ctrl-o", "up", "down", "pgup", "pgdn", "home", "end", "é", "tab"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
	// A lone Esc, with nothing following, is Esc.
	if k, _ := ReadKey(bufio.NewReader(strings.NewReader("\x1b"))); k != "esc" {
		t.Errorf("esc = %q", k)
	}
}

func TestViewer(t *testing.T) {
	t.Parallel()
	v := NewViewer(30, 10)
	for _, k := range []string{"pgdn", "pgdn", "pgdn"} {
		v.Key(k)
	}
	if v.Top() != 20 {
		t.Errorf("top %d", v.Top())
	}
	v.Key("home")
	v.Key("up")
	if v.Top() != 0 || !v.Key("q") {
		t.Error("viewer keys")
	}
	if got := Wrap("abcdef\nxy", 4); !reflect.DeepEqual(got, []string{"abcd", "ef", "xy"}) {
		t.Errorf("Wrap = %q", got)
	}
}
