package jvm

import "strings"

var exactRemove = map[string]struct{}{
	"-XX:-PrintCommandLineFlags":         {},
	"-XX:+UseG1GC":                       {},
	"-XX:+UseCompressedOops":             {},
	"-XX:+PerfDisableSharedMem":          {},
	"-XX:+UseBiasedLocking":              {},
	"-XX:-UseBiasedLocking":              {},
	"-XX:+UseStringDeduplication":        {},
	"-XX:+UseNUMA":                       {},
	"-XX:+DisableAttachMechanism":        {},
	"-XX:+UseDynamicNumberOfGCThreads":   {},
	"-XX:+AlwaysActAsServerClassMachine": {},
	"-XX:+UseXMMForArrayCopy":            {},
	"-XX:+UseFPUForSpilling":             {},
	"-XX:-DontCompileHugeMethods":        {},
	"-XX:+AlwaysPreTouch":                {},
	"-XX:+ParallelRefProcEnabled":        {},
	"-XX:+DisableExplicitGC":             {},
	"-XX:+G1UseAdaptiveIHOP":             {},
	"-XX:+UnlockExperimentalVMOptions":   {},
	"-XX:+UseThreadPriorities":           {},
	"-XX:-UseThreadPriorities":           {},
	"-XX:+UseCounterDecay":               {},
	"-XX:-UseCounterDecay":               {},
	"-XX:+UseLargePages":                 {},
	"-XX:-UseLargePages":                 {},
}

var prefixRemove = []string{
	"-XX:MaxGCPauseMillis=",
	"-XX:MetaspaceSize=",
	"-XX:MaxMetaspaceSize=",
	"-XX:G1HeapRegionSize=",
	"-XX:G1NewSizePercent=",
	"-XX:G1MaxNewSizePercent=",
	"-XX:G1ReservePercent=",
	"-XX:G1HeapWastePercent=",
	"-XX:G1MixedGCCountTarget=",
	"-XX:InitiatingHeapOccupancyPercent=",
	"-XX:G1MixedGCLiveThresholdPercent=",
	"-XX:G1RSetUpdatingPauseTimePercent=",
	"-XX:G1SATBBufferEnqueueingThresholdPercent=",
	"-XX:G1ConcRSHotCardLimit=",
	"-XX:G1ConcRefinementServiceIntervalMillis=",
	"-XX:GCTimeRatio=",
	"-XX:SurvivorRatio=",
	"-XX:MaxTenuringThreshold=",
	"-XX:ParallelGCThreads=",
	"-XX:ConcGCThreads=",
	"-XX:SoftRefLRUPolicyMSPerMB=",
	"-XX:ReservedCodeCacheSize=",
	"-XX:NonNMethodCodeHeapSize=",
	"-XX:ProfiledCodeHeapSize=",
	"-XX:NonProfiledCodeHeapSize=",
	"-XX:MaxInlineLevel=",
	"-XX:FreqInlineSize=",
	"-XX:InlineSmallCode=",
	"-XX:MaxNodeLimit=",
	"-XX:NodeLimitFudgeFactor=",
	"-XX:NmethodSweepActivity=",
	"-XX:AllocatePrefetchStyle=",
	"-XX:LargePageSizeInBytes=",
	"-XX:AutoBoxCacheMax=",
	"-XX:ThreadPriorityPolicy=",
	"-XX:CompileThresholdScaling=",
	"-Dsun.reflect.inflationThreshold=",
	"-Xms",
	"-Xmx",
}

// splitArgs partitions the launcher's argv into JVM flags, the main class,
// and arguments passed to main().
func splitArgs(args []string) (jvm []string, mainClass string, app []string) {
	for i := 0; i < len(args); {
		a := args[i]
		if a == "-classpath" || a == "-cp" || a == "-jar" {
			jvm = append(jvm, a)
			i++
			if i < len(args) {
				jvm = append(jvm, args[i])
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			jvm = append(jvm, a)
			i++
			continue
		}
		mainClass = a
		app = args[i+1:]
		return
	}
	return
}

func shouldRemove(arg string) bool {
	if _, ok := exactRemove[arg]; ok {
		return true
	}
	for _, p := range prefixRemove {
		if strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

// FilterArgs strips launcher-injected flags that conflict with ours,
// then splices the generated flags back in, preserving the original
// main class and app arguments.
func FilterArgs(orig, injected []string) []string {
	jvmArgs, mainClass, app := splitArgs(orig)

	filtered := make([]string, 0, len(jvmArgs))
	for _, a := range jvmArgs {
		if !shouldRemove(a) {
			filtered = append(filtered, a)
		}
	}

	result := make([]string, 0, len(filtered)+len(injected)+1+len(app))
	result = append(result, filtered...)
	result = append(result, injected...)
	if mainClass != "" {
		result = append(result, mainClass)
	}
	return append(result, app...)
}
