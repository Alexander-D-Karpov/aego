package edit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aego/app/assembly/builtin"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/project/remoteexample"
)

// launcherRemote only orchestrates the existing checked store and HTTPS
// transport. It never starts a process or executes project code.
type launcherRemote struct {
	root          string
	clientFactory func() (*remoteexample.Client, error)
}

func (r launcherRemote) storeRoot() (string, error) {
	root := r.root
	if root == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(config, "aego", "remote-examples")
	}
	return filepath.Abs(root)
}

func (r launcherRemote) store() (*remoteexample.Store, error) {
	root, err := r.storeRoot()
	if err != nil {
		return nil, err
	}
	return remoteexample.NewStore(root, remoteexample.Limits{})
}

// existingProject recognizes managed copies even when opened via Recent or a
// path, so those routes do not accidentally drop the remote import safeguards.
// A project elsewhere is local and does not create or initialize the store.
func (r launcherRemote) existingProject(root string) (launcher.RemoteCopy, bool, error) {
	storeRoot, err := r.storeRoot()
	if err != nil {
		return launcher.RemoteCopy{}, false, err
	}
	resolvedStore, err := filepath.EvalSymlinks(storeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return launcher.RemoteCopy{}, false, nil
	}
	if err != nil {
		return launcher.RemoteCopy{}, false, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return launcher.RemoteCopy{}, false, err
	}
	rel, err := filepath.Rel(filepath.Join(resolvedStore, "projects"), resolvedRoot)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return launcher.RemoteCopy{}, false, nil
	}
	if rel == "." || !filepath.IsLocal(rel) || strings.ContainsRune(rel, filepath.Separator) {
		return launcher.RemoteCopy{}, true, fmt.Errorf("invalid project path inside remote example store")
	}
	store, err := r.store()
	if err != nil {
		return launcher.RemoteCopy{}, true, err
	}
	copy, err := store.Reopen(rel)
	if err != nil {
		return launcher.RemoteCopy{}, true, err
	}
	return launcherCopy(copy), true, nil
}

func (r launcherRemote) client() (*remoteexample.Client, error) {
	if r.clientFactory != nil {
		return r.clientFactory()
	}
	return remoteexample.NewClient(remoteexample.Policy{EngineVersion: engine.Version, InstalledModules: builtin.Default().IDs()})
}

func launcherCopy(c remoteexample.Copy) launcher.RemoteCopy {
	return launcher.RemoteCopy{ID: c.ID, Name: c.Manifest.ExampleID, Version: c.Manifest.Version, Source: c.ManifestURL, Directory: c.Directory, Entry: c.Entry}
}

func (r launcherRemote) List(ctx context.Context) ([]launcher.RemoteCopy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := r.store()
	if err != nil {
		return nil, err
	}
	copies, err := store.Copies()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(copies) > launcher.MaxRecent {
		copies = copies[:launcher.MaxRecent]
	}
	out := make([]launcher.RemoteCopy, 0, len(copies))
	for _, c := range copies {
		out = append(out, launcherCopy(c))
	}
	return out, nil
}

func (r launcherRemote) Open(ctx context.Context, source, approved string, clone bool) (launcher.RemoteCopy, error) {
	if err := ctx.Err(); err != nil {
		return launcher.RemoteCopy{}, err
	}
	origin, err := remoteexample.Origin(source)
	if err != nil || approved != origin {
		return launcher.RemoteCopy{}, fmt.Errorf("explicit confirmation of the exact HTTPS origin is required")
	}
	store, err := r.store()
	if err != nil {
		return launcher.RemoteCopy{}, err
	}
	if !clone {
		copy, err := store.OpenRecent(source)
		if err == nil {
			return launcherCopy(copy), ctx.Err()
		}
		if !errors.Is(err, os.ErrNotExist) {
			return launcher.RemoteCopy{}, err
		}
	}
	client, err := r.client()
	if err != nil {
		return launcher.RemoteCopy{}, err
	}
	defer client.Close()
	copy, err := store.Clone(ctx, client, source, approved)
	if err != nil {
		return launcher.RemoteCopy{}, err
	}
	return launcherCopy(copy), ctx.Err()
}

func (r launcherRemote) Reopen(ctx context.Context, id string) (launcher.RemoteCopy, error) {
	if err := ctx.Err(); err != nil {
		return launcher.RemoteCopy{}, err
	}
	store, err := r.store()
	if err != nil {
		return launcher.RemoteCopy{}, err
	}
	copy, err := store.Reopen(id)
	if err != nil {
		return launcher.RemoteCopy{}, err
	}
	return launcherCopy(copy), ctx.Err()
}

func (r launcherRemote) Check(ctx context.Context, id, approved string) (launcher.RemoteUpdate, error) {
	if err := ctx.Err(); err != nil {
		return launcher.RemoteUpdate{}, err
	}
	store, err := r.store()
	if err != nil {
		return launcher.RemoteUpdate{}, err
	}
	copy, err := store.Reopen(id)
	if err != nil {
		return launcher.RemoteUpdate{}, err
	}
	origin, err := remoteexample.Origin(copy.ManifestURL)
	if err != nil || approved != origin {
		return launcher.RemoteUpdate{}, fmt.Errorf("explicit confirmation of the exact HTTPS origin is required")
	}
	client, err := r.client()
	if err != nil {
		return launcher.RemoteUpdate{}, err
	}
	defer client.Close()
	update, err := client.CheckUpdate(ctx, copy, approved)
	if err != nil {
		return launcher.RemoteUpdate{}, err
	}
	return launcher.RemoteUpdate{Available: update.Available, Current: update.Current, Remote: update.Remote}, nil
}

var _ launcher.RemoteExamples = launcherRemote{}
