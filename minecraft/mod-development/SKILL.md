---
name: mod-development
description: Agent-driven workflow for developing Minecraft NeoForge mods (MC 26.1.x / calver). How to lay out a multi-mod Gradle repo, look up the REAL deobfuscated API from the decompiled sources jar (not training data), the unit-test-first strategy, launch and DRIVE the game on Wayland/sway with screenshots, run fully unattended via a headless offscreen harness, plus a pile of hard-won MC 26.1.x API gotchas (minecart sync, the render-pipeline rework, world-building). Use whenever working on a NeoForge mod.
---

# Minecraft NeoForge mod development (agent-driven)

Hard-won workflow for building NeoForge mods with an LLM agent in the loop — written against
MC 26.1.x. Read this first; it saves hours. Concrete class names below (e.g. a path-driven ride)
are illustrative; substitute your own.

## TL;DR golden rules
1. **Iterate in unit tests, not the game.** Launching + driving the client is ~60s/cycle. Put all pure logic in MC-free classes (geometry, state machines, math) and test with `./gradlew :<mod>:test` (seconds).
1b. **For VISUAL work, iterate fully AUTONOMOUSLY via the headless offscreen harness** (Xvfb + software GL + an env-gated auto-demo + ffmpeg → GIF/PNG you can Read). See "Headless OFFSCREEN capture" below, and the `playtest-loop` skill for the generic harness. This renders + records WITHOUT ever showing on the user's screen — so you can build→see→fix on your own (no need for the user to launch/watch), and it can't spoil a surprise.
2. **Look up the real API in the decompiled sources** (see below) — don't guess. MC 26.1.x renamed a lot.
3. **A player riding a vanilla minecart is simulated client-side** — server `setPos`/velocity is ignored for the rider UNLESS the world has the *Minecart Improvements* experiment (then carts are server-authoritative). See the movement section.
4. **Commit at checkpoints.** Branch off `main` first.

## Repo layout (Gradle multi-project)
A multi-mod repo where each mod builds to its own jar and loads independently:
```
my-mods/
  settings.gradle          # include 'modb'  (moda can be the ROOT project)
  build.gradle             # moda (root mod)
  src/main/java/dev/you/moda/...
  modb/                    # second mod — its own jar, loads alone
    build.gradle           # self-contained: mod_id/version/neo_version are local `def`s
                           #   (subproject gradle.properties are NOT read as project props!)
    src/main/java/dev/you/modb/...
    src/main/resources/assets/modb/...      # items/, models/item/, lang/, textures/
    src/main/templates/META-INF/neoforge.mods.toml  # expanded by generateModMetadata
    src/test/java/dev/you/modb/...          # JUnit (pure-logic) tests
  tools/keysend/main.go    # Go uinput keyboard injector (bundled with this skill; see "Driving the game")
  rfcs/                    # design docs
```
Each mod builds independently: `./gradlew :modb:build` → `modb/build/libs/modb-0.1.0.jar`.
Gotcha worth repeating: a subproject's `gradle.properties` is NOT auto-read as project properties — declare `mod_id`/`version`/`neo_version` as local `def`s in the subproject `build.gradle`.

## Versions & toolchain
- **Minecraft uses calver since 2026**: `26.1`, `26.1.1`, `26.1.2` … (year.release.patch). `26.1` = "Tiny Takeover" (Mar 2026).
- **NeoForge** is 4-part: `26.1.2.73` = MC 26.1.2 + Neo build 73. Find the newest at
  `https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml` (WebFetch the XML).
- **Java 25**, Gradle 9.2.1, ModDevGradle 2.0.141 (versions current as of MC 26.1.x).
- You can't build against a NeoForge version that doesn't exist yet (e.g. a brand-new MC before Neo ships for it). Pin in the subproject `build.gradle`'s `neo_version` def.

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
- **Minecart Improvements experiment ON** → carts become *server-authoritative*; server `setPos` drives the rider smoothly. Requires the experiment toggled at world creation.
- **Drive the ride by `setPos` along a precomputed 3D path** (your own path logic → world point per distance), NOT rail physics — so it follows arbitrary turns/helix/drops, can't stall or derail, and the speed is fully controlled. Note: `ServerTickEvent.Post` fires *after* entities tick, so velocity set there gets overwritten — use `setPos`, not velocity.
- Empty (rider-less) carts unload if they travel >~render distance from any player; only matters for solo runs.
- **Best pattern: an invisible server-controlled ARMOR-STAND SEAT the player rides** (the visible minecart is moved alongside, not ridden) → server `setPos` carries the rider correctly in SP *and* MP with **no experiment needed**.

### Making a `setPos`-driven ride feel STEADY (hard-won)
A naive per-step `setPos` ride twitches. Three independent causes, each fixed in pure (unit-tested) logic:
1. **Polyline kink** — linearly interpolating between integer path points snaps the travel direction at every vertex. Fix: sample a **Catmull-Rom spline** through the points — C1-continuous, passes exactly through control points (so the track mesh is unchanged).
2. **Speed jolt** — discrete per-step slope made speed jump each step boundary. Fix: **continuous slope** from the spline derivative + **jerk limiting** (cap acceleration) so speed eases toward its target.
3. **Sync extrapolation jitter** — sending per-tick velocity (`setDeltaMovement` + `hurtMarked`) AND `setPos` makes the client extrapolate ahead then snap back. So: `setDeltaMovement(Vec3.ZERO)`, no `hurtMarked`.
4. **THE big one — the ridden entity's sync cadence.** If the rider rides an **`ArmorStand`**, it has **no client `InterpolationHandler` and updates only every 3 ticks** (`EntityType` default `updateInterval=3`). So plain `setPos` made the rider hard-teleport ~1.2 blocks every 150ms = stutter. Fix: set `entity.needsSync = true` (public field) **every tick the entity moves** → `ServerEntity.sendChanges` emits a position packet every tick → the client's 1-tick render lerp smooths it. Set it on both the seat AND the visible minecart (the minecart *does* have an interp handler, but per-tick keeps them locked together). This was the actual cure; position-only sync alone just exposed the bare 3-tick stepping.
Orient the visible cart from the **smooth tangent** (not the noisy per-tick delta), eased via `Mth.degreesDifference`. Use the **minecart's OWN angle convention**, not entity-facing: off-rail `AbstractMinecartRenderer.oldRender` renders `YP(180 - yRot)` with `yRot = atan2(dz,dx)°` (NO −90 offset — that renders it 90° sideways) and pitch `atan(dy)*73`. Cover the motion model with a unit test that replays it and asserts lateral steadiness (turn < threshold).
- **World-Y gotcha:** if your path builder shifts the whole track UP (e.g. by `-minY` so descents clear y=0), the station can sit at a very different Y than the finish. Any world-Y logic (e.g. a "seat players near the station" AABB) must anchor on the actual built structure's Y, NOT a nominal base Y — a box at the wrong Y matches nobody.

## Testing pyramid (cheap → expensive)
1. **Unit tests (JUnit, no MC):** all pure logic lives in MC-free classes (path geometry, slope→speed, scoring, winner). `./gradlew :<mod>:test` — seconds, no launch. Wire build/deps in the subproject `build.gradle` (mavenCentral + junit-bom).
2. **Headless server load-check:** `timeout --signal=KILL 90 ./gradlew :<mod>:runServer --console=plain > /tmp/srv.log 2>&1` then grep `/tmp/srv.log` for your mod's "loaded" line and any `Exception`. No window — safe to run anytime. Confirms registration + static init without crashing.
3. **Live client (visual only):** last resort; see below.
What you CANNOT unit-test: actual minecart physics, rendering, the experiment. Cover those with (2) + a quick (3).

## Running the game
```bash
./gradlew :<mod>:runClient --console=plain > /tmp/mc.log 2>&1          # opens the client (title screen)
./gradlew :<mod>:runClient -PquickWorld="New World (1)" ...            # boot STRAIGHT into a world (skips menus)
./gradlew :<mod>:runServer --console=plain ...                         # headless dedicated server (no window)
```
Wire `-PquickWorld` in the subproject `build.gradle` to pass `--quickPlaySingleplayer`. Saves are in `<mod>/run/saves/`. Detect newest world: `basename "$(ls -dt <mod>/run/saves/*/ | head -1)"` (quote it — names have spaces!).
Wait for "in world": poll `swaymsg -t get_tree | grep -q Singleplayer`.

## Driving the game on Wayland/sway (no X)
There is often **no `wtype`/`ydotool`** installed and **Wayland has no targeted-input protocol**. What works (the `keysend` injector is bundled in `tools/keysend`):
- **Keyboard → `/dev/uinput`** (usually writable, no sudo). Build the Go injector once:
  `cd tools/keysend && go build -o keysend .`  Usage:
  `tools/keysend/keysend tap:slash type:"mymod build" tap:enter` (args run in order; `tap:<named>` / `type:"text"` / `sleep:ms`).
  Supports uppercase letters and `~` (shift map), so `/data get entity @s Pos` and `~ ~ ~` relative `/tp` work.
  uinput is GLOBAL — it goes to the FOCUSED window. **Focus the MC window first AND verify focus succeeded** (`swaymsg '[title="Minecraft NeoForge.*"] focus'` returns `{"success": true}`) — if it fails, DON'T send keys or they land in the user's other terminals/apps.
- **GOTCHA — wait for the WINDOW, not the log.** The log can be stale (each `runClient` overwrites `/tmp/mc.log`; a dying previous instance's "BUILD SUCCESSFUL"/world-load lines mislead). Poll `swaymsg -t get_tree | grep -q "Minecraft NeoForge"` for the actual window before driving. Also the first few keystrokes after the virtual device is created can be dropped/duplicated — open chat then re-type if a command shows a stray char.
- **Mouse → sway IPC:** `swaymsg 'seat - cursor set X Y'`, `cursor press button1/button3`.
- **Screenshots → grim, scoped to the MC window** (doesn't capture the user's other windows):
  ```bash
  GEO=$(swaymsg -t get_tree | jq -r '.. | objects | select(.name?!=null and (.name|tostring|test("Minecraft NeoForge"))) | .rect | "\(.x),\(.y) \(.width)x\(.height)"' | head -1)
  grim -g "$GEO" /tmp/shot.png   # then Read /tmp/shot.png
  ```
- **GOTCHA — singleplayer PAUSES (freezes the server tick) when the window loses focus.** During an automated test the user clicking elsewhere freezes everything. Mitigations: re-`focus` the MC window before each screenshot in a loop; OR set `pauseOnLostFocus:false` in `<mod>/run/options.txt` (edit while the client is NOT running, it rewrites on exit); OR just use a **dedicated server / multiplayer** (no pause).
- Open chat with `tap:slash` (opens `/`), then `type:"command"`, then `tap:enter`. Re-focus right before, and don't let the user type elsewhere mid-burst (your keys would land in their window).
- This whole client-driving path is FRAGILE and slow. Prefer unit tests + headless. Only drive the client for a final visual confirmation.

## Headless OFFSCREEN capture — screenshots/GIFs WITHOUT touching the real screen ⭐
The single most valuable trick: render + record the game on a **virtual display** so it NEVER appears
on the user's monitor (essential when the work is a surprise, or you just don't want a window
stealing focus). Spoiler-proof by construction — worst case is a crash, never a Minecraft window
popping up where it can be seen. This is the MC-specific instance of the generic `playtest-loop` harness.

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
   export LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe MYMOD_DEMO=1
   exec ./gradlew --no-daemon :<mod>:runClient -PquickWorld="New World (2)"
   ```
   Then: `nohup bash /tmp/launch_demo.sh > /tmp/mc_xvfb.log 2>&1 &`
3. **Drive it with NO input** — you can't send keyboard/mouse to a headless display (keysend/uinput goes
   to the real compositor's focused window, not Xvfb; xdotool isn't installed). So instead add an
   **env-gated auto-demo** to the mod that builds + seats the player + starts the scenario on its own
   (e.g. `MYMOD_DEMO=1` → a `runDemo` hook, off by default, zero effect on a normal launch).
4. **Grab + convert with ffmpeg** (and `import` for stills):
   ```bash
   DISPLAY=:99 import -window root /tmp/shot.png                 # single frame → Read it
   DISPLAY=:99 ffmpeg -y -f x11grab -video_size 1280x720 -framerate 12 -i :99 -t 14 /tmp/clip.mp4
   ffmpeg -y -i /tmp/clip.mp4 -vf "fps=12,scale=520:-1:flags=lanczos" ~/Pictures/clip.gif
   ffmpeg -y -i /tmp/clip.mp4 -vf fps=1 /tmp/f_%02d.png          # sample frames → Read them
   ```

**Gotchas / tips:**
- Poll `/tmp/mc_xvfb.log` for a demo start marker (e.g. `Riders ready`) before recording; if the auto-demo
  loops, to catch an early feature record right after a FRESH start marker appears.
- First-person ride view mostly faces the sky/forward — fine for lava/ocean (you're inside them) but bad
  for seeing geometry shape; for an external beauty shot, add a demo phase that teleports to a vantage HIGH
  above the terrain (low vantages spawn inside hills). **A teleported player FALLS** (no flight) — set
  `p.setGameMode(GameType.SPECTATOR)` to hover at the vantage, then back to `SURVIVAL` before riding.
  The vantage must be PRE-ride (teleporting a moving rider ejects them = the "fall through" bug).
- **Don't poll for a chat line your COMMAND emits** to time a capture if the demo calls the underlying
  method directly (silently) — the chat line won't fire. Use the demo's own log markers instead, or record
  a full cycle and pick frames.
- Check tools exist first: `Xvfb`, `ffmpeg`, `import` (ImageMagick), Mesa `swrast`/`llvmpipe` in
  `/usr/lib/dri`. `glxinfo` on `:99` confirms the GL version.
- Kill leftovers between runs by PID (`ps -eo pid,cmd | grep -iE "launch_demo|fml.modFold|DevLaunch"`);
  `pkill -f` often misses the long java cmdline. The harness's own shell can false-match the grep pattern.
- **Dirty dev world?** Old builds leave debris (esp. after a layout change → orphaned structures the new
  build doesn't clear). Regenerate just the affected area: stop the client, delete the region/entities/poi
  `.mca` files for that region under `run/saves/<world>/dimensions/minecraft/overworld/`, relaunch → fresh
  terrain, no debris. (Compute the region from the structure's X/Z ÷ 512.) Spawn region is untouched.

## Custom world rendering (26.1.x render-pipeline rework)
Drawing a custom mesh (e.g. a smooth banked rail from a spline) — the 26.1.x rework moved/renamed a lot; verified against the decompiled sources:
- **Hook:** `RenderLevelStageEvent` is now **staged subclasses**, not an enum. Subscribe to `RenderLevelStageEvent.AfterTranslucentBlocks` (game bus, `@EventBusSubscriber(modid=…, value=Dist.CLIENT)`). Use `event.getPoseStack()`.
- **RenderType lives in `net.minecraft.client.renderer.rendertype`** (`RenderType`, `RenderTypes`). `RenderTypes.entityCutout(Identifier)` = textured, **no backface cull** (great for a mesh where winding is fiddly). `entitySolid`, `entityTranslucent` also there.
- **`Identifier` = `net.minecraft.resources.Identifier`** (not `ResourceLocation` here). `Identifier.withDefaultNamespace("textures/block/iron_block.png")` reuses a vanilla texture (full path incl. `textures/` + `.png`); UVs 0..1 map the whole texture.
- **Vertex API:** `vc.addVertex(pose, x,y,z).setColor(r,g,b,a).setUv(u,v).setOverlay(OverlayTexture.NO_OVERLAY).setLight(0xF000F0).setNormal(pose, nx,ny,nz)`. `pose = event.getPoseStack().last()`.
- **Buffer:** `var buf = Minecraft.getInstance().renderBuffers().bufferSource(); var vc = buf.getBuffer(type); … ; buf.endBatch(type);`. **GOTCHA — the immediate `BufferSource` keeps ONE render type building at a time.** Fetching a 2nd buffer ends the 1st, so finish + `endBatch(typeA)` before `getBuffer(typeB)`, else you get `IllegalStateException: Not building!` on the first vertex. (Do one pass per render type.)
- **Float precision:** at large world coordinates (e.g. origin ~1,000,000) do `worldPos - cameraPos` in **double**, then cast to float per vertex (camera from `mc.gameRenderer.getMainCamera().position()`). Do NOT translate the PoseStack by -1e6 (float cancellation → jitter).
- **Always wrap the render body in try/catch** + log-once: an exception in a render event spams and can take down the frame loop.
- Cull far segments (distance from camera) to bound per-frame cost; if the mesh is rebuilt each frame, keep the radius small (or cache in a VertexBuffer).
- **Camera ROLL (e.g. true loop inversion):** subscribe to `ViewportEvent.ComputeCameraAngles` (client) and call `e.setRoll(deg)`. Gate it on `mc.player.isPassenger()` + proximity to the relevant path, and compute a SMOOTH roll from the player's fractional progress (nearest sampled point), not integer nodes.
- **SoundEvents are mixed types:** some fields are `Holder<SoundEvent>` (e.g. `NOTE_BLOCK_PLING`, `NOTE_BLOCK_BELL`, `WIND_CHARGE_BURST`) → pass `.value()` (or use the `Holder` overload); others are plain `SoundEvent` (e.g. `GENERIC_SPLASH`, `BLAZE_SHOOT`, `CHAIN_PLACE`, `WIND_CHARGE_THROW`, `ELYTRA_FLYING`) → pass directly. `Level.playSound` has overloads for both. Grep `net/minecraft/sounds/SoundEvents.java` to check a field's type before using it.
- **Particles for SPEED:** scattered slow `CLOUD` reads as "snow". For motion streaks use a few `END_ROD` spawned just AHEAD of the entity with high backward velocity (`sendParticles(..., count=0, -dirX,-dirY,-dirZ, bigSpeed)`) — they trail into lines.

## The Minecart Improvements experiment
A base-game experiment toggled at world creation (Experiments menu); a mod can't enable it at runtime. With it ON, minecarts become *server-authoritative* (server `setPos` drives the rider smoothly). You can avoid depending on it entirely with the armor-stand-seat pattern (see the movement section) — which is preferable, since it needs no special world setup and works in any normal world.

## Build & deploy
```bash
./gradlew :<mod>:build
scp <mod>/build/libs/<mod>-0.1.0.jar <user>@<ip>:.../PrismLauncher/instances/26.1.x/minecraft/mods/
```
Match the target launcher's MC/Neo version to the build. If the mod relies on a world-creation experiment, ship the experiment-enabled world folder alongside the jar.

## Asset generation
Item/block textures via the `image-generation:generate-with-refs` skill (Gemini). Generate on a magenta bg → key it out + downscale with ImageMagick to a 32×32 RGBA PNG:
`magick in.png -fuzz 24% -transparent '#FF00FF' -filter point -resize 32x32 -define png:format=png32 out.png`. Wire each item with `assets/<mod>/items/<id>.json` + `models/item/<id>.json` + `lang/en_us.json`. For custom entity models/textures, see the `npc-design` skill (offline-first cube-model + atlas pipeline).

## Programmatic world-building lessons (hard-won)
Building large structures from code instead of structure files:
- **Far-from-spawn builds need chunks force-generated first.** A structure ~250+ blocks from world spawn sits in chunks that aren't loaded → `level.getHeight()` returns the VOID floor (-64) and you build the whole thing at y=-65. Fix: `for cx,cz over the rect: level.getChunk(cx,cz)` (generates to FULL) BEFORE sampling/placing. `setBlock`/`getBlockState` also auto-load, but the heightmap lies until generated.
- **Anchor to a structure, not terrain.** Pick a fixed plaza/base Y derived from your build (e.g. just below the lowest rail) and flood-fill ground to it, rather than following noisy terrain height — but SKIP footprints that dig down by design (chasms, water tanks).
- **Isolate every build step in try/catch** (`step(level, name, fn)`) so one broken feature can't abort the whole build. Invaluable when adding lots autonomously.
- **Build-once guard + region sentinel.** A `static boolean built` skips a huge re-flatten on repeat calls within a JVM; a hidden underground sentinel block skips it across loads (the play client loads the prebuilt region instantly instead of freezing). Wipe the region files to force a true rebuild while iterating.
- **Paintings are a DATAPACK registry** (`data/<ns>/painting_variant/<name>.json` → `{asset_id,width,height}`); look up at runtime via `level.registryAccess().lookupOrThrow(Registries.PAINTING_VARIANT)`. Two variants can SHARE one texture (same `asset_id`) at different block sizes. A multi-block painting needs a solid backing wall behind its whole footprint or `survives()` fails.
- **Display entities (`Display.ItemDisplay/BlockDisplay`) have PRIVATE setters in 26.1.1** (transformation/billboard/item set only via NBT/codec) → awkward to configure in code. For floating decorations, render them in-world via `RenderLevelStageEvent.AfterOpaqueBlocks` instead (billboarded textured quad + camera-relative double precision — same technique as the mesh rendering above).
- **Vanilla bubble elevator** (soul sand + water column in a glass tube) to reach an elevated platform: gate the water with WALK-THROUGH wall signs (attach to the side glass, facing along it) and DON'T cap the tube over the exit (a glass cap traps the rider at the top). Leave the top open + an exit lip onto the landing.
- **MC 26.1 removed `Level.setDayTime`** (new WorldClock/Timeline system). Set the world spawn via `((PrimaryLevelData) level.getLevelData()).setSpawn(LevelData.RespawnData.of(dim, pos, yaw, pitch))`.
- **Density >> size for "feels alive":** themed ground-texturing (probabilistic per-column block choice) + dense scatter (trees/flowers/benches/lamps) + paths connecting everything beats a big empty footprint.

## Autonomous visual review loop (offscreen)
Use the offscreen demo (a `MYMOD_DEMO=1`-style hook on `DISPLAY=:99`) to drive iteration; see the `playtest-loop` skill for the full LLM-judge methodology. MC-specific tips:
- A per-cycle `log("vantage cycle start")` marker lets a capture script sync; have the demo do a high ORBIT (overview) then a GROUND TOUR teleporting a spectator to each landmark for clean close-ups, then run the scenario. Capture with `import -window root` and Read the PNGs.
- The software (llvmpipe) renderer fogs/washes-out distant geometry — for clean shots get the camera CLOSE (ground tour) or steeper/top-down; don't trust far orbit shots for color/detail.
- Run an in-character reviewer subagent (e.g. a kid-focused persona) on the captured frames — it gives concrete, buildable feedback (readable signage, fill empty space, clear focal points) that maps directly to code changes.
