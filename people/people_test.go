package people

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	me    = &Person{AccountID: "acc-me", DisplayName: "Max Mustermann", Email: "max@example.com"}
	alice = &Person{AccountID: "acc-alice", DisplayName: "Alice Smith", Email: "alice@example.com"}
	bob   = &Person{AccountID: "acc-bob", DisplayName: "Bob Jones"}
)

func TestLabelsInOwnMode(t *testing.T) {
	t.Parallel()
	p := New(Own, *me)
	if got := p.Person(me); got != MeLabel {
		t.Errorf("me = %q", got)
	}
	if got := p.Person(alice); got != "Person A" {
		t.Errorf("alice = %q", got)
	}
	if got := p.Person(bob); got != "Person B" {
		t.Errorf("bob = %q", got)
	}
	if got := p.Person(alice); got != "Person A" {
		t.Errorf("alice again = %q", got)
	}
	if got := p.Person(nil); got != "" {
		t.Errorf("nil = %q", got)
	}
	if got := p.ID("", "Deleted User", ""); got != "someone" {
		t.Errorf("no account id = %q", got)
	}
}

func TestLabelsInOffMode(t *testing.T) {
	t.Parallel()
	p := New(Off, *me)
	if got := p.Person(alice); got != "Alice Smith" {
		t.Errorf("alice = %q", got)
	}
	if got := p.Person(me); got != "Max Mustermann" {
		t.Errorf("me = %q", got)
	}
	if id, _, ok := p.Resolve("alice smith"); !ok || id != "acc-alice" {
		t.Errorf("resolve = %q %v", id, ok)
	}
	if got := p.Text("ask Alice Smith"); got != "ask Alice Smith" {
		t.Errorf("text changed in off mode: %q", got)
	}
}

func TestLetters(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{1: "A", 26: "Z", 27: "AA", 28: "AB", 52: "AZ", 53: "BA"} {
		if got := letters(n); got != want {
			t.Errorf("letters(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	p := New(Own, *me)
	p.Person(alice)
	if id, display, ok := p.Resolve("person a"); !ok || id != "acc-alice" || display != "Alice Smith" {
		t.Errorf("Person A -> %q %q %v", id, display, ok)
	}
	if id, _, ok := p.Resolve("Me"); !ok || id != "acc-me" {
		t.Errorf("Me -> %q %v", id, ok)
	}
	if _, _, ok := p.Resolve("Alice Smith"); ok {
		t.Error("real name must not resolve in own mode")
	}
	if _, _, ok := p.Resolve("Person B"); ok {
		t.Error("unseen label resolved")
	}
}

func TestText(t *testing.T) {
	t.Parallel()
	p := New(Own, *me)
	p.Person(alice)
	p.Person(&Person{AccountID: "acc-al", DisplayName: "Al"})
	tests := []struct{ in, want string }{
		{"please ask Alice Smith", "please ask Person A"},
		{"ALICE SMITH said", "Person A said"},
		{"mail alice@example.com", "mail Person A"},
		{"Alice Smithson is someone else", "Alice Smithson is someone else"},
		{"Al is too short to replace safely", "Al is too short to replace safely"},
		{"Max Mustermann is me", "Max Mustermann is me"},
		{"x" + string(rune(0xe4)) + "Alice Smith", "x" + string(rune(0xe4)) + "Alice Smith"},
		{"(Alice Smith)", "(Person A)"},
	}
	for _, tt := range tests {
		if got := p.Text(tt.in); got != tt.want {
			t.Errorf("Text(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMentionTextNeverReplacesRealName(t *testing.T) {
	t.Parallel()
	p := New(Own, *me)
	if got := p.Mention("acc-alice", "@Ali (old name)"); got != "Person A" {
		t.Fatalf("mention label = %q", got)
	}
	p.Person(alice)
	got := p.Text("Alice Smith and Ali (old name) and alice@example.com")
	if got != "Person A and Person A and Person A" {
		t.Errorf("Text = %q", got)
	}
	if _, display, _ := p.Resolve("Person A"); display != "Alice Smith" {
		t.Errorf("display = %q, want the reliable name", display)
	}
}

func TestOffModeLabelUpgradesToReliableName(t *testing.T) {
	t.Parallel()
	p := New(Off, *me)
	if got := p.Mention("acc-alice", "@Ali"); got != "Ali" {
		t.Fatalf("label from mention = %q", got)
	}
	if got := p.Person(alice); got != "Alice Smith" {
		t.Errorf("label after structured name = %q", got)
	}
	if got := p.Mention("acc-alice", "@Ali"); got != "Alice Smith" {
		t.Errorf("stale mention text took over again: %q", got)
	}
}

func TestLabelsAcrossSessions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "profile", "test.labels.json")
	me := Person{AccountID: "acc-me", DisplayName: "Max Mustermann"}
	first := New(Own, me).WithStore(NewFileStore(path))
	if a, b := first.ID("acc-1", "Alice Smith", ""), first.ID("acc-2", "Bob Jones", ""); a != "Person A" || b != "Person B" {
		t.Fatalf("labels %s, %s", a, b)
	}
	// A later session: the same labels, whatever the order, and a label
	// shown before resolves without being shown again.
	second := New(Own, me).WithStore(NewFileStore(path))
	if b := second.ID("acc-2", "Bob Jones", ""); b != "Person B" {
		t.Errorf("acc-2 is %s in the second session", b)
	}
	if id, display, ok := second.Resolve("person a"); !ok || id != "acc-1" || display != "Person A" {
		t.Errorf("Resolve = %q %q %v", id, display, ok)
	}
	if _, _, ok := second.Resolve("Person Z"); ok {
		t.Error("an unknown label resolved")
	}
	if _, _, ok := second.Resolve("Me"); !ok {
		t.Error("Me does not resolve")
	}
	if c := second.ID("acc-3", "Carol", ""); c != "Person C" {
		t.Errorf("new person is %s", c)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"Alice", "Bob", "Carol", "Max"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("label file holds %q: %s", bad, raw)
		}
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("label file mode %v", st.Mode())
	}
	// Mode off neither uses nor resolves stored labels.
	off := New(Off, me).WithStore(NewFileStore(path))
	if l := off.ID("acc-1", "Alice Smith", ""); l != "Alice Smith" {
		t.Errorf("off mode shows %q", l)
	}
	if _, _, ok := off.Resolve("Person A"); ok {
		t.Error("off mode resolved a stored label")
	}
}

func TestLabelsSharedByPrograms(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "labels.json")
	const n = 40
	labels := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := New(Own, Person{AccountID: "acc-me"}).WithStore(NewFileStore(path))
			labels[i] = p.ID(fmt.Sprintf("acc-%d", i), "", "")
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, l := range labels {
		if seen[l] {
			t.Fatalf("label %s given twice: %v", l, labels)
		}
		seen[l] = true
	}
}

type failingStore struct{}

func (failingStore) Label(string) (string, error)  { return "", errors.New("disk full") }
func (failingStore) Account(string) (string, bool) { return "", false }

func TestLabelsWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	p := New(Own, Person{AccountID: "acc-me"}).WithStore(failingStore{})
	if l := p.ID("acc-1", "Alice", ""); l != "Person ?A" {
		t.Errorf("label %q", l)
	}
	if id, _, ok := p.Resolve("Person ?A"); !ok || id != "acc-1" {
		t.Error("session label does not resolve")
	}
}
