# Community benchmarks

[![eng](https://img.shields.io/badge/lang-English-blue)](./BENCHMARKS.en.md)
[![ru](https://img.shields.io/badge/lang-Russian-blue)](./BENCHMARKS.md)

This document describes how to capture a standardised STALCRAFT performance recording and attach it to [Discussions](../../discussions) so the JVM tuning profile can be validated on hardware we don't own.

## What is collected — and what is NOT

A CapFrameX capture contains:

- CPU / GPU / motherboard model, RAM size and speed
- drivers, OS build, PresentMon version
- frametime sample array (frametime.ms) and aggregate metrics (avg, p99, etc.)

The capture does **not** contain:

- username, file paths, Windows machine ID
- IP addresses, in-game account, character name or coordinates
- raw JVM flags, launcher arguments
- any game-process data beyond frametime

The data is public and safe to attach to a Discussion.

## Protocol

Conditions are identical across submissions — otherwise captures can't be compared.

### 1. System prep

- Close **Discord, Telegram, Steam overlay, OBS, the browser** and any background program that might compete for CPU/RAM.
- Turn off Windows notifications for the duration of the test.
- Plug laptops into AC.

### 2. CapFrameX settings

In CapFrameX → `Capture`:

- **Capture time**: `360` seconds.
- **Capture delay**: `5` seconds.
- **Display-synced frametime measurement** *(exact option name and screenshots to be added)*: pick the setting that measures frametime against displayed frames rather than raw presents. This makes the numbers reflect what you actually see on screen.

> Screenshots of the CapFrameX UI for these settings will be added later. Ask in the Discussion if unsure.

### 3. Test scenario

1. Launch STALCRAFT.
2. Reach the character-select menu.
3. **First login** into the game on a fresh session — freshly initialised JIT, warm but not saturated caches.
4. Optionally disable non-essential HUD (compass, chats, map), keep what matters for smoothness perception.
5. Teleport to **морятник (moryatnik)** — a busy naval area — or a comparably crowded location.
6. Wait at least **1 minute** before starting the recording so the area populates with players / NPCs. Otherwise the first 2 of 6 minutes end up as effectively a static scene and the test becomes meaningless.
7. Start the CapFrameX recording.
8. Play normally for the full 360 seconds — combat, movement, interactions.

### 4. Number of runs

Ideally — **3 runs**. Rare events (zone transitions, shader compilation) are Poisson-distributed, so max frametime and large-stutter counts vary a lot between runs of the same config. Three recordings average out that variance.

Minimum — **1 run**. One is better than nothing.

## What to attach to the Discussion

Open [Discussions → Benchmark Submission](../../discussions/new?category=benchmarks) and fill in:

- **CPU** (e.g. `AMD Ryzen 9 9900X3D`)
- **GPU** (e.g. `NVIDIA RTX 5080`)
- **RAM: size + type + Configured Memory Clock Speed**, e.g. `32 GB DDR5 @ 6000 MT/s`. The **actually configured speed** matters, not the DIMM's rating. You can read it from CPU-Z (Memory tab → DRAM Frequency × 2) or from `cli.exe` after installation — the menu prints a line like `6000 MT/s (fast tier)`.
- **Wrapper version** (from `wrapper.log` or `cli.exe --status`).
- **Active config name** (`default.json`, `my_setup.json`, etc.).
- **Config content** — the JSON from `jvm_wrapper/configs/<name>.json`, paste it into the "Config" field.
- **Links to CapFrameX JSON recordings** — upload to GitHub (drag-and-drop into the Discussion), Google Drive, or any file host with a direct link.
- **A short subjective note** — "smooth", "occasional hitches", "stutter during crowd fights". Numbers matter, perception matters too.

## Why this helps

JVM tuning is more hardware-dependent than it looks. Our own measurements only cover two specific rigs:

- **9900X3D + DDR5-6000 + RTX 5080** — fast-tier X3D
- **i5-10400F + DDR4-2666 + RTX 4060 Ti** — slow-tier non-X3D

Between those two sits a huge range: DDR4-3200 / 3600, Zen 3/4 without V-Cache, older i7 / i9 parts with varied memory, laptops with LPDDR5. Community data lets us:

- Validate the **mid tier** profile, which nobody has benchmarked yet.
- Catch regressions on uncommon configurations.
- Spot unexpected patterns (e.g. DDR4 CL14 vs CL19 at the same 3200 MT/s).

Thanks for contributing. Every recording makes the utility better for everyone.
