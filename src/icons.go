package main

import "math"

// Small status icons drawn by the program (no font has all of these on
// every Windows), as 32 bit BGRA pixels with alpha, top row first.

const (
	iconOK       = iota // available: green tick
	iconDownload        // can be downloaded: amber arrow
	iconBrowse          // must be browsed for: orange folder
	iconGear            // edit
	iconCount
)

type rgb struct{ r, g, b float64 }

// drawIcon renders icon k at n×n pixels.
func drawIcon(k, n int) []byte {
	const ss = 4 // supersampling per axis
	px := make([]byte, n*n*4)
	var col rgb
	var inside func(x, y float64) bool // coordinates 0..1
	segDist := func(x, y, ax, ay, bx, by float64) float64 {
		dx, dy := bx-ax, by-ay
		t := ((x-ax)*dx + (y-ay)*dy) / (dx*dx + dy*dy)
		t = math.Max(0, math.Min(1, t))
		return math.Hypot(x-ax-t*dx, y-ay-t*dy)
	}
	switch k {
	case iconOK:
		col = rgb{0x2E, 0x9E, 0x3E}
		inside = func(x, y float64) bool {
			return segDist(x, y, 0.16, 0.54, 0.40, 0.78) < 0.085 || segDist(x, y, 0.40, 0.78, 0.86, 0.24) < 0.085
		}
	case iconDownload:
		col = rgb{0xD8, 0x9A, 0x00}
		inside = func(x, y float64) bool {
			if x > 0.40 && x < 0.60 && y > 0.08 && y < 0.56 {
				return true // shaft
			}
			if y >= 0.44 && y < 0.80 { // head
				h := (0.80 - y) / 0.36 * 0.36
				if math.Abs(x-0.5) < h {
					return true
				}
			}
			return y > 0.86 && y < 0.95 && x > 0.14 && x < 0.86 // tray
		}
	case iconBrowse:
		col = rgb{0xE0, 0x7B, 0x00}
		inside = func(x, y float64) bool {
			if x > 0.08 && x < 0.46 && y > 0.18 && y < 0.34 {
				return true // tab
			}
			return x > 0.08 && x < 0.92 && y > 0.28 && y < 0.84
		}
	case iconGear:
		col = rgb{0x5A, 0x5A, 0x5A}
		inside = func(x, y float64) bool {
			dx, dy := x-0.5, y-0.5
			r := math.Hypot(dx, dy)
			a := math.Atan2(dy, dx)
			outer := 0.33
			if math.Cos(8*a) > 0.25 {
				outer = 0.46
			}
			return r < outer && r > 0.14
		}
	default:
		return px
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			hit := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := (float64(x) + (float64(sx)+0.5)/ss) / float64(n)
					fy := (float64(y) + (float64(sy)+0.5)/ss) / float64(n)
					if inside(fx, fy) {
						hit++
					}
				}
			}
			a := float64(hit) / (ss * ss)
			i := (y*n + x) * 4
			// premultiplied alpha, as image lists expect
			px[i+0] = byte(col.b*a + 0.5)
			px[i+1] = byte(col.g*a + 0.5)
			px[i+2] = byte(col.r*a + 0.5)
			px[i+3] = byte(255*a + 0.5)
		}
	}
	return px
}
