package studio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SkillInfo is the skill summary carried in State (§12.2).
type SkillInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
}

type HistoryFlags struct {
	CanUndo bool `json:"canUndo"`
	CanRedo bool `json:"canRedo"`
}

// State is what the browser renders (§12.2).
type State struct {
	Rev        int64                   `json:"rev"`
	Project    *Project                `json:"project"`
	View       SharedView              `json:"view"`
	History    map[string]HistoryFlags `json:"history"`
	Checks     Checks                  `json:"checks"`
	OpenIssues int                     `json:"openIssues"`
	Skills     []SkillInfo             `json:"skills"`
	Source     string                  `json:"source,omitempty"`
	Summary    string                  `json:"summary,omitempty"`
}

// Result is a committed action's response (§12.2, §13.4).
type Result struct {
	OK            bool       `json:"ok"`
	Rev           int64      `json:"rev"`
	Summary       string     `json:"summary"`
	ChangedIDs    []string   `json:"changedIds"`
	CreatedID     string     `json:"createdId,omitempty"`
	Selection     *Selection `json:"selection"`
	OpenIssues    int        `json:"openIssues"`
	FailingChecks Checks     `json:"failingChecks"`
}

// Publisher receives every new state (the SSE hub).
type Publisher interface {
	Publish(data []byte)
}

// Store is the single in-process owner of the project (§2).
type Store struct {
	mu      sync.Mutex
	dir     string // project directory (absolute)
	data    string // <dir>/.agentcanvas
	project *Project
	view    SharedView
	tabSel  map[string]*Selection
	hist    map[string]*history
	rev     int64

	pub    Publisher
	skills func() []SkillInfo
	now    func() time.Time
	logw   io.Writer

	saveTimer *time.Timer
	saveDelay time.Duration
	dirty     bool
}

const (
	projectKey = "agentcanvas.project.v1"
	uiKey      = "agentcanvas.ui.v1"
)

// Options configure a Store.
type Options struct {
	Log       io.Writer     // warnings (default os.Stderr)
	SaveDelay time.Duration // debounce (default 300ms)
	Now       func() time.Time
	InMemory  bool // never touch disk (tests)
}

// Open loads (or seeds) the project in <dir>/.agentcanvas.
func Open(dir string, o Options) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: abs, data: filepath.Join(abs, ".agentcanvas"), now: o.Now, logw: o.Log, saveDelay: o.SaveDelay,
		tabSel: map[string]*Selection{}, hist: map[string]*history{}}
	if s.now == nil {
		s.now = time.Now
	}
	if s.logw == nil {
		s.logw = os.Stderr
	}
	if s.saveDelay == 0 {
		s.saveDelay = 300 * time.Millisecond
	}
	for _, t := range Tabs {
		s.hist[t] = &history{}
	}
	s.view = DefaultView()
	if o.InMemory {
		s.data = ""
		s.project = MakeSeed()
		return s, nil
	}
	if err := os.MkdirAll(s.data, 0o755); err != nil {
		return nil, err
	}
	s.project = s.loadProject()
	s.loadUI()
	fixView(s.project, &s.view, s.tabSel)
	if err := s.writeFiles(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir is the project directory.
func (s *Store) Dir() string { return s.dir }

// DataDir is <project dir>/.agentcanvas.
func (s *Store) DataDir() string { return s.data }

// SetPublisher wires the SSE hub.
func (s *Store) SetPublisher(p Publisher) {
	s.mu.Lock()
	s.pub = p
	s.mu.Unlock()
}

// SetSkills wires the skills listing used in State and get_canvas.
func (s *Store) SetSkills(f func() []SkillInfo) {
	s.mu.Lock()
	s.skills = f
	s.mu.Unlock()
}

func (s *Store) warnf(format string, a ...any) {
	fmt.Fprintf(s.logw, "agent-canvas: warning: "+format+"\n", a...)
}

func (s *Store) quarantine(path string) {
	bad := fmt.Sprintf("%s.bad-%d", path, s.now().Unix())
	if err := os.Rename(path, bad); err != nil {
		s.warnf("could not rename %s: %v", path, err)
		return
	}
	s.warnf("%s was corrupt; moved it to %s and started fresh", filepath.Base(path), filepath.Base(bad))
}

var requiredKeys = []string{"version", "tokens", "flows", "nodes", "edges", "draw", "doc", "pins", "pinCounter"}

// migrate is the hook for future project versions (§12.1).
func migrate(p *Project) *Project { return p }

func parseProject(b []byte) (*Project, error) {
	var env struct {
		Key     string          `json:"key"`
		Project json.RawMessage `json:"project"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Key != projectKey {
		return nil, fmt.Errorf("key is %q, want %q", env.Key, projectKey)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(env.Project, &keys); err != nil {
		return nil, err
	}
	for _, k := range requiredKeys {
		if _, ok := keys[k]; !ok {
			return nil, fmt.Errorf("missing key %q", k)
		}
	}
	var p Project
	if err := json.Unmarshal(env.Project, &p); err != nil {
		return nil, err
	}
	if p.Version != 1 {
		return nil, fmt.Errorf("version %d, want 1", p.Version)
	}
	p.normalize()
	return migrate(&p), nil
}

func (s *Store) loadProject() *Project {
	path := filepath.Join(s.data, "project.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return MakeSeed()
	}
	if err != nil {
		s.warnf("reading %s: %v", path, err)
		return MakeSeed()
	}
	p, err := parseProject(b)
	if err != nil {
		s.warnf("project.json: %v", err)
		s.quarantine(path)
		return MakeSeed()
	}
	return p
}

type persistedView struct {
	Tab           string       `json:"tab"`
	View          ViewModes    `json:"view"`
	Breakpoint    string       `json:"breakpoint"`
	CurrentNodeID CurrentNodes `json:"currentNodeId"`
	ShowPins      bool         `json:"showPins"`
	ShowGrid      bool         `json:"showGrid"`
	ShowSpecs     bool         `json:"showSpecs"`
}

func (s *Store) loadUI() {
	path := filepath.Join(s.data, "ui.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var env struct {
		Key  string         `json:"key"`
		View *persistedView `json:"view"`
	}
	if err := json.Unmarshal(b, &env); err != nil || env.Key != uiKey || env.View == nil {
		if err == nil {
			err = fmt.Errorf("bad key or missing view")
		}
		s.warnf("ui.json: %v", err)
		s.quarantine(path)
		return
	}
	pv := env.View
	s.view = SharedView{Tab: pv.Tab, View: pv.View, Breakpoint: pv.Breakpoint, CurrentNodeID: pv.CurrentNodeID,
		ShowPins: pv.ShowPins, ShowGrid: pv.ShowGrid, ShowSpecs: pv.ShowSpecs}
}

func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// writeFiles writes project.json and ui.json now. Caller holds mu (or is Open).
func (s *Store) writeFiles() error {
	if s.data == "" {
		return nil
	}
	if err := writeAtomic(filepath.Join(s.data, "project.json"), map[string]any{"key": projectKey, "project": s.project}); err != nil {
		return err
	}
	v := s.view
	pv := persistedView{Tab: v.Tab, View: v.View, Breakpoint: v.Breakpoint, CurrentNodeID: v.CurrentNodeID,
		ShowPins: v.ShowPins, ShowGrid: v.ShowGrid, ShowSpecs: v.ShowSpecs}
	return writeAtomic(filepath.Join(s.data, "ui.json"), map[string]any{"key": uiKey, "view": pv})
}

func (s *Store) scheduleSave() {
	if s.data == "" {
		return
	}
	s.dirty = true
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(s.saveDelay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.flushLocked()
	})
}

func (s *Store) flushLocked() {
	if !s.dirty {
		return
	}
	if err := s.writeFiles(); err != nil {
		s.warnf("saving project: %v", err)
		return
	}
	s.dirty = false
}

// Flush writes pending changes now (shutdown).
func (s *Store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.flushLocked()
}

// ---- state ----

func (s *Store) historyFlags() map[string]HistoryFlags {
	m := map[string]HistoryFlags{}
	for _, t := range Tabs {
		h := s.hist[t]
		m[t] = HistoryFlags{CanUndo: len(h.past) > 0, CanRedo: len(h.future) > 0}
	}
	return m
}

func (s *Store) skillList() []SkillInfo {
	if s.skills == nil {
		return []SkillInfo{}
	}
	l := s.skills()
	if l == nil {
		return []SkillInfo{}
	}
	return l
}

func (s *Store) stateLocked() State {
	return State{
		Rev:        s.rev,
		Project:    s.project,
		View:       s.view,
		History:    s.historyFlags(),
		Checks:     ComputeChecks(s.project, s.view.Breakpoint),
		OpenIssues: OpenIssues(s.project),
		Skills:     s.skillList(),
	}
}

// StateJSON returns the current State, optionally tagged with source/summary.
func (s *Store) StateJSON(source, summary string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateJSONLocked(source, summary)
}

func (s *Store) stateJSONLocked(source, summary string) []byte {
	st := s.stateLocked()
	st.Source, st.Summary = source, summary
	b, err := json.Marshal(st)
	if err != nil {
		panic(err)
	}
	return b
}

// Snapshot returns a deep copy of the project and view for read-only use.
func (s *Store) Snapshot() (*Project, SharedView, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.project.Clone(), s.view.clone(), s.rev
}

func (s *Store) publishLocked(source, summary string) {
	if s.pub == nil {
		return
	}
	s.pub.Publish(s.stateJSONLocked(source, summary))
}

// Notify bumps rev and publishes without a project change (e.g. skills changed).
// SPEC: rev also increases for skills changes so browsers (which only accept a
// higher rev) pick up the new skills list.
func (s *Store) Notify(source, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rev++
	s.publishLocked(source, summary)
}

// ---- apply ----

// Apply runs one action from source ("ui", "mcp", "cli").
func (s *Store) Apply(a Action, source string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	before := s.project
	var out Outcome
	switch a.Type {
	case "undo", "redo":
		o, err := s.undoRedo(a)
		if err != nil {
			return nil, err
		}
		out = o
	case "reset_project":
		// SPEC: reset replaces the whole project with the seed, so pinCounter returns to 2.
		s.project = MakeSeed()
		for _, t := range Tabs {
			s.hist[t] = &history{}
		}
		s.view.Selection = nil
		s.view.Preview = false
		s.tabSel = map[string]*Selection{}
		fixView(s.project, &s.view, s.tabSel)
		out = Outcome{Summary: "Reset the project to the Hearth seed"}
	default:
		p := s.project.Clone()
		v := s.view.clone()
		ts := map[string]*Selection{}
		for k, x := range s.tabSel {
			ts[k] = x
		}
		o, err := Reduce(p, &v, ts, a, now)
		if err != nil {
			return nil, err
		}
		if o.History != "" {
			h := s.hist[o.History]
			prev := sliceJSON(s.project, o.History)
			next := sliceJSON(p, o.History)
			if !bytes.Equal(prev, next) {
				coalesced := a.Coalesce != "" && h.openKey == a.Coalesce && now.Sub(h.lastAt) < CoalesceWindow
				if !coalesced {
					h.push(prev)
				}
				h.future = nil
			}
			if a.Commit {
				h.openKey = ""
			} else {
				h.openKey = a.Coalesce
			}
			h.lastAt = now
		}
		s.project, s.view, s.tabSel = p, v, ts
		out = o
	}
	s.rev++
	s.scheduleSave()
	s.publishLocked(source, out.Summary)
	return &Result{
		OK:            true,
		Rev:           s.rev,
		Summary:       out.Summary,
		ChangedIDs:    ChangedIDs(before, s.project),
		CreatedID:     out.CreatedID,
		Selection:     s.view.Selection,
		OpenIssues:    OpenIssues(s.project),
		FailingChecks: ComputeChecks(s.project, s.view.Breakpoint).Failing(),
	}, nil
}

func (s *Store) undoRedo(a Action) (Outcome, error) {
	var pl struct {
		Tab string `json:"tab"`
	}
	if len(a.Payload) > 0 && string(a.Payload) != "null" {
		if err := json.Unmarshal(a.Payload, &pl); err != nil {
			return Outcome{}, errInvalid("bad payload: "+err.Error(), "send {\"tab\":\"web\"}")
		}
	}
	tab := pl.Tab
	if tab == "" {
		tab = s.view.Tab
	}
	if !contains(Tabs, tab) {
		return Outcome{}, errInvalid(fmt.Sprintf("tab must be one of web, story, draw, doc (got %q)", tab), "send a valid tab")
	}
	h := s.hist[tab]
	from, to := &h.past, &h.future
	verb := "Undid"
	if a.Type == "redo" {
		from, to = &h.future, &h.past
		verb = "Redid"
	}
	if len(*from) == 0 {
		return Outcome{}, errInv("empty-history", fmt.Sprintf("nothing to %s in the %s history", a.Type, tab), "make a change first, or check history flags in get_canvas")
	}
	snap := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, sliceJSON(s.project, tab))
	if len(h.past) > HistoryCap {
		h.past = h.past[len(h.past)-HistoryCap:]
	}
	h.openKey = ""
	p := s.project.Clone()
	restoreSlice(p, tab, snap)
	s.project = p
	fixView(s.project, &s.view, s.tabSel)
	return Outcome{Summary: fmt.Sprintf("%s the last %s change", verb, tabTitle(tab))}, nil
}
