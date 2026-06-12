---
name: npc-design
description: How to design a custom NPC/character entity for the karoland mod — offline-first. Author the cube model + textures and iterate to an Aphmau 10/10 WITHOUT launching the game (preview.go renders in ~1s), then wire the entity/renderer/registration, then verify in-game with cameras + functional asserts. Use when asked to add or redesign any character, mascot, vendor, or creature entity.
---

# Designing a custom NPC (offline-first)

The Kirby soda-seller was built with this exact loop: **offline model iteration → Aphmau 10/10 →
code wiring → in-game proof**. Total game launches needed: ~3 (verification only). Never iterate
geometry or textures in-game — preview renders take ~1s, a launch takes ~4 min.

## Phase 1 — model + texture, fully offline

### 1. Author the geometry JSON
`karoland/src/main/resources/assets/karoland/train_models/<name>.json` — cubes with `part`, `pos`,
`size` (+ optional `pivot`/`rot`, see drop_car for rotated rings). Conventions (the Pip standard):
- **+Y is DOWN** (renderer flips). Origin at the GROUND under the character. NEGATIVE y = up.
- **+X is FORWARD** (the face/snout side). Entity yaw 0 + the standard renderer transform
  (`mulPose(YP, 180 - yaw)`) makes +X point world **south (+Z)**.
- 16 units = 1 block — UNLESS the NPC needs a detailed face: author at **2x units** (32u = 1 block)
  and have the renderer scale by 0.5; the atlas is 1px/unit, so 2x doubles face resolution.
- Write a `_datum` line stating the scale, orientation and renderer expectations.
- Avoid coplanar faces: sink attached parts (arms, ears) ≥2u INTO the body; make feet/snouts
  protrude PAST the body face, never end flush on it (z-fighting doctrine, see CLAUDE.md).

### 2. Register it in the art pipeline (`karoland/art/train/`)
- `hdtexture.go`: add a `classify("<name>", part)` case (colors/materials per part) + a
  `process("<name>")` call in main. Run `go run hdtexture.go` after EVERY geometry edit — it
  bin-packs UVs back into the JSON and paints the atlas PNG into resources.
- `preview.go`: add `render("<name>")`. `go run preview.go` → `preview_<name>.png` with
  SIDE/FRONT/TOP/ISO panes + UV strip.

### 3. The face: generate with generate-with-refs, blit in hdtexture
Gemini cannot hit UV coordinates — generate the FACE ART as a standalone square and blit it:
- Prompt: "FLAT 2D texture of the face only, dead-on, skin color #XXXXXX edge to edge, no body
  outline, no background" + the brand refs (`karoland/art/brand/`). Put it in
  `karoland/art/train/gen/<name>/face-chosen.png`.
- In hdtexture.go: load it (see `loadKirbyFace`) and `blitResized` onto the body cube's `fr.east`
  rect (= the +X/front face), **UNROTATED** — east/west faces render upright in-game. Only
  NORTH/SOUTH faces need a 180° pre-rotation (CLAUDE.md "MC UV Orientation"; both rules are
  capture-verified). If unsure for a new face, one studio orbit (below) settles it in minutes.
- Make the face art's background hex EXACTLY the classify() base color of the body so the blitted
  face blends seamlessly with the painted faces.

### 4. Preview-reading rules (so you don't chase ghosts)
- The preview is **vertically MIRRORED vs the game** (feet appear at top). Judge proportions,
  silhouette, colors, part placement — never up/down orientation.
- east/west faces used to render 90°-rotated in preview (fixed 2026-06; uniform fills had hidden
  it for years). If face content ever looks rotated in preview but the atlas strip looks right,
  suspect the PREVIEWER before the pipeline — the game truth is the documented 180° rule.

### 5. Iterate with Aphmau until 10/10 — offline
Spawn the Aphmau reviewer (general-purpose subagent, see the playtest skill) with
`preview_<name>.png` + the face art + the geometry numbers. Tell her the mirror caveat. She gives
cube-level edits (sizes/positions in model units) — apply, `go run hdtexture.go && go run
preview.go`, re-review. Each round costs ~1 minute. Do NOT proceed to code before her 10/10.

## Phase 2 — code wiring (one compile, no launches yet)

Per-entity quartet (copy the closest existing: Pip = static greeter, KirbyVendor = merchant,
RideableProp subclasses = rides):
- `<X>Entity extends Entity` — ctor sets `noPhysics = true` + `setNoGravity(true)`; override
  `isPickable()` (true if right-clickable), `canBeCollidedWith` → false, `hurtServer` → false,
  `tick()` calls super + `setDeltaMovement(Vec3.ZERO)`; empty ValueInput/ValueOutput save.
  - **Shopkeeper?** Implement vanilla `net.minecraft.world.item.trading.Merchant` — then
    `interact()` calls `setTradingPlayer(player)` + `openTradingScreen(...)` and the kid gets the
    REAL trade GUI. See KirbyVendorEntity for the full method set (javap the interface if unsure).
- `<X>Model extends EntityModel<<X>RenderState>` — LAYER constant + `createLayer()` →
  `KarolandModels.load("<name>")`.
- `<X>RenderState extends ParkRideRenderState` (gives you the `rideAge` game-time clock).
- `<X>Renderer extends EntityRenderer` — in `extractRenderState`: interpolate yaw
  (`Mth.rotLerp(partialTick, e.yRotO, e.getYRot())`), set `state.rideAge =
  level().getGameTime() + partialTick` (NEVER ageInTicks for anything riders/server sync depends
  on), and set a light floor `LightCoordsUtil.lightCoordsWithEmission(state.lightCoords, 8)` so
  the NPC never renders black at night. In `submit`: bob/look-around off `rideAge`, then
  `mulPose(YP, 180 - yaw)`, then `scale(-1,-1,1)` — or `scale(-0.5,-0.5,0.5)` for 2x models.
- Register: `KarolandEntities` (MobCategory.MISC, `.noLootTable().sized(w,h)`), `KarolandClient`
  (renderer + layer definition).
- Spawning: an `ensure(level)`-style placer deduped by an entity tag, called from
  `KarolandEvents.onServerTick` (and reset the static flag in `onServerStarted`). For stall
  NPCs, gate on the stall block existing (see FoodTraders).

### Placement pitfall (cost us a whole round)
Spots tuned for a tall thin villager can SWALLOW a round/short custom model — the Kirby ball
vanished behind the counter with only the nameplate showing. Place custom NPCs in the OPEN
(greeter position, e.g. `pos + face.getStep* * 2.5`), then verify visually.

## Phase 3 — in-game verification (the only launches)

1. **THE NPC PHOTO STUDIO — use this FIRST**: `KAROLAND_NPCSTUDIO=<entity_id>` (e.g.
   `kirby_vendor`) on the offscreen harness builds a floating quartz stage in the sky (uniform
   backdrop, full light), spawns ONE fresh instance, and auto-orbits the spectator camera 360°
   at eye level (30s/lap, alternating with a high pass). Capture stills every 5s for one lap +
   record the mp4. You get every angle in the REAL renderer with zero camera math and zero park
   clutter. This caught an upside-down face in ONE orbit after four park camera attempts failed
   to even frame the NPC (stall counters, café tables, awning shade all block sightlines).
   Iterate texture/orientation fixes against the studio, not the park.
2. **Functional assert** (no pixels needed): an env-gated hook in KarolandEvents that exercises
   the interaction server-side and logs PASS/FAIL — e.g. KIRBYTEST right-clicks the merchant and
   checks `player.containerMenu instanceof MerchantMenu` + offer count. Pattern: one
   `KAROLAND_<X>TEST=1` flag, one log line, grep it.
3. **In-situ check LAST**: one `KAROLAND_TPCAM` capture of the NPC at its real spot (placement,
   stand occlusion, nameplate) — composition only; the model itself was already proven in the
   studio. Camera at the NPC's EYE height on its facing line (+X = where entity yaw points);
   furniture blocks sightlines.
4. Record mp4s to `karoland/qa-recordings/` (ffmpeg x11grab — user watches these async).
5. **Aphmau in-game round**: placement, scale next to a player, lighting, the interaction flow.
   Items that genuinely need a human eyeball on real hardware (emoji glyphs in nameplates, color
   banding under llvmpipe) — say so explicitly instead of burning offscreen rounds on them.

## Don'ts
- Don't launch the game to check geometry/texture — that's what preview.go is for.
- Don't hand-author UVs (hdtexture.go owns them) or let Gemini draw onto atlas coordinates.
- Don't drive animation off `ageInTicks` (client tracking age ≠ server clock).
- Don't trust "it compiled" for merchant/interaction paths — write the functional assert.
