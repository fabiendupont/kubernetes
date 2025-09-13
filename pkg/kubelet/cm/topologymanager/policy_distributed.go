/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package topologymanager

import (
	"k8s.io/klog/v2"
)

type distributedPolicy struct {
	// numaInfo represents list of NUMA Nodes available on the underlying machine and distances between them
	numaInfo *NUMAInfo
	opts     PolicyOptions
}

var _ Policy = &distributedPolicy{}

// PolicyDistributed policy name.
const PolicyDistributed string = "distributed"

// NewDistributedPolicy returns distributed policy.
// This policy distributes resources across NUMA nodes for workloads that
// benefit from distributed memory access or load balancing scenarios.
func NewDistributedPolicy(numaInfo *NUMAInfo, opts PolicyOptions) Policy {
	return &distributedPolicy{numaInfo: numaInfo, opts: opts}
}

func (p *distributedPolicy) Name() string {
	return PolicyDistributed
}

func (p *distributedPolicy) canAdmitPodResult(hint *TopologyHint) bool {
	// Distributed policy accepts any valid allocation
	return hint.NUMANodeAffinity != nil
}

func (p *distributedPolicy) Merge(providersHints []map[string][]TopologyHint) (TopologyHint, bool) {
	// Distributed policy requires enhanced topology hints to function optimally
	if !EnhancedTopologyHintsEnabled() {
		klog.InfoS("Distributed policy works best with EnhancedTopologyHints feature enabled, falling back to best-effort behavior")
		// Fall back to best-effort behavior when enhanced hints are disabled
		filteredHints := filterProvidersHints(providersHints)
		merger := NewHintMerger(p.numaInfo, filteredHints, "best-effort", p.opts)
		bestHint := merger.Merge()
		return bestHint, bestHint.NUMANodeAffinity != nil
	}

	// Check if we have multiple resource types that can be distributed
	if !p.hasMultipleResourceTypes(providersHints) {
		klog.V(4).InfoS("Single resource type detected, using standard enhanced merging")
		enhancedMerger := NewEnhancedHintMerger(p.numaInfo, p.numaInfo.DefaultAffinityMask(), providersHints)
		bestHint := enhancedMerger.Merge()
		admit := p.canAdmitPodResult(&bestHint)
		return bestHint, admit
	}

	// Apply distributed merging logic for multiple resource types
	enhancedMerger := NewEnhancedHintMerger(p.numaInfo, p.numaInfo.DefaultAffinityMask(), providersHints)
	enhancedMerger.SetDistributedMerging(true)
	distributedHint := p.applyDistributionLogic(enhancedMerger, providersHints)
	
	admit := p.canAdmitPodResult(&distributedHint)
	return distributedHint, admit
}

// hasMultipleResourceTypes checks if multiple resource types are being requested
func (p *distributedPolicy) hasMultipleResourceTypes(providersHints []map[string][]TopologyHint) bool {
	resourceTypes := make(map[string]bool)
	
	for _, providerHints := range providersHints {
		for resourceName := range providerHints {
			resourceTypes[resourceName] = true
		}
	}
	
	return len(resourceTypes) > 1
}

// applyDistributionLogic implements the core distributed placement algorithm
func (p *distributedPolicy) applyDistributionLogic(merger *EnhancedHintMerger, providersHints []map[string][]TopologyHint) TopologyHint {
	// Get the base merged hint
	baseHint := merger.Merge()
	
	// If we only have one NUMA node, can't distribute
	if len(p.numaInfo.Nodes) <= 1 {
		klog.V(4).InfoS("Single NUMA node system, cannot distribute resources")
		return baseHint
	}
	
	// Try to create a distributed hint that spans multiple NUMA nodes
	distributedHint := p.createDistributedHint(providersHints)
	
	// If distributed hint is viable and has enhanced metrics, prefer it
	if distributedHint.NUMANodeAffinity != nil && distributedHint.NUMANodeAffinity.Count() > 1 {
		// For distributed policy, we prefer spreading across nodes
		// even if it's not marked as "preferred" by individual providers
		if EnhancedTopologyHintsEnabled() && distributedHint.hasEnhancedFields() {
			klog.V(4).InfoS("Using distributed hint with enhanced topology metrics", 
				"numaNodes", distributedHint.NUMANodeAffinity.Count(),
				"hopCount", distributedHint.GetHopCount(),
				"distance", distributedHint.GetDistance())
		}
		return distributedHint
	}
	
	// Fall back to base hint if distribution is not possible
	klog.V(4).InfoS("Distribution not possible, using base hint")
	return baseHint
}

// createDistributedHint creates a hint that distributes resources across NUMA nodes
func (p *distributedPolicy) createDistributedHint(providersHints []map[string][]TopologyHint) TopologyHint {
	// Start with all NUMA nodes available
	distributedAffinity := p.numaInfo.DefaultAffinityMask()
	
	var (
		totalHopCount  = 0
		totalDistance  = 0
		totalBandwidth = 0.0
		totalScore     = 0.0
		hintCount      = 0
		allPreferred   = true
	)
	
	// Collect metrics from all hints across providers
	for _, providerHints := range providersHints {
		for _, hints := range providerHints {
			for _, hint := range hints {
				// Consider all hints that have NUMA affinity
				if hint.NUMANodeAffinity != nil {
					hintCount++
					
					// Track if all hints are preferred
					if !hint.Preferred {
						allPreferred = false
					}
					
					// Aggregate enhanced metrics
					if EnhancedTopologyHintsEnabled() && hint.hasEnhancedFields() {
						totalHopCount += hint.GetHopCount()
						totalDistance += hint.GetDistance()
						totalBandwidth += hint.GetBandwidth()
						totalScore += hint.GetScore()
					}
				}
			}
		}
	}
	
	// Create distributed hint
	distributedHint := TopologyHint{
		NUMANodeAffinity: distributedAffinity,
		Preferred:        allPreferred, // Only preferred if all constituent hints are preferred
	}
	
	// Set enhanced fields if available
	if EnhancedTopologyHintsEnabled() && hintCount > 0 {
		// For distributed placement, use average metrics since resources are spread
		avgHopCount := totalHopCount / hintCount
		avgDistance := totalDistance / hintCount
		avgBandwidth := totalBandwidth / float64(hintCount)
		avgScore := totalScore / float64(hintCount)
		
		// Adjust score for distribution penalty (cross-NUMA access is more expensive)
		distributionPenalty := float64(distributedAffinity.Count() - 1) * 5.0 // 5 units per additional NUMA node
		adjustedScore := avgScore + distributionPenalty
		
		distributedHint.HopCount = &avgHopCount
		distributedHint.Distance = &avgDistance
		distributedHint.Bandwidth = &avgBandwidth
		distributedHint.Score = &adjustedScore
	}
	
	return distributedHint
}
