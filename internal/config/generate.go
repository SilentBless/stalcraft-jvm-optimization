package config

import "github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"

// Generate produces a performance-oriented Config for the given hardware.
//
// The profile targets a single goal: STALCRAFT running as smoothly as
// possible on a default.json. Values are NOT scaled down to save
// resources — we pick the largest safe number every time.
//
// Only heap size, G1 region size and GC thread count actually depend
// on memory and core count; the JIT/inlining block is scaled by L3
// cache size (X3D-class parts get deeper inlining and a larger node
// budget because their compiled hot path fits entirely in L3).
// Everything else is a fixed, tested default compatible with OpenJDK 9.
func Generate(sys sysinfo.Info) Config {
	heap := sizeHeap(sys.TotalGB())
	parallel, concurrent := gcThreads(sys.CPUThreads)
	jit := jitProfile(sys)

	// Memory-bandwidth-aware frame-pacing profile. Young-GC copy cost
	// is bandwidth-bound, so the realistic pause target and the
	// granularity of mixed-GC work both scale with configured memory
	// speed. Three tiers were validated against CapFrameX captures:
	//
	//   slow (≤ 2933 MT/s)   — stutters >50 ms dominated by fixed RSet
	//                          scan cost per mixed-GC pass. Fewer,
	//                          longer passes (mixedCount=4) amortise
	//                          the scan overhead; a looser pause
	//                          target (150 ms) stops G1 from slicing
	//                          young collections into more pauses
	//                          than the memory can actually complete.
	//   fast (≥ 4800 MT/s)   — RSet scan is cheap, so more, shorter
	//                          mixed passes (mixedCount=8, Oracle
	//                          default) smooth out the p99 tail. The
	//                          tighter pause target (80 ms) is
	//                          attainable on DDR5 bandwidth.
	//   mid                   — interpolated; also the fallback when
	//                          the SMBIOS probe fails (MemSpeedMTs=0).
	//
	// X3D-class L3 halves the effective pause budget on top of the
	// tier — the huge cache makes reference walks much cheaper, so
	// the marking and copy phases complete in roughly half the time
	// a non-X3D part needs at the same memory speed.
	//
	// Other pauses-sensitive flags (survivor sizing, tenuring, IHOP,
	// live-region threshold, soft-ref retention) are not bandwidth-
	// dependent and stay common across tiers.
	var (
		pauseMs          int
		mixedCountTarget int
		rsetUpdatingPct  int
		newSizePercent   int
	)
	switch sys.MemTier() {
	case sysinfo.MemSlow:
		pauseMs = 150
		mixedCountTarget = 4
		rsetUpdatingPct = 5
		newSizePercent = 30
	case sysinfo.MemFast:
		pauseMs = 80
		mixedCountTarget = 8
		rsetUpdatingPct = 10
		newSizePercent = 35
	default: // MemMid and unknown
		pauseMs = 100
		mixedCountTarget = 6
		rsetUpdatingPct = 8
		newSizePercent = 33
	}

	// Combat-biased baseline: STALCRAFT is effectively always in combat
	// (projectile events, hit registration, particle bursts, AI ticks).
	// An earlier ihop of 35 and tenuring of 6 optimised for idle /
	// animation-heavy states and left combat bursts to spill into the
	// old gen mid-fight, producing the 1-second stalls a 5700X +
	// DDR4-3600 tester reported. Lower IHOP starts concurrent marking
	// before the burst fills old gen; tenuring=3 lets short-lived combat
	// objects die in survivor before being force-promoted; a larger
	// survivor (ratio 12 instead of the Oracle default 8) absorbs the
	// burst without overflowing into old gen in the first place.
	ihop := 25
	softRefMs := 50
	tenuring := 3
	survivorRatio := 12

	if sys.HasBigCache() {
		// X3D-class parts can hit a tighter pause target thanks to
		// the huge L3 and high memory bandwidth headroom. Soft-ref
		// retention is extended so texture caches stay hot across
		// the larger working set V-Cache can absorb.
		pauseMs /= 2
		softRefMs = 100
		// V-Cache is the render thread's hottest asset. Community
		// testing on a 9800X3D + DDR5-6200 rig showed that running
		// 4-5 concurrent GC workers at 280+ FPS causes perceived
		// stutter even when frametime stays flat, because concurrent
		// marking evicts render-thread cache lines. Cap hard at 2
		// workers (3 on 16+ thread parts) to leave the V-Cache to the
		// game and rely on UseDynamicNumberOfGCThreads to scale up
		// only under real allocation pressure.
		if sys.CPUThreads >= 16 {
			concurrent = 3
		} else {
			concurrent = 2
		}
	}

	return Config{
		HeapSizeGB:  int(heap),
		PreTouch:    sys.TotalGB() >= 12,
		MetaspaceMB: 512,

		MaxGCPauseMillis:               pauseMs,
		G1HeapRegionSizeMB:             regionSize(heap),
		G1NewSizePercent:               newSizePercent,
		G1MaxNewSizePercent:            50,
		G1ReservePercent:               15,
		G1HeapWastePercent:             10,
		G1MixedGCCountTarget:           mixedCountTarget,
		InitiatingHeapOccupancyPercent: ihop,
		G1MixedGCLiveThresholdPercent:  85,
		G1RSetUpdatingPauseTimePercent: rsetUpdatingPct,
		SurvivorRatio:                  survivorRatio,
		MaxTenuringThreshold:           tenuring,

		G1SATBBufferEnqueueingThresholdPercent: 30,
		G1ConcRSHotCardLimit:                   16,
		G1ConcRefinementServiceIntervalMillis:  150,
		GCTimeRatio:                            99,
		UseDynamicNumberOfGCThreads:            true,
		UseStringDeduplication:                 false,

		ParallelGCThreads:       parallel,
		ConcGCThreads:           concurrent,
		SoftRefLRUPolicyMSPerMB: softRefMs,

		ReservedCodeCacheSizeMB: 400,
		MaxInlineLevel:          jit.maxInlineLevel,
		FreqInlineSize:          jit.freqInlineSize,
		InlineSmallCode:         jit.inlineSmallCode,
		MaxNodeLimit:            jit.maxNodeLimit,
		NodeLimitFudgeFactor:    8000,
		NmethodSweepActivity:    1,
		DontCompileHugeMethods:  false,
		AllocatePrefetchStyle:   3,
		AlwaysActAsServerClass:  true,
		UseXMMForArrayCopy:      true,
		UseFPUForSpilling:       true,

		UseLargePages: sys.LargePages,

		ReflectionInflationThreshold: 0,
		AutoBoxCacheMax:              4096,
		UseThreadPriorities:          true,
		ThreadPriorityPolicy:         1,
		UseCounterDecay:              false,
		CompileThresholdScaling:      0.5,
	}
}

// jitProfile scales C2 inlining limits with L3 cache. On normal CPUs
// a deeply inlined hot path spills out of L3; on X3D-class parts the
// full compiled working set fits, so deeper inlining is pure win.
type jitLimits struct {
	maxInlineLevel  int
	freqInlineSize  int
	inlineSmallCode int
	maxNodeLimit    int
}

func jitProfile(sys sysinfo.Info) jitLimits {
	if sys.HasBigCache() {
		return jitLimits{
			maxInlineLevel:  20,
			freqInlineSize:  750,
			inlineSmallCode: 6000,
			maxNodeLimit:    320000,
		}
	}
	return jitLimits{
		maxInlineLevel:  15,
		freqInlineSize:  500,
		inlineSmallCode: 4000,
		maxNodeLimit:    240000,
	}
}

// sizeHeap picks a heap size between 2 and 8 GB based on total RAM.
//
// We cap at 8 GB on purpose: STALCRAFT's live working set is ~2-3 GB,
// and larger heaps only inflate G1 scan time without helping throughput.
// The 2 GB floor is the minimum that lets G1 run efficiently; anything
// below and the game runs, but full GCs dominate.
func sizeHeap(totalGB uint64) uint64 {
	switch {
	case totalGB >= 24:
		return 8
	case totalGB >= 16:
		return 6
	case totalGB >= 12:
		return 5
	case totalGB >= 8:
		return 4
	case totalGB >= 6:
		return 3
	default:
		return 2
	}
}

// gcThreads derives the STW and concurrent GC worker counts from the
// total logical thread count reported by the OS (runtime.NumCPU).
//
// Parallel workers only run during STW — the game thread is stopped
// anyway, so HT/SMT siblings are free to do GC work without any
// contention. We scale parallel as "threads − 2" (leaving two threads
// to the OS and background services even during STW) and cap at 10
// where G1 hits diminishing returns on consumer hardware.
//
// Concurrent workers share CPU with the running game, so they stay
// a bit more conservative: roughly half of parallel, floor 1, ceiling 5.
// Non-X3D parts can soak five concurrent workers under combat-burst
// allocation pressure without the render thread noticing — cache
// contention is not the bottleneck when L3 is modest. X3D parts get
// their concurrent count clamped tighter inside Generate() because
// V-Cache contention is the real cost there, not raw throughput.
//
// Using logical threads (runtime.NumCPU) instead of physical_cores×2
// is essential for correctness on CPUs without SMT/HT: an Intel
// i5-9600KF is 6C/6T, not 6C/12T, and feeding 10 parallel workers to
// a 6-thread CPU oversubscribes it by 1.67× — context switching
// overhead wipes out the throughput gain from extra workers.
func gcThreads(threads int) (parallel, concurrent int) {
	parallel = clamp(threads-2, 2, 10)
	concurrent = clamp(parallel/2, 1, 5)
	return
}

// regionSize matches G1 region granularity to heap size. JVM only
// accepts powers of two between 1 and 32 MB; larger regions mean fewer
// RSet scans, smaller regions mean finer mixed-GC control. sizeHeap
// caps heap at 8 GB, and CapFrameX measurements on both an X3D with
// 8 GB heap and an i5-10400F with 5 GB heap showed 8 MB regions
// outperforming 16 MB — more regions gives mixed-GC selection finer
// granularity so each pass evacuates a smaller, more focused set.
// Stalcraft's large mesh data lives in LWJGL direct buffers off-heap,
// so the 4 MB humongous threshold at 8 MB regions is not a concern.
func regionSize(heapGB uint64) int {
	if heapGB <= 3 {
		return 4
	}
	return 8
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
