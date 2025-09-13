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
	"fmt"
	"testing"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

func TestEnhancedHintMergingIntegration(t *testing.T) {
	testCases := []struct {
		name              string
		featureEnabled    bool
		policyName        string
		providersHints    []map[string][]TopologyHint
		expectedAdmit     bool
		expectedEnhanced  bool
		expectedSpread    bool // For distributed policy
		description       string
	}{
		{
			name:           "Best-effort policy with enhanced hints - feature enabled",
			featureEnabled: true,
			policyName:     PolicyBestEffort,
			providersHints: []map[string][]TopologyHint{
				{
					"cpu": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
					},
				},
				{
					"memory": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(80.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(85.0),
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: true,
			expectedSpread:   false,
			description:      "Should merge enhanced hints from multiple providers",
		},
		{
			name:           "Best-effort policy with enhanced hints - feature disabled",
			featureEnabled: false,
			policyName:     PolicyBestEffort,
			providersHints: []map[string][]TopologyHint{
				{
					"cpu": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: false,
			expectedSpread:   false,
			description:      "Should fall back to basic merging when feature disabled",
		},
		{
			name:           "Single-NUMA policy with enhanced hints",
			featureEnabled: true,
			policyName:     PolicySingleNumaNode,
			providersHints: []map[string][]TopologyHint{
				{
					"device": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(200.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(98.0),
						},
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(150.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(75.0),
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: true,
			expectedSpread:   false,
			description:      "Should select best single-NUMA hint with enhanced scoring",
		},
		{
			name:           "Distributed policy with multiple resource types",
			featureEnabled: true,
			policyName:     PolicyDistributed,
			providersHints: []map[string][]TopologyHint{
				{
					"cpu": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
					},
				},
				{
					"device": {
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(80.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(80.0),
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: true,
			expectedSpread:   true,
			description:      "Should distribute resources across NUMA nodes",
		},
		{
			name:           "Restricted policy with conflicting hints",
			featureEnabled: true,
			policyName:     PolicyRestricted,
			providersHints: []map[string][]TopologyHint{
				{
					"cpu": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
					},
				},
				{
					"device": {
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(80.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(80.0),
						},
					},
				},
			},
			expectedAdmit:    false,
			expectedEnhanced: false,
			expectedSpread:   false,
			description:      "Should reject when no single NUMA node satisfies all requirements",
		},
		{
			name:           "Mixed enhanced and basic hints",
			featureEnabled: true,
			policyName:     PolicyBestEffort,
			providersHints: []map[string][]TopologyHint{
				{
					"enhanced-resource": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(0),
							Bandwidth:        float64Ptr(100.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
					},
				},
				{
					"basic-resource": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							// No enhanced fields
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: true, // Should still be enhanced due to one provider having enhanced fields
			expectedSpread:   false,
			description:      "Should handle mixed enhanced and basic hints gracefully",
		},
		{
			name:           "Multi-NUMA spanning hints",
			featureEnabled: true,
			policyName:     PolicyBestEffort,
			providersHints: []map[string][]TopologyHint{
				{
					"large-resource": {
						{
							NUMANodeAffinity: NewTestBitMask(0, 1),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(150.0),
							Distance:         intPtr(15),
							Score:            float64Ptr(85.0),
						},
					},
				},
			},
			expectedAdmit:    true,
			expectedEnhanced: true,
			expectedSpread:   false,
			description:      "Should handle hints that span multiple NUMA nodes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			numaInfo := commonNUMAInfoTwoNodes()
			var policy Policy

			switch tc.policyName {
			case PolicyBestEffort:
				policy = NewBestEffortPolicy(numaInfo, PolicyOptions{})
			case PolicyRestricted:
				policy = NewRestrictedPolicy(numaInfo, PolicyOptions{})
			case PolicySingleNumaNode:
				policy = NewSingleNumaNodePolicy(numaInfo, PolicyOptions{})
			case PolicyDistributed:
				policy = NewDistributedPolicy(numaInfo, PolicyOptions{})
			default:
				t.Fatalf("Unknown policy: %s", tc.policyName)
			}

			hint, admit := policy.Merge(tc.providersHints)

			if admit != tc.expectedAdmit {
				t.Errorf("Expected admit=%v, got %v", tc.expectedAdmit, admit)
			}

			if !admit {
				// Skip further checks if not admitted
				return
			}

			// Check enhanced fields presence
			hasEnhanced := hint.hasEnhancedFields()
			if tc.featureEnabled && tc.expectedEnhanced && !hasEnhanced {
				t.Errorf("Expected enhanced fields when feature enabled and enhanced hints provided")
			}
			if !tc.featureEnabled && hasEnhanced {
				t.Errorf("Should not have enhanced fields when feature disabled")
			}

			// Check NUMA node spread for distributed policy
			if tc.expectedSpread && hint.NUMANodeAffinity != nil {
				if hint.NUMANodeAffinity.Count() <= 1 {
					t.Errorf("Expected hint to span multiple NUMA nodes for distributed policy, got count=%d", hint.NUMANodeAffinity.Count())
				}
			}

			// Verify hint validity
			if hint.NUMANodeAffinity == nil {
				t.Errorf("Result hint should have valid NUMANodeAffinity")
			}

			// For enhanced hints, verify score calculation is reasonable
			if tc.featureEnabled && hasEnhanced {
				score := hint.GetScore()
				if score < 0 || score > 100 {
					t.Errorf("Enhanced hint score should be in range [0,100], got %f", score)
				}
			}
		})
	}
}

func TestEnhancedHintMergingPerformanceEdgeCases(t *testing.T) {
	testCases := []struct {
		name           string
		providersCount int
		hintsPerProvider int
		description    string
	}{
		{
			name:             "Many providers with few hints each",
			providersCount:   10,
			hintsPerProvider: 2,
			description:      "Should handle many providers efficiently",
		},
		{
			name:             "Few providers with many hints each",
			providersCount:   2,
			hintsPerProvider: 10,
			description:      "Should handle many hints per provider efficiently",
		},
		{
			name:             "Moderate load",
			providersCount:   5,
			hintsPerProvider: 5,
			description:      "Should handle moderate provider/hint combinations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			numaInfo := commonNUMAInfoTwoNodes()
			policy := NewBestEffortPolicy(numaInfo, PolicyOptions{})

			// Generate test hints
			providersHints := make([]map[string][]TopologyHint, tc.providersCount)
			for i := 0; i < tc.providersCount; i++ {
				hints := make([]TopologyHint, tc.hintsPerProvider)
				for j := 0; j < tc.hintsPerProvider; j++ {
					nodeID := j % 2 // Alternate between nodes 0 and 1
					hints[j] = TopologyHint{
						NUMANodeAffinity: NewTestBitMask(nodeID),
						Preferred:        true,
						HopCount:         intPtr(nodeID),
						Bandwidth:        float64Ptr(100.0 - float64(j*10)),
						Distance:         intPtr(10 + nodeID*10),
						Score:            float64Ptr(90.0 - float64(j*5)),
					}
				}
				providersHints[i] = map[string][]TopologyHint{
					fmt.Sprintf("resource-%d", i): hints,
				}
			}

			// This should complete without timeout or excessive resource usage
			hint, admit := policy.Merge(providersHints)

			if !admit {
				t.Errorf("Should admit valid hint combinations")
			}

			if hint.NUMANodeAffinity == nil {
				t.Errorf("Should produce valid result hint")
			}

			// Verify enhanced fields are present and reasonable
			if !hint.hasEnhancedFields() {
				t.Errorf("Should have enhanced fields for enhanced hints")
			}

			score := hint.GetScore()
			if score < 0 || score > 100 {
				t.Errorf("Score should be in valid range, got %f", score)
			}
		})
	}
}

func TestEnhancedHintMergingWithNUMADistances(t *testing.T) {
	// Create NUMA info with realistic distance matrix
	numaInfo := &NUMAInfo{
		Nodes: []int{0, 1, 2, 3},
		NUMADistances: NUMADistances{
			0: []uint64{10, 20, 30, 40},
			1: []uint64{20, 10, 40, 30},
			2: []uint64{30, 40, 10, 20},
			3: []uint64{40, 30, 20, 10},
		},
	}

	testCases := []struct {
		name           string
		providersHints []map[string][]TopologyHint
		expectedNodes  []int
		description    string
	}{
		{
			name: "Close NUMA nodes should be preferred",
			providersHints: []map[string][]TopologyHint{
				{
					"cpu": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							Distance:         intPtr(10),
							Score:            float64Ptr(95.0),
						},
						{
							NUMANodeAffinity: NewTestBitMask(2),
							Preferred:        true,
							Distance:         intPtr(30),
							Score:            float64Ptr(75.0),
						},
					},
				},
			},
			expectedNodes: []int{0}, // Closer node should win
			description:   "Should prefer hints with lower NUMA distances",
		},
		{
			name: "Bandwidth vs distance tradeoff",
			providersHints: []map[string][]TopologyHint{
				{
					"device": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							Bandwidth:        float64Ptr(200.0),
							Distance:         intPtr(10),
							Score:            float64Ptr(90.0),
						},
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							Bandwidth:        float64Ptr(300.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(85.0),
						},
					},
				},
			},
			expectedNodes: []int{0}, // Higher score should win despite lower bandwidth
			description:   "Should consider overall score including distance penalty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			policy := NewBestEffortPolicy(numaInfo, PolicyOptions{})
			hint, admit := policy.Merge(tc.providersHints)

			if !admit {
				t.Errorf("Should admit valid hints")
			}

			if hint.NUMANodeAffinity == nil {
				t.Errorf("Should have valid NUMA affinity")
			}

			// Check if the result matches expected nodes
			for _, expectedNode := range tc.expectedNodes {
				if !hint.NUMANodeAffinity.IsSet(expectedNode) {
					t.Errorf("Expected node %d to be set in result affinity", expectedNode)
				}
			}
		})
	}
}

// Note: intPtr, float64Ptr helper functions are defined in enhanced_topology_test.go