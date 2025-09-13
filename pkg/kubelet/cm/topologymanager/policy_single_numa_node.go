/*
Copyright 2019 The Kubernetes Authors.

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

type singleNumaNodePolicy struct {
	// numaInfo represents list of NUMA Nodes available on the underlying machine and distances between them
	numaInfo *NUMAInfo
	opts     PolicyOptions
}

var _ Policy = &singleNumaNodePolicy{}

// PolicySingleNumaNode policy name.
const PolicySingleNumaNode string = "single-numa-node"

// NewSingleNumaNodePolicy returns single-numa-node policy.
func NewSingleNumaNodePolicy(numaInfo *NUMAInfo, opts PolicyOptions) Policy {
	return &singleNumaNodePolicy{numaInfo: numaInfo, opts: opts}
}

func (p *singleNumaNodePolicy) Name() string {
	return PolicySingleNumaNode
}

func (p *singleNumaNodePolicy) canAdmitPodResult(hint *TopologyHint) bool {
	return hint.Preferred
}

// Return hints that have valid bitmasks with exactly one bit set.
func filterSingleNumaHints(allResourcesHints [][]TopologyHint) [][]TopologyHint {
	var filteredResourcesHints [][]TopologyHint
	for _, oneResourceHints := range allResourcesHints {
		var filtered []TopologyHint
		for _, hint := range oneResourceHints {
			if hint.NUMANodeAffinity == nil && hint.Preferred {
				filtered = append(filtered, hint)
			}
			if hint.NUMANodeAffinity != nil && hint.NUMANodeAffinity.Count() == 1 && hint.Preferred {
				filtered = append(filtered, hint)
			}
		}
		filteredResourcesHints = append(filteredResourcesHints, filtered)
	}
	return filteredResourcesHints
}

func (p *singleNumaNodePolicy) Merge(providersHints []map[string][]TopologyHint) (TopologyHint, bool) {
	filteredHints := filterProvidersHints(providersHints)
	// Filter to only include don't cares and hints with a single NUMA node.
	singleNumaHints := filterSingleNumaHints(filteredHints)

	var bestHint TopologyHint
	if EnhancedTopologyHintsEnabled() {
		// Use enhanced merger when feature is enabled, but only with single NUMA hints
		// Create filtered providers hints for single NUMA
		singleNumaProvidersHints := make([]map[string][]TopologyHint, len(providersHints))
		for i, providerHints := range providersHints {
			singleNumaProvidersHints[i] = make(map[string][]TopologyHint)
			for resource, hints := range providerHints {
				var filteredHints []TopologyHint
				for _, hint := range hints {
					// Only include single NUMA hints or don't care hints
					if hint.NUMANodeAffinity == nil || hint.NUMANodeAffinity.Count() <= 1 {
						filteredHints = append(filteredHints, hint)
					}
				}
				if len(filteredHints) > 0 {
					singleNumaProvidersHints[i][resource] = filteredHints
				}
			}
		}
		enhancedMerger := NewEnhancedHintMerger(p.numaInfo, p.numaInfo.DefaultAffinityMask(), singleNumaProvidersHints)
		bestHint = enhancedMerger.Merge()
	} else {
		// Use traditional merger for backward compatibility
		merger := NewHintMerger(p.numaInfo, singleNumaHints, p.Name(), p.opts)
		bestHint = merger.Merge()
	}

	if bestHint.NUMANodeAffinity.IsEqual(p.numaInfo.DefaultAffinityMask()) {
		bestHint = TopologyHint{NUMANodeAffinity: nil, Preferred: bestHint.Preferred}
	}

	admit := p.canAdmitPodResult(&bestHint)
	return bestHint, admit
}
