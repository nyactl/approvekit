package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var store = Store{App: "approvekit-test", ProfileEnv: "APPROVEKIT_TEST_PROFILE", ConfigEnv: "APPROVEKIT_TEST_CONFIG"}

type settings struct {
	Account
	Extra string `json:"extra,omitempty"`
}

func (s *settings) Validate() error { return s.Account.Validate() }

func TestNormalizeSite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want, wantErr string
	}{
		{in: "team.atlassian.net", want: "https://team.atlassian.net"},
		{in: "https://Team.Atlassian.net/", want: "https://team.atlassian.net"},
		{in: " https://team.atlassian.net ", want: "https://team.atlassian.net"},
		{in: "http://team.atlassian.net", wantErr: "must use https"},
		{in: "https://team.atlassian.net/jira", wantErr: "without a path"},
		{in: "https://u:p@team.atlassian.net", wantErr: "credentials"},
		{in: "https://team.atlassian.net?x=1", wantErr: "query"},
		{in: "", wantErr: "required"},
		{in: "https://", wantErr: "no host"},
	}
	for _, tt := range tests {
		got, err := NormalizeSite(tt.in)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%q: err = %v, want containing %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%q: got %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	t.Setenv(store.ConfigEnv, path)

	var got settings
	if err := store.Load("", &got); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Load before save: %v", err)
	}
	s := &settings{Account: Account{Site: "team.atlassian.net", Email: "me@example.com", TokenExpiry: "2027-01-31"}, Extra: "x"}
	if err := store.Save("", s); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
	if err := store.Load("", &got); err != nil {
		t.Fatal(err)
	}
	if got != (settings{Account: Account{Site: "https://team.atlassian.net", Email: "me@example.com", TokenExpiry: "2027-01-31"}, Extra: "x"}) {
		t.Errorf("loaded %+v", got)
	}
	// Embedding keeps the account fields at the top level of the file.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"site": "https://team.atlassian.net"`) {
		t.Errorf("file = %s", data)
	}
}

func TestValidateRejects(t *testing.T) {
	t.Parallel()
	base := Account{Site: "https://team.atlassian.net", Email: "me@example.com"}
	for name, mutate := range map[string]func(*Account){
		"invalid email":        func(a *Account) { a.Email = "not-an-email" },
		"invalid email ":       func(a *Account) { a.Email = "Me <me@example.com>" },
		"invalid token expiry": func(a *Account) { a.TokenExpiry = "31.01.2027" },
		"invalid cloud ID":     func(a *Account) { a.ScopedToken, a.CloudID = true, "x" },
	} {
		a := base
		mutate(&a)
		if err := a.Validate(); err == nil || !strings.Contains(err.Error(), strings.TrimSpace(name)) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestExpiryWarning(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)
	for expiry, want := range map[string]string{
		"":           "",
		"2026-12-31": "",
		"2026-10-20": "in 12 days",
		"2026-10-08": "in 0 days",
		"2026-10-01": "expired",
	} {
		a := Account{TokenExpiry: expiry}
		got := a.ExpiryWarning(now, "run `x auth login`")
		if (want == "") != (got == "") || !strings.Contains(got, want) || (got != "" && !strings.HasSuffix(got, "run `x auth login`")) {
			t.Errorf("expiry %q: got %q, want containing %q", expiry, got, want)
		}
	}
}

func TestProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(store.ConfigEnv, "")
	t.Setenv(store.ProfileEnv, "")

	def, _ := store.Path("")
	test, _ := store.Path("test")
	if filepath.Base(def) != "config.json" || filepath.Base(test) != "test.json" || filepath.Dir(filepath.Dir(test)) != filepath.Dir(def) {
		t.Errorf("paths: %s %s", def, test)
	}
	if store.Explicit("") {
		t.Error("no profile chosen, but Explicit")
	}
	t.Setenv(store.ProfileEnv, "test")
	if p, _ := store.Path(""); p != test || !store.Explicit("") {
		t.Errorf("%s ignored: %s", store.ProfileEnv, p)
	}
	if p, _ := store.Path("default"); p != def {
		t.Errorf("explicit profile must win over the environment: %s", p)
	}
	for _, bad := range []string{"../x", "Test", "a/b", "-x", strings.Repeat("a", 33)} {
		if _, err := store.Path(bad); err == nil {
			t.Errorf("profile %q accepted", bad)
		}
	}
	s := &settings{Account: Account{Site: "a.atlassian.net", Email: "me@example.com"}}
	for _, p := range []string{"default", "test"} {
		if err := store.Save(p, s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.List()
	if err != nil || strings.Join(got, ",") != "default,test" {
		t.Errorf("List = %v, %v", got, err)
	}
}

func TestDevBuilds(t *testing.T) {
	t.Parallel()
	for v, want := range map[string]bool{
		"v1.2.3":                               true,
		"v0.2.0-rc.1":                          true,
		"dev":                                  false,
		"v0.0.0-20261009122403-4c701510dbd1":   false,
		"v0.1.2-0.20261012122457-ce7ab0ac5aca": false,
		"v1.2.3+dirty":                         false,
	} {
		if IsRelease(v) != want {
			t.Errorf("IsRelease(%q) = %v", v, !want)
		}
	}
	allowed := &Account{AllowDevBuilds: true}
	for _, tc := range []struct {
		version, name string
		account       *Account
		ok            bool
	}{
		{"v1.0.0", Default, nil, true},
		{"dev", "test", allowed, true},
		{"dev", Default, allowed, false},
		{"dev", "test", &Account{}, false},
		{"dev", "test", nil, false},
	} {
		err := CheckDevBuild(tc.version, tc.name, tc.account)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrDevBuildNotAllowed)) {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}
