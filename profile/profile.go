// Package profile stores a tool's non-secret settings per profile, such as
// "default" for production and "test" for a test site, and keeps
// development builds away from production.
//
// Each tool defines its own settings type, which embeds Account, and its
// own Store. API tokens do not belong here; see package keychain.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Default is the profile used when none is selected. It is meant for
// production.
const Default = "default"

// ErrNotConfigured is returned by Load when a profile has no settings yet.
var ErrNotConfigured = errors.New("this profile is not set up yet")

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Store locates the profiles of one tool.
type Store struct {
	// App names the directory in the user's config directory, such as
	// "tix-for-jira".
	App string
	// ProfileEnv names the environment variable that selects a profile
	// when none is given, such as "TIX_JIRA_PROFILE".
	ProfileEnv string
	// ConfigEnv names the environment variable that overrides the settings
	// file, such as "TIX_JIRA_CONFIG". Optional.
	ConfigEnv string
}

// Settings is a tool's settings type. Validate checks and normalises it.
type Settings interface {
	Validate() error
}

// Resolve returns name, or the profile named by ProfileEnv, or Default.
func (s Store) Resolve(name string) (string, error) {
	if name == "" && s.ProfileEnv != "" {
		name = os.Getenv(s.ProfileEnv)
	}
	if name == "" {
		return Default, nil
	}
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("invalid profile name %q: use lowercase letters, digits and dashes", name)
	}
	return name, nil
}

// Explicit reports whether a profile was chosen by flag, ProfileEnv or
// ConfigEnv rather than falling back to Default.
func (s Store) Explicit(name string) bool {
	return name != "" || (s.ProfileEnv != "" && os.Getenv(s.ProfileEnv) != "") || (s.ConfigEnv != "" && os.Getenv(s.ConfigEnv) != "")
}

func (s Store) dir() (string, error) {
	if s.App == "" {
		return "", errors.New("profile: Store.App is not set")
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, s.App), nil
}

// Path returns the settings file of a profile: config.json for Default and
// profiles/<name>.json for the others, unless ConfigEnv overrides it.
func (s Store) Path(name string) (string, error) {
	if s.ConfigEnv != "" {
		if p := os.Getenv(s.ConfigEnv); p != "" {
			return p, nil
		}
	}
	name, err := s.Resolve(name)
	if err != nil {
		return "", err
	}
	d, err := s.dir()
	if err != nil {
		return "", err
	}
	if name == Default {
		return filepath.Join(d, "config.json"), nil
	}
	return filepath.Join(d, "profiles", name+".json"), nil
}

// List returns the profiles that have a settings file.
func (s Store) List() ([]string, error) {
	d, err := s.dir()
	if err != nil {
		return nil, err
	}
	var out []string
	if _, err := os.Stat(filepath.Join(d, "config.json")); err == nil {
		out = append(out, Default)
	}
	entries, err := os.ReadDir(filepath.Join(d, "profiles"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok && nameRe.MatchString(name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// Load reads a profile's settings into v and validates them.
func (s Store) Load(name string, v Settings) error {
	p, err := s.Path(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p) //nolint:gosec // G304: the path is the user's own config location
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotConfigured
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("reading %s: %w", p, err)
	}
	if err := v.Validate(); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	return nil
}

// Save validates v and writes it with owner-only permissions, replacing the
// file atomically.
func (s Store) Save(name string, v Settings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	p, err := s.Path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

var cloudIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Account is what every tool stores about a login. Embed it in the tool's
// settings type.
type Account struct {
	Site        string `json:"site"`
	Email       string `json:"email"`
	TokenExpiry string `json:"tokenExpiry,omitempty"` // YYYY-MM-DD
	// AllowDevBuilds lets development builds use this profile. It is never
	// allowed for the default profile, which is meant for production.
	AllowDevBuilds bool `json:"allowDevBuilds,omitempty"`
	// ScopedToken marks a scoped API token, which only works through
	// Atlassian's API gateway for the site with CloudID.
	ScopedToken bool   `json:"scopedToken,omitempty"`
	CloudID     string `json:"cloudId,omitempty"`
}

// Validate checks every field and normalises the site.
func (a *Account) Validate() error {
	site, err := NormalizeSite(a.Site)
	if err != nil {
		return err
	}
	a.Site = site
	if _, err := mail.ParseAddress(a.Email); err != nil || strings.ContainsAny(a.Email, "<> ") {
		return fmt.Errorf("invalid email %q", a.Email)
	}
	if a.TokenExpiry != "" {
		if _, err := time.Parse(time.DateOnly, a.TokenExpiry); err != nil {
			return fmt.Errorf("invalid token expiry %q (want YYYY-MM-DD)", a.TokenExpiry)
		}
	}
	if a.ScopedToken && !cloudIDRe.MatchString(a.CloudID) {
		return fmt.Errorf("invalid cloud ID %q for a scoped token; log in again", a.CloudID)
	}
	return nil
}

// ExpiryWarning returns a message when the recorded token expiry is within
// two weeks or has passed, and "" otherwise. renew tells how to renew it,
// such as "run `tix auth login`".
func (a *Account) ExpiryWarning(now time.Time, renew string) string {
	if a.TokenExpiry == "" {
		return ""
	}
	exp, err := time.Parse(time.DateOnly, a.TokenExpiry)
	if err != nil {
		return ""
	}
	days := int(exp.Sub(now.Truncate(24*time.Hour)).Hours() / 24)
	switch {
	case days < 0:
		return fmt.Sprintf("your API token expired on %s; create a new one and %s", a.TokenExpiry, renew)
	case days <= 14:
		return fmt.Sprintf("your API token expires on %s (in %d days); create a new one and %s", a.TokenExpiry, days, renew)
	}
	return ""
}

// NormalizeSite accepts "example.atlassian.net" or "https://example.atlassian.net/"
// and returns "https://example.atlassian.net". Only https is allowed.
func NormalizeSite(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("site is required, e.g. https://your-team.atlassian.net")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid site: %w", err)
	}
	switch {
	case u.Scheme != "https":
		return "", fmt.Errorf("site must use https, got %q", u.Scheme)
	case u.Hostname() == "":
		return "", errors.New("site has no host")
	case u.User != nil:
		return "", errors.New("site must not contain credentials")
	case u.RawQuery != "" || u.Fragment != "":
		return "", errors.New("site must not contain a query or fragment")
	case strings.Trim(u.Path, "/") != "":
		return "", errors.New("site must be the bare site URL without a path")
	}
	return "https://" + strings.ToLower(u.Host), nil
}

var (
	releaseRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)
	pseudoRe  = regexp.MustCompile(`[0-9]{14}-[0-9a-f]{12}`)
)

// IsRelease reports whether version is a tagged release, as installed with
// `go install ...@v1.2.3` or from a release archive. Builds from a checkout
// carry "dev", a pseudo-version such as v0.0.0-20261009122403-4c701510dbd1,
// or "+dirty".
func IsRelease(version string) bool {
	return releaseRe.MatchString(version) && !pseudoRe.MatchString(version)
}

// ErrDevBuildNotAllowed is returned by CheckDevBuild.
var ErrDevBuildNotAllowed = errors.New("development builds may only use profiles that opted in to development builds; the default profile never can")

// CheckDevBuild keeps development builds away from production: they may
// only use a profile other than Default whose account allows them.
func CheckDevBuild(version, name string, a *Account) error {
	if IsRelease(version) {
		return nil
	}
	if name == Default || a == nil || !a.AllowDevBuilds {
		return fmt.Errorf("%w (profile %s)", ErrDevBuildNotAllowed, name)
	}
	return nil
}
