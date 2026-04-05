# stalcraft-wrapper

JVM wrapper for STALCRAFT that dynamically tunes JVM flags and boosts process priority based on your hardware.

## What it does

- **Detects** your system: RAM (total + free), CPU cores, large page support
- **Generates** optimal JVM flags: heap size, GC threads, G1 region size, metaspace, code cache, and more
- **Replaces** default JVM arguments from the launcher with tuned ones
- **Boosts** the game process: `HIGH_PRIORITY_CLASS`, memory priority, I/O priority
- **Installs** transparently via Windows IFEO — no game files modified

## Install

Download `wrapper.exe` from [Releases](../../releases), place it anywhere, and run as admin.

Just run `wrapper.exe` — an interactive menu will appear:

```
STALCRAFT JVM Optimization Wrapper
-----------------------------------
  > Install
    Uninstall
    Status
    Exit
```

Use arrow keys to select, Enter to confirm.

Done. Every game launch now goes through the wrapper automatically.

Both versions are supported:
- `stalcraft.exe` (main launcher) → `java.exe`
- `stalcraftw.exe` (Steam) → `javaw.exe`

### Terminal commands

```
wrapper.exe --install     # register IFEO hook
wrapper.exe --status      # check if installed
wrapper.exe --uninstall   # remove IFEO hook
```

## How it works

Windows [Image File Execution Options](https://learn.microsoft.com/en-us/previous-versions/windows/desktop/xperf/image-file-execution-options) intercepts `stalcraft.exe` / `stalcraftw.exe` launch and redirects it through the wrapper. The wrapper:

1. Detects hardware via `GlobalMemoryStatusEx`, `runtime.NumCPU`, `GetLargePageMinimum`
2. Calculates optimal JVM flags based on available resources
3. Strips conflicting flags from the original launcher arguments
4. Launches the real `java.exe` / `javaw.exe` with tuned flags and `HIGH_PRIORITY_CLASS`
5. Applies post-launch boost: disables priority decay, sets max memory and I/O priority

### Dynamic tuning

| Parameter | Formula |
|-----------|---------|
| Heap | 50% free RAM, floor 25% total, cap min(16g, 75% total) |
| ParallelGCThreads | cores - 2, min 2 |
| ConcGCThreads | parallel / 4, min 1 |
| G1HeapRegionSize | 4m / 8m / 16m / 32m based on heap |
| Metaspace | 128m / 256m / 512m based on heap |
| CodeCache | heap/16, clamped 128-512m |
| SurvivorRatio | 32 (≤4 cores) or 8 (>4 cores) |
| Large Pages | enabled only if `SeLockMemoryPrivilege` is available |

### Stderr output

```
[wrapper] System: 16 cores, 32.0GB total, 18.4GB free, large pages: yes
[wrapper] Heap: 9g | GC: parallel=14 concurrent=3 | Region: 16m
[wrapper] Flags: 28 injected, 3 removed
[wrapper] Started PID 12345
[wrapper] Process boosted
```

## Large Pages (optional)

For best performance, enable large pages:

1. Run `secpol.msc`
2. Local Policies → User Rights Assignment → Lock pages in memory
3. Add your user, reboot

The wrapper will detect this automatically and add `-XX:+UseLargePages`.

## Build

```
go build -o wrapper.exe -ldflags="-s -w" .
```

## Requirements

- Windows 10/11
- Administrator privileges (for `--install` / `--uninstall`)
- STALCRAFT installed

## License

MIT
