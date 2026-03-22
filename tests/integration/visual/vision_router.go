// Package visual provides vision provider routing strategies.
package visual

import (
	"fmt"
	"math"
	"sort"
)

// RouterStrategy determines how to select a vision provider.
type RouterStrategy int

const (
	// RouteByCost selects the cheapest provider.
	RouteByCost RouterStrategy = iota

	// RouteByQuality selects the highest quality provider.
	RouteByQuality

	// RouteBySpeed selects the fastest provider.
	RouteBySpeed

	// RouteByAvailability selects any available provider.
	RouteByAvailability
)

// String returns the string representation of the strategy.
func (s RouterStrategy) String() string {
	switch s {
	case RouteByCost:
		return "cost"
	case RouteByQuality:
		return "quality"
	case RouteBySpeed:
		return "speed"
	case RouteByAvailability:
		return "availability"
	default:
		return "unknown"
	}
}

// ProviderInfo contains metadata about a vision provider.
type ProviderInfo struct {
	Name     string

	// Cost is the estimated cost per 1K images (in USD).
	Cost float64

	// Quality is a subjective quality score (0-100).
	Quality int

	// Speed is a subjective speed score (0-100, higher is faster).
	Speed int

	// Available indicates if the provider is currently available.
	Available bool

	// Latency is the average response time in milliseconds.
	Latency int
}

// VisionRouter selects vision providers based on strategy.
type VisionRouter struct {
	strategy RouterStrategy
	fallback []string // Fallback order

	// Provider metadata cache
	providerInfo map[string]*ProviderInfo

	// Performance tracking
	latencyHistory map[string][]int
	callCount      map[string]int
	errorCount     map[string]int
}

// NewVisionRouter creates a new vision router.
func NewVisionRouter(strategy RouterStrategy) *VisionRouter {
	return &VisionRouter{
		strategy:       strategy,
		fallback:       []string{"claude", "gpt", "gemini"},
		providerInfo:   make(map[string]*ProviderInfo),
		latencyHistory: make(map[string][]int),
		callCount:      make(map[string]int),
		errorCount:     make(map[string]int),
	}
}

// SetFallback sets the fallback provider order.
func (vr *VisionRouter) SetFallback(fallback []string) {
	vr.fallback = fallback
}

// SetStrategy sets the routing strategy.
func (vr *VisionRouter) SetStrategy(strategy RouterStrategy) {
	vr.strategy = strategy
}

// RegisterProvider registers provider metadata.
func (vr *VisionRouter) RegisterProvider(info *ProviderInfo) {
	vr.providerInfo[info.Name] = info
}

// SelectProvider selects the best provider based on strategy.
func (vr *VisionRouter) SelectProvider(providers map[string]VisionProvider) VisionProvider {
	// Build list of available providers
	var available []string
	for name := range providers {
		available = append(available, name)
	}

	if len(available) == 0 {
		return nil
	}

	// Try fallback order first
	for _, name := range vr.fallback {
		if _, ok := providers[name]; ok {
			if vr.isAvailable(name) {
				return providers[name]
			}
		}
	}

	// If fallback fails, use strategy to select from available
	switch vr.strategy {
	case RouteByCost:
		return vr.selectByCost(providers)
	case RouteByQuality:
		return vr.selectByQuality(providers)
	case RouteBySpeed:
		return vr.selectBySpeed(providers)
	case RouteByAvailability:
		return vr.selectByAvailability(providers)
	default:
		// Return first available
		for _, provider := range providers {
			return provider
		}
		return nil
	}
}

// selectByCost selects the cheapest provider.
func (vr *VisionRouter) selectByCost(providers map[string]VisionProvider) VisionProvider {
	type costPair struct {
		name string
		cost float64
	}

	var costs []costPair
	for name := range providers {
		info, ok := vr.providerInfo[name]
		if !ok {
			continue
		}
		costs = append(costs, costPair{name: name, cost: info.Cost})
	}

	// Sort by cost (ascending)
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].cost < costs[j].cost
	})

	if len(costs) > 0 {
		return providers[costs[0].name]
	}

	// Fallback to first available
	for _, provider := range providers {
		return provider
	}
	return nil
}

// selectByQuality selects the highest quality provider.
func (vr *VisionRouter) selectByQuality(providers map[string]VisionProvider) VisionProvider {
	type qualityPair struct {
		name    string
		quality int
	}

	var qualities []qualityPair
	for name := range providers {
		info, ok := vr.providerInfo[name]
		if !ok {
			// Unknown providers get default quality
			info = &ProviderInfo{Name: name, Quality: 50}
			vr.providerInfo[name] = info
		}
		qualities = append(qualities, qualityPair{name: name, quality: info.Quality})
	}

	// Sort by quality (descending)
	sort.Slice(qualities, func(i, j int) bool {
		return qualities[i].quality > qualities[j].quality
	})

	if len(qualities) > 0 {
		return providers[qualities[0].name]
	}

	// Fallback
	for _, provider := range providers {
		return provider
	}
	return nil
}

// selectBySpeed selects the fastest provider.
func (vr *VisionRouter) selectBySpeed(providers map[string]VisionProvider) VisionProvider {
	type speedPair struct {
		name  string
		speed int
	}

	var speeds []speedPair
	for name := range providers {
		info, ok := vr.providerInfo[name]
		if !ok {
			// Unknown providers get default speed
			info = &ProviderInfo{Name: name, Speed: 50}
			vr.providerInfo[name] = info
		}
		speeds = append(speeds, speedPair{name: name, speed: info.Speed})
	}

	// Sort by speed (descending)
	sort.Slice(speeds, func(i, j int) bool {
		return speeds[i].speed > speeds[j].speed
	})

	if len(speeds) > 0 {
		return providers[speeds[0].name]
	}

	// Fallback
	for _, provider := range providers {
		return provider
	}
	return nil
}

// selectByAvailability selects any available provider.
func (vr *VisionRouter) selectByAvailability(providers map[string]VisionProvider) VisionProvider {
	// Prefer providers with high availability
	for _, name := range vr.fallback {
		if provider, ok := providers[name]; ok {
			if vr.isAvailable(name) {
				return provider
			}
		}
	}

	// Return any available
	for _, provider := range providers {
		return provider
	}
	return nil
}

// isAvailable checks if a provider is available (low error rate).
func (vr *VisionRouter) isAvailable(name string) bool {
	info, ok := vr.providerInfo[name]
	if ok && !info.Available {
		return false
	}

	// Check error rate
	if vr.callCount[name] > 0 {
		errorRate := float64(vr.errorCount[name]) / float64(vr.callCount[name])
		// Consider provider unavailable if error rate > 50%
		if errorRate > 0.5 {
			return false
		}
	}

	return true
}

// RecordCall records a provider call for performance tracking.
func (vr *VisionRouter) RecordCall(name string, latency int, success bool) {
	vr.callCount[name]++
	vr.latencyHistory[name] = append(vr.latencyHistory[name], latency)

	if !success {
		vr.errorCount[name]++
	}

	// Update provider info
	info, ok := vr.providerInfo[name]
	if !ok {
		info = &ProviderInfo{Name: name}
		vr.providerInfo[name] = info
	}

	// Update latency (moving average)
	if len(vr.latencyHistory[name]) > 0 {
		sum := 0
		for _, lat := range vr.latencyHistory[name] {
			sum += lat
		}
		info.Latency = sum / len(vr.latencyHistory[name])

		// Update speed score based on latency (lower latency = higher speed)
		// Assume 100ms = 100 speed, 1000ms+ = 0 speed
		info.Speed = int(math.Max(0, 100-float64(info.Latency)/10))
	}

	// Update availability based on recent errors
	if vr.callCount[name] > 10 {
		errorRate := float64(vr.errorCount[name]) / float64(vr.callCount[name])
		info.Available = errorRate < 0.5
	}
}

// GetProviderStats returns statistics for a provider.
func (vr *VisionRouter) GetProviderStats(name string) *ProviderStats {
	info, ok := vr.providerInfo[name]
	if !ok {
		return nil
	}

	stats := &ProviderStats{
		Name:      name,
		CallCount: vr.callCount[name],
		ErrorCount: vr.errorCount[name],
	}

	if vr.callCount[name] > 0 {
		stats.ErrorRate = float64(vr.errorCount[name]) / float64(vr.callCount[name])
	}

	if len(vr.latencyHistory[name]) > 0 {
		sum := 0
		min := vr.latencyHistory[name][0]
		max := vr.latencyHistory[name][0]

		for _, lat := range vr.latencyHistory[name] {
			sum += lat
			if lat < min {
				min = lat
			}
			if lat > max {
				max = lat
			}
		}

		stats.AvgLatency = sum / len(vr.latencyHistory[name])
		stats.MinLatency = min
		stats.MaxLatency = max
	}

	// Copy metadata
	stats.Cost = info.Cost
	stats.Quality = info.Quality
	stats.Speed = info.Speed
	stats.Available = info.Available

	return stats
}

// GetAllStats returns statistics for all providers.
func (vr *VisionRouter) GetAllStats() map[string]*ProviderStats {
	stats := make(map[string]*ProviderStats)
	for name := range vr.providerInfo {
		stats[name] = vr.GetProviderStats(name)
	}
	return stats
}

// ProviderStats contains performance statistics for a provider.
type ProviderStats struct {
	Name        string
	CallCount   int
	ErrorCount  int
	ErrorRate   float64
	AvgLatency  int
	MinLatency  int
	MaxLatency  int
	Cost        float64
	Quality     int
	Speed       int
	Available   bool
}

// String returns a formatted stats string.
func (s *ProviderStats) String() string {
	return fmt.Sprintf("%s: calls=%d errors=%d (%.1f%%) latency=%dms [%d-%d] cost=$%.2f quality=%d speed=%d",
		s.Name,
		s.CallCount,
		s.ErrorCount,
		s.ErrorRate*100,
		s.AvgLatency,
		s.MinLatency,
		s.MaxLatency,
		s.Cost,
		s.Quality,
		s.Speed,
	)
}

// Reset clears all router statistics.
func (vr *VisionRouter) Reset() {
	vr.providerInfo = make(map[string]*ProviderInfo)
	vr.latencyHistory = make(map[string][]int)
	vr.callCount = make(map[string]int)
	vr.errorCount = make(map[string]int)
}

// RecommendStrategy recommends the best strategy based on historical data.
func (vr *VisionRouter) RecommendStrategy() RouterStrategy {
	// If cost is a concern, recommend cost-based routing
	totalCost := 0.0
	for _, info := range vr.providerInfo {
		totalCost += info.Cost
	}
	if totalCost > 5.0 { // More than $5 per 1K images
		return RouteByCost
	}

	// If there are quality issues, recommend quality-based
	avgQuality := 0.0
	count := 0
	for _, info := range vr.providerInfo {
		avgQuality += float64(info.Quality)
		count++
	}
	if count > 0 && avgQuality/float64(count) < 70 {
		return RouteByQuality
	}

	// Default to availability
	return RouteByAvailability
}
