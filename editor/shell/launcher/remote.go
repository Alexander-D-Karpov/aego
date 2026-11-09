package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/project"
	"aego/project/remoteexample"
	"aego/ui/core"
	"aego/ui/widgets"
)

// RemoteExamples is backed by the existing remoteexample store and HTTPS
// client. Implementations must not launch games, plugins or external commands.
type RemoteExamples interface {
	List(context.Context) ([]RemoteCopy, error)
	Open(context.Context, string, string, bool) (RemoteCopy, error)
	Reopen(context.Context, string) (RemoteCopy, error)
	Check(context.Context, string, string) (RemoteUpdate, error)
}

type RemoteCopy struct {
	ID, Name, Version, Source, Directory, Entry string
}
type RemoteUpdate struct {
	Available       bool
	Current, Remote string
}
type remoteRequest struct {
	kind, source, origin, id string
}
type remoteReply struct {
	kind   string
	copies []RemoteCopy
	copy   RemoteCopy
	update RemoteUpdate
	err    error
}
type remoteJob struct {
	cancel   context.CancelFunc
	done     chan remoteReply
	canceled bool
}
type remoteState struct {
	url      string
	copies   []RemoteCopy
	pending  *remoteRequest
	job      *remoteJob
	status   string
	closed   bool
	fromLink bool
}

// RequestExampleLink routes an OS URL request to the ordinary, explicit remote
// confirmation. Both scene and project links open an isolated editable project;
// neither imports into the current project nor starts Play. No URL parameter
// can choose a filesystem path, invoke a command or approve its own origin.
func (l *Launcher) RequestExampleLink(raw string) error {
	link, err := remoteexample.ParseLink(raw)
	if err != nil {
		return err
	}
	if l.opt.Remote == nil || l.remote.closed || l.remote.job != nil || l.remote.pending != nil {
		return fmt.Errorf("remote example request cannot replace an active operation")
	}
	l.SetTab(TabExamples)
	l.remote.url = link.ManifestURL
	l.remote.fromLink = true
	l.confirmRemote("open", link.ManifestURL, "")
	if l.remote.pending == nil {
		return fmt.Errorf("remote example confirmation could not be prepared")
	}
	return nil
}

// AwaitingExampleConfirmation lets the host defer unrelated automatic network
// work for a freshly opened protocol request. Explicit user actions remain
// separate, and an already running job cannot be canceled retroactively.
func (l *Launcher) AwaitingExampleConfirmation() bool {
	return l.remote.fromLink && l.remote.pending != nil
}

// Close cancels in-flight HTTPS before the launcher is destroyed. The worker
// only owns its result channel and never touches UI state from another thread.
func (l *Launcher) Close() {
	l.remote.closed = true
	l.cancelRemote()
}

// If another project was chosen while a worker was finishing, its late reply
// must never replace the user's selection or start a different project.
func (l *Launcher) cancelRemote() {
	l.remote.pending = nil
	if l.remote.job != nil {
		l.remote.job.canceled = true
		l.remote.job.cancel()
	}
}

func (l *Launcher) confirmRemote(kind, source, id string) {
	if l.opt.Remote == nil || l.remote.closed || l.remote.job != nil {
		return
	}
	source = strings.TrimSpace(source)
	origin, err := remoteexample.Origin(source)
	if err != nil {
		l.err = err.Error()
		return
	}
	l.err, l.remote.status = "", ""
	// Freeze both URL and origin. Edits to the text field cannot silently
	// change the operation the user is being asked to approve.
	l.remote.pending = &remoteRequest{kind: kind, source: source, origin: origin, id: id}
}

func (l *Launcher) startRemote(req remoteRequest) {
	if l.opt.Remote == nil || l.remote.closed || l.remote.job != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &remoteJob{cancel: cancel, done: make(chan remoteReply, 1)}
	l.remote.job = job
	l.err, l.remote.status = "", ""
	service := l.opt.Remote
	go func() {
		r := remoteReply{kind: req.kind}
		switch req.kind {
		case "list":
			r.copies, r.err = service.List(ctx)
		case "open", "clone":
			r.copy, r.err = service.Open(ctx, req.source, req.origin, req.kind == "clone")
		case "reopen":
			r.copy, r.err = service.Reopen(ctx, req.id)
		case "check":
			r.update, r.err = service.Check(ctx, req.id, req.origin)
		default:
			r.err = fmt.Errorf("unsupported remote example operation")
		}
		job.done <- r
	}()
}

func (l *Launcher) pollRemote() {
	job := l.remote.job
	if job == nil {
		return
	}
	select {
	case r := <-job.done:
		l.remote.job = nil
		job.cancel()
		if job.canceled || l.remote.closed {
			l.remote.status = i18n.Tr("launcher.remote.canceled", "Canceled. A copy already saved locally is kept.")
			return
		}
		if r.err != nil {
			l.err = r.err.Error()
			return
		}
		switch r.kind {
		case "list":
			l.remote.copies = r.copies
			if len(r.copies) == 0 {
				l.remote.status = i18n.Tr("launcher.remote.no_copies", "No saved remote examples yet.")
			}
		case "open", "clone", "reopen":
			if err := l.acceptRemoteCopy(r.copy); err != nil {
				l.err = err.Error()
			}
		case "check":
			if r.update.Available {
				l.remote.status = i18n.Tr("launcher.remote.update_available", "Version {version} is available. Your local copy was not changed.", i18n.Str("version", r.update.Remote))
			} else {
				l.remote.status = i18n.Tr("launcher.remote.up_to_date", "No newer version. Your local copy was not changed.")
			}
		}
	default:
	}
}

func (l *Launcher) acceptRemoteCopy(c RemoteCopy) error {
	root, err := filepath.Abs(c.Directory)
	if err != nil {
		return err
	}
	entry, err := filepath.Abs(c.Entry)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, entry)
	if err != nil || !filepath.IsLocal(rel) || !strings.EqualFold(filepath.Ext(rel), ".aescene") {
		return fmt.Errorf("%s", i18n.Tr("launcher.remote.bad_entry", "The remote entry scene is outside its project."))
	}
	st, err := os.Lstat(entry)
	if err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("%s", i18n.Tr("launcher.remote.bad_entry", "The remote entry scene is outside its project."))
	}
	p, err := project.Load(root)
	if err != nil {
		return err
	}
	if p.Native {
		return fmt.Errorf("%s", i18n.Tr("launcher.remote.native_blocked", "Remote examples cannot start a native Go project."))
	}
	if err = l.Open(root); err != nil {
		return err
	}
	l.resultEntry, l.resultRemote = filepath.ToSlash(rel), true
	return nil
}

func (l *Launcher) remoteTab(ui *core.Context) {
	if l.opt.Remote == nil {
		return
	}
	l.heading(ui, i18n.Tr("launcher.remote.heading", "Remote examples"))
	widgets.LabelWrapped(ui, i18n.Tr("launcher.remote.explanation", "HTTPS examples are checked and copied locally. Aego does not run the project automatically; external import tools are disabled."), false)
	widgets.TextFieldRect(ui, ui.ID("remote.url"), widgets.Field(ui, i18n.Tr("launcher.remote.url", "Manifest HTTPS URL")), &l.remote.url, false)
	if l.remote.job != nil {
		widgets.LabelDim(ui, i18n.Tr("launcher.remote.working", "Checking or downloading the example…"))
		if widgets.Button(ui, i18n.Tr("launcher.remote.cancel", "Cancel")) {
			l.remote.job.canceled = true
			l.remote.job.cancel()
		}
		return
	}
	if l.remote.pending != nil {
		r := l.remote.pending
		widgets.LabelWrapped(ui, i18n.Tr("launcher.remote.confirm_origin", "Allow network access to this origin for this operation: {origin}", i18n.Str("origin", r.origin)), true)
		widgets.LabelWrapped(ui, r.source, true)
		if widgets.Button(ui, i18n.Tr("launcher.remote.confirm", "Confirm origin and continue")) {
			l.remote.pending = nil
			l.startRemote(*r)
		}
		if widgets.Button(ui, i18n.Tr("launcher.remote.cancel", "Cancel")) {
			l.remote.pending = nil
		}
		return
	}
	if widgets.Button(ui, i18n.Tr("launcher.remote.open", "Open saved copy or clone")) {
		l.confirmRemote("open", l.remote.url, "")
	}
	if widgets.Button(ui, i18n.Tr("launcher.remote.clone", "Clone a new copy")) {
		l.confirmRemote("clone", l.remote.url, "")
	}
	if widgets.Button(ui, i18n.Tr("launcher.remote.refresh", "Show saved remote examples")) {
		l.startRemote(remoteRequest{kind: "list"})
	}
	if l.remote.status != "" {
		widgets.LabelWrapped(ui, l.remote.status, false)
	}
	for _, item := range l.remote.copies {
		ui.PushID(item.ID)
		row := ui.Cursor().Next(0)
		buttonW := min(float32(120), max(float32(55), (row.W-8)/3))
		ui.TextIn(mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-2*buttonW-8), H: row.H}, item.Name+" · "+item.Version, ui.Theme.Text, core.AlignLeft)
		if widgets.ButtonRect(ui, ui.ID("open"), mathx.Rect{X: row.X + row.W - 2*buttonW - 4, Y: row.Y, W: buttonW, H: row.H}, i18n.Tr("launcher.remote.reopen", "Open offline")) {
			l.startRemote(remoteRequest{kind: "reopen", id: item.ID})
		}
		if widgets.ButtonRect(ui, ui.ID("check"), mathx.Rect{X: row.X + row.W - buttonW, Y: row.Y, W: buttonW, H: row.H}, i18n.Tr("launcher.remote.check", "Check updates")) {
			l.confirmRemote("check", item.Source, item.ID)
		}
		ui.PopID()
	}
}
