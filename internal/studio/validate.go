package studio

import (
	"errors"
	"fmt"
	"os"
	"regexp"
)

var (
	idRe       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	strictHex  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	blockTypes = BlockTypes
	edgeKinds  = []string{"primary", "alt", "back", "cross"}
	ctaSizes   = []string{"sm", "md", "lg"}
	aligns     = []string{"left", "center"}
	layouts    = []string{"center", "bottom", "split"}
	pinTargets = []string{"node", "board", "draw"}
	channels   = []string{ChanWeb, ChanMobile}
)

// Validate checks a loaded project field by field: every id, enum, token, and
// color key must be one the reducer could have produced, and every §6.1 invariant
// must hold. Exports interpolate these values into HTML/CSS, so a hand-edited or
// hostile project.json is quarantined instead of trusted (§11.6).
func Validate(p *Project) error {
	var errs []error
	bad := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	id := func(what, v string) {
		if !idRe.MatchString(v) {
			bad("%s id %q is not [A-Za-z0-9_-]{1,64}", what, v)
		}
	}
	enum := func(what, v string, allowed []string) {
		if !contains(allowed, v) {
			bad("%s %q is not one of %v", what, v, allowed)
		}
	}
	space := func(what string, v int) {
		if v < 0 || v > 7 {
			bad("%s %d is not a space index 0–7", what, v)
		}
	}
	weight := func(what string, v int) {
		if v != 400 && v != 500 && v != 600 {
			bad("%s %d is not 400, 500, or 600", what, v)
		}
	}
	ref := func(what string, r Ref) {
		if r != "" {
			id(what, string(r))
		}
	}

	for _, k := range ColorKeys {
		if !strictHex.MatchString(p.Tokens.Colors[k]) {
			bad("token color %s %q is not #RRGGBB", k, p.Tokens.Colors[k])
		}
	}
	if len(p.Tokens.Colors) != len(ColorKeys) {
		bad("tokens.colors must have exactly %v", ColorKeys)
	}
	for _, k := range TypeKeys {
		t, ok := p.Tokens.Type[k]
		if !ok || t.Size < 6 || t.Size > 200 || t.LH < 6 || t.LH > 300 {
			bad("token type %s is missing or out of range", k)
		}
		weight("token type "+k+" weight", t.Weight)
	}
	if len(p.Tokens.Type) != len(TypeKeys) {
		bad("tokens.type must have exactly %v", TypeKeys)
	}

	for _, f := range p.Flows {
		id("flow", f.ID)
		enum("flow channel", f.Channel, channels)
		ref("flow startId", f.StartID)
	}
	for _, n := range p.Nodes {
		id("node", n.ID)
		id("node flowId", n.FlowID)
		switch n.Kind {
		case "page":
			for _, b := range n.Blocks {
				id("block", b.ID)
				enum("block type", b.Type, blockTypes)
				enum("block fill", b.Fill, ColorKeys)
				enum("block ink", b.Ink, ColorKeys)
				space("block pad", b.Pad)
				space("block gap", b.Gap)
				enum("block typeRole", b.TypeRole, TypeKeys)
				weight("block weight", b.Weight)
				enum("block align", b.Align, aligns)
				if b.Type == "hero" {
					enum("hero bodyRole", b.BodyRole, TypeKeys)
				}
				if b.Type == "cta" {
					enum("cta buttonFill", b.ButtonFill, ColorKeys)
					enum("cta buttonInk", b.ButtonInk, ColorKeys)
				}
				for _, l := range append(append([]*Link{}, b.Links...), b.CTA, b.Secondary, b.Button) {
					if l != nil && (l.ID != "" || l.Label != "" || l.ToID != "") {
						id("link", l.ID)
						ref("link toId", l.ToID)
					}
				}
				for _, it := range b.Items {
					id("feature item", it.ID)
				}
			}
		case "screen":
			enum("screen archetype", n.Archetype, archetypes)
			enum("screen layout", n.Layout, layouts)
			enum("screen fill", n.Fill, ColorKeys)
			enum("screen ink", n.Ink, ColorKeys)
			enum("screen typeRole", n.TypeRole, TypeKeys)
			weight("screen weight", n.Weight)
			enum("screen align", n.Align, aligns)
			space("screen pad", n.Pad)
			space("screen gap", n.Gap)
			if n.CTA != nil {
				enum("screen cta size", n.CTA.Size, ctaSizes)
				enum("screen cta fill", n.CTA.Fill, ColorKeys)
				enum("screen cta ink", n.CTA.Ink, ColorKeys)
				ref("screen cta toId", n.CTA.ToID)
			}
			for _, h := range n.Hotspots {
				id("hotspot", h.ID)
				ref("hotspot toId", h.ToID)
			}
		default:
			bad("node %s kind %q is not page or screen", n.ID, n.Kind)
		}
	}
	for _, e := range p.Edges {
		id("edge", e.ID)
		enum("edge kind", e.Kind, edgeKinds)
	}
	for _, s := range p.Draw.Shapes {
		id("shape", s.ID)
		enum("shape type", s.Type, ShapeTypes)
		switch s.Type {
		case "text":
			enum("text role", s.Role, TypeKeys)
			enum("text ink", s.Ink, ColorKeys)
		default:
			enum("shape stroke", s.Stroke, ColorKeys)
			if s.Type == "rect" || s.Type == "ellipse" {
				enum("shape fill", s.Fill, append(append([]string{}, ColorKeys...), "none"))
			}
		}
	}
	for _, x := range p.Pins {
		id("pin", x.ID)
		enum("pin kind", x.Kind, pinKinds)
		enum("pin target", x.Target.Type, pinTargets)
		if x.Target.NodeID != "" {
			id("pin target node", x.Target.NodeID)
		}
		if x.Target.BlockID != "" {
			id("pin target block", x.Target.BlockID)
		}
	}
	for _, msg := range CheckInvariants(p) {
		bad("%s", msg)
	}
	if len(errs) > 5 {
		errs = append(errs[:5], fmt.Errorf("… and %d more", len(errs)-5))
	}
	return errors.Join(errs...)
}

// noSymlink refuses a path that exists and is a symbolic link, so writes under the
// project directory cannot be redirected outside it by a checked-out symlink.
func noSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link; refusing to write through it", path)
	}
	return nil
}
