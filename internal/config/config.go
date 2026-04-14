// Package config models the JVM tuning profile: persistence on disk
// (configs/*.json), the "active" pointer in HKCU, and auto-generation
// from detected hardware.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

const registryPath = `Software\StalcraftWrapper`

// ErrNotFound is returned when a config file does not exist on disk.
var ErrNotFound = errors.New("config not found")

type Config struct {
	HeapSizeGB  int  `json:"heap_size_gb"`
	PreTouch    bool `json:"pre_touch"`
	MetaspaceMB int  `json:"metaspace_mb"`

	MaxGCPauseMillis               int `json:"max_gc_pause_millis"`
	G1HeapRegionSizeMB             int `json:"g1_heap_region_size_mb"`
	G1NewSizePercent               int `json:"g1_new_size_percent"`
	G1MaxNewSizePercent            int `json:"g1_max_new_size_percent"`
	G1ReservePercent               int `json:"g1_reserve_percent"`
	G1HeapWastePercent             int `json:"g1_heap_waste_percent"`
	G1MixedGCCountTarget           int `json:"g1_mixed_gc_count_target"`
	InitiatingHeapOccupancyPercent int `json:"initiating_heap_occupancy_percent"`
	G1MixedGCLiveThresholdPercent  int `json:"g1_mixed_gc_live_threshold_percent"`
	G1RSetUpdatingPauseTimePercent int `json:"g1_rset_updating_pause_time_percent"`
	SurvivorRatio                  int `json:"survivor_ratio"`
	MaxTenuringThreshold           int `json:"max_tenuring_threshold"`

	G1SATBBufferEnqueueingThresholdPercent int  `json:"g1_satb_buffer_enqueuing_threshold_percent"`
	G1ConcRSHotCardLimit                   int  `json:"g1_conc_rs_hot_card_limit"`
	G1ConcRefinementServiceIntervalMillis  int  `json:"g1_conc_refinement_service_interval_millis"`
	GCTimeRatio                            int  `json:"gc_time_ratio"`
	UseDynamicNumberOfGCThreads            bool `json:"use_dynamic_number_of_gc_threads"`
	UseStringDeduplication                 bool `json:"use_string_deduplication"`

	ParallelGCThreads int `json:"parallel_gc_threads"`
	ConcGCThreads     int `json:"conc_gc_threads"`

	SoftRefLRUPolicyMSPerMB int `json:"soft_ref_lru_policy_ms_per_mb"`

	ReservedCodeCacheSizeMB int  `json:"reserved_code_cache_size_mb"`
	MaxInlineLevel          int  `json:"max_inline_level"`
	FreqInlineSize          int  `json:"freq_inline_size"`
	InlineSmallCode         int  `json:"inline_small_code"`
	MaxNodeLimit            int  `json:"max_node_limit"`
	NodeLimitFudgeFactor    int  `json:"node_limit_fudge_factor"`
	NmethodSweepActivity    int  `json:"nmethod_sweep_activity"`
	DontCompileHugeMethods  bool `json:"dont_compile_huge_methods"`
	AllocatePrefetchStyle   int  `json:"allocate_prefetch_style"`
	AlwaysActAsServerClass  bool `json:"always_act_as_server_class"`
	UseXMMForArrayCopy      bool `json:"use_xmm_for_array_copy"`
	UseFPUForSpilling       bool `json:"use_fpu_for_spilling"`

	UseLargePages bool `json:"use_large_pages"`

	// Java 9-era flags: reflection fast path, integer boxing cache,
	// thread scheduling, JIT counter retention, and a C1→C2 promotion
	// multiplier. Loop Strip Mining is intentionally absent —
	// -XX:LoopStripMiningIter was only added in JDK 10.
	ReflectionInflationThreshold int     `json:"reflection_inflation_threshold"`
	AutoBoxCacheMax              int     `json:"auto_box_cache_max"`
	UseThreadPriorities          bool    `json:"use_thread_priorities"`
	ThreadPriorityPolicy         int     `json:"thread_priority_policy"`
	UseCounterDecay              bool    `json:"use_counter_decay"`
	CompileThresholdScaling      float64 `json:"compile_threshold_scaling"`
}

// Dir returns the configs directory next to the executable.
// Falls back to ./configs if the executable path can't be resolved.
func Dir() string {
	self, err := os.Executable()
	if err != nil {
		return filepath.Join(".", "configs")
	}
	return filepath.Join(filepath.Dir(self), "configs")
}

// Ensure makes sure the configs directory and a "default" config exist,
// and that an active config is selected in the registry.
func Ensure(sys sysinfo.Info) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create configs dir: %w", err)
	}

	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("scan configs dir: %w", err)
	}
	if len(entries) == 0 {
		if err := Generate(sys).Save("default"); err != nil {
			return fmt.Errorf("save default config: %w", err)
		}
	}

	if ActiveName() == "" {
		if err := SetActive("default"); err != nil {
			return fmt.Errorf("set active config: %w", err)
		}
	}
	return nil
}

// Save writes the config to configs/<name>.json.
func (c Config) Save(name string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create configs dir: %w", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Load reads configs/<name>.json.
func Load(name string) (Config, error) {
	path := filepath.Join(Dir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// LoadActive reads the config currently selected in the registry.
// If the selection points at a name with no corresponding file on
// disk (the user deleted their custom profile after selecting it),
// or if no selection has been made at all, the function falls back
// to "default" automatically. The returned name is the config that
// was actually loaded — comparing it against ActiveName() lets the
// caller detect that a fallback happened and warn the user.
func LoadActive() (cfg Config, loadedName string, err error) {
	requested := ActiveName()
	if requested == "" {
		requested = "default"
	}
	cfg, err = Load(requested)
	if errors.Is(err, ErrNotFound) && requested != "default" {
		// Selection refers to a profile that no longer exists.
		// Try the default profile as a safety net.
		if fallbackCfg, fallbackErr := Load("default"); fallbackErr == nil {
			return fallbackCfg, "default", nil
		}
	}
	return cfg, requested, err
}

// ActiveExists reports whether the currently selected active config
// name has a corresponding file on disk. It returns false when no
// selection has been made or when the selection has been deleted.
// Callers can use it to surface a "missing, will fall back to default"
// notice in the UI without having to actually load the config.
func ActiveExists() bool {
	name := ActiveName()
	if name == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(Dir(), name+".json"))
	return err == nil
}

// List returns the names (without .json) of every config on disk.
func List() ([]string, error) {
	entries, err := filepath.Glob(filepath.Join(Dir(), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("scan configs: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		base := filepath.Base(e)
		names = append(names, base[:len(base)-len(".json")])
	}
	return names, nil
}

// SetActive records the active config name in HKCU.
func SetActive(name string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue("ActiveConfig", name); err != nil {
		return fmt.Errorf("set ActiveConfig: %w", err)
	}
	return nil
}

// ActiveName reads the active config name from HKCU, empty string if unset.
func ActiveName() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	val, _, err := key.GetStringValue("ActiveConfig")
	if err != nil {
		return ""
	}
	return val
}
