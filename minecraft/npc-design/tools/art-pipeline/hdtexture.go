// HD train texture pipeline: per-cube UV UNWRAP + detailed per-face painting.
//
// Reads the shared geometry JSON (loco.json / coach.json), computes each cube's MC box-UV footprint,
// BIN-PACKS them into a single atlas (no more shared 4-colour regions → no repetition, room for detail),
// writes the unique uv back into the JSON, and paints a detailed atlas per cube + per face: material
// (painted metal / brushed brass / brushed iron / wood planks) plus targeted details — boiler lining
// bands, brass sheen, wood planking, wheel spokes+rim, smokebox number plate, coach beltline stripe.
//
// preview.go renders from the same JSON+PNG, so preview == game. Run: go run hdtexture.go
package main

import (
	"encoding/json"
	"image"
	"image/color"
	_ "image/jpeg" // Gemini returns JPEG bytes (even with a .png name); register the decoder
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
)

type Cube struct {
	Part  string     `json:"part,omitempty"`
	UV    []int      `json:"uv"`
	Pivot *[]float64 `json:"pivot,omitempty"`
	Rot   *[]float64 `json:"rot,omitempty"`
	Pos   []float64  `json:"pos"`
	Size  []float64  `json:"size"`
}
type Model struct {
	Texture string `json:"texture"`
	TexW    int    `json:"texW"`
	TexH    int    `json:"texH"`
	Datum   string `json:"_datum,omitempty"`
	Cubes   []Cube `json:"cubes"`
}

const ATLAS_W = 512
const PAD = 2

// atlasW: the jukebox is authored at 4x (64u = 1 block) for marquee texel density, so its cabinet
// faces (2(W+D) = 640) overflow the shared 512 atlas — it gets a 1024-wide atlas of its own.
func atlasW(model string) int {
	if model == "jukebox" || model == "slidehead" {
		return 1024
	}
	return ATLAS_W
}

func hx(s uint32) color.RGBA { return color.RGBA{uint8(s >> 16), uint8(s >> 8), uint8(s), 255} }
func cl(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
func sh(c color.RGBA, f float64) color.RGBA {
	return color.RGBA{cl(float64(c.R) * f), cl(float64(c.G) * f), cl(float64(c.B) * f), 255}
}
func mix(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{cl(float64(a.R)*(1-t) + float64(b.R)*t), cl(float64(a.G)*(1-t) + float64(b.G)*t), cl(float64(a.B)*(1-t) + float64(b.B)*t), 255}
}

// ---- palette per (model, part) ----
type matKind int

const (
	mPaint matKind = iota // painted metal (base + subtle panel + grain)
	mIron                 // brushed dark iron
	mBrass                // polished brass (sheen)
	mWood                 // planking
)

type mat struct {
	kind matKind
	base color.RGBA
	trim color.RGBA // lining / accent
}

// classify a part into a material + colours, per model.
func classify(model, part string) mat {
	gold := hx(0xD7B254)
	black := hx(0x15171A)
	switch model {
	case "loco":
		switch part {
		case "boiler", "boiler_d", "cab_side", "cab_hdr", "cab_back":
			return mat{mPaint, hx(0x1F3D2B), gold} // green + gold lining
		case "band", "dome_steam", "dome_sand", "headlamp", "handrail", "rod", "bell":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)}
		case "buffer", "pilot", "hub":
			return mat{mPaint, hx(0x8B2230), hx(0x5A1620)} // red
		case "smokebox", "stack_base", "stack_flare", "stack_cap", "cab_roof", "footplate", "wheel":
			return mat{mIron, black, hx(0x2A2D33)}
		}
		return mat{mIron, black, hx(0x2A2D33)}
	case "pip":
		switch part {
		case "tailtip", "chest", "paw":
			return mat{mPaint, hx(0xFAF5EC), hx(0xE2D8C8)} // bright cream/white fur
		case "nose", "eye", "eartip":
			return mat{mIron, hx(0x141414), hx(0x141414)} // black
		}
		return mat{mPaint, hx(0xD9722E), hx(0xB85A1E)} // orange fox fur
	case "kirby_vendor":
		switch part {
		case "arm":
			return mat{mPaint, hx(0xF285B5), hx(0xE06CA0)} // ~10% deeper pink: arms separate from the ball
		case "foot":
			return mat{mPaint, hx(0xD6354D), hx(0xB02440)} // red shoes
		}
		return mat{mPaint, hx(0xFFA6C9), hx(0xF285B5)} // kirby pink (matches the face art bg)
	case "jukebox":
		teal := hx(0x2EE6D6)
		switch part {
		case "plinth":
			return mat{mIron, hx(0x241B2E), teal} // dark stage base
		case "body":
			return mat{mPaint, hx(0xF285B5), hx(0xD96EA5)} // Flauschi pink cabinet
		case "arch_a":
			return mat{mPaint, hx(0xF49FC2), teal} // arch tiers lighten upward
		case "arch_b":
			return mat{mPaint, hx(0xFABFD6), teal}
		case "arch_c":
			return mat{mPaint, hx(0xFFD7E6), teal}
		case "marquee":
			return mat{mIron, hx(0x1A1126), hx(0xD7B254)} // dark backing so the art pops
		case "window":
			return mat{mIron, hx(0x10142A), teal} // navy glass
		case "cone_l", "cone_r":
			return mat{mPaint, hx(0xF6F0E8), hx(0x241B2E)} // cream speaker panels
		case "buttons":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)}
		case "tube_l", "tube_r":
			return mat{mPaint, hx(0xCFF7F0), teal} // bubble tubes
		}
		return mat{mPaint, hx(0xF285B5), hx(0xD96EA5)}
	case "flauschi":
		switch part {
		case "paw":
			return mat{mPaint, hx(0xF7BBD0), hx(0xF2A6BE)} // lighter mitten paws
		case "spike":
			return mat{mPaint, hx(0xC2487E), hx(0xA83468)} // darker-magenta dino spikes (ref)
		case "snout":
			return mat{mPaint, hx(0xF2A6BE), hx(0xE07FA4)} // hood snout = onesie pink + nostril dots
		}
		// ONE soft pastel onesie pink for hood+torso+limbs (Aphmau: head/body must match);
		// the face blit is cropped inside its darker hood ring so edges land near this hex.
		return mat{mPaint, hx(0xF2A6BE), hx(0xE07FA4)}
	case "slidehead":
		switch part {
		case "teeth_top", "teeth_bot":
			return mat{mPaint, hx(0xF8F8F6), hx(0xE2E2DC)} // white teeth strips
		case "eye":
			return mat{mPaint, hx(0xF8F8F6), hx(0xE2E2DC)} // white sclera (iris painted on +X)
		case "spike":
			return mat{mPaint, hx(0xC2487E), hx(0xA83468)} // magenta crest spikes
		case "paw":
			return mat{mPaint, hx(0xF7BBD0), hx(0xF2A6BE)} // lighter mitten paws draped over the tower
		}
		return mat{mPaint, hx(0xF2A6BE), hx(0xE07FA4)} // the Flauschi onesie pink
	case "dive_ring":
		// sunken treasure gold — bright polished brass so it reads through 2 blocks of water
		return mat{mBrass, hx(0xE8BC4A), hx(0xFFF0B0)}
	case "flamingo":
		switch part {
		case "beak":
			return mat{mPaint, hx(0xF2B233), hx(0xD89A22)} // orange-yellow bill (tip dipped black)
		case "tail":
			return mat{mPaint, hx(0xE068A0), hx(0xC8508C)} // deeper pink tail bump
		}
		return mat{mPaint, hx(0xF584B0), hx(0xE068A0)} // inflatable flamingo pink (vinyl sheen-ish)
	case "aphmau":
		switch part {
		case "head", "hairback", "haircap":
			return mat{mIron, hx(0x1B1B1D), hx(0x35353B)} // glossy black bob (matches face art, measured)
		case "hand":
			return mat{mPaint, hx(0xCA8A5C), hx(0xB87A4E)} // skin (measured at the art's chin edge)
		case "sleeve", "body":
			return mat{mPaint, hx(0x8B6FD0), hx(0x7658BC)} // purple cardigan
		case "leg":
			return mat{mPaint, hx(0xCA8A5C), hx(0xB87A4E)} // painted into bands by aphmauLeg
		}
		return mat{mPaint, hx(0x8B6FD0), hx(0x7658BC)}
	case "elon":
		switch part {
		case "haircap", "hairback":
			return mat{mIron, hx(0x4A382C), hx(0x3A2C22)} // short brown hair
		case "hand", "head":
			return mat{mPaint, hx(0xE6B796), hx(0xD4A584)} // skin (face blit lands near this)
		case "leg":
			return mat{mIron, hx(0x1C1E22), hx(0x26282E)} // black jeans
		}
		return mat{mIron, hx(0x17171A), hx(0x222226)} // all-black SpaceX tee/blazer
	case "peach":
		switch part {
		case "skirt_a":
			return mat{mPaint, hx(0xEE86B6), hx(0xD96EA5)} // gown hem tier, deepest rose (Aphmau: wider tier spread)
		case "skirt_b":
			return mat{mPaint, hx(0xF49FC2), hx(0xE383B2)}
		case "skirt_c", "sleeve":
			return mat{mPaint, hx(0xFABFD6), hx(0xEC95BE)} // lightest tier + puff sleeves
		case "bodice":
			return mat{mPaint, hx(0xEE86B6), hx(0xD7B254)} // rose bodice, gold trim
		case "glove":
			return mat{mPaint, hx(0xFAF7F0), hx(0xE8E2D6)} // white evening gloves
		case "neck", "head":
			return mat{mPaint, hx(0xFFD5AF), hx(0xF0BE92)} // skin (matches the face art bg, measured)
		case "crown":
			return mat{mBrass, hx(0xE3BC4A), hx(0xF6E08A)}
		case "jewel":
			return mat{mPaint, hx(0xD6354D), hx(0xF06A80)} // crown ruby
		case "earring":
			return mat{mPaint, hx(0x3E6BD6), hx(0x6FA0F0)} // blue stud earrings
		}
		return mat{mPaint, hx(0xFFD36B), hx(0xE9B544)} // blonde hair (hair/bang/lock)
	case "dk":
		switch part {
		case "muzzle", "chest", "ear":
			return mat{mPaint, hx(0xE8C39A), hx(0xD4A878)} // tan muzzle + chest slab + ear pucks
		case "tie", "knot":
			return mat{mPaint, hx(0xD03A2C), hx(0xA82A20)} // the red tie
		case "tuft":
			return mat{mPaint, hx(0x6B4527), hx(0x58361D)} // hair swirl ~15% lighter so it reads
		}
		return mat{mPaint, hx(0x58361D), hx(0x432815)} // dark brown fur (matches the face art bg, measured)
	case "carousel":
		switch part {
		case "pole", "seatpole", "topper":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)}
		case "canopy_a":
			return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)} // pink stripe
		case "canopy_b":
			return mat{mPaint, hx(0xF6F0E8), hx(0xE8568C)} // white stripe
		case "horsebody", "coat_palomino":
			return mat{mPaint, hx(0xF3E4C2), hx(0xFFF6E0)} // palomino coat
		case "coat_white":
			return mat{mPaint, hx(0xF5F4EE), hx(0xFFFFFF)} // white coat
		case "coat_chestnut":
			return mat{mPaint, hx(0x9A5A30), hx(0xB87A48)} // chestnut bay
		case "coat_dapple":
			return mat{mPaint, hx(0xB7B2AB), hx(0xCFCAC2)} // dapple grey
		case "horsemane":
			return mat{mPaint, hx(0x4A3320), hx(0x35230F)} // mane / tail / dark
		case "saddle_a", "seat_a":
			return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)} // pink saddle
		case "saddle_b", "seat_b":
			return mat{mPaint, hx(0x46C7E0), hx(0xFFFFFF)} // cyan saddle
		case "saddle_c", "seat_c":
			return mat{mPaint, hx(0xF0C838), hx(0xFFFFFF)} // yellow saddle
		case "saddle_d", "seat_d":
			return mat{mPaint, hx(0xC050B0), hx(0xFFFFFF)} // magenta saddle
		}
		return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)}
	case "teacups":
		switch part {
		case "pole", "topper":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)}
		case "canopy_a":
			return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)}
		case "canopy_b":
			return mat{mPaint, hx(0xF6F0E8), hx(0xE8568C)}
		case "rim_a", "rim_b", "rim_c", "rim_d":
			return mat{mPaint, hx(0xF6F0E8), hx(0xC9A13B)} // white rim
		case "cup_a", "handle_a":
			return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)} // pink
		case "cup_b", "handle_b":
			return mat{mPaint, hx(0x46C7E0), hx(0xFFFFFF)} // cyan
		case "cup_c", "handle_c":
			return mat{mPaint, hx(0xF0C838), hx(0xFFFFFF)} // yellow
		case "cup_d", "handle_d":
			return mat{mPaint, hx(0xC050B0), hx(0xFFFFFF)} // magenta
		}
		return mat{mPaint, hx(0xE8568C), hx(0xFFFFFF)}
	case "ferris_wheel":
		switch part {
		case "hub", "axle":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)} // gold hub
		case "spoke":
			return mat{mIron, hx(0xB9C2CC), hx(0x8A949E)} // bright steel spokes
		case "rim":
			return mat{mPaint, hx(0xF6F0E8), hx(0xE8568C)} // white felloe + pink accent
		}
		return mat{mIron, hx(0xB9C2CC), hx(0x8A949E)}
	case "swing":
		switch part {
		case "topper", "hub", "arm":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)} // gold rigging
		case "canopy_a":
			return mat{mPaint, hx(0xE0445A), hx(0xFFFFFF)} // red canopy
		case "canopy_b":
			return mat{mPaint, hx(0xF6F0E8), hx(0xE0445A)} // cream/red
		case "chain":
			return mat{mIron, hx(0xB9C2CC), hx(0x8A949E)} // steel chain
		case "chair", "chairback":
			return mat{mPaint, hx(0x46C7E0), hx(0xFFFFFF)} // cyan seats
		}
		return mat{mPaint, hx(0xE0445A), hx(0xFFFFFF)}
	case "drop_car":
		switch part {
		case "topper", "post":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)} // gold rigging
		case "ring":
			return mat{mPaint, hx(0xF0C838), hx(0xFFFFFF)} // sunny yellow floor ring
		case "canopy":
			return mat{mPaint, hx(0xE0445A), hx(0xFFFFFF)} // red canopy ring
		case "seat", "seatback":
			return mat{mPaint, hx(0x46C7E0), hx(0xFFFFFF)} // cyan seats
		}
		return mat{mPaint, hx(0xF0C838), hx(0xFFFFFF)}
	case "ferris_gondola":
		switch part {
		case "hanger":
			return mat{mIron, hx(0xB9C2CC), hx(0x8A949E)} // steel yoke
		case "roof":
			return mat{mPaint, hx(0xF0C838), hx(0xFFFFFF)} // sunny yellow roof
		case "cabin":
			return mat{mPaint, hx(0xD6455A), hx(0xFFFFFF)} // cherry-red tub
		case "trim":
			return mat{mBrass, hx(0xC9A13B), hx(0xF0D78A)} // gold rail
		case "seat":
			return mat{mWood, hx(0x6A4326), hx(0x4A2E18)}
		}
		return mat{mPaint, hx(0xD6455A), hx(0xFFFFFF)}
	default: // coach
		switch part {
		case "waist":
			return mat{mPaint, hx(0x6E2230), hx(0xE8DCC0)} // maroon + cream beltline
		case "post":
			return mat{mPaint, hx(0x5A1B27), hx(0xE8DCC0)} // darker maroon pillars
		case "cant", "roof_edge", "endrail":
			return mat{mPaint, hx(0xE8DCC0), hx(0xC9A13B)} // cream trim
		case "deck", "platform", "bench":
			return mat{mWood, hx(0x6A4326), hx(0x4A2E18)}
		case "roof", "clerestory", "frame", "wheel":
			return mat{mIron, black, hx(0x2A2D33)}
		}
		return mat{mIron, black, hx(0x2A2D33)}
	}
}

// material fill at local face pixel (fx,fy) within a face of size (fw,fh)
func fill(m mat, rng *rand.Rand, fx, fy, fw, fh int) color.RGBA {
	switch m.kind {
	case mWood:
		plank := fy / 4
		if fy%4 == 0 && fh > 6 {
			return sh(m.base, 0.6) // plank gap
		}
		pr := rand.New(rand.NewSource(int64(plank*2654435761) & 0x7fffffff))
		f := 0.88 + pr.Float64()*0.24
		f += math.Sin(float64(fx)*0.5+float64(plank)) * 0.03
		return sh(m.base, f)
	case mBrass:
		f := 1.0 + math.Exp(-math.Pow((float64(fy)-float64(fh)*0.35)/(float64(fh)*0.32+1), 2))*0.30
		f += math.Sin(float64(fy)*1.3)*0.05 + (rng.Float64()-0.5)*0.04
		return sh(m.base, f)
	case mIron:
		f := 1.0 + (rng.Float64()-0.5)*0.10 + math.Sin(float64(fx)*1.7)*0.03
		return sh(m.base, f)
	default: // painted
		f := 1.0 + (1.0-float64(fy)/float64(fh+1))*0.10 + (rng.Float64()-0.5)*0.045
		return sh(m.base, f)
	}
}

// ---- MC box-UV face rects (in atlas px) for a cube at (u,v) with dims w,h,d ----
type rect struct{ x, y, w, h int }
type faceRects struct {
	top, bottom, west, north, east, south rect
}

func faces(u, v, w, h, d int) faceRects {
	return faceRects{
		top:    rect{u + d, v, w, d},
		bottom: rect{u + d + w, v, w, d},
		west:   rect{u, v + d, d, h},
		north:  rect{u + d, v + d, w, h},
		east:   rect{u + d + w, v + d, d, h},
		south:  rect{u + 2*d + w, v + d, w, h},
	}
}
func fpW(w, h, d int) int { return 2*d + 2*w }
func fpH(w, h, d int) int { return d + h }

func paintRect(img *image.RGBA, r rect, m mat, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for fy := 0; fy < r.h; fy++ {
		for fx := 0; fx < r.w; fx++ {
			img.SetRGBA(r.x+fx, r.y+fy, fill(m, rng, fx, fy, r.w, r.h))
		}
	}
}

// overlay horizontal lining bands (for boiler/cab green + brass-ish lines)
func lining(img *image.RGBA, r rect, col color.RGBA) {
	if r.h < 8 {
		return
	}
	for _, frac := range []float64{0.18, 0.82} {
		yy := r.y + int(float64(r.h)*frac)
		for fx := 0; fx < r.w; fx++ {
			img.SetRGBA(r.x+fx, yy, col)
		}
	}
}

// beltline: a single thick stripe near the top (coach waist)
func beltline(img *image.RGBA, r rect, col color.RGBA) {
	if r.h < 6 {
		return
	}
	y0 := r.y + int(float64(r.h)*0.12)
	for dy := 0; dy < 3 && y0+dy < r.y+r.h; dy++ {
		for fx := 0; fx < r.w; fx++ {
			img.SetRGBA(r.x+fx, y0+dy, col)
		}
	}
}

// wheel spokes + rim on a round (w≈h) face
func wheelFace(img *image.RGBA, r rect) {
	if r.w < 4 || r.h < 4 {
		return
	}
	cx, cy := float64(r.x)+float64(r.w)/2, float64(r.y)+float64(r.h)/2
	rad := math.Min(float64(r.w), float64(r.h)) / 2
	rim := hx(0x3A3D44)
	spoke := hx(0x55585F)
	hub := hx(0xC9A13B)
	for fy := 0; fy < r.h; fy++ {
		for fx := 0; fx < r.w; fx++ {
			dx, dy := float64(r.x+fx)-cx, float64(r.y+fy)-cy
			dist := math.Hypot(dx, dy)
			if dist > rad-1 { // rim
				img.SetRGBA(r.x+fx, r.y+fy, rim)
			} else if dist < rad*0.22 { // hub
				img.SetRGBA(r.x+fx, r.y+fy, hub)
			} else {
				ang := math.Atan2(dy, dx)
				if math.Mod(math.Abs(ang)+math.Pi, math.Pi/4) < 0.22 {
					img.SetRGBA(r.x+fx, r.y+fy, spoke) // spoke
				}
			}
		}
	}
}

// number plate on the smokebox front (east face): brass plate w/ dark "number" bars
func numberPlate(img *image.RGBA, r rect) {
	pw, ph := r.w*3/5, r.h*2/5
	px, py := r.x+(r.w-pw)/2, r.y+(r.h-pw/4)/2
	if pw < 4 || ph < 3 {
		return
	}
	for fy := 0; fy < ph; fy++ {
		for fx := 0; fx < pw; fx++ {
			c := hx(0xC9A13B)
			if fx == 0 || fy == 0 || fx == pw-1 || fy == ph-1 {
				c = hx(0x8A6E20)
			}
			img.SetRGBA(px+fx, py+fy, c)
		}
	}
	// a couple of dark digit bars
	for i := 0; i < 2; i++ {
		bx := px + pw/4 + i*pw/3
		for fy := 2; fy < ph-2; fy++ {
			if bx < px+pw {
				img.SetRGBA(bx, py+fy, hx(0x33240A))
			}
		}
	}
}

// radialStripes: alternating candy wedges around the centre of a (square-ish) face.
// The carnival/carousel big-top look — used on canopy top + underside faces.
func radialStripes(img *image.RGBA, r rect, a, b color.RGBA, wedges int) {
	if r.w < 4 || r.h < 4 {
		return
	}
	cx := float64(r.x) + float64(r.w)/2 - 0.5
	cy := float64(r.y) + float64(r.h)/2 - 0.5
	for fy := 0; fy < r.h; fy++ {
		for fx := 0; fx < r.w; fx++ {
			ang := math.Atan2(float64(r.y+fy)-cy, float64(r.x+fx)-cx) // -pi..pi
			seg := int(math.Floor((ang + math.Pi) / (2 * math.Pi) * float64(wedges)))
			col := a
			if seg%2 == 0 {
				col = b
			}
			// gentle darkening toward the rim so the dome reads as round, not flat
			dist := math.Hypot(float64(r.x+fx)-cx, float64(r.y+fy)-cy)
			rad := math.Min(float64(r.w), float64(r.h)) / 2
			f := 1.0 - 0.18*math.Min(1, dist/(rad+1))
			img.SetRGBA(r.x+fx, r.y+fy, sh(col, f))
		}
	}
}

// barberPole: diagonal stripes spiralling up a tall cube's 4 side faces (the carousel column).
func barberPole(img *image.RGBA, faces []rect, a, b color.RGBA, stripe int) {
	if stripe < 1 {
		stripe = 1
	}
	for _, r := range faces {
		for fy := 0; fy < r.h; fy++ {
			for fx := 0; fx < r.w; fx++ {
				col := a
				if ((fx+(r.h-1-fy))/stripe)%2 == 0 {
					col = b
				}
				// brass sheen down the centre of the column
				f := 1.0 + math.Exp(-math.Pow((float64(fx)-float64(r.w)*0.4)/(float64(r.w)*0.4+1), 2))*0.18
				img.SetRGBA(r.x+fx, r.y+fy, sh(col, f))
			}
		}
	}
}

// bulbStrip: a row of evenly-spaced round "light bulbs" along a face's wide axis
// (Ferris-wheel rim lights). Each bulb gets a white hot-centre for a lit look.
func bulbStrip(img *image.RGBA, r rect, col color.RGBA, spacing int) {
	if r.w < 4 || r.h < 3 || spacing < 2 {
		return
	}
	cy := float64(r.y) + float64(r.h)/2
	rad := math.Min(float64(spacing)*0.32, float64(r.h)*0.42)
	for bx := spacing / 2; bx < r.w; bx += spacing {
		cx := float64(r.x + bx)
		for fy := 0; fy < r.h; fy++ {
			for dx := -int(rad) - 1; dx <= int(rad)+1; dx++ {
				xx := r.x + bx + dx
				if xx < r.x || xx >= r.x+r.w {
					continue
				}
				dist := math.Hypot(float64(xx)-cx, float64(r.y+fy)-cy)
				if dist <= rad {
					c := col
					if dist <= rad*0.45 {
						c = mix(col, hx(0xFFFFFF), 0.6) // lit hot-centre
					}
					img.SetRGBA(xx, r.y+fy, c)
				}
			}
		}
	}
}

// canopyImg: optional Gemini-generated ornate carousel-canopy texture (gen/canopy/canopy-chosen.png),
// loaded once in main(); nil ⇒ fall back to procedural stripes.
var canopyImg image.Image

// kirbyFaceImg: Gemini-generated Kirby face (gen/kirby/face-chosen.png), blitted (pre-rotated 180°,
// see the UV orientation rule) onto the vendor body front face; nil => plain pink.
var kirbyFaceImg image.Image

func loadKirbyFace() {
	f, err := os.Open("gen/kirby/face-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		// EAST faces render UNROTATED in-game (studio-orbit proven: a 180 pre-rotation showed the
		// face upside down — the "all side faces 180" rule holds for N/S faces only)
		kirbyFaceImg = im
		println("loaded Gemini kirby face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

// peachFaceImg: Gemini-generated Princess Peach face (gen/peach/face-chosen.png), blitted UNROTATED
// onto the head's +X (east) face — same rule the Kirby orbit proved; nil => plain skin.
var peachFaceImg image.Image

func loadPeachFace() {
	f, err := os.Open("gen/peach/face-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		peachFaceImg = im
		println("loaded Gemini peach face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

// jukeboxMarqueeImg: Gemini-generated FLAUSCHI marquee (gen/jukebox/marquee-chosen.png), blitted
// UNROTATED onto the marquee panel's +X (east/front) face; nil => dark panel with gold lining.
var jukeboxMarqueeImg image.Image

func loadJukeboxMarquee() {
	f, err := os.Open("gen/jukebox/marquee-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		jukeboxMarqueeImg = im
		println("loaded Gemini jukebox marquee:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

// flauschiFaceImg / aphmauFaceImg: Gemini faces for the Splash Party NPCs, blitted UNROTATED
// onto each head's +X (east/front) face; nil => plain painted head.
var flauschiFaceImg, aphmauFaceImg, elonFaceImg image.Image

func loadElonFace() {
	f, err := os.Open("gen/elon/face-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		elonFaceImg = im
		println("loaded Gemini Elon face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

func loadSplashFaces() {
	if f, err := os.Open("gen/flauschi/face-chosen.png"); err == nil {
		if im, _, err := image.Decode(f); err == nil {
			flauschiFaceImg = im
			println("loaded Gemini flauschi face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
		}
		f.Close()
	}
	if f, err := os.Open("gen/aphmau/face-chosen.png"); err == nil {
		if im, _, err := image.Decode(f); err == nil {
			aphmauFaceImg = im
			println("loaded Gemini aphmau face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
		}
		f.Close()
	}
}

// dkFaceImg: Gemini-generated Donkey Kong face (gen/dk/face-chosen.png), blitted UNROTATED onto
// the head's +X face; the lower face (nostrils/grin) is the painted 3D muzzle box instead.
var dkFaceImg image.Image

func loadDkFace() {
	f, err := os.Open("gen/dk/face-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		dkFaceImg = im
		println("loaded Gemini dk face:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

func loadCanopy() {
	f, err := os.Open("gen/canopy/canopy-chosen.png")
	if err != nil {
		return
	}
	defer f.Close()
	if im, _, err := image.Decode(f); err == nil {
		canopyImg = im
		println("loaded Gemini canopy:", im.Bounds().Dx(), "x", im.Bounds().Dy())
	}
}

// blitResized: AREA-AVERAGE downscale of src into face rect r (a proper box filter — far cleaner than
// nearest when shrinking a 1K Gemini image down to ~112px texel density).
func blitResized(dst *image.RGBA, r rect, src image.Image) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for fy := 0; fy < r.h; fy++ {
		for fx := 0; fx < r.w; fx++ {
			sx0 := sb.Min.X + fx*sw/r.w
			sx1 := sb.Min.X + (fx+1)*sw/r.w
			sy0 := sb.Min.Y + fy*sh/r.h
			sy1 := sb.Min.Y + (fy+1)*sh/r.h
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			var rr, gg, bb, n uint64
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					cr, cg, cb, _ := src.At(xx, yy).RGBA()
					rr += uint64(cr >> 8)
					gg += uint64(cg >> 8)
					bb += uint64(cb >> 8)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(r.x+fx, r.y+fy, color.RGBA{uint8(rr / n), uint8(gg / n), uint8(bb / n), 255})
		}
	}
}

func paintCube(img *image.RGBA, model string, c Cube, seed int64) {
	w := int(math.Round(c.Size[0])); h := int(math.Round(c.Size[1])); d := int(math.Round(c.Size[2]))
	if w < 1 { w = 1 }; if h < 1 { h = 1 }; if d < 1 { d = 1 }
	u, v := c.UV[0], c.UV[1]
	m := classify(model, c.Part)
	fr := faces(u, v, w, h, d)
	all := []rect{fr.top, fr.bottom, fr.west, fr.north, fr.east, fr.south}
	for i, r := range all {
		paintRect(img, r, m, seed+int64(i)*131)
	}
	// targeted details
	switch {
	case model == "loco" && (c.Part == "boiler" || c.Part == "boiler_d" || c.Part == "cab_side"):
		lining(img, fr.north, m.trim); lining(img, fr.south, m.trim)
		lining(img, fr.west, m.trim); lining(img, fr.east, m.trim)
	case model == "loco" && c.Part == "smokebox":
		numberPlate(img, fr.east)
	case c.Part == "wheel":
		wheelFace(img, fr.north); wheelFace(img, fr.south)
	case model == "coach" && c.Part == "waist":
		beltline(img, fr.north, m.trim); beltline(img, fr.south, m.trim)
		beltline(img, fr.west, m.trim); beltline(img, fr.east, m.trim)
	case (model == "carousel" || model == "teacups" || model == "swing") && (c.Part == "canopy_a" || c.Part == "canopy_b"):
		// big-top candy stripes on the dome top + underside; gold-piped step edges.
		// Carousel (the showpiece) uses a Gemini-generated ornate canopy image if present, area-averaged
		// down to texel density; teacups/swing keep the lean procedural stripes.
		gold := hx(0xD7B254)
		if model == "carousel" && canopyImg != nil {
			blitResized(img, fr.top, canopyImg)
			blitResized(img, fr.bottom, canopyImg)
		} else {
			radialStripes(img, fr.top, m.base, m.trim, 16)
			radialStripes(img, fr.bottom, m.base, m.trim, 16)
		}
		for _, e := range []rect{fr.west, fr.north, fr.east, fr.south} {
			paintRect(img, e, mat{mBrass, gold, hx(0xF0D78A)}, seed)
		}
	case model == "carousel" && c.Part == "pole":
		barberPole(img, []rect{fr.west, fr.north, fr.east, fr.south}, hx(0xD7B254), hx(0xFBF3DD), 4)
	case model == "ferris_wheel" && c.Part == "rim":
		festive := []color.RGBA{hx(0xFF4D6D), hx(0xFFD23F), hx(0x4DD0E1), hx(0x9B5DE5), hx(0x80ED99)}
		col := festive[int(seed/7919)%len(festive)]
		bulbStrip(img, fr.north, col, 8); bulbStrip(img, fr.south, col, 8)
	case model == "ferris_gondola" && c.Part == "roof":
		// little striped parasol roof so the gondolas match the carnival big-tops
		radialStripes(img, fr.top, m.base, hx(0xFFFFFF), 12)
		radialStripes(img, fr.bottom, m.base, hx(0xFFFFFF), 12)
	case model == "drop_car" && c.Part == "ring":
		// festive bulbs around the collar's outer band — the falling car must READ from the plaza,
		// day and night (the car is self-lit, so these glow during the night plunge too).
		bulbStrip(img, fr.north, hx(0xFF4D6D), 7)
		bulbStrip(img, fr.south, hx(0xFF4D6D), 7)
	case model == "kirby_vendor" && c.Part == "body":
		if kirbyFaceImg != nil {
			blitResized(img, fr.east, kirbyFaceImg) // +X face = the front (pip convention)
		}
	case model == "jukebox" && c.Part == "marquee":
		if jukeboxMarqueeImg != nil {
			blitResized(img, fr.east, jukeboxMarqueeImg) // +X face = the front, unrotated (kirby rule)
		} else {
			lining(img, fr.east, hx(0xD7B254)) // placeholder until the Gemini marquee lands
		}
	case model == "jukebox" && (c.Part == "cone_l" || c.Part == "cone_r"):
		speakerCone(img, fr.east)
	case model == "jukebox" && c.Part == "window":
		vinylWindow(img, fr.east)
	case model == "jukebox" && c.Part == "buttons":
		buttonRow(img, fr.east)
	case model == "jukebox" && (c.Part == "tube_l" || c.Part == "tube_r"):
		bubbleColumn(img, fr.east, seed)
		bubbleColumn(img, fr.north, seed+7)
		bubbleColumn(img, fr.south, seed+13)
	case model == "jukebox" && (c.Part == "arch_a" || c.Part == "arch_b" || c.Part == "arch_c"):
		// neon piping wraps the front + the z-side faces. Studio-orbit ground truth (2026-06,
		// Aphmau leg bands, both sides): N/S faces render UNROTATED on these models — flat paint.
		neonEdge(img, fr.east, hx(0x2EE6D6), false)
		neonEdge(img, fr.north, hx(0x2EE6D6), false)
		neonEdge(img, fr.south, hx(0x2EE6D6), false)
	case model == "slidehead" && c.Part == "skull":
		// the open-mouth interior shows on the skull's front (+X) face between snout and jaw:
		// dark maroon mouth band (model y -92..-36 of the -144..-36 span → v rows 52..108) + tongue
		slideheadMouth(img, fr.east)
	case model == "slidehead" && c.Part == "snout":
		// the snout's front face IS the mug the boarding kid meets (it overhangs the skull face)
		// — big white-rimmed cartoon eyes up top, nostril dots mid (Aphmau: the face must POP
		// from queue distance; the side eye pucks vanish head-on)
		skullEyes(img, fr.east)
		nostrils(img, fr.east)
	case model == "slidehead" && c.Part == "eye":
		eyeIris(img, fr.east)
	case model == "slidehead" && c.Part == "paw":
		pawPad(img, fr.east)
	case model == "flauschi" && c.Part == "head":
		if flauschiFaceImg != nil {
			// crop inside the art's dark outer hood ring so the face edges blend with the
			// pastel-painted head (Aphmau blocker 1: head vs body pink mismatch)
			b := flauschiFaceImg.Bounds()
			in := image.Rect(b.Min.X+b.Dx()*11/100, b.Min.Y+b.Dy()*11/100,
					b.Max.X-b.Dx()*11/100, b.Max.Y-b.Dy()*11/100)
			blitResized(img, fr.east, &croppedImg{flauschiFaceImg, in})
		}
	case model == "flamingo" && c.Part == "beak":
		// black-dipped bill tip: the +X end cap (real flamingo bills end black — instantly readable)
		paintRect(img, fr.east, mat{mIron, hx(0x17171A), hx(0x17171A)}, seed)
	case model == "flamingo" && c.Part == "head":
		eyeDot(img, fr.north)
		eyeDot(img, fr.south)
	case model == "flauschi" && c.Part == "body":
		flauschiBelly(img, fr.east)
	case model == "flauschi" && c.Part == "snout":
		nostrils(img, fr.east)
	case model == "flauschi" && c.Part == "paw":
		pawPad(img, fr.east)
	case model == "elon" && c.Part == "head":
		if elonFaceImg != nil {
			blitResized(img, fr.east, elonFaceImg) // +X face = the front (pip convention), unrotated
		}
	case model == "aphmau" && c.Part == "head":
		if aphmauFaceImg != nil {
			// the art puts a wide hair margin left of the face — crop to centre her face on the
			// head cube (DK eye-band lesson: blitting the raw square shrinks the features)
			b := aphmauFaceImg.Bounds()
			band := image.Rect(b.Min.X+b.Dx()*20/100, b.Min.Y+b.Dy()*14/100,
					b.Max.X, b.Min.Y+b.Dy()*96/100)  // keep eyes AND smile/chin in frame
			blitResized(img, fr.east, &croppedImg{aphmauFaceImg, band})
		}
	case model == "aphmau" && c.Part == "body":
		// studio-orbit ground truth (2026-06): on THESE 6x character cubes the N/S faces render
		// UNROTATED like E/W — the v-flipped side bands came out upside down. Paint all four flat.
		aphmauTorso(img, fr.east, false)
		aphmauTorso(img, fr.west, false)
		aphmauTorso(img, fr.north, false)
		aphmauTorso(img, fr.south, false)
	case model == "aphmau" && c.Part == "leg":
		aphmauLeg(img, fr.east, false)
		aphmauLeg(img, fr.west, false)
		aphmauLeg(img, fr.north, false)
		aphmauLeg(img, fr.south, false)
	case model == "aphmau" && (c.Part == "hairback" || c.Part == "haircap"):
		// strand streaks so the bob reads as hair, not an obsidian slab, under night lamps
		for i, fc := range []rect{fr.top, fr.bottom, fr.west, fr.north, fr.east, fr.south} {
			hairStreaks(img, fc, int64(i+1)*31)
		}
	case model == "peach" && c.Part == "head":
		if peachFaceImg != nil {
			blitResized(img, fr.east, peachFaceImg) // +X face = the front, unrotated (kirby rule)
		}
	case model == "dk" && c.Part == "head":
		if dkFaceImg != nil {
			// crop to the brow-and-eye band (middle ~56%) — blitting the full square crushed the
			// eyes to dots on the short head face (Aphmau blocker 2)
			b := dkFaceImg.Bounds()
			band := image.Rect(b.Min.X, b.Min.Y+b.Dy()*22/100, b.Max.X, b.Min.Y+b.Dy()*78/100)
			blitResized(img, fr.east, &croppedImg{dkFaceImg, band})
		}
	case model == "dk" && c.Part == "muzzle":
		dkMuzzle(img, fr.east)
	case model == "dk" && c.Part == "tie":
		dkTieLetters(img, fr.east) // gold "D"/"K" — THE identifier (Aphmau blocker 1)
	case model == "peach" && c.Part == "bodice":
		brooch(img, fr.east)
	case model == "peach" && c.Part == "skirt_a":
		// Peach's signature deeper-pink hem ring around the gown bottom (Aphmau blocker 2).
		// N/S faces render 180°-rotated in-game, E/W unrotated (v=0 = world top) — paint accordingly.
		hemBand(img, fr.east, false)
		hemBand(img, fr.west, false)
		hemBand(img, fr.north, true)
		hemBand(img, fr.south, true)
	case model == "peach" && c.Part == "crown":
		// band jewels: blue front/back, ruby sides (Aphmau blocker 3)
		jewelDot(img, fr.east, hx(0x3E6BD6))
		jewelDot(img, fr.west, hx(0x3E6BD6))
		jewelDot(img, fr.north, hx(0xC8323C))
		jewelDot(img, fr.south, hx(0xC8323C))
	case model == "drop_car" && c.Part == "canopy":
		// circus-valance skirt on the canopy ring's long faces (white cloth, gold piping, scalloped
		// red hem with cutout notches) — the white trim that keeps the red top readable vs the
		// magenta bow at the hang (Aphmau), at native texel density (faces are only 5px tall).
		valance(img, fr.north)
		valance(img, fr.south)
	}
}

// brooch: Peach's sapphire chest brooch — a small blue diamond with a bright core, upper-centre of
// the bodice front face (east faces blit unrotated; v=0 = world top, so small v = upper chest).
func brooch(img *image.RGBA, r rect) {
	if r.w < 8 || r.h < 8 {
		return
	}
	cx, cy := r.x+r.w/2, r.y+r.h*3/10
	deep, bright := hx(0x2E5FD8), hx(0x6FA0F0)
	rad := r.h / 6
	if rad < 3 {
		rad = 3
	}
	for dy := -rad; dy <= rad; dy++ {
		for dx := -rad; dx <= rad; dx++ {
			d := abs(dx) + abs(dy)
			if d > rad {
				continue
			}
			c := deep
			if d <= rad/3 {
				c = bright
			}
			px, py := cx+dx, cy+dy
			if px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

// croppedImg: a windowed view of an image (for blitting only a band of the source art).
type croppedImg struct {
	src image.Image
	r   image.Rectangle
}

func (c *croppedImg) ColorModel() color.Model     { return c.src.ColorModel() }
func (c *croppedImg) Bounds() image.Rectangle     { return c.r }
func (c *croppedImg) At(x, y int) color.Color     { return c.src.At(x, y) }

// dkTieLetters: chunky gold pixel "D" above "K" on the tie front (22w x 50h, v=0 = world top).
func dkTieLetters(img *image.RGBA, r rect) {
	gold := hx(0xF4C542)
	dRows := []string{
		"XXXXXX..",
		"XXXXXXX.",
		"XX...XX.",
		"XX....XX",
		"XX....XX",
		"XX....XX",
		"XX....XX",
		"XX...XX.",
		"XXXXXXX.",
		"XXXXXX..",
	}
	kRows := []string{
		"XX....XX",
		"XX...XX.",
		"XX..XX..",
		"XX.XX...",
		"XXXX....",
		"XXXX....",
		"XX.XX...",
		"XX..XX..",
		"XX...XX.",
		"XX....XX",
	}
	draw := func(rows []string, ox, oy int) {
		for ry, row := range rows {
			for rx, ch := range row {
				if ch != 'X' {
					continue
				}
				for sy := 0; sy < 2; sy++ {
					for sx := 0; sx < 2; sx++ { // 2x scale -> 16x20 per letter
						px, py := r.x+ox+rx*2+sx, r.y+oy+ry*2+sy
						if px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h {
							img.SetRGBA(px, py, gold)
						}
					}
				}
			}
		}
	}
	draw(dRows, (r.w-16)/2, 4)
	draw(kRows, (r.w-16)/2, 27)
}

// dkMuzzle: nostrils + a wide friendly grin on the muzzle's front (east) face. v=0 = world top.
func dkMuzzle(img *image.RGBA, r rect) {
	dark := hx(0x3A2414)
	// two nostril dots in the upper third
	for _, nx := range []int{r.w / 3, r.w * 2 / 3} {
		for dy := 0; dy < 3; dy++ {
			for dx := -2; dx < 2; dx++ {
				px, py := r.x+nx+dx, r.y+r.h/4+dy
				if px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h {
					img.SetRGBA(px, py, dark)
				}
			}
		}
	}
	// wide grin: a horizontal line with upturned ends in the lower third
	gy := r.y + r.h*2/3
	for x := r.w / 6; x < r.w*5/6; x++ {
		img.SetRGBA(r.x+x, gy, dark)
		img.SetRGBA(r.x+x, gy+1, dark)
	}
	for d := 1; d <= 2; d++ {
		img.SetRGBA(r.x+r.w/6, gy-d, dark)
		img.SetRGBA(r.x+r.w*5/6-1, gy-d, dark)
	}
}

// hemBand: deeper-pink ring along the gown's WORLD-bottom edge. E/W faces show v=h-1 at world
// bottom; N/S faces are 180°-rotated in-game so their world bottom is v=0.
func hemBand(img *image.RGBA, r rect, rotated bool) {
	band := 6 // 6 units = 1/16 block at 96u/block
	if band > r.h {
		band = r.h
	}
	c := hx(0xE0529A)
	for y := 0; y < band; y++ {
		fy := r.h - 1 - y
		if rotated {
			fy = y
		}
		for x := 0; x < r.w; x++ {
			img.SetRGBA(r.x+x, r.y+fy, c)
		}
	}
}

// jewelDot: a centred 6x6 gem with a bright core on a crown band face.
func jewelDot(img *image.RGBA, r rect, deep color.RGBA) {
	bright := mix(deep, hx(0xFFFFFF), 0.45)
	cx, cy := r.x+r.w/2, r.y+r.h/2
	for dy := -3; dy < 3; dy++ {
		for dx := -3; dx < 3; dx++ {
			c := deep
			if dx >= -1 && dx < 1 && dy >= -1 && dy < 1 {
				c = bright
			}
			px, py := cx+dx, cy+dy
			if px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// valance: white cloth band with gold top piping and a scalloped red hem; the bottom row is cut out
// (alpha 0) between scallop tips so the canopy edge reads as hanging fabric, not a slab.
func valance(img *image.RGBA, r rect) {
	if r.w < 4 || r.h < 3 {
		return
	}
	gold, white, red := hx(0xD7B254), hx(0xFBF3DD), hx(0xE0445A)
	clear := color.RGBA{0, 0, 0, 0}
	for x := 0; x < r.w; x++ {
		p := x % 8
		for y := 0; y < r.h; y++ {
			// side-face texture content shows 180°-rotated in-game (see CLAUDE.md UV orientation):
			// paint upside down so the gold piping ends up on TOP and the scallops hang BELOW.
			fy := r.h - 1 - y
			c := white
			if fy == 0 {
				c = gold
			} else if fy == r.h-2 && (p < 2 || p > 5) {
				c = red
			} else if fy == r.h-1 {
				if p >= 3 && p <= 4 {
					c = red // scallop tip
				} else {
					c = clear // notch between scallops
				}
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
}

// slideheadMouth: the gaping-mouth interior on the giant dino skull's front face — dark maroon
// band where the jaw gap sits (lower ~52%) with a happy pink tongue oval at its floor.
func slideheadMouth(img *image.RGBA, r rect) {
	// the rider boards by walking INTO this face — up close it fills the whole screen, so it
	// must read MOUTH (warm red + big tongue), not "featureless dark wall" (user: "have to run
	// through a wall"). Brighter band + a tongue that spans most of the maw.
	mouth := hx(0x96293F)
	throat := hx(0x6E1C30)
	tongue := hx(0xEE6E8C)
	tongueHi := hx(0xF7A0B4)
	y0 := r.h * 48 / 100
	for y := y0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			c := mouth
			// darker oval throat in the middle — depth without going black
			dx := (float64(x) - float64(r.w-1)/2) / (float64(r.w) * 0.22)
			dy := (float64(y) - float64(r.h)*0.62) / (float64(r.h) * 0.16)
			if dx*dx+dy*dy <= 1 {
				c = throat
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
	cx := float64(r.w-1) / 2
	cy := float64(r.h) * 0.88
	for y := y0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			dx := (float64(x) - cx) / (float64(r.w) * 0.42)
			dy := (float64(y) - cy) / (float64(r.h) * 0.20)
			if dx*dx+dy*dy <= 1 {
				img.SetRGBA(r.x+x, r.y+y, tongue)
			}
			if dx*dx+(dy+0.35)*(dy+0.35) <= 0.18 {
				img.SetRGBA(r.x+x, r.y+y, tongueHi) // glossy highlight stripe
			}
		}
	}
}

// eyeIris: a big friendly green iris + black pupil + white glint on an eye puck's front face.
func eyeIris(img *image.RGBA, r rect) {
	// WHITE sclera fills the whole puck face — dark-on-dark dots vanished at queue distance
	// (Aphmau final polish note); a white-rimmed eye pops from the boarding deck.
	sclera := hx(0xFFFFFF)
	iris := hx(0x3E8E4E)
	pupil := hx(0x14181A)
	cx := float64(r.w-1) / 2
	cy := float64(r.h-1) / 2
	maxR := math.Min(cx, cy)
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d <= maxR*0.40 {
				img.SetRGBA(r.x+x, r.y+y, pupil)
			} else if d <= maxR*0.72 {
				img.SetRGBA(r.x+x, r.y+y, iris)
			} else {
				img.SetRGBA(r.x+x, r.y+y, sclera)
			}
		}
	}
	for dy := 0; dy < 2; dy++ {
		for dx := 0; dx < 3; dx++ { // big glossy glint
			img.SetRGBA(r.x+r.w*32/100+dx, r.y+r.h*28/100+dy, hx(0xFFFFFF))
		}
	}
}

// skullEyes: two big white-sclera cartoon eyes with green iris + glossy glint on the skull's
// FRONT face upper half (above the painted maw) — readable from the whole boarding deck.
func skullEyes(img *image.RGBA, r rect) {
	for _, fx := range []int{r.w * 30 / 100, r.w * 70 / 100} {
		cx := r.x + fx
		cy := r.y + r.h*26/100
		rad := r.h * 11 / 100
		if rad < 4 {
			rad = 4
		}
		for dy := -rad; dy <= rad; dy++ {
			for dx := -rad; dx <= rad; dx++ {
				d := dx*dx + dy*dy
				if d > rad*rad {
					continue
				}
				c := hx(0xFFFFFF)
				if d <= rad*rad*45/100 {
					c = hx(0x3E8E4E) // friendly green iris
				}
				if d <= rad*rad*16/100 {
					c = hx(0x14181A) // pupil
				}
				img.SetRGBA(cx+dx, cy+dy, c)
			}
		}
		for gy := 0; gy < 2; gy++ {
			for gx := 0; gx < 3; gx++ {
				img.SetRGBA(cx-rad/3+gx, cy-rad/3+gy, hx(0xFFFFFF)) // glint
			}
		}
	}
}

// nostrils: two darker-pink dots on the dino snout's forward face.
// eyeDot: a round black cartoon eye with a white glint, centred on a head side face (centred
// horizontally on purpose — N/S face u-direction mirroring differs per side, centre can't be wrong).
func eyeDot(img *image.RGBA, r rect) {
	black := hx(0x17171A)
	cx, cy := r.x+r.w/2, r.y+r.h*38/100
	rad := r.h / 5
	if rad < 2 {
		rad = 2
	}
	for dy := -rad; dy <= rad; dy++ {
		for dx := -rad; dx <= rad; dx++ {
			if dx*dx+dy*dy <= rad*rad {
				img.SetRGBA(cx+dx, cy+dy, black)
			}
		}
	}
	img.SetRGBA(cx-rad/2, cy-rad/2, hx(0xFFFFFF))
}

func nostrils(img *image.RGBA, r rect) {
	d := hx(0xC2487E)
	for _, fx := range []int{r.w * 30 / 100, r.w * 70 / 100} {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if dx*dx+dy*dy <= 4 {
					img.SetRGBA(r.x+fx+dx, r.y+r.h/2+dy, d)
				}
			}
		}
	}
}

// pawPad: a big pad oval + three toe dots on a mitten front (deeper pink than the mitten, per ref).
func pawPad(img *image.RGBA, r rect) {
	pad := hx(0xD96898)
	cx := float64(r.w-1) / 2
	cy := float64(r.h) * 0.62
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			dx := (float64(x) - cx) / (float64(r.w) * 0.30)
			dy := (float64(y) - cy) / (float64(r.h) * 0.24)
			if dx*dx+dy*dy <= 1 {
				img.SetRGBA(r.x+x, r.y+y, pad)
			}
		}
	}
	for _, fx := range []int{r.w * 25 / 100, r.w * 50 / 100, r.w * 75 / 100} {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				img.SetRGBA(r.x+fx+dx, r.y+r.h*30/100+dy, pad)
			}
		}
	}
}

// flauschiBelly: the onesie's lighter belly oval + three button dots on the front face.
func flauschiBelly(img *image.RGBA, r rect) {
	light := hx(0xF8E4EC)
	btn := hx(0xC2487E)
	cx := float64(r.w-1) / 2
	cy := float64(r.h) * 0.55
	rx := float64(r.w) * 0.32
	ry := float64(r.h) * 0.38
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			if dx*dx+dy*dy <= 1 {
				img.SetRGBA(r.x+x, r.y+y, light)
			}
		}
	}
	for _, by := range []int{r.h * 32 / 100, r.h * 50 / 100, r.h * 68 / 100} {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if dx*dx+dy*dy <= 5 {
					img.SetRGBA(r.x+r.w/2+dx, r.y+by+dy, btn)
				}
			}
		}
	}
}

// aphmauTorso: white tank-top band down the middle of the open purple cardigan.
// E/W faces render unrotated; N/S render 180° in-game — same band either way (vertical symmetry),
// but the neckline skin strip must flip.
func aphmauTorso(img *image.RGBA, r rect, rotated bool) {
	white := hx(0xFAFAF8) // bright — the painted base shading made F2F0EC read gray (Aphmau)
	skin := hx(0xCA8A5C)
	x0 := r.w * 30 / 100
	x1 := r.w * 70 / 100
	for y := 0; y < r.h; y++ {
		for x := x0; x <= x1 && x < r.w; x++ {
			img.SetRGBA(r.x+x, r.y+y, white)
		}
	}
	for t := 0; t < r.h*12/100; t++ { // off-shoulder neckline: skin strip at the top
		y := t
		if rotated {
			y = r.h - 1 - t
		}
		for x := 0; x < r.w; x++ {
			img.SetRGBA(r.x+x, r.y+y, skin)
		}
	}
}

// hairStreaks: a few near-black vertical strand lines over the glossy bob faces.
func hairStreaks(img *image.RGBA, r rect, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	c := hx(0x322E3E) // purple-tinted highlight — visible against the near-black bob
	for i := 0; i < max(2, r.w/8); i++ {
		x := rng.Intn(max(1, r.w))
		for y := 0; y < r.h; y++ {
			if rng.Intn(5) > 0 {
				img.SetRGBA(r.x+x, r.y+y, c)
			}
		}
	}
}

// aphmauLeg: denim shorts → skin → purple striped sock → white sneaker, top to bottom.
func aphmauLeg(img *image.RGBA, r rect, rotated bool) {
	denim := hx(0x5C7BA8)
	skin := hx(0xCA8A5C)
	sockA := hx(0x7B5FC0)
	sockB := hx(0xB9A7E8)
	shoe := hx(0xFAFAF8)
	for t := 0; t < r.h; t++ {
		y := t
		if rotated {
			y = r.h - 1 - t
		}
		var c color.RGBA
		p := t * 100 / r.h
		switch {
		case p < 28:
			c = denim
		case p < 55:
			c = skin
		case p < 85:
			c = sockA
			if (t/3)%2 == 1 {
				c = sockB
			}
		default:
			c = shoe
		}
		for x := 0; x < r.w; x++ {
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
}

// speakerCone: concentric woofer ripples with a gold dust-cap + outer ring, on a dark corner panel.
func speakerCone(img *image.RGBA, r rect) {
	dark := hx(0x241B2E)
	gold := hx(0xD7B254)
	cx := float64(r.w-1) / 2
	cy := float64(r.h-1) / 2
	maxR := math.Min(cx, cy)
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			var c color.RGBA
			switch {
			case d > maxR:
				c = hx(0x1A1126) // panel corner
			case d > maxR-2:
				c = gold // surround ring
			case d <= maxR*0.18:
				c = gold // dust cap
			default:
				if int(d/3)%2 == 0 {
					c = dark
				} else {
					c = sh(dark, 1.5) // ripple highlight
				}
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
}

// vinylWindow: the spinning-record porthole — teal glowing frame, navy glass, grooved vinyl disc.
func vinylWindow(img *image.RGBA, r rect) {
	navy := hx(0x10142A)
	teal := hx(0x2EE6D6)
	cx := float64(r.w-1) / 2
	cy := float64(r.h-1) / 2
	discR := math.Min(cx, cy) - 3
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			c := navy
			if x < 2 || y < 2 || x >= r.w-2 || y >= r.h-2 {
				c = teal
			} else if d <= discR {
				switch {
				case d <= discR*0.18:
					c = hx(0xE8568C) // pink label
				case int(d)%4 == 0:
					c = hx(0x32323C) // groove sheen
				default:
					c = hx(0x0A0A0E) // vinyl
				}
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
}

// buttonRow: four chunky candy-colored song buttons on the brass panel.
func buttonRow(img *image.RGBA, r rect) {
	cols := []color.RGBA{hx(0xE8568C), hx(0x46C7E0), hx(0xF0C838), hx(0x9B5DE5)}
	n := len(cols)
	for i, c := range cols {
		bx := r.w*i/n + 2
		bw := r.w/n - 4
		for y := 3; y < r.h-3; y++ {
			for x := bx; x < bx+bw && x < r.w; x++ {
				cc := c
				if y < 6 {
					cc = sh(c, 1.3) // top bevel highlight
				}
				img.SetRGBA(r.x+x, r.y+y, cc)
			}
		}
	}
}

// bubbleColumn: a frosted tube face with rising teal/pink bubbles (classic jukebox bubble tube).
func bubbleColumn(img *image.RGBA, r rect, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	base := hx(0xCFF7F0)
	deep := hx(0x9FE8DC)
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			c := base
			if x == 0 || x == r.w-1 {
				c = deep // tube edge shading
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
	for i := 0; i < r.h/5; i++ {
		bx := 1 + rng.Intn(max(1, r.w-3))
		by := rng.Intn(max(1, r.h-2))
		col := hx(0x2EE6D6)
		if rng.Intn(3) == 0 {
			col = hx(0xF285B5)
		}
		img.SetRGBA(r.x+bx, r.y+by, col)
		img.SetRGBA(r.x+bx, r.y+by+1, col)
	}
}

// neonEdge: bright piping along a face's TOP + both sides. E/W faces render unrotated (v=0 = top);
// N/S faces render 180°-rotated in-game, so their top band paints at the BOTTOM of the atlas rect.
func neonEdge(img *image.RGBA, r rect, col color.RGBA, rotated bool) {
	hot := mix(col, hx(0xFFFFFF), 0.35)
	for x := 0; x < r.w; x++ {
		for t := 0; t < 3 && t < r.h; t++ {
			y := t
			if rotated {
				y = r.h - 1 - t
			}
			c := col
			if t == 1 {
				c = hot
			}
			img.SetRGBA(r.x+x, r.y+y, c)
		}
	}
	for y := 0; y < r.h; y++ {
		for t := 0; t < 2 && t < r.w; t++ {
			img.SetRGBA(r.x+t, r.y+y, col)
			img.SetRGBA(r.x+r.w-1-t, r.y+y, col)
		}
	}
}

// shelf bin-pack: returns atlas height; sets each cube's UV.
func pack(cubes []Cube, aw int) int {
	type item struct{ idx, w, h int }
	items := make([]item, len(cubes))
	for i, c := range cubes {
		w := int(math.Round(c.Size[0])); h := int(math.Round(c.Size[1])); d := int(math.Round(c.Size[2]))
		if w < 1 { w = 1 }; if h < 1 { h = 1 }; if d < 1 { d = 1 }
		items[i] = item{i, fpW(w, h, d) + PAD, fpH(w, h, d) + PAD}
		if items[i].w > aw {
			panic("cube footprint wider than the atlas: " + c.Part)
		}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].h > items[b].h })
	x, y, shelfH := 0, 0, 0
	for _, it := range items {
		if x+it.w > aw {
			x = 0
			y += shelfH
			shelfH = 0
		}
		cubes[it.idx].UV = []int{x, y}
		x += it.w
		if it.h > shelfH {
			shelfH = it.h
		}
	}
	return y + shelfH
}

func process(name string) {
	dir := "../../src/main/resources/assets/karoland"
	raw, err := os.ReadFile(filepath.Join(dir, "train_models", name+".json"))
	must(err)
	var m Model
	must(json.Unmarshal(raw, &m))

	aw := atlasW(name)
	h := pack(m.Cubes, aw)
	// round atlas height up to a multiple of 16
	h = ((h + 15) / 16) * 16
	m.TexW = aw
	m.TexH = h

	img := image.NewRGBA(image.Rect(0, 0, aw, h))
	// magenta background to spot any unmapped sampling
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	for y := 0; y < h; y++ {
		for x := 0; x < aw; x++ {
			img.SetRGBA(x, y, hx(0x101014))
		}
	}
	for i, c := range m.Cubes {
		paintCube(img, name, c, int64(i+1)*7919)
	}

	// write PNG
	pf, err := os.Create(filepath.Join(dir, "textures", "entity", m.Texture))
	must(err)
	must(png.Encode(pf, img))
	pf.Close()

	// write JSON back (with new uv + texW/texH)
	out, err := json.MarshalIndent(m, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(dir, "train_models", name+".json"), out, 0o644))

	println("HD", name, "-> atlas", aw, "x", h, "px,", len(m.Cubes), "cubes unwrapped")
}

func main() {
	loadCanopy()
	loadKirbyFace()
	loadPeachFace()
	loadDkFace()
	loadJukeboxMarquee()
	loadSplashFaces()
	loadElonFace()
	process("loco")
	process("coach")
	process("pip")
	process("carousel")
	process("teacups")
	process("ferris_wheel")
	process("ferris_gondola")
	process("swing")
	process("drop_car")
	process("kirby_vendor")
	process("peach")
	process("dk")
	process("jukebox")
	process("flauschi")
	process("aphmau")
	process("slidehead")
	process("dive_ring")
	process("flamingo")
	process("elon")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
