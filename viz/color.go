package viz

// palette provides cycling color assignment for graph categories.
type palette struct {
	colors []string
}

// defaultColors are neo4j-inspired muted colors for graph categories.
var defaultColors = []string{
	"#4C8EDA", // blue
	"#DA7194", // pink
	"#569480", // teal
	"#D9A460", // gold
	"#6B5B95", // purple
	"#C1666B", // red
	"#48A9A6", // cyan
	"#D4B483", // tan
	"#7EC8E3", // light blue
	"#95B46A", // green
}

func newPalette(colors []string) *palette {
	if len(colors) == 0 {
		colors = defaultColors
	}
	return &palette{colors: colors}
}

func (p *palette) colorFor(index int) string {
	return p.colors[index%len(p.colors)]
}
