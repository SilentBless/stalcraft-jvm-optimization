package main

import "fmt"

func generateFlags(sys SystemInfo) []string {
	heap := calcHeap(sys)
	parallel, conc := calcGCThreads(sys)
	region := calcRegionSize(heap)
	meta := calcMetaspace(heap)
	cc := calcCodeCache(heap)
	surv, tenure := calcSurvivor(sys)
	softRef := calcSoftRef(heap)

	flags := []string{
		fmt.Sprintf("-Xmx%dg", heap),
		fmt.Sprintf("-Xms%dg", heap),
		"-XX:+AlwaysPreTouch",

		fmt.Sprintf("-XX:MetaspaceSize=%dm", meta),
		fmt.Sprintf("-XX:MaxMetaspaceSize=%dm", meta),

		"-XX:+UseG1GC",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:MaxGCPauseMillis=50",
		fmt.Sprintf("-XX:G1HeapRegionSize=%dm", region),
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1ReservePercent=15",
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:+G1UseAdaptiveIHOP",
		"-XX:InitiatingHeapOccupancyPercent=35",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		fmt.Sprintf("-XX:SurvivorRatio=%d", surv),
		fmt.Sprintf("-XX:MaxTenuringThreshold=%d", tenure),

		fmt.Sprintf("-XX:ParallelGCThreads=%d", parallel),
		fmt.Sprintf("-XX:ConcGCThreads=%d", conc),

		"-XX:+ParallelRefProcEnabled",
		"-XX:+DisableExplicitGC",
		fmt.Sprintf("-XX:SoftRefLRUPolicyMSPerMB=%d", softRef),

		"-XX:+UseCompressedOops",
		fmt.Sprintf("-XX:ReservedCodeCacheSize=%dm", cc),
		fmt.Sprintf("-XX:NonNMethodCodeHeapSize=%dm", cc*3/100+8),
		fmt.Sprintf("-XX:ProfiledCodeHeapSize=%dm", cc*40/100),
		fmt.Sprintf("-XX:NonProfiledCodeHeapSize=%dm", cc*57/100),
		"-XX:MaxInlineLevel=15",
		"-XX:FreqInlineSize=500",

		"-XX:+PerfDisableSharedMem",
		"-Djdk.nio.maxCachedBufferSize=131072",
	}

	if sys.LargePages {
		flags = append(flags,
			"-XX:+UseLargePages",
			fmt.Sprintf("-XX:LargePageSizeInBytes=%dm", sys.LargePageSize/(1024*1024)),
		)
	}

	return flags
}

func calcHeap(sys SystemInfo) uint64 {
	free := bytesToGB(sys.FreeRAM)
	total := bytesToGB(sys.TotalRAM)

	heap := free / 2

	floor := total / 4
	if floor < 2 {
		floor = 2
	}
	cap := total * 3 / 4
	if cap > 16 {
		cap = 16
	}

	if heap < floor {
		heap = floor
	}
	if heap > cap {
		heap = cap
	}
	if heap < 2 {
		heap = 2
	}
	return heap
}

func calcGCThreads(sys SystemInfo) (parallel, concurrent int) {
	parallel = sys.CPUCores - 2
	if parallel < 2 {
		parallel = 2
	}
	concurrent = parallel / 4
	if concurrent < 1 {
		concurrent = 1
	}
	return
}

func calcRegionSize(heapGB uint64) uint64 {
	switch {
	case heapGB <= 4:
		return 4
	case heapGB <= 8:
		return 8
	case heapGB <= 16:
		return 16
	default:
		return 32
	}
}

func calcMetaspace(heapGB uint64) uint64 {
	switch {
	case heapGB <= 4:
		return 128
	case heapGB <= 8:
		return 256
	default:
		return 512
	}
}

func calcCodeCache(heapGB uint64) uint64 {
	cc := heapGB * 1024 / 16
	if cc < 128 {
		cc = 128
	}
	if cc > 512 {
		cc = 512
	}
	return cc
}

func calcSurvivor(sys SystemInfo) (ratio, tenuring int) {
	if sys.CPUCores <= 4 {
		return 32, 1
	}
	return 8, 4
}

func calcSoftRef(heapGB uint64) int {
	switch {
	case heapGB <= 4:
		return 10
	case heapGB <= 8:
		return 25
	default:
		return 50
	}
}
