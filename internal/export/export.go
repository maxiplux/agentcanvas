// Package export generates the §11.4 / §11.5 files from project data.
// Exports are built from the project, never from the live DOM.
package export

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"agentcanvas/internal/studio"
)

// File is one generated export.
type File struct {
	Name        string
	ContentType string
	Body        []byte
}

// Kinds lists the export kinds in Export-menu order.
var Kinds = []string{"page", "web-flows", "mobile-flow", "draw-svg", "design-md"}

const (
	ctHTML = "text/html; charset=utf-8"
	ctSVG  = "image/svg+xml"
	ctMD   = "text/markdown; charset=utf-8"
)

// fontsLink is allowed in exports: the files are meant for use elsewhere (§11.4).
const fontsLink = `<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>` +
	`<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,400;9..144,500;9..144,600&family=Inter:wght@400;500;600&display=swap">`

// Generate builds the export of the given kind.
func Generate(p *studio.Project, kind string, q url.Values) (*File, error) {
	if p == nil {
		return nil, fmt.Errorf("no project")
	}
	switch kind {
	case "page":
		return pageFile(p, q.Get("nodeId"))
	case "web-flows":
		return webFlowsFile(p)
	case "mobile-flow":
		return mobileFlowFile(p, q.Get("flowId"))
	case "draw-svg":
		return drawFile(p), nil
	case "design-md":
		return &File{Name: "DESIGN.md", ContentType: ctMD, Body: []byte(DesignMD(p))}, nil
	}
	return nil, fmt.Errorf("unknown export kind %q (valid kinds: %s)", kind, strings.Join(Kinds, ", "))
}

// Write saves f into the project directory (DESIGN.md at the root, everything
// else under .agentcanvas/exports/) and returns the absolute path. Writing
// replaces the file.
func Write(projectDir, kind string, f *File) (string, error) {
	if f == nil {
		return "", fmt.Errorf("nothing to write")
	}
	known := false
	for _, k := range Kinds {
		if k == kind {
			known = true
		}
	}
	if !known {
		return "", fmt.Errorf("unknown export kind %q (valid kinds: %s)", kind, strings.Join(Kinds, ", "))
	}
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	dir := abs
	name := "DESIGN.md"
	if kind != "design-md" {
		dir = filepath.Join(abs, ".agentcanvas", "exports")
		name = filepath.Base(f.Name)
		if name == "." || name == string(filepath.Separator) || name == "" {
			return "", fmt.Errorf("export has no file name")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, "."+name+".tmp-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(f.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

// ---- names ----

// Slug lowercases s and joins runs of letters/digits with hyphens.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return b.String()
}

// ProjectName is the first `#` heading of the doc up to the first " — ".
func ProjectName(p *studio.Project) string {
	for _, line := range strings.Split(p.Doc.Markdown, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "# ") {
			name := strings.TrimSpace(line[2:])
			if i := strings.Index(name, " — "); i >= 0 {
				name = name[:i]
			}
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// projectSlug is the file-name prefix ("hearth", or "project").
func projectSlug(p *studio.Project) string {
	if s := Slug(ProjectName(p)); s != "" {
		return s
	}
	return "project"
}

func displayName(p *studio.Project) string {
	if n := ProjectName(p); n != "" {
		return n
	}
	return "Project"
}

// nodeSlugs assigns every node of a channel a unique slug (dedupe with -2,
// -3 …), in node order. Each export file holds one channel, so slugs only need
// to be unique within it.
func nodeSlugs(p *studio.Project, ch string) map[string]string {
	out := map[string]string{}
	used := map[string]bool{}
	for _, n := range p.Nodes {
		if n.Channel() != ch {
			continue
		}
		base := Slug(n.Name)
		if base == "" {
			base = n.Kind
		}
		s := base
		for i := 2; used[s]; i++ {
			s = base + "-" + strconv.Itoa(i)
		}
		used[s] = true
		out[n.ID] = s
	}
	return out
}

// ---- shared helpers ----

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func color(p *studio.Project, key string) string {
	if c, ok := p.Tokens.Colors[key]; ok {
		return c
	}
	return "#000000"
}

func space(p *studio.Project, i int) int {
	sp := p.Tokens.Space
	if len(sp) != 8 {
		sp = studio.SpaceScale
	}
	if i < 0 {
		i = 0
	}
	if i > 7 {
		i = 7
	}
	return sp[i]
}

func typeSpec(p *studio.Project, role string) studio.TypeSpec {
	if t, ok := p.Tokens.Type[role]; ok {
		return t
	}
	return studio.TypeSpec{Size: 17, LH: 26, Weight: 400}
}

// firstWebNode is the first web flow's start node, else the first page.
func firstWebNode(p *studio.Project) *studio.Node {
	for _, f := range p.FlowsOf(studio.ChanWeb) {
		if f.StartID != "" {
			if n := p.Node(string(f.StartID)); n != nil && n.Kind == "page" {
				return n
			}
		}
	}
	for _, n := range p.Nodes {
		if n.Kind == "page" {
			return n
		}
	}
	return nil
}
