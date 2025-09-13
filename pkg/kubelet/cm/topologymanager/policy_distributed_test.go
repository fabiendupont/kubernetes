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
	"testing"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

func TestPolicyDistributedName(t *testing.T) {
	numaInfo := commonNUMAInfoTwoNodes()
	policy := NewDistributedPolicy(numaInfo, PolicyOptions{})
	
	if policy.Name() != PolicyDistributed {
		t.Errorf("Expected policy name to be %s, got %s", PolicyDistributed, policy.Name())
	}
}

func TestPolicyDistributedCanAdmitPodResult(t *testing.T) {
	testCases := []struct {
		name     string
		hint     TopologyHint
		expected bool
	}{
		{
			name:     "Hint with valid NUMA affinity should be admitted",
			hint:     TopologyHint{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
			expected: true,
		},
		{
			name:     "Hint with valid multi-NUMA affinity should be admitted",
			hint:     TopologyHint{NUMANodeAffinity: NewTestBitMask(0, 1), Preferred: true},
			expected: true,
		},
		{
			name:     "Hint with nil NUMA affinity should not be admitted",
			hint:     TopologyHint{NUMANodeAffinity: nil, Preferred: true},
			expected: false,
		},
		{
			name:     "Non-preferred hint with valid affinity should still be admitted",
			hint:     TopologyHint{NUMANodeAffinity: NewTestBitMask(0, 1), Preferred: false},
			expected: true,
		},
	}

	numaInfo := commonNUMAInfoTwoNodes()
	policy := &distributedPolicy{numaInfo: numaInfo, opts: PolicyOptions{}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := policy.canAdmitPodResult(&tc.hint)
			if result != tc.expected {
				t.Errorf("Expected canAdmitPodResult() to return %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestPolicyDistributedMerge(t *testing.T) {
	testCases := []struct {
		name                 string
		featureEnabled       bool
		providersHints       []map[string][]TopologyHint
		expectedAdmit        bool
		expectedDistributed  bool
		expectFallback       bool
	}{
		{
			name:           "Feature disabled - should fall back to best-effort",
			featureEnabled: false,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: true},
					},
				},
			},
			expectedAdmit:       true,
			expectedDistributed: false,
			expectFallback:      true,
		},
		{
			name:           "Single resource type - should use standard enhanced merging",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
			},
			expectedAdmit:       true,
			expectedDistributed: false,
			expectFallback:      false,
		},
		{
			name:           "Multiple resource types - should use distributed merging",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: true},
					},
				},
			},
			expectedAdmit:       true,
			expectedDistributed: true,
			expectFallback:      false,
		},
		{
			name:           "Single NUMA node system - cannot distribute",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
			},
			expectedAdmit:       true,
			expectedDistributed: false,
			expectFallback:      false,
		},
		{
			name:           "No hints provided",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{},
			expectedAdmit:    true,
			expectedDistributed: false,
			expectFallback:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			var numaInfo *NUMAInfo
			if tc.name == "Single NUMA node system - cannot distribute" {
				numaInfo = commonNUMAInfoOneNode()
			} else {
				numaInfo = commonNUMAInfoTwoNodes()
			}
			
			policy := NewDistributedPolicy(numaInfo, PolicyOptions{})
			hint, admit := policy.Merge(tc.providersHints)

			if admit != tc.expectedAdmit {
				t.Errorf("Expected admit=%v, got %v", tc.expectedAdmit, admit)
			}

			if tc.expectedDistributed {
				// Should span multiple NUMA nodes
				if hint.NUMANodeAffinity != nil && hint.NUMANodeAffinity.Count() <= 1 {
					t.Errorf("Expected distributed hint to span multiple NUMA nodes, got count=%d", hint.NUMANodeAffinity.Count())
				}
			}

			// For multi-resource scenarios with feature enabled, we expect some form of hint
			if tc.featureEnabled && len(tc.providersHints) > 1 && hint.NUMANodeAffinity == nil {
				t.Errorf("Expected valid NUMA affinity for multi-resource scenario")
			}
		})
	}
}

func TestPolicyDistributedHasMultipleResourceTypes(t *testing.T) {
	testCases := []struct {
		name           string
		providersHints []map[string][]TopologyHint
		expected       bool
	}{
		{
			name:           "No providers",
			providersHints: []map[string][]TopologyHint{},
			expected:       false,
		},
		{
			name: "Single resource type",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
			},
			expected: false,
		},
		{
			name: "Multiple providers, same resource type",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: true},
					},
				},
			},
			expected: false,
		},
		{
			name: "Multiple resource types",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: true},
					},
				},
			},
			expected: true,
		},
		{
			name: "Multiple resource types across multiple providers",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: true},
					},
				},
				{
					"resource3": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
			},
			expected: true,
		},
	}

	numaInfo := commonNUMAInfoTwoNodes()
	policy := &distributedPolicy{numaInfo: numaInfo, opts: PolicyOptions{}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := policy.hasMultipleResourceTypes(tc.providersHints)
			if result != tc.expected {
				t.Errorf("Expected hasMultipleResourceTypes() to return %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestPolicyDistributedCreateDistributedHint(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		providersHints []map[string][]TopologyHint
		expectEnhanced bool
	}{
		{
			name:           "Feature disabled - basic hint",
			featureEnabled: false,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
			},
			expectEnhanced: false,
		},
		{
			name:           "Feature enabled - enhanced hint with metrics",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(50.0),
						},
					},
				},
				{
					"resource2": {
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         intPtr(2),
							Bandwidth:        float64Ptr(80.0),
							Distance:         intPtr(30),
							Score:            float64Ptr(70.0),
						},
					},
				},
			},
			expectEnhanced: true,
		},
		{
			name:           "Mixed preferred and non-preferred hints",
			featureEnabled: true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{NUMANodeAffinity: NewTestBitMask(0), Preferred: true},
					},
				},
				{
					"resource2": {
						{NUMANodeAffinity: NewTestBitMask(1), Preferred: false},
					},
				},
			},
			expectEnhanced: false, // Should not be preferred overall
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			numaInfo := commonNUMAInfoTwoNodes()
			policy := &distributedPolicy{numaInfo: numaInfo, opts: PolicyOptions{}}

			hint := policy.createDistributedHint(tc.providersHints)

			// Should always have default affinity (spans all NUMA nodes)
			if !hint.NUMANodeAffinity.IsEqual(numaInfo.DefaultAffinityMask()) {
				t.Errorf("Expected distributed hint to span all NUMA nodes")
			}

			if tc.expectEnhanced && tc.featureEnabled {
				if !hint.hasEnhancedFields() {
					t.Errorf("Expected enhanced fields when feature enabled and enhanced hints provided")
				}
			}

			// Check preference logic
			if tc.name == "Mixed preferred and non-preferred hints" {
				if hint.Preferred {
					t.Errorf("Expected distributed hint to be non-preferred when mixing preferred and non-preferred hints")
				}
			}
		})
	}
}

func TestPolicyDistributedApplyDistributionLogic(t *testing.T) {
	testCases := []struct {
		name           string
		numaInfo       *NUMAInfo
		providersHints []map[string][]TopologyHint
		expectDistrib  bool
	}{
		{
			name:     "Single NUMA node - cannot distribute",
			numaInfo: commonNUMAInfoOneNode(),
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {{NUMANodeAffinity: NewTestBitMask(0), Preferred: true}},
				},
			},
			expectDistrib: false,
		},
		{
			name:     "Multi NUMA nodes - can distribute",
			numaInfo: commonNUMAInfoTwoNodes(),
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {{NUMANodeAffinity: NewTestBitMask(0), Preferred: true}},
				},
				{
					"resource2": {{NUMANodeAffinity: NewTestBitMask(1), Preferred: true}},
				},
			},
			expectDistrib: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			policy := &distributedPolicy{numaInfo: tc.numaInfo, opts: PolicyOptions{}}
			merger := NewEnhancedHintMerger(tc.numaInfo, tc.numaInfo.DefaultAffinityMask(), tc.providersHints)

			hint := policy.applyDistributionLogic(merger, tc.providersHints)

			if tc.expectDistrib {
				if hint.NUMANodeAffinity == nil || hint.NUMANodeAffinity.Count() <= 1 {
					t.Errorf("Expected distributed hint to span multiple NUMA nodes")
				}
			} else {
				// For single NUMA, should fall back to base hint
				if hint.NUMANodeAffinity != nil && hint.NUMANodeAffinity.Count() > 1 {
					t.Errorf("Expected single NUMA hint for single-node system")
				}
			}
		})
	}
}

// Helper function to create single NUMA node info for testing
func commonNUMAInfoOneNode() *NUMAInfo {
	return &NUMAInfo{
		Nodes:         []int{0},
		NUMADistances: NUMADistances{0: []uint64{10}},
	}
}

// Note: intPtr and float64Ptr helper functions are defined in enhanced_topology_test.go