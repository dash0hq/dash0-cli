package pods

// Ditt0's colours, copied from ditt0-cli-design-system/components/foundations/tokens/tokens.js, which copies them from Dash0's design
// system (dash0/components/ui). Change them there first, then here.

type theme struct {
	// Status colours: src/design-tokens/css/semantics.css. inline is the colour for text on a plain surface
	errorInk, warningInk, healthyInk, infoInk, debugInk, unknownInk string
	// Log severities reuse the status colours (src/lib/colors/palettes/schemes/logSeverities.ts: FATAL and ERROR error,
	// WARN warning, INFO info, DEBUG and TRACE debug, else unknown)
	// The CTA's fill and the ink drawn on it
	ctaBg, ctaText string
	// Chart series colours, the auto palette (src/lib/colors/palettes/schemes/auto.css), already in the order the
	// color manager hands them out (color-manager.js AUTO_ORDER: 0, 1, 7, 4, 8, 6, 2, 3, 5)
	series []string
	// A selected toggle's border: Claude Code's 'suggestion' theme colour, which Button draws a selected toggle with
	selected string
}

var dark = theme{
	errorInk: "#e33238", warningInk: "#ffe54f", healthyInk: "#48b78c", infoInk: "#7eaacf", debugInk: "#b9c8e0", unknownInk: "#939396",
	ctaBg: "#7c3aed", ctaText: "#f5f3ff",
	series:   []string{"#828aff", "#225584", "#d8b4fe", "#b89d6a", "#74d1f0", "#d0af57", "#829ed6", "#6d459b", "#a39999"},
	selected: "#b1b9f9",
}

var light = theme{
	errorInk: "#e33238", warningInk: "#ca8a04", healthyInk: "#389e77", infoInk: "#2563eb", debugInk: "#7eaacf", unknownInk: "#939396",
	ctaBg: "#8b5cf6", ctaText: "#f5f3ff",
	series:   []string{"#4ca8a5", "#4038c0", "#b35b0a", "#7975f6", "#dcc223", "#641ec9", "#e1831a", "#c54474", "#3e68ee"},
	selected: "#5769f7",
}
