package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aego/app/updates"
	"aego/editor/core/prefs"
	upd "aego/update"
)

// cmdUpdate checks for, installs or undoes an update of this copy: aego update [status|check|apply|restore] [--channel nightly].
func cmdUpdate(args []string) int {
	verb, rest := firstArg(args)
	if verb == "" {
		verb = "status"
	}
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	channel := fs.String("channel", "", "stable or nightly; the editor's choice by default")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	dir := prefsDir()
	pr, _ := prefs.Load(dir)
	ch := pr.Updates.Channel
	if *channel != "" {
		ch = upd.ParseChannel(*channel)
	}
	in, ierr := updates.Installed()
	switch verb {
	case "status":
		fmt.Printf("aego %s, %s updates, channel %s\n", updates.Build().Version, pr.Updates.Mode, upd.ParseChannel(string(ch)))
		if ierr != nil {
			fmt.Println(updates.ErrNotInstalled)
		} else {
			fmt.Printf("installed as %s at %s\n", in.Kind, in.Root)
			if in.HasPrevious() {
				fmt.Println("the previous version is kept; aego update restore goes back to it")
			}
		}
		return 0
	case "restore":
		if ierr != nil {
			return fail(updates.ErrNotInstalled)
		}
		a, err := in.Restore()
		if err != nil {
			return fail(err)
		}
		if a.Exit != nil {
			// the kept installer waits for this process to exit before it replaces aego.exe
			if err := a.Exit.Start(); err != nil {
				return fail(err)
			}
			fmt.Println("the previous version's installer is putting it back")
			return 0
		}
		fmt.Println("restored the previous version; it starts next time")
		return 0
	case "check", "apply":
	default:
		fmt.Fprintln(os.Stderr, "usage: aego update [status|check|apply|restore] [--channel stable|nightly]")
		return 2
	}
	r, err := updates.Source(upd.ParseChannel(string(ch))).Check(context.Background())
	pr.Updates.LastCheck = time.Now()
	_ = prefs.Save(dir, pr)
	if err != nil {
		return fail(err)
	}
	b := updates.Build()
	if !r.Newer(b) {
		fmt.Printf("aego %s is up to date (%s %s)\n", b.Version, r.Channel, r.Version)
		return 0
	}
	fmt.Printf("aego %s is available, this is %s\n", r.Version, b.Version)
	if verb == "check" {
		return 0
	}
	if ierr != nil {
		return fail(updates.ErrNotInstalled)
	}
	file, err := updates.Fetch(context.Background(), r, in, func(done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "\rdownloading %d%%", done*100/total)
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fail(err)
	}
	a, err := updates.Apply(in, file, r)
	if err != nil {
		return fail(err)
	}
	if a.Exit != nil {
		a.Exit.Stdout, a.Exit.Stderr = os.Stdout, os.Stderr
		if err := a.Exit.Start(); err != nil {
			return fail(err)
		}
		fmt.Println("the installer is updating Aego")
		return 0
	}
	fmt.Printf("installed aego %s; it starts next time\n", r.Version)
	return 0
}

// prefsDir is where prefs.aetext lives, next to the launcher's recent list.
func prefsDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "aego")
}

// cmdRelease makes the signed release.json of a release: aego release keygen | manifest.
func cmdRelease(args []string) int {
	verb, rest := firstArg(args)
	switch verb {
	case "keygen":
		return releaseKeygen(rest)
	case "manifest":
		return releaseManifest(rest)
	}
	fmt.Fprintln(os.Stderr, "usage: aego release keygen [--out base] | aego release manifest --version v --channel stable|nightly --base-url url [--notes file] [--min v] [--key-file file] [--out dir] <files>…")
	return 2
}

func releaseKeygen(args []string) int {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", "", "base path of the pair: <out>.key stays secret, <out>.pub is the public key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		*out = filepath.Join(prefsDir(), "keys", "release")
	}
	base := strings.TrimSuffix(strings.TrimSuffix(*out, ".key"), ".pub")
	for _, f := range []string{base + ".key", base + ".pub"} {
		if _, err := os.Stat(f); err == nil {
			return fail(fmt.Errorf("%s exists; move it away to make a new key", f))
		}
	}
	pub, priv, err := upd.GenerateKey()
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(base+".key", []byte(priv+"\n"), 0o600); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(base+".pub", []byte(pub+"\n"), 0o644); err != nil {
		return fail(err)
	}
	fmt.Println("private key:", base+".key")
	fmt.Println("public key: ", base+".pub")
	fmt.Println(pub)
	return 0
}

// assetOf reads what a release file is from its name, as release.sh and export name them.
func assetOf(name string) (goos, arch string, kind upd.Kind, ok bool) {
	lower := strings.ToLower(name)
	arch = "amd64"
	if strings.Contains(lower, "arm64") || strings.Contains(lower, "aarch64") {
		arch = "arm64"
	}
	switch {
	case strings.HasSuffix(lower, ".appimage"):
		return "linux", arch, upd.KindAppImage, true
	case strings.HasSuffix(lower, "-setup.exe"):
		return "windows", arch, upd.KindInstaller, true
	case strings.HasSuffix(lower, ".app.zip"):
		return "darwin", arch, upd.KindApp, true
	case strings.HasSuffix(lower, ".tar.gz") && strings.Contains(lower, "linux"):
		return "linux", arch, upd.KindTarball, true
	case strings.HasSuffix(lower, ".tar.gz") && (strings.Contains(lower, "darwin") || strings.Contains(lower, "macos")):
		return "darwin", arch, upd.KindTarball, true
	case strings.HasSuffix(lower, ".zip") && strings.Contains(lower, "windows"):
		return "windows", arch, upd.KindZip, true
	}
	return "", "", "", false
}

func releaseManifest(args []string) int {
	fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
	version := fs.String("version", "", "version of the release")
	channel := fs.String("channel", "stable", "stable or nightly")
	base := fs.String("base-url", "", "URL the files are downloaded from")
	notes := fs.String("notes", "", "file with the release notes")
	min := fs.String("min", "", "oldest version that may update to this one")
	keyFile := fs.String("key-file", "", "private key; AEGO_RELEASE_KEY holds it when empty")
	out := fs.String("out", "", "folder for release.json and release.json.sig; the folder of the first file by default")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *version == "" || *base == "" || fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "aego release manifest: --version, --base-url and at least one file are needed")
		return 2
	}
	key := os.Getenv("AEGO_RELEASE_KEY")
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			return fail(err)
		}
		key = string(b)
	}
	if strings.TrimSpace(key) == "" {
		return fail(errors.New("no signing key: pass --key-file or set AEGO_RELEASE_KEY; an unsigned release.json is never installed"))
	}
	r := &upd.Release{Version: strings.TrimPrefix(*version, "v"), Channel: upd.ParseChannel(*channel), Published: time.Now().UTC().Truncate(time.Second), MinVersion: *min}
	if *notes != "" {
		b, err := os.ReadFile(*notes)
		if err != nil {
			return fail(err)
		}
		r.Notes = string(b)
	}
	for _, f := range fs.Args() {
		goos, arch, kind, ok := assetOf(filepath.Base(f))
		if !ok {
			fmt.Fprintf(os.Stderr, "aego release manifest: %s is not an asset an update installs, left out\n", filepath.Base(f))
			continue
		}
		sum, size, err := upd.Sum(f)
		if err != nil {
			return fail(err)
		}
		r.Assets = append(r.Assets, upd.Asset{OS: goos, Arch: arch, Kind: kind, URL: strings.TrimSuffix(*base, "/") + "/" + filepath.Base(f), Size: size, SHA256: sum})
	}
	data, err := upd.Encode(r)
	if err != nil {
		return fail(err)
	}
	sig, err := upd.Sign(data, key)
	if err != nil {
		return fail(err)
	}
	dir := *out
	if dir == "" {
		dir = filepath.Dir(fs.Arg(0))
	}
	if err := os.WriteFile(filepath.Join(dir, upd.ReleaseFile), data, 0o644); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, upd.SigFile), sig, 0o644); err != nil {
		return fail(err)
	}
	fmt.Printf("wrote %s with %d assets, signed\n", filepath.Join(dir, upd.ReleaseFile), len(r.Assets))
	return 0
}
