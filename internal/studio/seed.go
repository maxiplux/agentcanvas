package studio

import "fmt"

// SeedMarkdown is the §7.5 DESIGN.md.
const SeedMarkdown = `# Hearth — DESIGN.md

## Intent
Hearth sells small-batch ceramics directly from the studio. The site and app should feel like the studio: unhurried, warm, exact. One clear path to buy; no dark patterns.

## Users
- **Returning buyers** who know the glaze they want and need to reorder in under a minute.
- **Gift shoppers** who need confidence (photos, size, shipping date) before paying.
- **The studio** (two people) who update stock from a phone between firings.

## Principles
1. **Paper first.** Warm paper background, charcoal ink, one vermillion accent. Color is for action, not decoration.
2. **Serif for the product, grotesque for the tools.** Content should read like a printed catalogue.
3. **One accent per screen.** If two things are vermillion, one of them is wrong.
4. **Thumbs before cursors.** Every mobile target is at least 44px.
5. **Say less.** Headlines under eight words; bodies under thirty.

## Decisions
- Checkout is its own flow so Marketing can change without touching payment.
- Onboarding ends by jumping into Account › Home; there is no separate "done" screen.
- Shop and Cart share the nav so the cart is always one tap away.
- Features grid collapses 3 → 2 → 1 rather than scrolling horizontally.
`

// DefaultTokens returns the §5 seed tokens.
func DefaultTokens() Tokens {
	return Tokens{
		Colors: map[string]string{
			"paper":  "#F4EFE6",
			"ink":    "#1C1B19",
			"muted":  "#6B665C",
			"accent": "#D8432A",
			"wash":   "#E9E2D4",
			"line":   "#CFC6B5",
		},
		Type: map[string]TypeSpec{
			"xs":      {12, 16, 400},
			"sm":      {14, 20, 400},
			"md":      {17, 26, 400},
			"lg":      {22, 30, 500},
			"xl":      {32, 38, 500},
			"display": {52, 56, 600},
		},
		Space: append([]int(nil), SpaceScale...),
	}
}

func lnk(id, label, to string) *Link { return &Link{ID: id, Label: label, ToID: Ref(to)} }

func seedNav(prefix, nodeID string) *Block {
	return &Block{ID: "bk_" + prefix + "_nav", Type: "nav", Fill: "paper", Ink: "ink", Pad: 3, Gap: 4,
		TypeRole: "sm", Weight: 400, Align: "left", Brand: "Hearth",
		Links: []*Link{
			lnk("ln_"+prefix+"_shop", "Shop", "pg_shop"),
			lnk("ln_"+prefix+"_story", "Our story", ""),
			lnk("ln_"+prefix+"_cart", "Cart", "pg_cart"),
		}}
}

func seedFooter(blockPrefix, linkPrefix string) *Block {
	return &Block{ID: "bk_" + blockPrefix + "_footer", Type: "footer", Fill: "ink", Ink: "paper", Pad: 4, Gap: 3,
		TypeRole: "sm", Weight: 400, Align: "left", Brand: "Hearth", Note: "© Hearth Studio",
		Links: []*Link{
			lnk("ln_"+linkPrefix+"_story", "Our story", ""),
			lnk("ln_"+linkPrefix+"_contact", "Contact", ""),
		}}
}

// MakeSeed returns the exact §7 starter project.
func MakeSeed() *Project {
	p := &Project{Version: 1, Tokens: DefaultTokens()}

	p.Flows = []*Flow{
		{ID: "fl_marketing", Name: "Marketing", Channel: ChanWeb, StartID: "pg_home"},
		{ID: "fl_checkout", Name: "Checkout", Channel: ChanWeb, StartID: "pg_cart"},
		{ID: "fl_onboarding", Name: "Onboarding", Channel: ChanMobile, StartID: "sc_welcome"},
		{ID: "fl_account", Name: "Account", Channel: ChanMobile, StartID: "sc_home"},
	}

	home := &Node{Kind: "page", ID: "pg_home", FlowID: "fl_marketing", Name: "Home", Blocks: []*Block{
		seedNav("home", "pg_home"),
		{ID: "bk_home_hero", Type: "hero", Fill: "paper", Ink: "ink", Pad: 2, Gap: 1, TypeRole: "display", Weight: 600, Align: "left",
			Eyebrow: "Studio shop", Headline: "Everyday ceramics, fired slowly.",
			Body:      "Small batches thrown in a converted barn. Each piece carries the mark of the hand that made it.",
			BodyRole:  "xs",
			CTA:       lnk("ln_home_cta", "Browse the collection", "pg_shop"),
			Secondary: lnk("ln_home_sec", "Read our story", "")},
		{ID: "bk_home_features", Type: "features", Fill: "wash", Ink: "ink", Pad: 5, Gap: 4, TypeRole: "lg", Weight: 500, Align: "left",
			Heading: "Why Hearth", Items: []*Item{
				{ID: "ft_1", Title: "Stoneware that lasts", Body: "Fired to 1280°C, so every piece is safe in the dishwasher and the oven."},
				{ID: "ft_2", Title: "Glazes mixed in-house", Body: "Each glaze is tested on dozens of tiles before it reaches a mug."},
				{ID: "ft_3", Title: "Shipped with care", Body: "Wrapped in recycled paper and sent within three working days."},
			}},
		{ID: "bk_home_quote", Type: "quote", Fill: "paper", Ink: "muted", Pad: 5, Gap: 3, TypeRole: "xl", Weight: 500, Align: "center",
			Text: "The mug I reach for every morning.", Cite: "Ana, Lisbon"},
		seedFooter("home", "hf"),
	}}

	shop := &Node{Kind: "page", ID: "pg_shop", FlowID: "fl_marketing", Name: "Shop", Blocks: []*Block{
		seedNav("shop", "pg_shop"),
		{ID: "bk_shop_hero", Type: "hero", Fill: "paper", Ink: "ink", Pad: 5, Gap: 3, TypeRole: "xl", Weight: 500, Align: "left",
			Eyebrow: "Collection", Headline: "Mugs, bowls, and a few surprises.", Body: "Twelve pieces this season.",
			BodyRole: "md", CTA: lnk("ln_shop_cta", "Add the Dawn mug", "pg_cart")},
		{ID: "bk_shop_features", Type: "features", Fill: "paper", Ink: "ink", Pad: 5, Gap: 4, TypeRole: "lg", Weight: 500, Align: "left",
			Heading: "This season", Items: []*Item{
				{ID: "ft_dawn", Title: "Dawn mug", Body: "Speckled white stoneware, 300 ml. €28."},
				{ID: "ft_ember", Title: "Ember bowl", Body: "Iron-red glaze, 16 cm across. €34."},
				{ID: "ft_ash", Title: "Ash plate", Body: "Matte grey with a raw rim, 26 cm. €38."},
			}},
		{ID: "bk_shop_cta", Type: "cta", Fill: "wash", Ink: "ink", Pad: 5, Gap: 3, TypeRole: "lg", Weight: 500, Align: "left",
			Headline: "Ready when you are.", Body: "Free shipping over €60.",
			Button: lnk("ln_shop_band", "Add to cart", "pg_cart"), ButtonFill: "wash", ButtonInk: "muted"},
		seedFooter("shop", "sf"),
	}}

	cart := &Node{Kind: "page", ID: "pg_cart", FlowID: "fl_checkout", Name: "Cart", Blocks: []*Block{
		seedNav("cart", "pg_cart"),
		{ID: "bk_cart_hero", Type: "hero", Fill: "paper", Ink: "ink", Pad: 5, Gap: 3, TypeRole: "xl", Weight: 500, Align: "left",
			Eyebrow: "Checkout", Headline: "Your cart", Body: "1 item · Dawn mug · €28", BodyRole: "md",
			CTA: lnk("ln_cart_pay", "Pay €28", "pg_confirm"), Secondary: lnk("ln_cart_back", "Keep shopping", "pg_shop")},
		seedFooter("cart", "cartf"),
	}}

	confirm := &Node{Kind: "page", ID: "pg_confirm", FlowID: "fl_checkout", Name: "Confirm", Blocks: []*Block{
		seedNav("conf", "pg_confirm"),
		{ID: "bk_conf_hero", Type: "hero", Fill: "paper", Ink: "ink", Pad: 5, Gap: 3, TypeRole: "display", Weight: 600, Align: "left",
			Eyebrow: "Order 1042", Headline: "Thank you.", Body: "We'll email you when it ships.", BodyRole: "md",
			CTA: lnk("ln_conf_home", "Back to Hearth", "pg_home")},
		seedFooter("conf", "conff"),
	}}

	cta := func(label, to, size string) *ScreenCTA {
		return &ScreenCTA{Label: label, ToID: Ref(to), Size: size, Fill: "accent", Ink: "paper"}
	}
	welcome := &Node{Kind: "screen", ID: "sc_welcome", FlowID: "fl_onboarding", Name: "Welcome",
		Archetype: "title", Layout: "center", Fill: "paper", Ink: "ink", TypeRole: "display", Weight: 600, Align: "center", Pad: 4, Gap: 3,
		Title: "Hearth", Body: "Small-batch ceramics from the studio, in your pocket.",
		CTA:      cta("Get started", "sc_craft", "md"),
		Hotspots: []*Hotspot{{ID: "hs_skip", Label: "Skip", X: 300, Y: 60, W: 70, H: 44, ToID: "sc_home"}}}
	craft := &Node{Kind: "screen", ID: "sc_craft", FlowID: "fl_onboarding", Name: "Pick your craft",
		Archetype: "split", Layout: "split", Fill: "wash", Ink: "ink", TypeRole: "xl", Weight: 500, Align: "left", Pad: 4, Gap: 3,
		Title: "Pick your craft", Body: "Mugs, bowls, or plates. We'll show what you collect first.",
		CTA: cta("Next", "sc_notify", "md"), Hotspots: []*Hotspot{}}
	notify := &Node{Kind: "screen", ID: "sc_notify", FlowID: "fl_onboarding", Name: "Notifications",
		Archetype: "cta", Layout: "bottom", Fill: "paper", Ink: "ink", TypeRole: "xl", Weight: 500, Align: "left", Pad: 4, Gap: 3,
		Title: "Know when the kiln opens", Body: "We fire twice a month. Get a note when new pieces are ready.",
		CTA:      cta("Allow and finish", "sc_home", "md"),
		Hotspots: []*Hotspot{{ID: "hs_later", Label: "Not now", X: 95, Y: 760, W: 200, H: 44, ToID: "sc_home"}}}
	mhome := &Node{Kind: "screen", ID: "sc_home", FlowID: "fl_account", Name: "Home",
		Archetype: "photo", Layout: "bottom", Fill: "ink", Ink: "paper", TypeRole: "xl", Weight: 500, Align: "left", Pad: 4, Gap: 3,
		Title: "This season", Body: "Twelve pieces, fired slowly.",
		CTA:      cta("Shop the season", "", "md"),
		Hotspots: []*Hotspot{{ID: "hs_avatar", Label: "Profile", X: 326, Y: 56, W: 44, H: 44, ToID: "sc_profile"}}}
	profile := &Node{Kind: "screen", ID: "sc_profile", FlowID: "fl_account", Name: "Profile",
		Archetype: "title", Layout: "center", Fill: "paper", Ink: "ink", TypeRole: "lg", Weight: 500, Align: "center", Pad: 4, Gap: 3,
		Title: "Your profile", Body: "Orders, addresses, and saved glazes.",
		CTA:      cta("Sign out", "sc_home", "sm"),
		Hotspots: []*Hotspot{{ID: "hs_back", Label: "Back", X: 16, Y: 56, W: 44, H: 44, ToID: "sc_home"}}}

	p.Nodes = []*Node{home, shop, cart, confirm, welcome, craft, notify, mhome, profile}

	e := func(id, from, to, label, kind string) *Edge {
		return &Edge{ID: id, FromID: from, ToID: to, Label: label, Kind: kind}
	}
	p.Edges = []*Edge{
		e("ed_w1", "pg_home", "pg_shop", "Shop", "primary"),
		e("ed_w2", "pg_home", "pg_shop", "Browse the collection", "primary"),
		e("ed_w3", "pg_home", "pg_cart", "Cart", "cross"),
		e("ed_w4", "pg_shop", "pg_cart", "Cart", "cross"),
		e("ed_w5", "pg_shop", "pg_cart", "Add the Dawn mug", "cross"),
		e("ed_w6", "pg_shop", "pg_cart", "Add to cart", "cross"),
		e("ed_w7", "pg_cart", "pg_shop", "Shop", "cross"),
		e("ed_w8", "pg_cart", "pg_confirm", "Pay €28", "primary"),
		e("ed_w9", "pg_cart", "pg_shop", "Keep shopping", "cross"),
		e("ed_w10", "pg_confirm", "pg_shop", "Shop", "cross"),
		e("ed_w11", "pg_confirm", "pg_cart", "Cart", "cross"),
		e("ed_w12", "pg_confirm", "pg_home", "Back to Hearth", "cross"),
		e("ed_m1", "sc_welcome", "sc_craft", "Get started", "primary"),
		e("ed_m2", "sc_welcome", "sc_home", "Skip", "cross"),
		e("ed_m3", "sc_craft", "sc_notify", "Next", "primary"),
		e("ed_m4", "sc_notify", "sc_home", "Allow and finish", "cross"),
		e("ed_m5", "sc_notify", "sc_home", "Not now", "cross"),
		e("ed_m6", "sc_home", "sc_profile", "Profile", "primary"),
		e("ed_m7", "sc_profile", "sc_home", "Sign out", "back"),
		e("ed_m8", "sc_profile", "sc_home", "Back", "back"),
	}

	p.Draw = Draw{
		Shapes: []*Shape{
			{ID: "sh_rect", Type: "rect", X: 120, Y: 120, W: 220, H: 140, Stroke: "ink", Fill: "wash", SW: 1.5},
			{ID: "sh_ellipse", Type: "ellipse", X: 420, Y: 140, W: 120, H: 100, Stroke: "accent", Fill: "none", SW: 1.5},
			{ID: "sh_arrow", Type: "arrow", X1: 340, Y1: 190, X2: 420, Y2: 190, Stroke: "ink", SW: 1.5},
			{ID: "sh_title", Type: "text", X: 120, Y: 90, Text: "Hearth — kiln map", Role: "lg", Ink: "ink"},
			{ID: "sh_pen", Type: "pen", Stroke: "muted", SW: 2, Points: [][2]float64{
				{620, 300}, {652, 284}, {690, 290}, {712, 316}, {706, 350}, {680, 372},
				{644, 376}, {614, 360}, {602, 334}, {612, 308}, {640, 296}, {668, 302},
			}},
		},
		View: DrawView{X: 0, Y: 0, Zoom: 1},
	}

	p.Doc = Doc{Markdown: SeedMarkdown}

	p.Pins = []*Pin{
		{ID: "pin_1", N: 1, Kind: "issue", Target: PinTarget{Type: "node", NodeID: "pg_home", BlockID: "bk_home_hero"},
			X: 0.55, Y: 0.62, CreatedAt: 1767225600000,
			Body: "Body copy is set in `xs` with 8px padding. Reads cramped next to the display headline."},
		{ID: "pin_2", N: 2, Kind: "issue", Target: PinTarget{Type: "node", NodeID: "pg_shop", BlockID: "bk_shop_cta"},
			X: 0.5, Y: 0.7, CreatedAt: 1767225600000,
			Body: "\"Add to cart\" button is muted-on-wash. Fails AA."},
	}
	p.PinCounter = 2
	p.normalize()
	return p
}

// SPEC: new projects start blank (owner's decision, superseding §7's "a new
// project starts from the seed"). The Hearth seed above stays available as the
// sample: `serve --sample`, `init --sample`, or reset_project {template: "sample"}.

// Templates are the starting points reset_project accepts.
const (
	TemplateBlank  = "blank"
	TemplateSample = "sample"
)

// Templates lists the reset_project templates.
var Templates = []string{TemplateBlank, TemplateSample}

// BlankMarkdown is the DESIGN.md of a new blank project named name.
func BlankMarkdown(name string) string {
	if name == "" {
		name = "Untitled"
	}
	return "# " + name + " — DESIGN.md\n\n" +
		"## Product\n\nWhat this is, who it is for, and how it should feel.\n\n" +
		"## Principles\n\n- \n\n" +
		"## Decisions\n\n- \n"
}

// MakeBlank returns an empty project: default tokens, one web flow holding one
// empty page, one mobile flow with no screens yet, no edges, shapes, or pins.
func MakeBlank(name string) *Project {
	return &Project{
		Version: 1,
		Tokens:  DefaultTokens(),
		Flows: []*Flow{
			{ID: "fl_web", Name: "Website", Channel: ChanWeb, StartID: "pg_home"},
			{ID: "fl_app", Name: "App", Channel: ChanMobile},
		},
		Nodes: []*Node{{Kind: "page", ID: "pg_home", FlowID: "fl_web", Name: "Home", Blocks: []*Block{}}},
		Edges: []*Edge{},
		Draw:  Draw{Shapes: []*Shape{}, View: DrawView{X: 0, Y: 0, Zoom: 1}},
		Doc:   Doc{Markdown: BlankMarkdown(name)},
		Pins:  []*Pin{},
	}
}

// MakeTemplate returns a fresh project for template ("" means blank).
func MakeTemplate(template, name string) (*Project, error) {
	switch template {
	case "", TemplateBlank:
		return MakeBlank(name), nil
	case TemplateSample:
		return MakeSeed(), nil
	}
	return nil, errInvalid(fmt.Sprintf("template %q is not one of %v", template, Templates), `use "blank" or "sample"`)
}
