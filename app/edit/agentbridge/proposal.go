package agentbridge

import (
	"errors"
	"maps"
	"strings"
	"time"

	"aego/app/agent"
	"aego/editor/core/document"
)

type State uint8

const (
	Pending State = iota
	Approved
	Applied
	Rejected
	Expired
	Failed
	Undone
)

const (
	DefaultTimeout = 10 * time.Minute
	keepProposals  = 200
)

var (
	errRejected  = errors.New("the user rejected this change; ask before trying it again")
	errExpired   = errors.New("no answer within the time limit, nothing was applied")
	errCancelled = errors.New("the agent conversation was cancelled; nothing was applied")
)

// undoRef is where an applied proposal sits in the history of its document.
type undoRef struct {
	doc     document.ID
	label   string
	depth   int
	script  *scriptUndo
	drawing *drawingCommand
}

// Proposal is one write call from the model, previewed as a dry run and waiting for the user (D342).
type Proposal struct {
	ID          int
	Command     string
	Args        agent.Args
	Preview     *agent.Output
	Destructive bool
	State       State
	Result      *agent.Output
	Err         string
	undo        *undoRef
	decide      chan bool
	// Via names where the change came from when it was not the panel's agent: "via CLI", "via agent: claude-code".
	Via string
	// generation is zero for external calls, which outlive the panel conversation.
	generation uint64
}

func destructive(c *agent.Command, out *agent.Output) bool {
	if c.Destroys {
		return true
	}
	for _, ch := range out.Changes {
		if strings.HasPrefix(ch.Op, "remove") {
			return true
		}
	}
	return false
}

func problemsErr(out *agent.Output) error {
	var msgs []string
	for _, p := range out.Problems {
		if p.Error {
			msgs = append(msgs, p.Message)
		}
	}
	return errors.New("the change has errors, nothing was applied: " + strings.Join(msgs, "; "))
}

func (b *Bridge) timeout() time.Duration {
	if b.cfg.Timeout > 0 {
		return b.cfg.Timeout
	}
	return DefaultTimeout
}

// propose previews a write call on the main thread, waits for the user off it, then applies it on the current state.
func (b *Bridge) propose(c *agent.Command, a agent.Args, generation uint64) (*agent.Output, error) {
	return b.proposeVia(c, a, "", false, generation)
}

// proposeVia previews a write and applies it at once when direct, else waits for the user like the panel's own agent.
func (b *Bridge) proposeVia(c *agent.Command, a agent.Args, via string, direct bool, generation uint64) (*agent.Output, error) {
	var p *Proposal
	var err error
	var st State
	var res *agent.Output
	var msg string
	if e := b.main(func() {
		if b.cancelled(generation) {
			err = errCancelled
			return
		}
		if p, err = b.preview(c, a, via, direct); p != nil {
			p.generation = generation
			st, res, msg = p.State, p.Result, p.Err
		}
	}); e != nil {
		return nil, e
	}
	if err != nil {
		return nil, err
	}
	switch st {
	case Applied:
		return res, nil
	case Failed:
		return nil, errors.New(msg)
	}
	ok, err := b.await(p)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errRejected
	}
	var out *agent.Output
	if e := b.main(func() {
		if b.cancelled(generation) {
			p.State, p.Err, err = Rejected, errCancelled.Error(), errCancelled
			return
		}
		out, err = b.apply(p)
	}); e != nil {
		return nil, e
	}
	return out, err
}

func (b *Bridge) preview(c *agent.Command, a agent.Args, via string, direct bool) (*Proposal, error) {
	dry := maps.Clone(a)
	dry["dry-run"] = true
	out, err := agent.Run(b.sess, c.Name, dry)
	if err != nil {
		return nil, err
	}
	if out.Errors() {
		return nil, problemsErr(out)
	}
	b.next++
	p := &Proposal{ID: b.next, Command: c.Name, Args: maps.Clone(a), Preview: out, Destructive: destructive(c, out), decide: make(chan bool, 1), Via: via}
	b.keep(p)
	if direct || b.auto && !p.Destructive {
		b.apply(p)
		return p, nil
	}
	if b.cfg.OnProposal != nil {
		b.cfg.OnProposal(p)
	}
	return p, nil
}

// keep holds the latest proposals for the panel; pending ones are never dropped.
func (b *Bridge) keep(p *Proposal) {
	b.proposals = append(b.proposals, p)
	for len(b.proposals) > keepProposals {
		i := 0
		for i < len(b.proposals) && b.proposals[i].State == Pending {
			i++
		}
		if i == len(b.proposals) {
			return
		}
		b.proposals = append(b.proposals[:i], b.proposals[i+1:]...)
	}
}

func (b *Bridge) apply(p *Proposal) (*agent.Output, error) {
	b.last, b.via = nil, p.Via
	defer func() { b.via = "" }()
	var out *agent.Output
	var err error
	if p.Preview.Drawing != nil {
		out, err = b.applyDrawing(p)
	} else {
		out, err = b.run(p.Command, p.Args)
	}
	p.undo = b.last
	if err != nil {
		p.State, p.Err = Failed, err.Error()
		return nil, err
	}
	p.State, p.Result = Applied, out
	return out, nil
}

func (b *Bridge) await(p *Proposal) (bool, error) {
	t := time.NewTimer(b.timeout())
	defer t.Stop()
	select {
	case ok := <-p.decide:
		return ok, nil
	case <-t.C:
	}
	expired := false
	if e := b.main(func() {
		if p.State == Pending {
			p.State, expired = Expired, true
		}
	}); e != nil {
		return false, e
	}
	if expired {
		return false, errExpired
	}
	return <-p.decide, nil
}

func (b *Bridge) find(id int) *Proposal {
	for _, p := range b.proposals {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Proposals are the recent proposals, oldest first; read them on the main thread.
func (b *Bridge) Proposals() []*Proposal {
	return b.proposals
}

func (b *Bridge) Pending() []*Proposal {
	var out []*Proposal
	for _, p := range b.proposals {
		if p.State == Pending {
			out = append(out, p)
		}
	}
	return out
}

// Decide applies or rejects a pending proposal; it reports false when the proposal is no longer pending.
func (b *Bridge) Decide(id int, apply bool) bool {
	p := b.find(id)
	if p == nil || p.State != Pending {
		return false
	}
	p.State = Rejected
	if apply {
		p.State = Approved
	}
	p.decide <- apply
	return true
}

func (b *Bridge) RejectAll() {
	b.panelGeneration.Add(1)
	if closer, ok := b.cfg.Exporter.(interface{ CloseAgentPreview() }); ok {
		closer.CloseAgentPreview()
	}
	for _, p := range b.Pending() {
		b.Decide(p.ID, false)
	}
	for _, p := range b.proposals {
		if p.generation != 0 && p.State == Approved {
			p.State, p.Err = Rejected, errCancelled.Error()
		}
	}
	b.dismissQuestions()
}

func (b *Bridge) cancelled(generation uint64) bool {
	return generation != 0 && generation != b.panelGeneration.Load()
}

var errPlanOnly = errors.New("the user turned on Plan only: describe the changes and their order instead of making them")

func (b *Bridge) PlanOnly() bool {
	var on bool
	b.planMu.Lock()
	on = b.planOnly
	b.planMu.Unlock()
	return on
}

// SetPlanOnly refuses write tools, so the agent plans the work and the user reads the plan first.
func (b *Bridge) SetPlanOnly(on bool) {
	b.planMu.Lock()
	b.planOnly = on
	b.planMu.Unlock()
}

func (b *Bridge) AutoApply() bool {
	return b.auto
}

// SetAutoApply applies new proposals at once, except destructive ones.
func (b *Bridge) SetAutoApply(on bool) {
	b.auto = on
}

// CanUndo reports whether an applied proposal is still the last step of its document.
func (b *Bridge) CanUndo(id int) bool {
	p := b.find(id)
	if p == nil || p.undo == nil || p.State != Applied {
		return false
	}
	if p.undo.drawing != nil {
		if p.undo.drawing.check(true) != nil {
			return false
		}
		if p.undo.doc == 0 {
			return true
		}
	}
	if p.undo.script != nil {
		return b.canUndoScript(p.undo.script)
	}
	d := b.ctx.Docs.Get(p.undo.doc)
	if d == nil {
		return false
	}
	label, depth := d.History.Top()
	return depth == p.undo.depth && label == p.undo.label
}

func (b *Bridge) Undo(id int) error {
	if !b.CanUndo(id) {
		return errors.New("this change is no longer the last step; undo it with Edit → Undo")
	}
	p := b.find(id)
	if p.undo.drawing != nil && p.undo.doc == 0 {
		if err := p.undo.drawing.Undo(nil); err != nil {
			return err
		}
		p.State = Undone
		return nil
	}
	if p.undo.script != nil {
		if err := b.undoScript(p.undo.script); err != nil {
			return err
		}
		p.State = Undone
		return nil
	}
	d := b.ctx.Docs.Get(p.undo.doc)
	if err := d.History.Undo(); err != nil {
		return err
	}
	b.ctx.PruneSelectionOf(d.ID)
	p.State = Undone
	return nil
}

func (b *Bridge) note(d *document.SceneDocument, label string) {
	_, depth := d.History.Top()
	b.last = &undoRef{doc: d.ID, label: label, depth: depth}
}
