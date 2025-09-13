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

func TestFeatureGateToggleEdgeCases(t *testing.T) {
	testCases := []struct {
		name                string
		initialFeatureState bool
		toggleFeatureState  bool
		providersHints      []map[string][]TopologyHint
		policyName          string
		expectConsistency   bool
		description         string
	}{
		{
			name:                "Feature enabled to disabled - should fall back gracefully",
			initialFeatureState: true,
			toggleFeatureState:  false,
			providersHints: []map[string][]TopologyHint{
				{
					"resource": {
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
			policyName:        PolicyBestEffort,
			expectConsistency: true,
			description:       "Should maintain consistency when feature is disabled after being enabled",
		},
		{
			name:                "Feature disabled to enabled - should enhance gracefully",
			initialFeatureState: false,
			toggleFeatureState:  true,
			providersHints: []map[string][]TopologyHint{
				{
					"resource": {
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
			policyName:        PolicyBestEffort,
			expectConsistency: true,
			description:       "Should start using enhanced features when feature is enabled",
		},
		{
			name:                "Multiple toggles - should remain stable",
			initialFeatureState: true,
			toggleFeatureState:  false, // Will toggle multiple times in test
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
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        float64Ptr(80.0),
							Distance:         intPtr(20),
							Score:            float64Ptr(85.0),
						},
					},
				},
			},
			policyName:        PolicyDistributed,
			expectConsistency: true,
			description:       "Should handle multiple feature toggles without issues",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			numaInfo := commonNUMAInfoTwoNodes()

			// Set initial feature state
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.initialFeatureState)

			// Create policy
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

			// Get initial result
			initialHint, initialAdmit := policy.Merge(tc.providersHints)

			// Toggle feature state
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.toggleFeatureState)

			// Get result after toggle
			toggledHint, toggledAdmit := policy.Merge(tc.providersHints)

			// Verify consistency
			if tc.expectConsistency {
				// Admission should remain consistent
				if initialAdmit != toggledAdmit {
					t.Errorf("Admission changed after feature toggle: initial=%v, toggled=%v", initialAdmit, toggledAdmit)
				}

				// NUMA affinity should remain consistent for admitted pods
				if initialAdmit && toggledAdmit {
					if initialHint.NUMANodeAffinity == nil || toggledHint.NUMANodeAffinity == nil {
						t.Errorf("NUMA affinity should not be nil for admitted pods")
					} else if !initialHint.NUMANodeAffinity.IsEqual(toggledHint.NUMANodeAffinity) {
						t.Errorf("NUMA affinity changed after feature toggle")
					}
				}
			}

			// Additional test for multiple toggles
			if tc.name == "Multiple toggles - should remain stable" {
				// Toggle back and forth multiple times
				for i := 0; i < 3; i++ {
					featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
					hint1, admit1 := policy.Merge(tc.providersHints)

					featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)
					hint2, admit2 := policy.Merge(tc.providersHints)

					// Should remain consistent
					if admit1 != admit2 {
						t.Errorf("Multiple toggle %d: admission inconsistent", i)
					}
					if admit1 && admit2 && !hint1.NUMANodeAffinity.IsEqual(hint2.NUMANodeAffinity) {
						t.Errorf("Multiple toggle %d: NUMA affinity inconsistent", i)
					}
				}
			}
		})
	}
}

func TestFeatureGateWithInvalidEnhancedData(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hints          []TopologyHint
		expectPanic    bool
		description    string
	}{
		{
			name:           "Nil enhanced fields with feature enabled",
			featureEnabled: true,
			hints: []TopologyHint{
				{
					NUMANodeAffinity: NewTestBitMask(0),
					Preferred:        true,
					HopCount:         nil,
					Bandwidth:        nil,
					Distance:         nil,
					Score:            nil,
				},
			},
			expectPanic: false,
			description: "Should handle nil enhanced fields gracefully",
		},
		{
			name:           "Extreme values with feature enabled",
			featureEnabled: true,
			hints: []TopologyHint{
				{
					NUMANodeAffinity: NewTestBitMask(0),
					Preferred:        true,
					HopCount:         intPtr(999999),
					Bandwidth:        float64Ptr(1e20),
					Distance:         intPtr(-1000),
					Score:            float64Ptr(-999.0),
				},
			},
			expectPanic: false,
			description: "Should handle extreme values without panic",
		},
		{
			name:           "Enhanced fields with feature disabled",
			featureEnabled: false,
			hints: []TopologyHint{
				{
					NUMANodeAffinity: NewTestBitMask(0),
					Preferred:        true,
					HopCount:         intPtr(1),
					Bandwidth:        float64Ptr(100.0),
					Distance:         intPtr(10),
					Score:            float64Ptr(95.0),
				},
			},
			expectPanic: false,
			description: "Should ignore enhanced fields when feature disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			numaInfo := commonNUMAInfoTwoNodes()
			policy := NewBestEffortPolicy(numaInfo, PolicyOptions{})

			providersHints := []map[string][]TopologyHint{
				{"resource": tc.hints},
			}

			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic but didn't get one")
					}
				}()
			}

			hint, admit := policy.Merge(providersHints)

			if !tc.expectPanic {
				// Verify basic properties
				if hint.NUMANodeAffinity == nil && admit {
					t.Errorf("Admitted hint should have valid NUMA affinity")
				}

				// When feature disabled, enhanced fields should not affect result
				if !tc.featureEnabled {
					hasEnhanced := hint.hasEnhancedFields()
					if hasEnhanced {
						t.Errorf("Should not have enhanced fields when feature disabled")
					}
				}
			}
		})
	}
}

func TestFeatureGateVersionCompatibility(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hints          []TopologyHint
		validateOldAPI bool
		description    string
	}{
		{
			name:           "Backward compatibility with old hints",
			featureEnabled: true,
			hints: []TopologyHint{
				{
					NUMANodeAffinity: NewTestBitMask(0),
					Preferred:        true,
					// No enhanced fields - simulates old hint provider
				},
			},
			validateOldAPI: true,
			description:    "Should work with old-style hints even when feature enabled",
		},
		{
			name:           "Forward compatibility preparation",
			featureEnabled: false,
			hints: []TopologyHint{
				{
					NUMANodeAffinity: NewTestBitMask(0),
					Preferred:        true,
					HopCount:         intPtr(1),
					Bandwidth:        float64Ptr(100.0),
					Distance:         intPtr(10),
					Score:            float64Ptr(95.0),
				},
			},
			validateOldAPI: false,
			description:    "Should ignore enhanced fields when feature disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			numaInfo := commonNUMAInfoTwoNodes()
			policy := NewBestEffortPolicy(numaInfo, PolicyOptions{})

			providersHints := []map[string][]TopologyHint{
				{"resource": tc.hints},
			}

			hint, admit := policy.Merge(providersHints)

			if !admit {
				t.Errorf("Should admit compatible hints")
			}

			if tc.validateOldAPI {
				// Old API behavior: only basic fields matter
				if hint.NUMANodeAffinity == nil {
					t.Errorf("Should have valid NUMA affinity")
				}
				// Should not rely on enhanced features for basic functionality
			}

			// Test that enhanced features are properly gated
			if tc.featureEnabled {
				// May or may not have enhanced fields depending on input
			} else {
				// Should not produce enhanced output when feature disabled
				hasEnhanced := hint.hasEnhancedFields()
				if hasEnhanced {
					t.Errorf("Should not have enhanced fields when feature disabled")
				}
			}
		})
	}
}

func TestFeatureGateWithDifferentPolicies(t *testing.T) {
	policies := []string{
		PolicyBestEffort,
		PolicyRestricted,
		PolicySingleNumaNode,
		PolicyDistributed,
	}

	providersHints := []map[string][]TopologyHint{
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
					NUMANodeAffinity: NewTestBitMask(1),
					Preferred:        true,
					HopCount:         intPtr(1),
					Bandwidth:        float64Ptr(80.0),
					Distance:         intPtr(20),
					Score:            float64Ptr(85.0),
				},
			},
		},
	}

	numaInfo := commonNUMAInfoTwoNodes()

	for _, policyName := range policies {
		t.Run("Policy_"+policyName, func(t *testing.T) {
			// Test with feature enabled
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			var policy Policy
			switch policyName {
			case PolicyBestEffort:
				policy = NewBestEffortPolicy(numaInfo, PolicyOptions{})
			case PolicyRestricted:
				policy = NewRestrictedPolicy(numaInfo, PolicyOptions{})
			case PolicySingleNumaNode:
				policy = NewSingleNumaNodePolicy(numaInfo, PolicyOptions{})
			case PolicyDistributed:
				policy = NewDistributedPolicy(numaInfo, PolicyOptions{})
			default:
				t.Fatalf("Unknown policy: %s", policyName)
			}

			hintEnabled, admitEnabled := policy.Merge(providersHints)

			// Test with feature disabled
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

			hintDisabled, admitDisabled := policy.Merge(providersHints)

			// All policies should maintain basic functionality
			if !admitEnabled && !admitDisabled {
				// If neither admits, that's policy-specific behavior (e.g., restricted)
				return
			}

			// If one admits, both should admit (feature should not affect admission for valid cases)
			if admitEnabled != admitDisabled && policyName != PolicyRestricted {
				t.Errorf("Policy %s: admission differs between feature states: enabled=%v, disabled=%v", 
					policyName, admitEnabled, admitDisabled)
			}

			// Basic NUMA affinity should be consistent
			if admitEnabled && admitDisabled {
				if hintEnabled.NUMANodeAffinity == nil || hintDisabled.NUMANodeAffinity == nil {
					t.Errorf("Policy %s: NUMA affinity should not be nil", policyName)
				}
			}

			// Enhanced fields should only be present when feature enabled
			if admitEnabled && hintEnabled.hasEnhancedFields() {
				// This is expected when feature enabled and hints have enhanced data
			}
			if admitDisabled && hintDisabled.hasEnhancedFields() {
				t.Errorf("Policy %s: should not have enhanced fields when feature disabled", policyName)
			}
		})
	}
}

// Note: intPtr and float64Ptr helper functions are defined in enhanced_topology_test.go