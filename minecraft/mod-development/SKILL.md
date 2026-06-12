---
name: mod-development
description: How to develop the Minecraft NeoForge mods in this repo (firstmod, karoland). Repo layout, versions, looking up the real deobfuscated API, the unit-test-first strategy, how to launch and DRIVE the game (keyboard/mouse) on Wayland/sway with screenshots, the experiment requirement, and the MC 26.1.x API gotchas. Use this whenever working on any mod in first-mc-mod.
---

# Minecraft Mod Development (this repo)

Hard-won workflow for `first-mc-mod`. Read this first; it saves hours.

## TL;DR golden rules
1. **Iterate in unit tests, not the game.** Launching + driving the client is ~60s/cycle. Put all pure logic in MC-free classes (e.g. `RaceLogic`) and test with `./gradlew :<mod>:test` (seconds).
1b. **For VISUAL work, iterate fully AUTONOMOUSLY via the headless offscreen harness** (Xvfb + software GL + an env-gated auto-demo + ffmpeg → GIF/PNG you can Read). See "Headless OFFSCREEN capture" below. This renders + records WITHOUT ever showing on the user's screen — so you can build→see→fix on your own (no need for the user to launch/watch), and it can't spoil a surprise. This is the key to working without the user in the loop.
2. **Look up the real API in the decompiled sources** (see below) — don't guess. MC 26.1.x renamed a lot.
3. **A player riding a vanilla minecart is simulated client-side** — server `setPos`/velocity is ignored for the rider UNLESS the world has the *Minecart Improvements* experiment (then carts are server-authoritative).
4. **Commit at checkpoints.** Branch off `main` first. No "claude" co-author (per global CLAUDE.md).

## Repo layout (Gradle multi-project)
```
first-mc-mod/
  settings.gradle          # include 'karoland'  (firstmod is the ROOT project)
  build.gradle             # firstmod (root mod)
  src/main/java/dev/birdy/firstmod/...
  karoland/                # second mod — its own jar, loads alone
    build.gradle           # self-contained: mod_id/version/neo_version are local `def`s
                           #   (subproject gradle.properties are NOT read as project props!)
    src/main/java/dev/birdy/karoland/...
    src/main/resources/assets/karoland/...      # items/, models/item/, lang/, textures/
    src/main/templates/META-INF/neoforge.mods.toml  # expanded by generateModMetadata
    src/test/java/dev/birdy/karoland/...        # JUnit (pure-logic) tests
  tools/keysend/main.go    # Go uinput keyboard injector (see "Driving the game")
  rfcs/                    # design docs
  net/                     # hand-vendored MC/NeoForge stubs for reference (NOT on the build path)
  assets/                  # dev-time asset-gen scratch (not in the build)
```
Each mod builds independently: `./gradlew :karoland:build` → `karoland/build/libs/karoland-0.1.0.jar`.

## Versions & toolchain
- **Minecraft uses calver since 2026**: `26.1`, `26.1.1`, `26.1.2` … (year.release.patch). `26.1` = "Tiny Takeover" (Mar 2026).
- **NeoForge** is 4-part: `26.1.2.73` = MC 26.1.2 + Neo build 73. Find the newest at
  `https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml` (WebFetch the XML).
- **Java 25**, Gradle 9.2.1, ModDevGradle 2.0.141.
- Policy here: **bleeding edge, stability irrelevant** — but you can't build against a NeoForge version that doesn't exist yet (e.g. a brand-new MC before Neo ships for it). Pin in `karoland/build.gradle`'s `neo_version` def.

## Looking up the REAL API (don't guess)
ModDevGradle decompiles MC to a sources jar. Find + grep it:
```bash
# the decompiled SOURCES jar (has .java with real bodies):
JAR=$(find ~/.gradle/caches/neoformruntime -name "*.jar" \
  -exec sh -c 'unzip -l "$1" 2>/dev/null | grep -q "net/minecraft/world/entity/Entity.java" && echo "$1"' _ {} \; | head -1)
unzip -o "$JAR" "net/minecraft/world/entity/Entity.java" -d /tmp/mcsrc
grep -nE "public .* methodName" /tmp/mcsrc/net/minecraft/.../Foo.java
```
For class *locations*, list a CLASS jar instead (`preProcessJar_*.jar`) and grep paths.

### MC 26.1.x gotchas already discovered
- Minecart classes moved to `net.minecraft.world.entity.vehicle.minecart.*` (e.g. `AbstractMinecart`, `Minecart`, `MinecartBehavior`).
- `Entity.entityTags()` (not `getTags()`); `entity.addTag(...)` still exists.
- `GameProfile.name()` (record accessor), not `getName()`.
- No `Entity.hasImpulse` field; `Entity.hurtMarked` (public) still exists — set it to sync server-set velocity to the client.
- `CommandSourceStack` has no `hasPermission(int)` (uses `PermissionSet`); just omit `.requires(...)` for a personal mod.
- GameRules: `net.minecraft.world.level.gamerules.GameRules`; rules renamed (`ADVANCE_TIME`, `ADVANCE_WEATHER`, `SPAWN_MOBS`, `MAX_MINECART_SPEED`). API: `level.getGameRules().set(GameRules.X, value, server)` and `.get(GameRules.X)`.
- SavedData is codec-based `SavedDataType` via `serverLevel.getDataStorage().computeIfAbsent(...)`.
- Item `use(Level, Player, InteractionHand)` returns `InteractionResult`.
- Events: `ServerTickEvent.Post` and `RegisterCommandsEvent` and `PlayerInteractEvent.RightClickBlock` on `NeoForge.EVENT_BUS` via `@EventBusSubscriber(modid=...)`.
- ServerPlayer titles: `player.connection.send(new ClientboundSetTitleTextPacket(...))` (+ subtitle/animation packets).
- Fireworks: `new FireworkRocketEntity(level, x, y, z, itemStack)` where the stack has a `DataComponents.FIREWORKS` (`Fireworks(flight, List<FireworkExplosion>)`).

## The minecart / movement insight (important)
A **ridden vanilla minecart is simulated by the rider's client** → server `setPos`/`setDeltaMovement` on it is ignored for that rider (you'll see it move in server logs but the player sits still). Options:
- **Minecart Improvements experiment ON** → carts become *server-authoritative*; server `setPos` drives the rider smoothly. This is what karoland relies on.
- karoland's coaster is **driven by `setPos` along a precomputed 3D path** (`RaceLogic.buildPath()` → `Faire.lanePoint(lane, dist)`), NOT rail physics — so it follows arbitrary turns/helix/drops, can't stall or derail, and the speed is fully controlled (`RaceLogic.cartSpeed`). `ServerTickEvent.Post` fires *after* entities tick, so velocity set there gets overwritten — use `setPos`, not velocity.
- Empty (rider-less) carts unload if they travel >~render distance from any player; only matters for solo races.
- **The ride is now an invisible server-controlled ARMOR-STAND SEAT the player rides** (the visible minecart is moved alongside, not ridden) → server `setPos` carries the rider correctly in SP *and* MP with **no experiment needed**. (The experiment is no longer required.)

### Making a `setPos`-driven ride feel STEADY (hard-won)
A naive per-step `setPos` ride twitches. Three independent causes, each fixed in pure (unit-tested) logic:
1. **Polyline kink** — linearly interpolating between integer path points snaps the travel direction at every vertex. Fix: sample a **Catmull-Rom spline** through the points (`RaceLogic.sample`) — C1-continuous, passes exactly through control points (so the track mesh is unchanged).
2. **Speed jolt** — discrete per-step slope made speed jump each step boundary. Fix: **continuous slope** from the spline derivative (`RaceLogic.slopeSmooth`/`tangent`) + **jerk limiting** (`RaceLogic.approachSpeed`, `MAX_ACCEL`) so speed eases toward its target.
3. **Sync extrapolation jitter** — sending per-tick velocity (`setDeltaMovement` + `hurtMarked`) AND `setPos` makes the client extrapolate ahead then snap back. So: `setDeltaMovement(Vec3.ZERO)`, no `hurtMarked`.
4. **THE big one — the ridden entity's sync cadence.** The rider rides an **`ArmorStand`**, which has **no client `InterpolationHandler` and updates only every 3 ticks** (`EntityType` default `updateInterval=3`). So plain `setPos` made the rider hard-teleport ~1.2 blocks every 150ms = stutter. Fix: set `entity.needsSync = true` (public field) **every racing tick** → `ServerEntity.sendChanges` emits a position packet every tick → the client's 1-tick render lerp smooths it. Set it on both the seat AND the visible minecart (the minecart *does* have an interp handler, but per-tick keeps them locked together). This was the actual cure; position-only sync alone just exposed the bare 3-tick stepping.
Orient the visible cart from the **smooth tangent** (not the noisy per-tick delta), eased via `Mth.degreesDifference`. Use the **minecart's OWN angle convention**, not entity-facing: off-rail `AbstractMinecartRenderer.oldRender` renders `YP(180 - yRot)` with `yRot = atan2(dz,dx)°` (NO −90 offset — that renders it 90° sideways) and pitch `atan(dy)*73`. `simulatedRideIsLaterallySteady` replays the motion model (lateral turn < threshold); add to it when you touch motion.
- **World-Y gotcha:** `buildPath()` shifts the whole track UP by `-minY` so descents clear y=0, so the station can sit ~Y144 while the finish is ~Y100. Any world-Y logic (e.g. a "seat players near the station" AABB) must anchor on the actual structure (`RaceTrackBuilder.START_PAD.getY()`), NOT `Faire.Y` — a box at `Faire.Y` sits ~40 blocks below the platform and matches nobody.

## Testing pyramid (cheap → expensive)
1. **Unit tests (JUnit, no MC):** all pure logic lives in `RaceLogic` (path geometry, slope→speed, rubber-band, winner). `./gradlew :karoland:test` — seconds, no launch. Build/deps already wired in `karoland/build.gradle` (mavenCentral + junit-bom).
2. **Headless server load-check:** `timeout --signal=KILL 90 ./gradlew :karoland:runServer --console=plain > /tmp/srv.log 2>&1` then grep `/tmp/srv.log` for `Karoland loaded` and any `Exception`. No window — safe to run anytime. Confirms registration + static init (e.g. the path builds) without crashing.
3. **Live client (visual only):** last resort; see below.
What you CANNOT unit-test: actual minecart physics, rendering, the experiment. Cover those with (2) + a quick (3).

## Running the game
```bash
./gradlew :karoland:runClient --console=plain > /tmp/mc.log 2>&1          # opens the client (title screen)
./gradlew :karoland:runClient -PquickWorld="New World (1)" ...            # boot STRAIGHT into a world (skips menus)
./gradlew :karoland:runServer --console=plain ...                         # headless dedicated server (no window)
```
`-PquickWorld` is wired in `karoland/build.gradle` (passes `--quickPlaySingleplayer`). Saves are in `karoland/run/saves/`. Detect newest world: `basename "$(ls -dt karoland/run/saves/*/ | head -1)"` (quote it — names have spaces!).
Wait for "in world": poll `swaymsg -t get_tree | grep -q Singleplayer`.

## Driving the game on Wayland/sway (no X)
There is **no `wtype`/`ydotool`** installed and **Wayland has no targeted-input protocol**. What works:
- **Keyboard → `/dev/uinput`** (writable here, no sudo). Build the Go injector once:
  `cd tools/keysend && go build -o keysend .`  Usage:
  `tools/keysend/keysend tap:slash type:"karoland build" tap:enter` (args run in order; `tap:<named>` / `type:"text"` / `sleep:ms`).
  Now supports **uppercase letters and `~`** (shift map), so `/data get entity @s Pos` and `~ ~ ~` relative `/tp` work.
  uinput is GLOBAL — it goes to the FOCUSED window. **Focus the MC window first AND verify focus succeeded** (`swaymsg '[title="Minecraft NeoForge.*"] focus'` returns `{"success": true}`) — if it fails, DON'T send keys or they land in the user's other terminals/apps.
- **GOTCHA — wait for the WINDOW, not the log.** The log can be stale (each `runClient` overwrites `/tmp/mc.log`; a dying previous instance's "BUILD SUCCESSFUL"/world-load lines mislead). Poll `swaymsg -t get_tree | grep -q "Minecraft NeoForge"` for the actual window before driving. Also the first few keystrokes after the virtual device is created can be dropped/duplicated — open chat then re-type if a command shows a stray char.
- **Mouse → sway IPC:** `swaymsg 'seat - cursor set X Y'`, `cursor press button1/button3`.
- **Screenshots → grim, scoped to the MC window** (doesn't capture the user's other windows):
  ```bash
  GEO=$(swaymsg -t get_tree | jq -r '.. | objects | select(.name?!=null and (.name|tostring|test("Minecraft NeoForge"))) | .rect | "\(.x),\(.y) \(.width)x\(.height)"' | head -1)
  grim -g "$GEO" /tmp/shot.png   # then Read /tmp/shot.png
  ```
- **GOTCHA — singleplayer PAUSES (freezes the server tick) when the window loses focus.** During an automated test the user clicking elsewhere freezes the race. Mitigations: re-`focus` the MC window before each screenshot in a loop; OR set `pauseOnLostFocus:false` in `karoland/run/options.txt` (edit while the client is NOT running, it rewrites on exit); OR just use a **dedicated server / multiplayer** (no pause). On the real LAN party setup there is no pause.
- Open chat with `tap:slash` (opens `/`), then `type:"command"`, then `tap:enter`. Re-focus right before, and don't let the user type elsewhere mid-burst (your keys would land in their window).
- This whole client-driving path is FRAGILE and slow. Prefer unit tests + headless. Only drive the client for a final visual confirmation, and avoid it if the surprise-recipient (Karolina) is watching.

## Headless OFFSCREEN capture — screenshots/GIFs WITHOUT touching the real screen ⭐
The single most valuable trick: render + record the game on a **virtual display** so it NEVER appears
on the user's monitor (essential when the gift recipient is in the room). Spoiler-proof by construction —
worst case is a crash, never a Minecraft window popping up where it can be seen.

**How it works (4 parts):**
1. **Virtual display (Xvfb)** — an in-memory X11 screen, no monitor:
   ```bash
   Xvfb :99 -screen 0 1280x720x24 >/tmp/xvfb.log 2>&1 &
   ```
2. **Launch MC onto it, software-rendered, with `WAYLAND_DISPLAY` UNSET.** Unsetting it is the safety
   guarantee: GLFW can't use Wayland, so it falls back to X11 → Xvfb, or fails — it physically cannot
   render to the real desktop. Mesa `llvmpipe` gives OpenGL 4.6 in pure software (slow but renders fine).
   Use `--no-daemon` so env vars reach the forked game JVM. Example `/tmp/launch_demo.sh`:
   ```bash
   #!/usr/bin/env bash
   cd <repo>; export DISPLAY=:99; unset WAYLAND_DISPLAY
   export LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe KAROLAND_DEMO=1
   exec ./gradlew --no-daemon :karoland:runClient -PquickWorld="New World (2)"
   ```
   Then: `nohup bash /tmp/launch_demo.sh > /tmp/mc_xvfb.log 2>&1 &`
3. **Drive it with NO input** — you can't send keyboard/mouse to a headless display (keysend/uinput goes
   to the real compositor's focused window, not Xvfb; xdotool isn't installed). So instead add an
   **env-gated auto-demo** to the mod that builds + seats the player + starts + loops the ride on its own.
   karoland's is `KAROLAND_DEMO=1` → `KarolandEvents.runDemo` (off by default, zero effect on the gift).
4. **Grab + convert with ffmpeg** (and `import` for stills):
   ```bash
   DISPLAY=:99 import -window root /tmp/shot.png                 # single frame → Read it
   DISPLAY=:99 ffmpeg -y -f x11grab -video_size 1280x720 -framerate 12 -i :99 -t 14 /tmp/ride.mp4
   ffmpeg -y -i /tmp/ride.mp4 -vf "fps=12,scale=520:-1:flags=lanczos" ~/Pictures/clip.gif
   ffmpeg -y -i /tmp/ride.mp4 -vf fps=1 /tmp/f_%02d.png          # sample frames → Read them
   ```

**Gotchas / tips:**
- Poll `/tmp/mc_xvfb.log` for `Riders ready` (or your demo's start marker) before recording; the auto-demo
  loops, so to catch an early feature (e.g. a loop) record right after a FRESH start marker appears.
- First-person ride view mostly faces the sky/forward — fine for lava/ocean (you're inside them) but bad
  for seeing track shape; for an external beauty shot, add a demo phase that teleports to a vantage HIGH
  above the terrain (low vantages spawn inside hills). **A teleported player FALLS** (no flight) — set
  `p.setGameMode(GameType.SPECTATOR)` to hover at the vantage, then back to `SURVIVAL` before riding.
  The vantage must be PRE-ride (teleporting a RACING rider ejects them mid-ride = the "fall through" bug).
- **Don't poll for `Karoland built`** to time a capture — that chat line is only sent by the `/karoland
  build` COMMAND; the demo calls `build()` directly (silently). Use `Riders ready`/`HAPPY BIRTHDAY` as
  the cycle markers instead, or record a full ~75s cycle and pick frames.
- Check `tools` exist first: `Xvfb`, `ffmpeg`, `import` (ImageMagick), Mesa `swrast`/`llvmpipe` in
  `/usr/lib/dri`. `glxinfo` on `:99` confirms the GL version.
- Kill leftovers between runs by PID (`ps -eo pid,cmd | grep -iE "launch_demo|fml.modFold|DevLaunch"`);
  `pkill -f` often misses the long java cmdline. The harness's own shell can false-match the grep pattern.
- **Dirty dev world?** Old builds leave debris (esp. after a layout change → orphaned structures the new
  path doesn't clear). Regenerate just the faire area: stop the client, delete the region/entities/poi
  `.mca` files for the faire's region under `run/saves/<world>/dimensions/minecraft/overworld/` (faire at
  X/Z≈1.01M → region `r.1972.1972`), relaunch → fresh terrain, no debris. Spawn (r.0.0) is untouched.

## Custom world rendering (26.1.x render-pipeline rework)
karoland draws a smooth banked rail mesh from the spline (`KarolandTrackRenderer`). The 26.1.x rework moved/renamed a lot — verified against the decompiled sources:
- **Hook:** `RenderLevelStageEvent` is now **staged subclasses**, not an enum. Subscribe to `RenderLevelStageEvent.AfterTranslucentBlocks` (game bus, `@EventBusSubscriber(modid=…, value=Dist.CLIENT)`). Use `event.getPoseStack()`.
- **RenderType lives in `net.minecraft.client.renderer.rendertype`** (`RenderType`, `RenderTypes`). `RenderTypes.entityCutout(Identifier)` = textured, **no backface cull** (great for a mesh where winding is fiddly). `entitySolid`, `entityTranslucent` also there.
- **`Identifier` = `net.minecraft.resources.Identifier`** (not `ResourceLocation` here). `Identifier.withDefaultNamespace("textures/block/iron_block.png")` reuses a vanilla texture (full path incl. `textures/` + `.png`); UVs 0..1 map the whole texture.
- **Vertex API:** `vc.addVertex(pose, x,y,z).setColor(r,g,b,a).setUv(u,v).setOverlay(OverlayTexture.NO_OVERLAY).setLight(0xF000F0).setNormal(pose, nx,ny,nz)`. `pose = event.getPoseStack().last()`.
- **Buffer:** `var buf = Minecraft.getInstance().renderBuffers().bufferSource(); var vc = buf.getBuffer(type); … ; buf.endBatch(type);`. **GOTCHA — the immediate `BufferSource` keeps ONE render type building at a time.** Fetching a 2nd buffer ends the 1st, so finish + `endBatch(typeA)` before `getBuffer(typeB)`, else you get `IllegalStateException: Not building!` on the first vertex. (Do rails in one pass, ties in another.)
- **Float precision:** world origin is ~1,010,000. Do `worldPos - cameraPos` in **double**, then cast to float per vertex (camera from `mc.gameRenderer.getMainCamera().position()`). Do NOT translate the PoseStack by -1e6 (float cancellation → jitter).
- **Always wrap the render body in try/catch** + log-once: an exception in a render event spams and can take down the frame loop.
- Cull far segments (distance from camera) to bound per-frame cost; the mesh is rebuilt each frame (no VertexBuffer cache yet — fine within a small radius).
- **Camera ROLL (true loop inversion):** subscribe to `ViewportEvent.ComputeCameraAngles` (client) and call `e.setRoll(deg)`. karoland rolls 0→180→360 over the loop so the rider goes genuinely upside-down. Gate it on `mc.player.isPassenger()` + proximity to the loop path, and compute a SMOOTH roll from the player's fractional progress along the loop (nearest sampled point), not integer nodes.
- **SoundEvents are mixed types:** some fields are `Holder<SoundEvent>` (e.g. `NOTE_BLOCK_PLING`, `NOTE_BLOCK_BELL`, `WIND_CHARGE_BURST`) → pass `.value()` (or use the `Holder` overload); others are plain `SoundEvent` (e.g. `GENERIC_SPLASH`, `BLAZE_SHOOT`, `CHAIN_PLACE`, `WIND_CHARGE_THROW`, `ELYTRA_FLYING`) → pass directly. `Level.playSound` has overloads for both. Grep `net/minecraft/sounds/SoundEvents.java` to check a field's type before using it.
- **Particles for SPEED:** scattered slow `CLOUD` reads as "snow". For motion streaks use a few `END_ROD` spawned just AHEAD of the entity with high backward velocity (`sendParticles(..., count=0, -dirX,-dirY,-dirZ, bigSpeed)`) — they trail into lines.

## The Minecart Improvements experiment
**karoland NO LONGER needs this experiment** — the ride is driven by `setPos` on a server-controlled armor-stand seat (see the movement section), which works in any normal world. Kept here as background: it's a base-game experiment toggled at world creation (Experiments menu); a mod can't enable it at runtime. Earlier versions relied on it; the armor-stand seat removed that dependency, so the gift world needs no special setup.

## Build & deploy
```bash
./gradlew :karoland:build
scp karoland/build/libs/karoland-0.1.0.jar <kid>@<ip>:.../PrismLauncher/instances/26.1.x/minecraft/mods/
```
Update the kids' Prism instances to the same MC/Neo version. Ship the experiment-enabled world folder alongside the jar.

## Asset generation
Item/block textures via the `image-generation:generate-with-refs` skill (Gemini). Generates on a magenta bg → key it out + downscale with ImageMagick to a 32×32 RGBA PNG:
`magick in.png -fuzz 24% -transparent '#FF00FF' -filter point -resize 32x32 -define png:format=png32 out.png`. Reads `GOOGLE_API_KEY`. Wire each item with `assets/<mod>/items/<id>.json` + `models/item/<id>.json` + `lang/en_us.json`.

## karoland architecture cheat-sheet
- `RaceLogic` — PURE (no MC). `buildPath()` (turtle-style 3D path: lift, drop, turns, U-turn, 360° helix, hills), `cartSpeed(slope,boost,rubber)`, `rubberBonus(gap)`, `decideWinner(...)`. All unit-tested.
- `Faire` — config + path→world bridge (`lanePoint`, `slopeAt`, `startBlock`, boost zones, origin at 1_010_000 to avoid firstmod's arena at 1_000_000).
- `RaceManager` — IDLE/COUNTDOWN/RACING/FINISHED state machine on `ServerTickEvent.Post`; drives carts via `setPos` along the path; celebration (crown the **birthday girl by name**, not the winner; everyone-celebrates; fireworks/jingle/blast-off). In-memory singleton (rebuilt on reset), NOT hung off an entity.
- `RaceTrackBuilder` — lays the colored track bed along the path; station + GREEN LEVER start; finish/welcome arches; Pip the Fox mascot; spawns/respawns carts.
- `KarolandEvents` — tick + `RegisterCommandsEvent` + the lever `RightClickBlock`.
- Start = right-click the green lever between the carts (auto-seats nearby players + counts down). Set the birthday kid with `/karoland birthday <exact-username>`.
- `ParkBuilder` — the surrounding Disneyland-style PARK (coaster is ~1/5, SW corner). Themed lands (Kirby Land, DK Jungle, Minecraft Zone, Candy Buffet), castle hub, fountain, rides (carousel/teacups/ferris/swing/drop), maze, pond, food court, studio stage, pet zone, giant birthday cake, paths, gardens.

## Building a big park (ParkBuilder hard-won lessons)
- **Far-from-spawn builds need chunks force-generated first.** The park is ~250+ blocks from world spawn → its chunks aren't loaded → `level.getHeight()` returns the VOID floor (-64) and you build the whole park at y=-65. Fix: `for cx,cz over the rect: level.getChunk(cx,cz)` (generates to FULL) BEFORE sampling/placing. `setBlock`/`getBlockState` also auto-load, but the heightmap lies until generated.
- **Anchor the plaza to the structure, not terrain.** Plaza Y = `Faire.Y - 2` (just below the coaster's lowest rail) so drops keep open air below AND the coaster's support legs land ON the park. Then `layGround` floods the whole rect (incl. UNDER the coaster) to plaza level so the ride isn't floating in a pit — but SKIP the lava-chasm / ocean-tank footprints (they dig down by design).
- **Isolate every build step in try/catch** (`step(level, name, fn)`) so one broken feature can't abort the whole park. Invaluable when adding lots autonomously.
- **Build-once guard + region sentinel.** A `static boolean built` skips the ~3M-block flatten on repeat calls within a JVM; a hidden underground `GOLD_BLOCK` sentinel skips it across loads (the play client loads the prebuilt region instantly instead of freezing). Wipe the faire region files to force a true rebuild while iterating.
- **Painting variants are a DATAPACK registry** (`data/<ns>/painting_variant/<name>.json` → `{asset_id,width,height}`); look up at runtime via `level.registryAccess().lookupOrThrow(Registries.PAINTING_VARIANT)`. Two variants can SHARE one texture (same `asset_id`) at different block sizes — e.g. `poster_logo` (4x3) and `poster_logo_big` (8x5) reuse one PNG. A multi-block painting needs a solid backing wall behind its whole footprint or `survives()` fails.
- **Display entities (`Display.ItemDisplay/BlockDisplay`) have PRIVATE setters in 26.1.1** (transformation/billboard/item set only via NBT/codec) → awkward to configure in code. For floating decorations (the Kirby balloon-on-a-string above a player), render them in-world via `RenderLevelStageEvent.AfterOpaqueBlocks` instead (billboarded textured quad + a thin string quad, camera-relative double precision — same technique as the track mesh).
- **Vanilla bubble elevator** (soul sand + water column in a glass tube) to reach an elevated station: gate the water with WALK-THROUGH wall signs (attach them to the side glass, facing along it) and DON'T cap the tube over the exit — a glass cap traps the rider at the top. Leave the top open + a quartz exit lip onto the landing.
- **MC 26.1 removed `Level.setDayTime`** (new WorldClock/Timeline system). Set the world spawn via `((PrimaryLevelData) level.getLevelData()).setSpawn(LevelData.RespawnData.of(dim, pos, yaw, pitch))`.
- **Kill empty grass with themed ground-texturing** (probabilistic per-column block choice fading from each land's signature blocks) + dense scatter (trees/flowers/benches/hedges/lamps) + paths connecting every attraction to the hub. Density >> size for "fun to stay".
- **generate-with-refs for park art**: posters/billboards are full-frame paintings (NO magenta key needed — paintings are opaque RGB); magenta keying is only for transparent item/balloon textures. Resize posters to `width*64 × height*64` to match the variant aspect.

## Autonomous visual review loop (offscreen)
- Drive iteration with the offscreen demo (`KAROLAND_DEMO=1` on `DISPLAY=:99`): a per-cycle `log("vantage cycle start")` marker lets a capture script sync; the demo does a high ORBIT (overview) then a GROUND TOUR teleporting a spectator to each landmark for clean close-ups, then auto-runs a race. Capture with `import -window root` and Read the PNGs.
- The software (llvmpipe) renderer fogs/wash-outs distant geometry — for clean shots get the camera CLOSE (ground tour) or steeper/top-down; don't trust far orbit shots for color/detail.
- Run an in-character **"Aphmau" reviewer subagent** on the captured frames periodically — it gives concrete, kid-focused, buildable feedback (castle-as-icon, fill empty grass, glowing birthday "9", readable signage) that maps directly to code changes.
- `KAROLAND_BUILD=1` (the play launch) builds once on load (region-sentinel → fast) and drops the player at the entrance — for the real, visible gift session.
