// Command genbackgrounds renders the embedded example background pictures
// (internal/backgrounds/images/*.jpg) procedurally, so they need no source
// artwork and can be tweaked and regenerated:
//
//	cd backend && go run ./cmd/genbackgrounds
//
// Pictures are kept calm and low contrast; the widget cards stay opaque, so the
// picture only shows between and around them.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

const (
	width  = 1920
	height = 1200
)

type rgb [3]float64

func hex(v uint32) rgb { return rgb{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)} }

func mix(a, b rgb, t float64) rgb {
	return rgb{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func smooth(e0, e1, x float64) float64 {
	t := math.Min(1, math.Max(0, (x-e0)/(e1-e0)))
	return t * t * (3 - 2*t)
}

type canvas struct{ px []rgb }

func newCanvas() *canvas { return &canvas{px: make([]rgb, width*height)} }

func (c *canvas) at(x, y int) *rgb { return &c.px[y*width+x] }

type stop struct {
	at  float64
	col rgb
}

// gradient fills the canvas with a vertical gradient tilted by skew (x shifts the position).
func (c *canvas) gradient(skew float64, stops ...stop) {
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			t := float64(y)/height + skew*(float64(x)/width-0.5)
			*c.at(x, y) = ramp(stops, t)
		}
	}
}

func ramp(stops []stop, t float64) rgb {
	if t <= stops[0].at {
		return stops[0].col
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].at {
			return mix(stops[i-1].col, stops[i].col, (t-stops[i-1].at)/(stops[i].at-stops[i-1].at))
		}
	}
	return stops[len(stops)-1].col
}

// glow blends a soft gaussian blob over the canvas.
func (c *canvas) glow(cx, cy, rx, ry float64, col rgb, strength float64) {
	x0, x1 := int(math.Max(0, cx-3*rx)), int(math.Min(width-1, cx+3*rx))
	y0, y1 := int(math.Max(0, cy-3*ry)), int(math.Min(height-1, cy+3*ry))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			a := strength * math.Exp(-(dx*dx+dy*dy)/2)
			p := c.at(x, y)
			*p = mix(*p, col, a)
		}
	}
}

// ridge fills everything below a smooth, wavy horizon line.
func (c *canvas) ridge(seed int64, base, amp float64, top, bottom rgb) {
	r := rand.New(rand.NewSource(seed))
	type wave struct{ f, p, a float64 }
	waves := make([]wave, 4)
	for i := range waves {
		f := (0.8 + r.Float64()*0.6) * math.Pow(1.9, float64(i)) * math.Pi / width
		waves[i] = wave{f: f, p: r.Float64() * 2 * math.Pi, a: math.Pow(0.5, float64(i))}
	}
	for x := 0; x < width; x++ {
		h := 0.0
		for _, w := range waves {
			h += w.a * math.Sin(float64(x)*w.f+w.p)
		}
		edge := base + amp*h/1.9
		for y := int(math.Max(0, edge-2)); y < height; y++ {
			cov := smooth(-1, 1, float64(y)-edge) // 1px antialiased edge
			col := mix(top, bottom, math.Min(1, (float64(y)-edge)/(height-edge)))
			p := c.at(x, y)
			*p = mix(*p, col, cov)
		}
	}
}

func (c *canvas) stars(seed int64, n int, maxY float64) {
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < n; i++ {
		x, y := r.Float64()*width, math.Pow(r.Float64(), 1.4)*maxY
		b := 0.25 + r.Float64()*0.6
		rad := 0.7 + r.Float64()*r.Float64()*1.6
		c.glow(x, y, rad, rad, rgb{255, 255, 255}, b)
	}
}

// aurora draws a luminous curtain along a wavy line.
func (c *canvas) aurora(seed int64, baseY, amp, thick float64, col rgb, strength float64) {
	r := rand.New(rand.NewSource(seed))
	p1, p2 := r.Float64()*6, r.Float64()*6
	for x := 0; x < width; x++ {
		fx := float64(x) / width
		cy := baseY + amp*(math.Sin(fx*5+p1)*0.6+math.Sin(fx*11+p2)*0.4)
		ray := 0.65 + 0.35*math.Sin(fx*140+p2*3)*math.Sin(fx*37+p1) // vertical streaks
		fade := math.Sin(math.Pi * fx)                              // fade out toward the sides
		for y := 0; y < height; y++ {
			d := (float64(y) - cy) / thick
			// sharp lower edge, long soft glow upwards
			var a float64
			if d > 0 {
				a = math.Exp(-d * d * 6)
			} else {
				a = math.Exp(d * 1.6)
			}
			a *= strength * ray * fade
			if a < 0.002 {
				continue
			}
			p := c.at(x, y)
			*p = mix(*p, col, a)
		}
	}
}

func (c *canvas) write(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	r := rand.New(rand.NewSource(42))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := *c.at(x, y)
			n := r.Float64() - 0.5 // dither against banding in the smooth gradients
			img.SetRGBA(x, y, color.RGBA{clamp(p[0] + n), clamp(p[1] + n), clamp(p[2] + n), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 82})
}

func clamp(v float64) uint8 { return uint8(math.Min(255, math.Max(0, math.Round(v)))) }

func main() {
	out := flag.String("out", "internal/backgrounds/images", "output directory")
	flag.Parse()

	pictures := map[string]func(*canvas){
		// light
		"dawn": func(c *canvas) {
			c.gradient(0.1, stop{0, hex(0xc9d6f2)}, stop{0.45, hex(0xf3dcec)}, stop{0.75, hex(0xffe3cc)}, stop{1, hex(0xffd2b8)})
			c.glow(1380, 760, 330, 260, hex(0xfff1d6), 0.9)
			c.ridge(11, 880, 70, hex(0xf2c7c4), hex(0xf6d7cb))
			c.ridge(12, 960, 60, hex(0xe6b5bd), hex(0xefc9c8))
			c.ridge(13, 1050, 45, hex(0xd69fb0), hex(0xe3b4bc))
		},
		"mist": func(c *canvas) {
			c.gradient(0.35, stop{0, hex(0xdde8f6)}, stop{0.6, hex(0xe9eff8)}, stop{1, hex(0xd3e0f1)})
			c.glow(340, 260, 420, 300, hex(0xbcd3f3), 0.55)
			c.glow(1500, 380, 520, 320, hex(0xcfd9f5), 0.6)
			c.glow(960, 1020, 700, 260, hex(0xb7dbe9), 0.5)
			c.glow(1700, 1100, 380, 240, hex(0xd8c9f0), 0.45)
		},
		"meadow": func(c *canvas) {
			c.gradient(0, stop{0, hex(0xbfdcf5)}, stop{0.55, hex(0xe3f1fa)}, stop{1, hex(0xf6fbf4)})
			c.glow(1500, 230, 260, 200, hex(0xffffff), 0.7)
			c.glow(520, 330, 380, 70, hex(0xffffff), 0.5)
			c.ridge(21, 760, 90, hex(0xc3dfc4), hex(0xd2e8cf))
			c.ridge(22, 870, 80, hex(0xa9d1aa), hex(0xbfdfb9))
			c.ridge(23, 990, 60, hex(0x8cbf90), hex(0xa3d09f))
			c.ridge(24, 1090, 40, hex(0x78b07f), hex(0x8cc28b))
		},
		// dark
		"night": func(c *canvas) {
			c.gradient(0.15, stop{0, hex(0x070b1c)}, stop{0.6, hex(0x0f1a36)}, stop{1, hex(0x1a2a4d)})
			c.glow(1480, 250, 320, 320, hex(0x4a6aa8), 0.22)
			c.stars(31, 520, 800)
			c.glow(1480, 250, 38, 38, hex(0xf3f0e2), 0.95)
			c.glow(1480, 250, 70, 70, hex(0xdfe6ff), 0.18)
			c.ridge(32, 1000, 60, hex(0x0d1630), hex(0x0a1126))
			c.ridge(33, 1100, 40, hex(0x080e20), hex(0x060b19))
		},
		"aurora": func(c *canvas) {
			c.gradient(0, stop{0, hex(0x050a16)}, stop{0.7, hex(0x0a1a2a)}, stop{1, hex(0x0c2230)})
			c.stars(41, 380, 700)
			c.aurora(42, 430, 120, 150, hex(0x2fe0a0), 0.55)
			c.aurora(43, 520, 90, 120, hex(0x6a7cf0), 0.38)
			c.aurora(44, 360, 80, 90, hex(0xb06af0), 0.22)
			c.ridge(45, 1040, 55, hex(0x07121c), hex(0x050c14))
		},
		"dusk": func(c *canvas) {
			c.gradient(0.1, stop{0, hex(0x14102e)}, stop{0.5, hex(0x2c1f52)}, stop{0.8, hex(0x5a3366)}, stop{1, hex(0x7a4565)})
			c.glow(520, 820, 520, 220, hex(0xd9758a), 0.35)
			c.stars(51, 160, 450)
			c.ridge(52, 800, 80, hex(0x3a2552), hex(0x2c1c45))
			c.ridge(53, 920, 70, hex(0x2a1b43), hex(0x201436))
			c.ridge(54, 1030, 55, hex(0x1b1230), hex(0x130d25))
		},
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, draw := range pictures {
		c := newCanvas()
		draw(c)
		path := filepath.Join(*out, name+".jpg")
		if err := c.write(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
