package grafana

import (
	"regexp"
	"strings"
)

// namedColors are Grafana's palette names (as used in thresholds and
// overrides) mapped to CSS colors. Names are "<shade>-<hue>" with the shades
// super-light, light, (none), semi-dark and dark.
var namedColors = map[string]string{
	"super-light-red": "#FFA6B0", "light-red": "#FF7383", "red": "#F2495C", "semi-dark-red": "#E02F44", "dark-red": "#C4162A",
	"super-light-orange": "#FFCB7D", "light-orange": "#FFB357", "orange": "#FF9830", "semi-dark-orange": "#FA6400", "dark-orange": "#E55400",
	"super-light-yellow": "#FFF899", "light-yellow": "#FFEE52", "yellow": "#FADE2A", "semi-dark-yellow": "#F2CC0C", "dark-yellow": "#E0B400",
	"super-light-green": "#C8F2C2", "light-green": "#96D98D", "green": "#73BF69", "semi-dark-green": "#56A64B", "dark-green": "#37872D",
	"super-light-blue": "#C0D8FF", "light-blue": "#8AB8FF", "blue": "#5794F2", "semi-dark-blue": "#3274D9", "dark-blue": "#1F60C4",
	"super-light-purple": "#DEB6F2", "light-purple": "#CA95E5", "purple": "#B877D9", "semi-dark-purple": "#A352CC", "dark-purple": "#8F3BB8",
	"white": "#FFFFFF", "black": "#000000", "gray": "#8E8E8E", "grey": "#8E8E8E", "text": "#CCCCDC",
}

var cssColor = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|rgba?\([0-9.,\s%]+\)|hsla?\([0-9.,\s%deg]+\))$`)

// normalizeColor turns a Grafana color (palette name, hex, rgb()) into a CSS
// color. Unknown or unsafe values give "" so the frontend uses its own color.
func normalizeColor(c string) string {
	c = strings.TrimSpace(c)
	if hex, ok := namedColors[strings.ToLower(c)]; ok {
		return hex
	}
	if cssColor.MatchString(c) {
		return c
	}
	return ""
}
