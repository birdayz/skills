---
name: npc-design
description: Offline-first pipeline for designing a custom NPC/character entity for a Minecraft NeoForge mod. Author the cube model + textures and iterate to a persona-reviewer 10/10 WITHOUT launching the game (a ~1s offline previewer), then wire the entity/renderer/registration, then verify in-game with an auto-orbiting camera studio + functional asserts. Use when asked to add or redesign any character, mascot, vendor, or creature entity in a NeoForge mod.
---

# Designing a custom NPC (offline-first)

The loop, proven on a custom vendor NPC: **offline model iteration → reviewer 10/10 → code wiring →
in-game proof**. Total game launches needed: ~3 (verification only). Never iterate geometry or
textures in-game — preview renders take ~1s, a launch takes ~4 min. This builds on the
`playtest-loop` skill (the generic unattended-iteration harness) and uses the
`image-generation:generate-with-refs` skill for face art.

## Phase 1 — model + texture, fully offline

### 1. Author the geometry JSON
`src/main/resources/assets/<modid>/<models_dir>/<name>.json` — cubes with `part`, `pos`, `size`
(+ optional `pivot`/`rot` for rotated parts). Conventions (pick one and write them down):
- **+Y is DOWN** (renderer flips). Origin at the GROUND under the character. NEGATIVE y = up.
- **+X is FORWARD** (the face/snout side). Entity yaw 0 + the standard renderer transform
  (`mulPose(YP, 180 - yaw)`) makes +X point world **south (+Z)**.
- 16 units = 1 block — UNLESS the NPC needs a detailed face: author at **2x units** (32u = 1 block)
  and have the renderer scale by 0.5; the atlas is 1px/unit, so 2x doubles face resolution.
- Write a `_datum` line in the JSON stating the scale, orientation and renderer expectations.
- **Avoid coplanar faces (z-fighting):** sink attached parts (arms, ears) ≥2u INTO the body; make
  feet/snouts protrude PAST the body face, never end flush on it.

### 2. Register it in the art pipeline (`<modid>/art/`)
The pipeline is two Go tools, bundled with this skill (`tools/art-pipeline/`, see Phase-end):
- `hdtexture.go`: add a `classify("<name>", part)` case (colors/materials per part) + a
  `process("<name>")` call in main. Run `go run hdtexture.go` after EVERY geometry edit — it
  bin-packs UVs back into the JSON and paints the atlas PNG into resources.
- `preview.go`: add `render("<name>")`. `go run preview.go` → `preview_<name>.png` with
  SIDE/FRONT/TOP/ISO panes + UV strip.

### 3. The face: generate with generate-with-refs, blit in hdtexture
Gemini cannot hit UV coordinates — generate the FACE ART as a standalone square and blit it:
- Prompt: "FLAT 2D texture of the face only, dead-on, skin color #XXXXXX edge to edge, no body
  outline, no background" + your brand/reference images. Put it in `<modid>/art/gen/<name>/face-chosen.png`.
- In hdtexture.go: load it and `blitResized` onto the body cube's `fr.east` rect (= the +X/front
  face), **UNROTATED** — east/west faces render upright in-game. Only NORTH/SOUTH faces need a 180°
  pre-rotation. If unsure for a new face, one studio orbit (Phase 3) settles it in minutes.
- Make the face art's background hex EXACTLY the `classify()` base color of the body so the blitted
  face blends seamlessly with the painted faces.

### 4. Preview-reading rules (so you don't chase ghosts)
- The preview is **vertically MIRRORED vs the game** (feet appear at top). Judge proportions,
  silhouette, colors, part placement — never up/down orientation.
- If face content ever looks rotated in preview but the atlas strip looks right, suspect the
  PREVIEWER before the pipeline — the game truth is the documented 180°-north/south rule above.

### 5. Iterate with a persona reviewer until 10/10 — offline
Spawn a reviewer subagent (a kid-focused or domain-expert persona — see the `playtest-loop` skill's
LLM-judge section) with `preview_<name>.png` + the face art + the geometry numbers. Tell it the
mirror caveat. It gives cube-level edits (sizes/positions in model units) — apply, `go run
hdtexture.go && go run preview.go`, re-review. Each round costs ~1 minute. Do NOT proceed to code
before a 10/10.

## Phase 2 — code wiring (one compile, no launches yet)

Per-entity quartet (copy the closest existing entity — static greeter, merchant, or ride):
- `<X>Entity extends Entity` — ctor sets `noPhysics = true` + `setNoGravity(true)`; override
  `isPickable()` (true if right-clickable), `canBeCollidedWith` → false, `hurtServer` → false,
  `tick()` calls super + `setDeltaMovement(Vec3.ZERO)`; empty ValueInput/ValueOutput save.
  - **Shopkeeper?** Implement vanilla `net.minecraft.world.item.trading.Merchant` — then
    `interact()` calls `setTradingPlayer(player)` + `openTradingScreen(...)` and the player gets the
    REAL trade GUI. (javap the interface for the full method set.)
- `<X>Model extends EntityModel<<X>RenderState>` — LAYER constant + `createLayer()` →
  your `Models.load("<name>")` helper.
- `<X>RenderState extends` a base render state that carries a game-time clock (e.g. `rideAge`).
- `<X>Renderer extends EntityRenderer` — in `extractRenderState`: interpolate yaw
  (`Mth.rotLerp(partialTick, e.yRotO, e.getYRot())`), set the clock to
  `level().getGameTime() + partialTick` (NEVER ageInTicks for anything riders/server sync depends
  on), and set a light floor `LightCoordsUtil.lightCoordsWithEmission(state.lightCoords, 8)` so
  the NPC never renders black at night. In `submit`: bob/look-around off the game-time clock, then
  `mulPose(YP, 180 - yaw)`, then `scale(-1,-1,1)` — or `scale(-0.5,-0.5,0.5)` for 2x models.
- Register: your entity registry (MobCategory.MISC, `.noLootTable().sized(w,h)`), your client setup
  (renderer + layer definition).
- Spawning: an `ensure(level)`-style placer deduped by an entity tag, called from your mod's
  server-tick hook (and reset the static flag on server-start). For stall NPCs, gate on the stall
  block existing.

### Placement pitfall (cost a whole round)
Spots tuned for a tall thin villager can SWALLOW a round/short custom model — a ball-shaped NPC
vanished behind a counter with only the nameplate showing. Place custom NPCs in the OPEN (greeter
position, e.g. `pos + face.getStep* * 2.5`), then verify visually.

## Phase 3 — in-game verification (the only launches)

1. **THE NPC PHOTO STUDIO — use this FIRST**: an env hook `MYMOD_NPCSTUDIO=<entity_id>` on the
   offscreen harness builds a floating quartz stage in the sky (uniform backdrop, full light),
   spawns ONE fresh instance, and auto-orbits the spectator camera 360° at eye level (30s/lap,
   alternating with a high pass). Capture stills every 5s for one lap + record the mp4. You get
   every angle in the REAL renderer with zero camera math and zero scene clutter. This caught an
   upside-down face in ONE orbit after several in-world camera attempts failed to even frame the NPC
   (counters, tables, awning shade block sightlines). Iterate texture/orientation fixes against the
   studio, not the live scene.
2. **Functional assert** (no pixels needed): an env-gated hook that exercises the interaction
   server-side and logs PASS/FAIL — e.g. right-click the merchant and check
   `player.containerMenu instanceof MerchantMenu` + offer count. Pattern: one `MYMOD_<X>TEST=1`
   flag, one log line, grep it. (See the `playtest-loop` skill's functional-assert section.)
3. **In-situ check LAST**: one camera capture of the NPC at its real spot (placement, occlusion,
   nameplate) — composition only; the model itself was already proven in the studio. Camera at the
   NPC's EYE height on its facing line (+X = where entity yaw points); furniture blocks sightlines.
4. Record mp4s to a `qa-recordings/` dir (ffmpeg x11grab — the user watches these async).
5. **Persona reviewer in-game round**: placement, scale next to a player, lighting, the interaction
   flow. Items that genuinely need a human eyeball on real hardware (emoji glyphs in nameplates,
   color banding under llvmpipe) — say so explicitly instead of burning offscreen rounds on them.

## Don'ts
- Don't launch the game to check geometry/texture — that's what preview.go is for.
- Don't hand-author UVs (hdtexture.go owns them) or let Gemini draw onto atlas coordinates.
- Don't drive animation off `ageInTicks` (client tracking age ≠ server clock).
- Don't trust "it compiled" for merchant/interaction paths — write the functional assert.

## Bundled reference tools (`tools/art-pipeline/`, relative to this skill)

Three Go files the pipeline is built on, as working reference implementations — copy them into your
project's art dir and adapt:

- `preview.go` — THE iteration speedup: renders any JSON cube model + its atlas PNG to a
  side/front/top/iso sheet with a UV overlay in ~1s. Run `go run preview.go` after every edit;
  only launch the game for final verification. Adapt: the model-name list in `main()` and the
  resources path constant.
- `hdtexture.go` — atlas painter: bin-packs each cube's MC box-UV footprint, writes the UVs back
  into the model JSON, and paints per-part materials + detail painters + Gemini face blits.
  Adapt: the `classify(model, part)` palette switch and `process()` calls per model.
- `genferris.go` — example of GENERATING geometry JSON procedurally (rings/helixes needing exact
  rotations) instead of hand-authoring; verify with preview.go's SIDE view before wiring in-game.

They read/write the shared model JSON format described above, so previewer output == game input.
