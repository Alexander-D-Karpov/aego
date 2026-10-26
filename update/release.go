// Package update finds, downloads and installs new versions of the editor, the launcher and exported games (D362).
package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PublicKey verifies release.json; the release build sets it with -ldflags "-X aego/update.PublicKey=<base64>".
var PublicKey = ""

// Channel is a stream of releases: stable versions, or a build of every master commit.
type Channel string

const (
	Stable  Channel = "stable"
	Nightly Channel = "nightly"
)

// Kind is how an asset installs.
type Kind string

const (
	KindInstaller Kind = "installer"
	KindAppImage  Kind = "appimage"
	KindTarball   Kind = "tarball"
	KindApp       Kind = "app"
	KindZip       Kind = "zip"
)

// Asset is one downloadable file of a release.
type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kind   Kind   `json:"kind"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Release is release.json: a version, its channel and notes, the oldest version that may update to it, and its assets.
type Release struct {
	Version    string    `json:"version"`
	Channel    Channel   `json:"channel"`
	Published  time.Time `json:"published"`
	Notes      string    `json:"notes,omitempty"`
	MinVersion string    `json:"minVersion,omitempty"`
	Assets     []Asset   `json:"assets"`
}

var (
	ErrUnsigned  = errors.New("the release is not signed, or not by the key this build trusts")
	ErrNoKey     = errors.New("this build has no key to verify updates with")
	ErrChecksum  = errors.New("the download does not match the release's checksum")
	ErrNoAsset   = errors.New("the release has nothing for this system")
	ErrTooOld    = errors.New("this version is too old to update to the release; install it again")
	ErrNoInstall = errors.New("this copy was not installed by an Aego installer, so it cannot update itself")
)

// Asset is what the release has for a system and an install kind.
func (r *Release) Asset(goos, arch string, kind Kind) (*Asset, error) {
	for i := range r.Assets {
		a := &r.Assets[i]
		if a.OS == goos && (a.Arch == arch || a.Arch == "") && a.Kind == kind {
			if a.SHA256 == "" || a.URL == "" {
				return nil, ErrChecksum
			}
			return a, nil
		}
	}
	return nil, ErrNoAsset
}

// ParseKey reads a base64 public key.
func ParseKey(s string) (ed25519.PublicKey, error) {
	if s == "" {
		return nil, ErrNoKey
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("the update key is not a base64 Ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// Verify checks the signature of release.json and reads it; an unsigned or tampered file is refused.
func Verify(data, sig []byte, pub ed25519.PublicKey) (*Release, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || !ed25519.Verify(pub, data, raw) {
		return nil, ErrUnsigned
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("release.json: %w", err)
	}
	if r.Version == "" {
		return nil, errors.New("release.json names no version")
	}
	return &r, nil
}

// GenerateKey makes a signing pair, both base64: keep the private one secret, build the public one into the binary.
func GenerateKey() (public, private string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// Sign signs release.json with a base64 private key; the signature goes next to it as release.json.sig.
func Sign(data []byte, private string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(private))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("the signing key is not a base64 Ed25519 private key")
	}
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(b), data)) + "\n"), nil
}

// Encode writes release.json as the release scripts publish it.
func Encode(r *Release) ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	return append(b, '\n'), err
}

// version is a parsed "1.2.3" or "1.2.3-dev": the core numbers and the pre-release part.
type version struct {
	n   [3]int
	pre string
}

func parse(s string) (version, bool) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core, pre, _ := strings.Cut(s, "-")
	v.pre = pre
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.n[i] = n
	}
	return v, true
}

// Compare orders two versions: -1, 0 or 1; a release sorts after its pre-releases, unparsable versions sort first.
func Compare(a, b string) int {
	va, okA := parse(a)
	vb, okB := parse(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return -1
	case !okB:
		return 1
	}
	for i := range va.n {
		if va.n[i] != vb.n[i] {
			if va.n[i] < vb.n[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case va.pre == vb.pre:
		return 0
	case va.pre == "":
		return 1
	case vb.pre == "":
		return -1
	case va.pre < vb.pre:
		return -1
	}
	return 1
}

// Build is what the running copy knows about itself: its version and when it was built, for nightly builds of one version.
type Build struct {
	Version string
	Time    time.Time
}

// Newer reports whether the release replaces the build: a higher version, or the same pre-release version built later.
func (r *Release) Newer(b Build) bool {
	switch c := Compare(r.Version, b.Version); {
	case c > 0:
		return true
	case c < 0:
		return false
	}
	v, _ := parse(r.Version)
	return v.pre != "" && !b.Time.IsZero() && r.Published.After(b.Time.Add(time.Minute))
}

// Allowed reports whether the build may update to the release directly.
func (r *Release) Allowed(b Build) bool {
	return r.MinVersion == "" || Compare(b.Version, r.MinVersion) >= 0
}
