// Package skills validates, installs, lists, removes, and exports SKILL.md
// packages, and serves the embedded built-in skill (§14). Skills are text:
// they never add tools, change studio behavior, or run code.
package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	builtinskills "agentcanvas/skills"

	"agentcanvas/internal/studio"
)

// BuiltinName is the embedded skill that is always listed and cannot be removed.
const BuiltinName = "agentcanvas"

// MaxPackageBytes caps the total size of a skill folder (§14.1).
const MaxPackageBytes = 1 << 20

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
)

// Skill is a skill with its SKILL.md body.
type Skill struct {
	studio.SkillInfo
	Body string `json:"body"`
}

// Manifest is skill.json.
type Manifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// Store is the per-user skill folder plus the built-in skill.
type Store struct {
	dir string

	mu       sync.Mutex
	cacheSig string
	cache    []studio.SkillInfo
}

// Open returns a Store rooted at dir. The folder is created on first install.
func Open(dir string) *Store { return &Store{dir: dir} }

// Dir is the folder installed skills live in.
func (s *Store) Dir() string { return s.dir }

// DefaultDir is os.UserConfigDir()/agent-canvas/skills.
// SPEC: AGENT_CANVAS_SKILLS_DIR overrides it so tests never touch the real config dir.
func DefaultDir() (string, error) {
	if d := os.Getenv("AGENT_CANVAS_SKILLS_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agent-canvas", "skills"), nil
}

// ---- validation ----

// ValidName reports whether name is a valid kebab-case skill name.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// parseFrontmatter reads the leading YAML frontmatter of SKILL.md as flat key: value pairs.
func parseFrontmatter(md string) (map[string]string, error) {
	md = strings.TrimPrefix(md, "\ufeff")
	md = strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(md, "---\n") {
		return nil, errors.New("SKILL.md must start with YAML frontmatter (---)")
	}
	rest := md[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		if strings.HasPrefix(rest, "---") {
			end = 0
		} else {
			return nil, errors.New("SKILL.md frontmatter is not closed with ---")
		}
	}
	out := map[string]string{}
	for _, line := range strings.Split(rest[:end], "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // nested values are allowed but ignored
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			if v[0] == '"' {
				var s string
				if json.Unmarshal([]byte(v), &s) == nil {
					v = s
				} else {
					v = v[1 : len(v)-1]
				}
			} else {
				v = strings.ReplaceAll(v[1:len(v)-1], "''", "'")
			}
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}

// validateFS checks a skill package in fsys (rooted at the skill folder) whose folder name is folder.
func validateFS(fsys fs.FS, folder string) (*Manifest, string, error) {
	raw, err := fs.ReadFile(fsys, "skill.json")
	if err != nil {
		return nil, "", errors.New("missing skill.json")
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&m); err != nil {
		return nil, "", fmt.Errorf("skill.json: %v", err)
	}
	if !ValidName(m.Name) {
		return nil, "", fmt.Errorf("skill.json name %q must be kebab-case (^[a-z0-9]+(-[a-z0-9]+)*$)", m.Name)
	}
	if m.Name != folder {
		return nil, "", fmt.Errorf("skill.json name %q must equal the folder name %q", m.Name, folder)
	}
	if !semverRe.MatchString(m.Version) {
		return nil, "", fmt.Errorf("skill.json version %q is not semver (e.g. 1.0.0)", m.Version)
	}
	if strings.TrimSpace(m.Description) == "" || strings.ContainsAny(m.Description, "\r\n") {
		return nil, "", errors.New("skill.json description must be one non-empty line")
	}
	body, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return nil, "", errors.New("missing SKILL.md")
	}
	fm, err := parseFrontmatter(string(body))
	if err != nil {
		return nil, "", err
	}
	if fm["name"] != m.Name {
		return nil, "", fmt.Errorf("SKILL.md frontmatter name %q must equal skill.json name %q", fm["name"], m.Name)
	}
	if fm["description"] != m.Description {
		return nil, "", fmt.Errorf("SKILL.md frontmatter description must equal skill.json description")
	}
	return &m, string(body), nil
}

// Validate checks the skill folder at dir (§14.1): no symlinks, ≤1 MB, valid
// skill.json and SKILL.md frontmatter, name equal to the folder name.
func Validate(dir string) (*Manifest, error) {
	st, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink; skills may not contain symlinks", dir)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", dir)
	}
	var total int64
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed (%s)", p)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("only regular files are allowed (%s)", p)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > MaxPackageBytes {
			return fmt.Errorf("skill is larger than 1 MB")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	m, _, err := validateFS(os.DirFS(dir), filepath.Base(dir))
	return m, err
}

// ---- built-in ----

func builtinFS() fs.FS {
	sub, err := fs.Sub(builtinskills.FS, BuiltinName)
	if err != nil {
		panic(err)
	}
	return sub
}

// Builtin returns the embedded agentcanvas skill.
func Builtin() Skill {
	m, body, err := validateFS(builtinFS(), BuiltinName)
	if err != nil {
		panic("built-in skill is invalid: " + err.Error())
	}
	return Skill{SkillInfo: studio.SkillInfo{Name: m.Name, Version: m.Version, Description: m.Description, Builtin: true}, Body: body}
}

// ---- listing ----

func (s *Store) signature() string {
	st, err := os.Stat(s.dir)
	if err != nil {
		return "missing"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d", st.ModTime().UnixNano())
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "|%s:%d", e.Name(), info.ModTime().UnixNano())
	}
	return b.String()
}

func (s *Store) scan() []studio.SkillInfo {
	b := Builtin()
	out := []studio.SkillInfo{b.SkillInfo}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return out
	}
	var installed []studio.SkillInfo
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Name() == BuiltinName || !e.IsDir() {
			continue
		}
		m, err := Validate(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue // invalid folders are skipped
		}
		installed = append(installed, studio.SkillInfo{Name: m.Name, Version: m.Version, Description: m.Description})
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].Name < installed[j].Name })
	return append(out, installed...)
}

// List returns the built-in skill first, then installed skills sorted by name.
// It rescans the folder whenever its contents changed.
func (s *Store) List() []studio.SkillInfo {
	sig := s.signature()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil || sig != s.cacheSig {
		s.cache = s.scan()
		s.cacheSig = sig
	}
	return append([]studio.SkillInfo(nil), s.cache...)
}

// ListFull returns every skill with its SKILL.md body.
func (s *Store) ListFull() []Skill {
	var out []Skill
	for _, info := range s.List() {
		if sk, err := s.Get(info.Name); err == nil {
			out = append(out, *sk)
		}
	}
	return out
}

// Get returns one skill with its body.
func (s *Store) Get(name string) (*Skill, error) {
	if name == BuiltinName {
		b := Builtin()
		return &b, nil
	}
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid skill name %q", name)
	}
	dir := filepath.Join(s.dir, name)
	m, err := Validate(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %q is not installed", name)
		}
		return nil, fmt.Errorf("skill %q is invalid: %v", name, err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	return &Skill{SkillInfo: studio.SkillInfo{Name: m.Name, Version: m.Version, Description: m.Description}, Body: string(body)}, nil
}

// ---- install / remove / export ----

// copyTree copies every regular file of src (an fs.FS) into dst on disk.
func copyTree(src fs.FS, dst string) error {
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed (%s)", p)
		}
		in, err := src.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// placeAtomically copies src into <parent>/<name> via a temp dir and rename.
// An existing target is replaced only when force is set.
func placeAtomically(src fs.FS, parent, name string, force bool) (string, error) {
	target := filepath.Join(parent, name)
	if _, err := os.Lstat(target); err == nil && !force {
		return "", fmt.Errorf("%s already exists; use --force (or force: true) to replace it", target)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-"+name+"-")
	if err != nil {
		return "", err
	}
	if err := copyTree(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	var old string
	if _, err := os.Lstat(target); err == nil {
		old = filepath.Join(parent, fmt.Sprintf(".old-%s-%d", name, time.Now().UnixNano()))
		if err := os.Rename(target, old); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		os.RemoveAll(tmp)
		if old != "" {
			os.Rename(old, target)
		}
		return "", err
	}
	if old != "" {
		os.RemoveAll(old)
	}
	return target, nil
}

// Install validates the folder at path and copies it into the store,
// all-or-nothing. An installed skill of the same name is replaced only with force.
func (s *Store) Install(path string, force bool) (*studio.SkillInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	m, err := Validate(abs)
	if err != nil {
		return nil, fmt.Errorf("invalid skill package %s: %v", abs, err)
	}
	if m.Name == BuiltinName {
		return nil, fmt.Errorf("%q is the built-in skill and cannot be installed over", BuiltinName)
	}
	if _, err := os.Lstat(filepath.Join(s.dir, m.Name)); err == nil && !force {
		return nil, fmt.Errorf("skill %q is already installed; use --force (or force: true) to replace it", m.Name)
	}
	if _, err := placeAtomically(os.DirFS(abs), s.dir, m.Name, force); err != nil {
		return nil, err
	}
	s.invalidate()
	return &studio.SkillInfo{Name: m.Name, Version: m.Version, Description: m.Description}, nil
}

// Remove deletes an installed skill. The built-in skill cannot be removed.
func (s *Store) Remove(name string) error {
	if name == BuiltinName {
		return fmt.Errorf("%q is the built-in skill and cannot be removed", BuiltinName)
	}
	if !ValidName(name) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	target := filepath.Join(s.dir, name)
	if _, err := os.Lstat(target); err != nil {
		return fmt.Errorf("skill %q is not installed", name)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// Export copies a built-in or installed skill to <targetDir>/<name>/ and
// returns that path. An existing target is refused without force.
func (s *Store) Export(name, targetDir string, force bool) (string, error) {
	var src fs.FS
	if name == BuiltinName {
		src = builtinFS()
	} else {
		if !ValidName(name) {
			return "", fmt.Errorf("invalid skill name %q", name)
		}
		dir := filepath.Join(s.dir, name)
		if _, err := Validate(dir); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("skill %q is not installed", name)
			}
			return "", err
		}
		src = os.DirFS(dir)
	}
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return "", err
	}
	return placeAtomically(src, abs, name, force)
}

func (s *Store) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// Watch rescans the skills folder every interval and calls onChange when its
// contents change, until ctx ends.
func (s *Store) Watch(ctx context.Context, interval time.Duration, onChange func()) {
	last := s.signature()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if sig := s.signature(); sig != last {
				last = sig
				s.invalidate()
				if onChange != nil {
					onChange()
				}
			}
		}
	}
}
