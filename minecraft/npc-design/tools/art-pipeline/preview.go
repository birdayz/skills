// Offscreen debugger for the train models. Reads the SHARED model JSON (the same files the game loads)
// + the texture PNG, and renders the model from 4 angles with each cube face UV-mapped exactly per MC's
// box-UV formula (so colours match the game), plus a UV-overlay showing where every face samples the
// texture (to catch mismaps / overflow). Runs in ~1s — no Minecraft launch.
//
//   go run preview.go            # renders loco + coach -> preview_loco.png, preview_coach.png
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

type Cube struct {
	Part  string     `json:"part"`
	UV    [2]float64 `json:"uv"`
	Pivot [3]float64 `json:"pivot"`
	Rot   [3]float64 `json:"rot"`
	Pos   [3]float64 `json:"pos"`
	Size  [3]float64 `json:"size"`
}
type Model struct {
	Texture string `json:"texture"`
	TexW    int    `json:"texW"`
	TexH    int    `json:"texH"`
	Cubes   []Cube `json:"cubes"`
}

type V3 struct{ X, Y, Z float64 }

func add(a, b V3) V3   { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func sub(a, b V3) V3   { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func dot(a, b V3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross(a, b V3) V3 {
	return V3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func norm(a V3) V3 {
	l := math.Sqrt(dot(a, a))
	if l < 1e-9 {
		return V3{0, 0, 1}
	}
	return V3{a.X / l, a.Y / l, a.Z / l}
}

// rotate v by euler degrees (Rz*Ry*Rx, matching MC ModelPart order)
func rotate(v V3, rx, ry, rz float64) V3 {
	rx, ry, rz = rad(rx), rad(ry), rad(rz)
	// Rx
	cy, sy := math.Cos(rx), math.Sin(rx)
	v = V3{v.X, v.Y*cy - v.Z*sy, v.Y*sy + v.Z*cy}
	// Ry
	cy, sy = math.Cos(ry), math.Sin(ry)
	v = V3{v.X*cy + v.Z*sy, v.Y, -v.X*sy + v.Z*cy}
	// Rz
	cy, sy = math.Cos(rz), math.Sin(rz)
	v = V3{v.X*cy - v.Y*sy, v.X*sy + v.Y*cy, v.Z}
	return v
}
func rad(d float64) float64 { return d * math.Pi / 180 }

// a face: 4 world corners + a UV-rect centre colour + world normal
type Face struct {
	c    [4]V3
	col  color.RGBA
	nrm  V3
	uv   [4]float64 // x,y,w,h in texture px (for overlay)
}

func cubeFaces(cu Cube, tex image.Image) []Face {
	piv := V3{cu.Pivot[0], cu.Pivot[1], cu.Pivot[2]}
	w, h, d := cu.Size[0], cu.Size[1], cu.Size[2]
	// 8 corners (i=x,j=y,k=z in {0,1})
	corner := func(i, j, k float64) V3 {
		local := V3{cu.Pos[0] + i*w, cu.Pos[1] + j*h, cu.Pos[2] + k*d}
		return add(piv, rotate(local, cu.Rot[0], cu.Rot[1], cu.Rot[2]))
	}
	c000, c100 := corner(0, 0, 0), corner(1, 0, 0)
	c010, c110 := corner(0, 1, 0), corner(1, 1, 0)
	c001, c101 := corner(0, 0, 1), corner(1, 0, 1)
	c011, c111 := corner(0, 1, 1), corner(1, 1, 1)
	u, vv := cu.UV[0], cu.UV[1]
	// MC box-UV layout (W=w,H=h,D=d)
	mk := func(pts [4]V3, ux, uy, uw, uh float64, n V3) Face {
		return Face{c: pts, col: sampleRect(tex, ux, uy, uw, uh), nrm: norm(n), uv: [4]float64{ux, uy, uw, uh}}
	}
	return []Face{
		mk([4]V3{c000, c100, c101, c001}, u+d, vv, w, d, V3{0, -1, 0}),       // top (-Y up)
		mk([4]V3{c010, c110, c111, c011}, u+d+w, vv, w, d, V3{0, 1, 0}),      // bottom (+Y)
		// corner order: first edge HORIZONTAL (along Z), like north/south — the old vertical-first
		// winding texture-mapped east/west faces 90°-rotated (hidden for years by uniform fills,
		// exposed by the Kirby face + loco number plate).
		mk([4]V3{c000, c001, c011, c010}, u, vv+d, d, h, V3{-1, 0, 0}),       // west (-X)
		mk([4]V3{c100, c101, c111, c110}, u+d+w, vv+d, d, h, V3{1, 0, 0}),    // east (+X)
		mk([4]V3{c000, c100, c110, c010}, u+d, vv+d, w, h, V3{0, 0, -1}),     // north (-Z)
		mk([4]V3{c001, c101, c111, c011}, u+2*d+w, vv+d, w, h, V3{0, 0, 1}),  // south (+Z)
	}
}

func sampleRect(tex image.Image, x, y, w, h float64) color.RGBA {
	b := tex.Bounds()
	px := int(x + w/2)
	py := int(y + h/2)
	if px < 0 {
		px = 0
	}
	if py < 0 {
		py = 0
	}
	if px >= b.Dx() {
		px = b.Dx() - 1
	}
	if py >= b.Dy() {
		py = b.Dy() - 1
	}
	r, g, bl, _ := tex.At(b.Min.X+px, b.Min.Y+py).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
}

// ---- render one orthographic view into the dst tile ----
func renderView(dst *image.RGBA, ox, oy, tw, th int, faces []Face, tex image.Image, F V3, bounds [2]V3, scale float64, label string) {
	up := V3{0, -1, 0}
	if math.Abs(dot(F, up)) > 0.95 {
		up = V3{1, 0, 0}
	}
	R := norm(cross(up, F))
	U := norm(cross(F, R))
	light := norm(V3{-0.5, -0.9, -0.4})
	cx := (bounds[0].X + bounds[1].X) / 2
	cy := (bounds[0].Y + bounds[1].Y) / 2
	cz := (bounds[0].Z + bounds[1].Z) / 2
	center := V3{cx, cy, cz}
	proj := func(v V3) (float64, float64) {
		p := sub(v, center)
		return float64(ox+tw/2) + dot(p, R)*scale, float64(oy+th/2) + dot(p, U)*scale
	}
	// painter: far (large depth along F) first
	sort.Slice(faces, func(i, j int) bool {
		di := dot(centroid(faces[i].c), F)
		dj := dot(centroid(faces[j].c), F)
		return di > dj
	})
	for _, f := range faces {
		// back-face cull: skip faces pointing away (normal·F > 0 means facing along view = away)
		if dot(f.nrm, F) > 0.15 {
			continue
		}
		shade := 0.45 + 0.55*math.Max(0, dot(f.nrm, light))
		var pts [4][2]float64
		for i := 0; i < 4; i++ {
			x, y := proj(f.c[i])
			pts[i] = [2]float64{x, y}
		}
		fillQuadTex(dst, pts, f.uv, tex, shade, ox, oy, tw, th)
		edge := color.RGBA{0, 0, 0, 90}
		for i := 0; i < 4; i++ {
			drawLine(dst, pts[i], pts[(i+1)%4], edge)
		}
	}
	drawLabel(dst, ox+4, oy+4, label)
}

func centroid(c [4]V3) V3 {
	return V3{(c[0].X + c[1].X + c[2].X + c[3].X) / 4, (c[0].Y + c[1].Y + c[2].Y + c[3].Y) / 4, (c[0].Z + c[1].Z + c[2].Z + c[3].Z) / 4}
}

// fillQuadTex: per-pixel UV-textured quad. Corners map to UV-rect (TL,TR,BR,BL).
// Projection is orthographic, so affine barycentric UV interpolation is exact.
func fillQuadTex(dst *image.RGBA, p [4][2]float64, uv [4]float64, tex image.Image, shade float64, ox, oy, tw, th int) {
	st := [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	tb := tex.Bounds()
	edge := func(a, b, c [2]float64) float64 {
		return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
	}
	drawTri := func(i0, i1, i2 int) {
		a, b, c := p[i0], p[i1], p[i2]
		area := edge(a, b, c)
		if area == 0 {
			return
		}
		minx := int(math.Floor(math.Min(a[0], math.Min(b[0], c[0]))))
		maxx := int(math.Ceil(math.Max(a[0], math.Max(b[0], c[0]))))
		miny := int(math.Floor(math.Min(a[1], math.Min(b[1], c[1]))))
		maxy := int(math.Ceil(math.Max(a[1], math.Max(b[1], c[1]))))
		for y := miny; y <= maxy; y++ {
			for x := minx; x <= maxx; x++ {
				if x < ox || x >= ox+tw || y < oy || y >= oy+th {
					continue
				}
				pt := [2]float64{float64(x) + 0.5, float64(y) + 0.5}
				w0 := edge(b, c, pt) / area
				w1 := edge(c, a, pt) / area
				w2 := edge(a, b, pt) / area
				if w0 < -0.001 || w1 < -0.001 || w2 < -0.001 {
					continue
				}
				s := w0*st[i0][0] + w1*st[i1][0] + w2*st[i2][0]
				t := w0*st[i0][1] + w1*st[i1][1] + w2*st[i2][1]
				px := int(uv[0] + s*uv[2])
				py := int(uv[1] + t*uv[3])
				if px < 0 {
					px = 0
				}
				if py < 0 {
					py = 0
				}
				if px >= tb.Dx() {
					px = tb.Dx() - 1
				}
				if py >= tb.Dy() {
					py = tb.Dy() - 1
				}
				r, g, bl, _ := tex.At(tb.Min.X+px, tb.Min.Y+py).RGBA()
				dst.SetRGBA(x, y, color.RGBA{
					uint8(math.Min(255, float64(r>>8)*shade)),
					uint8(math.Min(255, float64(g>>8)*shade)),
					uint8(math.Min(255, float64(bl>>8)*shade)), 255})
			}
		}
	}
	drawTri(0, 1, 2)
	drawTri(0, 2, 3)
}

func fillQuad(dst *image.RGBA, p [4][2]float64, col color.RGBA, ox, oy, tw, th int) {
	tri := func(a, b, c [2]float64) {
		minx := int(math.Floor(math.Min(a[0], math.Min(b[0], c[0]))))
		maxx := int(math.Ceil(math.Max(a[0], math.Max(b[0], c[0]))))
		miny := int(math.Floor(math.Min(a[1], math.Min(b[1], c[1]))))
		maxy := int(math.Ceil(math.Max(a[1], math.Max(b[1], c[1]))))
		for y := miny; y <= maxy; y++ {
			for x := minx; x <= maxx; x++ {
				if x < ox || x >= ox+tw || y < oy || y >= oy+th {
					continue
				}
				if inTri([2]float64{float64(x) + 0.5, float64(y) + 0.5}, a, b, c) {
					dst.SetRGBA(x, y, col)
				}
			}
		}
	}
	tri(p[0], p[1], p[2])
	tri(p[0], p[2], p[3])
}

func inTri(p, a, b, c [2]float64) bool {
	d := func(p1, p2, p3 [2]float64) float64 {
		return (p1[0]-p3[0])*(p2[1]-p3[1]) - (p2[0]-p3[0])*(p1[1]-p3[1])
	}
	d1, d2, d3 := d(p, a, b), d(p, b, c), d(p, c, a)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

func drawLine(dst *image.RGBA, a, b [2]float64, col color.RGBA) {
	steps := int(math.Hypot(b[0]-a[0], b[1]-a[1])) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(a[0] + (b[0]-a[0])*t)
		y := int(a[1] + (b[1]-a[1])*t)
		if x >= 0 && x < dst.Bounds().Dx() && y >= 0 && y < dst.Bounds().Dy() {
			blend(dst, x, y, col)
		}
	}
}
func blend(dst *image.RGBA, x, y int, c color.RGBA) {
	o := dst.RGBAAt(x, y)
	a := float64(c.A) / 255
	dst.SetRGBA(x, y, color.RGBA{
		uint8(float64(c.R)*a + float64(o.R)*(1-a)),
		uint8(float64(c.G)*a + float64(o.G)*(1-a)),
		uint8(float64(c.B)*a + float64(o.B)*(1-a)), 255})
}

func modelBounds(faces []Face) [2]V3 {
	mn := V3{1e9, 1e9, 1e9}
	mx := V3{-1e9, -1e9, -1e9}
	for _, f := range faces {
		for _, c := range f.c {
			mn = V3{math.Min(mn.X, c.X), math.Min(mn.Y, c.Y), math.Min(mn.Z, c.Z)}
			mx = V3{math.Max(mx.X, c.X), math.Max(mx.Y, c.Y), math.Max(mx.Z, c.Z)}
		}
	}
	return [2]V3{mn, mx}
}

func fillBG(dst *image.RGBA, c color.RGBA) {
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

// ---- UV overlay: texture (scaled) with each face's sample-rect outlined ----
func uvOverlay(dst *image.RGBA, ox, oy int, tex image.Image, m *Model, faces []Face, sc int) {
	tb := tex.Bounds()
	for y := 0; y < tb.Dy(); y++ {
		for x := 0; x < tb.Dx(); x++ {
			r, g, b, _ := tex.At(tb.Min.X+x, tb.Min.Y+y).RGBA()
			c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
			for dy := 0; dy < sc; dy++ {
				for dx := 0; dx < sc; dx++ {
					dst.SetRGBA(ox+x*sc+dx, oy+y*sc+dy, c)
				}
			}
		}
	}
	cols := []color.RGBA{{255, 80, 80, 200}, {80, 255, 120, 200}, {120, 160, 255, 200}, {255, 230, 80, 200}}
	for i, f := range faces {
		c := cols[i%len(cols)]
		x0 := ox + int(f.uv[0])*sc
		y0 := oy + int(f.uv[1])*sc
		x1 := ox + int(f.uv[0]+f.uv[2])*sc
		y1 := oy + int(f.uv[1]+f.uv[3])*sc
		drawLine(dst, [2]float64{float64(x0), float64(y0)}, [2]float64{float64(x1), float64(y0)}, c)
		drawLine(dst, [2]float64{float64(x1), float64(y0)}, [2]float64{float64(x1), float64(y1)}, c)
		drawLine(dst, [2]float64{float64(x1), float64(y1)}, [2]float64{float64(x0), float64(y1)}, c)
		drawLine(dst, [2]float64{float64(x0), float64(y1)}, [2]float64{float64(x0), float64(y0)}, c)
	}
	drawLabel(dst, ox+4, oy+4, fmt.Sprintf("UV %s (%dx%d) - face sample rects", m.Texture, m.TexW, m.TexH))
}

func render(name string) {
	mj, err := os.ReadFile(filepath.Join("../../src/main/resources/assets/karoland/train_models", name+".json"))
	must(err)
	var m Model
	must(json.Unmarshal(mj, &m))
	tf, err := os.Open(filepath.Join("../../src/main/resources/assets/karoland/textures/entity", m.Texture))
	must(err)
	defer tf.Close()
	tex, err := png.Decode(tf)
	must(err)

	var faces []Face
	for _, cu := range m.Cubes {
		faces = append(faces, cubeFaces(cu, tex)...)
	}
	bounds := modelBounds(faces)
	ext := V3{bounds[1].X - bounds[0].X, bounds[1].Y - bounds[0].Y, bounds[1].Z - bounds[0].Z}
	maxext := math.Max(ext.X, math.Max(ext.Y, ext.Z))

	tw, th := 460, 300
	scale := float64(th) * 0.42 / (maxext / 2)
	// 2x2 views on top, UV overlay strip below
	uvsc := 1
	W := tw * 2
	uvH := m.TexH*uvsc + 20
	H := th*2 + uvH
	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	fillBG(dst, color.RGBA{210, 214, 220, 255})

	views := []struct {
		F     V3
		label string
		ox, oy int
	}{
		{V3{0, 0, 1}, "SIDE (+X left)", 0, 0},
		{V3{-1, 0, 0}, "FRONT (smokebox)", tw, 0},
		{V3{0, 1, 0}, "TOP", 0, th},
		{norm(V3{-0.8, 0.55, -0.7}), "ISO", tw, th},
	}
	for _, vw := range views {
		fc := make([]Face, len(faces))
		copy(fc, faces)
		renderView(dst, vw.ox, vw.oy, tw, th, fc, tex, norm(vw.F), bounds, scale, vw.label)
	}
	uvOverlay(dst, 0, th*2, tex, &m, faces, uvsc)

	out := "preview_" + name + ".png"
	f, err := os.Create(out)
	must(err)
	defer f.Close()
	must(png.Encode(f, dst))
	fmt.Printf("wrote %s  (model %.0fx%.0fx%.0f units, %d cubes, %d faces)\n", out, ext.X, ext.Y, ext.Z, len(m.Cubes), len(faces))
}

func main() {
	render("loco")
	render("coach")
	render("pip")
	render("carousel")
	render("teacups")
	render("ferris_wheel")
	render("ferris_gondola")
	render("swing")
	render("drop_car")
	render("kirby_vendor")
	render("peach")
	render("dk")
	render("jukebox")
	render("flauschi")
	render("aphmau")
	render("slidehead")
	render("dive_ring")
	render("flamingo")
	render("elon")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---- tiny 3x5 bitmap label ----
func drawLabel(dst *image.RGBA, x, y int, s string) {
	for i := 0; i < len(s)*6+4; i++ {
		for j := 0; j < 11; j++ {
			if x+i < dst.Bounds().Dx() && y+j < dst.Bounds().Dy() {
				blend(dst, x+i, y+j, color.RGBA{20, 20, 28, 200})
			}
		}
	}
	cx := x + 2
	for _, ch := range s {
		drawChar(dst, cx, y+1, ch, color.RGBA{240, 240, 245, 255})
		cx += 8
	}
}

var font = map[rune][]string{
	'A': {"###", "# #", "###", "# #", "# #"}, 'B': {"## ", "# #", "## ", "# #", "## "},
	'C': {"###", "#  ", "#  ", "#  ", "###"}, 'D': {"## ", "# #", "# #", "# #", "## "},
	'E': {"###", "#  ", "## ", "#  ", "###"}, 'F': {"###", "#  ", "## ", "#  ", "#  "},
	'G': {"###", "#  ", "# #", "# #", "###"}, 'H': {"# #", "# #", "###", "# #", "# #"},
	'I': {"###", " # ", " # ", " # ", "###"}, 'K': {"# #", "## ", "#  ", "## ", "# #"},
	'L': {"#  ", "#  ", "#  ", "#  ", "###"}, 'M': {"# #", "###", "###", "# #", "# #"},
	'N': {"# #", "###", "###", "###", "# #"}, 'O': {"###", "# #", "# #", "# #", "###"},
	'P': {"###", "# #", "###", "#  ", "#  "}, 'R': {"###", "# #", "## ", "# #", "# #"},
	'S': {"###", "#  ", "###", "  #", "###"}, 'T': {"###", " # ", " # ", " # ", " # "},
	'U': {"# #", "# #", "# #", "# #", "###"}, 'V': {"# #", "# #", "# #", "# #", " # "},
	'W': {"# #", "# #", "###", "###", "# #"}, 'X': {"# #", "# #", " # ", "# #", "# #"},
	'Y': {"# #", "# #", " # ", " # ", " # "}, 'Z': {"###", "  #", " # ", "#  ", "###"},
	'0': {"###", "# #", "# #", "# #", "###"}, '1': {" # ", "## ", " # ", " # ", "###"},
	'2': {"###", "  #", "###", "#  ", "###"}, '3': {"###", "  #", "###", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"}, '5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"}, '7': {"###", "  #", " # ", " # ", " # "},
	'8': {"###", "# #", "###", "# #", "###"}, '9': {"###", "# #", "###", "  #", "###"},
	'(': {" # ", "#  ", "#  ", "#  ", " # "}, ')': {" # ", "  #", "  #", "  #", " # "},
	'+': {"   ", " # ", "###", " # ", "   "}, '-': {"   ", "   ", "###", "   ", "   "},
	'.': {"   ", "   ", "   ", "   ", " # "}, '_': {"   ", "   ", "   ", "   ", "###"},
	'/': {"  #", "  #", " # ", "#  ", "#  "}, ' ': {"   ", "   ", "   ", "   ", "   "},
}

func drawChar(dst *image.RGBA, x, y int, ch rune, col color.RGBA) {
	if ch >= 'a' && ch <= 'z' {
		ch -= 32
	}
	g, ok := font[ch]
	if !ok {
		return
	}
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ {
			if g[r][c] == '#' {
				for dy := 0; dy < 2; dy++ {
					for dx := 0; dx < 2; dx++ {
						px, py := x+c*2+dx, y+r*2+dy
						if px >= 0 && px < dst.Bounds().Dx() && py >= 0 && py < dst.Bounds().Dy() {
							dst.SetRGBA(px, py, col)
						}
					}
				}
			}
		}
	}
}
