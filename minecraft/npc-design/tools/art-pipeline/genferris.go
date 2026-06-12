// Procedurally generates the Ferris-wheel geometry JSONs (shared by the game loader, preview.go and
// hdtexture.go). Two models:
//   ferris_wheel.json    — hub + 8 spokes + 24 rim segments (the SPINNING structure)
//   ferris_gondola.json  — one cabin (drawn N times by the renderer, kept LEVEL while it orbits)
//
// Authoring frame: model units (16u = 1 block), +Y DOWN (the renderer flips with scale(-1,-1,1));
// screen-up = model -Y in BOTH preview.go and the game. The wheel lies in the X-Y plane, depth in Z.
// Rim segment / spoke rotations are computed numerically (atan2) so the circle is exact.
//
//   go run genferris.go
package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
)

type gCube struct {
	Part string    `json:"part"`
	UV   []int     `json:"uv"`
	Piv  []float64 `json:"pivot,omitempty"`
	Rot  []float64 `json:"rot,omitempty"`
	Pos  []float64 `json:"pos"`
	Size []float64 `json:"size"`
}
type gModel struct {
	Texture string    `json:"texture"`
	TexW    int       `json:"texW"`
	TexH    int       `json:"texH"`
	Datum   string    `json:"_datum"`
	Cubes   []gCube   `json:"cubes"`
}

// degrees helper
func deg(r float64) float64 { return r * 180 / math.Pi }

func buildWheel() gModel {
	const R = 100.0   // rim centre radius (units) ~6.25 blocks
	const N_RIM = 24  // rim segments
	const N_SPOKE = 8 // spokes (== gondola count)
	var cubes []gCube

	// hub (axle boss) at centre
	cubes = append(cubes, gCube{Part: "hub", Pos: []float64{-13, -13, -12}, Size: []float64{26, 26, 24}})
	// decorative gold axle cap on the front
	cubes = append(cubes, gCube{Part: "axle", Pos: []float64{-6, -6, 12}, Size: []float64{12, 12, 4}})

	// spokes: thin bars from r=12 out to r=98, one per gondola
	for i := 0; i < N_SPOKE; i++ {
		a := float64(i) * 360.0 / N_SPOKE
		ar := a * math.Pi / 180
		rz := deg(math.Atan2(-math.Sin(ar), math.Cos(ar))) // == -a, via atan2 for safety
		cubes = append(cubes, gCube{
			Part: "spoke",
			Piv:  []float64{0, 0, 0},
			Rot:  []float64{0, 0, rz},
			Pos:  []float64{12, -3, -4},
			Size: []float64{86, 6, 8},
		})
	}

	// rim: 24 tangent segments forming the felloe
	segLen := 2*math.Pi*R/float64(N_RIM)*1.18 + 2 // overlap so corners meet
	for i := 0; i < N_RIM; i++ {
		a := float64(i) * 360.0 / float64(N_RIM)
		ar := a * math.Pi / 180
		cx := R * math.Cos(ar)
		cy := -R * math.Sin(ar) // screen-up = -Y
		rz := deg(math.Atan2(-math.Cos(ar), -math.Sin(ar)))
		cubes = append(cubes, gCube{
			Part: "rim",
			Piv:  []float64{cx, cy, 0},
			Rot:  []float64{0, 0, rz},
			Pos:  []float64{-segLen / 2, -5, -9},
			Size: []float64{segLen, 7, 18},
		})
	}

	return gModel{
		Texture: "ferris_wheel.png",
		TexW:    512, TexH: 256,
		Datum: "Ferris wheel SPINNING structure: hub + 8 spokes + 24 rim segments, X-Y plane, depth in Z. " +
			"+Y DOWN; renderer spins about Z. 16u=1 block.",
		Cubes: cubes,
	}
}

func buildGondola() gModel {
	// An open-front cabin that hangs below its rim attach point. Authored +Y DOWN: the hanger yoke is at
	// the TOP (small Y), the cabin tub below it (larger Y). Renderer translates it onto the rim, level.
	var cubes []gCube
	// hanger yoke (two short arms + a cross bar) just below the rim attach (origin)
	cubes = append(cubes, gCube{Part: "hanger", Pos: []float64{-11, 0, -2}, Size: []float64{22, 4, 4}})
	cubes = append(cubes, gCube{Part: "hanger", Pos: []float64{-11, 2, -2}, Size: []float64{4, 12, 4}})
	cubes = append(cubes, gCube{Part: "hanger", Pos: []float64{7, 2, -2}, Size: []float64{4, 12, 4}})
	// roof (overhanging dome lid)
	cubes = append(cubes, gCube{Part: "roof", Pos: []float64{-15, 12, -13}, Size: []float64{30, 4, 26}})
	// cabin tub: floor + back wall (at -Z / north) + two side walls; the OPEN front faces +Z (south),
	// toward visitors who approach the park from the south.
	cubes = append(cubes, gCube{Part: "cabin", Pos: []float64{-13, 30, -11}, Size: []float64{26, 4, 22}})  // floor
	cubes = append(cubes, gCube{Part: "cabin", Pos: []float64{-13, 16, -13}, Size: []float64{26, 18, 4}})  // back wall (north)
	cubes = append(cubes, gCube{Part: "cabin", Pos: []float64{-13, 16, -11}, Size: []float64{4, 18, 22}})  // left wall
	cubes = append(cubes, gCube{Part: "cabin", Pos: []float64{9, 16, -11}, Size: []float64{4, 18, 22}})    // right wall
	cubes = append(cubes, gCube{Part: "trim", Pos: []float64{-13, 28, -11}, Size: []float64{26, 3, 22}})   // top rail trim
	// bench seat against the back wall; riders face the open south view
	cubes = append(cubes, gCube{Part: "seat", Pos: []float64{-10, 26, -10}, Size: []float64{20, 4, 8}})

	return gModel{
		Texture: "ferris_gondola.png",
		TexW:    256, TexH: 128,
		Datum: "Ferris-wheel gondola: hanger yoke (top) + roof + open-front tub + bench. +Y DOWN; renderer " +
			"keeps it LEVEL as it orbits. 16u=1 block.",
		Cubes: cubes,
	}
}

func writeModel(name string, m gModel) {
	dir := "../../src/main/resources/assets/karoland/train_models"
	// zero-uv placeholder; hdtexture.go bin-packs + rewrites uv
	for i := range m.Cubes {
		m.Cubes[i].UV = []int{0, 0}
	}
	out, err := json.MarshalIndent(m, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(dir, name+".json"), out, 0o644))
	println("wrote", name+".json", "—", len(m.Cubes), "cubes")
}

// buildSwing — a chair-o-plane TOP: tiered canopy + 8 arms, each with a chain + chair flung OUTWARD.
// Spins about Y (renderer); the centrifugal fly-out is baked in (constant spin), so it's one rigid model.
// Chain/chair use a compound rotation: lean ψ from vertical, then azimuth — euler order is Rx then Ry
// (matches KarolandModels.load / preview.go), so rot = (ψ, atan2(cosθ,sinθ), 0).
func buildSwing() gModel {
	const N = 12       // fuller ring of chairs (was 8)
	const rArm = 42.0  // longer arm reach at the top (was 30) — grander wingspan
	const chainLen = 52.0
	const psi = 40.0 // outward lean from vertical (deg)
	var cubes []gCube

	// tiered canopy above the hub (negative Y = up), like the carousel — scaled up to match the wider ring
	cubes = append(cubes, gCube{Part: "canopy_a", Pos: []float64{-40, -8, -40}, Size: []float64{80, 4, 80}})
	cubes = append(cubes, gCube{Part: "canopy_b", Pos: []float64{-26, -12, -26}, Size: []float64{52, 4, 52}})
	cubes = append(cubes, gCube{Part: "canopy_a", Pos: []float64{-13, -16, -13}, Size: []float64{26, 4, 26}})
	cubes = append(cubes, gCube{Part: "topper", Pos: []float64{-3, -22, -3}, Size: []float64{6, 8, 6}})
	cubes = append(cubes, gCube{Part: "hub", Pos: []float64{-6, -2, -6}, Size: []float64{12, 6, 12}})

	for i := 0; i < N; i++ {
		th := float64(i) * 360.0 / N
		thr := th * math.Pi / 180
		ex, ez := rArm*math.Cos(thr), rArm*math.Sin(thr)
		// arm: hub -> rim along +X, swung to azimuth θ (rot y = -θ)
		cubes = append(cubes, gCube{
			Part: "arm", Piv: []float64{0, 0, 0}, Rot: []float64{0, -th, 0},
			Pos: []float64{6, -3, -1.5}, Size: []float64{rArm - 6, 3, 3},
		})
		// chain + chair hang from the arm end, leaning OUT (lean ψ, azimuth via Ry=atan2(cosθ,sinθ))
		ry := deg(math.Atan2(math.Cos(thr), math.Sin(thr)))
		piv := []float64{ex, -2, ez}
		cubes = append(cubes, gCube{
			Part: "chain", Piv: piv, Rot: []float64{psi, ry, 0},
			Pos: []float64{-1.5, 0, -1.5}, Size: []float64{3, chainLen, 3},
		})
		cubes = append(cubes, gCube{
			Part: "chair", Piv: piv, Rot: []float64{psi, ry, 0},
			Pos: []float64{-6, chainLen, -6}, Size: []float64{12, 7, 12},
		})
		cubes = append(cubes, gCube{ // seat back
			Part: "chairback", Piv: piv, Rot: []float64{psi, ry, 0},
			Pos: []float64{-6, chainLen - 8, -7}, Size: []float64{12, 8, 3},
		})
	}

	return gModel{
		Texture: "swing.png",
		TexW:    512, TexH: 192,
		Datum: "Chair-o-plane TOP: tiered canopy + 8 arms with chains + chairs flung outward (lean baked " +
			"in). Spins about Y. +Y DOWN. 16u=1 block.",
		Cubes: cubes,
	}
}

// buildDropCar — a drop-tower CAR: an octagonal collar (floor ring + top canopy ring) that rides up the
// central mast, with 8 outward-facing seats. Renderer drives it up/down (eased sawtooth) + slow gyro spin.
// Centre is hollow (the mast passes through). +Y DOWN, 16u=1 block.
func buildDropCar() gModel {
	const N = 8
	const r = 42.0 // ring radius (units) ~2.6 blocks — clears the mast + its lantern arms
	var cubes []gCube

	ringSeg := func(part string, y, thick, depth float64) {
		segLen := 2*math.Pi*r/float64(N)*1.18 + 2
		for i := 0; i < N; i++ {
			th := float64(i) * 360.0 / N
			thr := th * math.Pi / 180
			cx, cz := r*math.Cos(thr), r*math.Sin(thr)
			ry := deg(math.Atan2(-math.Cos(thr), -math.Sin(thr)))
			cubes = append(cubes, gCube{
				Part: part, Piv: []float64{cx, y, cz}, Rot: []float64{0, ry, 0},
				Pos: []float64{-segLen / 2, -thick / 2, -depth / 2}, Size: []float64{segLen, thick, depth},
			})
		}
	}
	ringSeg("ring", 2, 5, 14)       // floor ring (Y≈2)
	ringSeg("canopy", -20, 5, 16)   // top canopy ring (Y≈-20, "up")

	for i := 0; i < N; i++ {
		th := float64(i) * 360.0 / N
		ry := -th
		// seat (faces outward at radius r) + seat back + a support post to the canopy
		cubes = append(cubes, gCube{Part: "seat", Piv: []float64{0, 0, 0}, Rot: []float64{0, ry, 0},
			Pos: []float64{r - 9, -9, -5}, Size: []float64{9, 4, 10}})
		cubes = append(cubes, gCube{Part: "seatback", Piv: []float64{0, 0, 0}, Rot: []float64{0, ry, 0},
			Pos: []float64{r - 12, -18, -5}, Size: []float64{3, 11, 10}}) // back on the INNER side → faces out
		cubes = append(cubes, gCube{Part: "post", Piv: []float64{0, 0, 0}, Rot: []float64{0, ry, 0},
			Pos: []float64{r - 4, -20, -2}, Size: []float64{3, 22, 3}})
	}
	cubes = append(cubes, gCube{Part: "topper", Pos: []float64{-4, -26, -4}, Size: []float64{8, 6, 8}})

	return gModel{
		Texture: "drop_car.png",
		TexW:    512, TexH: 160,
		Datum: "Drop-tower CAR: octagonal floor + canopy rings + 8 outward seats, hollow centre (mast passes " +
			"through). Renderer rides it up/down + slow gyro spin. +Y DOWN. 16u=1 block.",
		Cubes: cubes,
	}
}

func main() {
	writeModel("ferris_wheel", buildWheel())
	writeModel("ferris_gondola", buildGondola())
	writeModel("swing", buildSwing())
	writeModel("drop_car", buildDropCar())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
