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

func TestTopologyHintNilHandling(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hint           TopologyHint
		expectedPanic  bool
		description    string
	}{
		{
			name:           "All nil fields - feature enabled",
			featureEnabled: true,
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:        nil,
				Distance:         nil,
				Score:            nil,
			},
			expectedPanic: false,
			description:   "Should safely handle all nil enhanced fields",
		},
		{
			name:           "All nil fields - feature disabled",
			featureEnabled: false,
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:        nil,
				Distance:         nil,
				Score:            nil,
			},
			expectedPanic: false,
			description:   "Should safely handle all nil enhanced fields when feature disabled",
		},
		{
			name:           "Partial nil fields - hop count only",
			featureEnabled: true,
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        nil,
				Distance:         nil,
				Score:            nil,
			},
			expectedPanic: false,
			description:   "Should safely handle partial nil enhanced fields",
		},
		{
			name:           "Nil NUMANodeAffinity",
			featureEnabled: true,
			hint: TopologyHint{
				NUMANodeAffinity: nil,
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			expectedPanic: false,
			description:   "Should handle nil NUMANodeAffinity gracefully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			if tc.expectedPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic but didn't get one")
					}
				}()
			}

			// Test getter methods with nil values
			hopCount := tc.hint.GetHopCount()
			bandwidth := tc.hint.GetBandwidth()
			distance := tc.hint.GetDistance()
			score := tc.hint.GetScore()

			// Verify default values are returned for nil fields
			if tc.hint.HopCount == nil && hopCount != 0 {
				t.Errorf("Expected default hop count 0 for nil field, got %d", hopCount)
			}
			if tc.hint.Bandwidth == nil && bandwidth != 0.0 {
				t.Errorf("Expected default bandwidth 0.0 for nil field, got %f", bandwidth)
			}
			if tc.hint.Distance == nil && distance != 10 {
				t.Errorf("Expected default distance 10 for nil field, got %d", distance)
			}
			if tc.hint.Score == nil && score != 0.0 {
				t.Errorf("Expected default score 0.0 for nil field, got %f", score)
			}

			// Test hasEnhancedFields method
			hasEnhanced := tc.hint.hasEnhancedFields()
			expectedHasEnhanced := tc.hint.HopCount != nil || tc.hint.Bandwidth != nil || tc.hint.Distance != nil || tc.hint.Score != nil
			if hasEnhanced != expectedHasEnhanced {
				t.Errorf("hasEnhancedFields() returned %v, expected %v", hasEnhanced, expectedHasEnhanced)
			}

			// Test that the hint structure is valid (TopologyHint doesn't have String method)
			// Just verify we can access the fields without panic
			_ = tc.hint.NUMANodeAffinity

			// Test IsEqual method with nil values
			equalHint := tc.hint
			if !tc.hint.IsEqual(equalHint) {
				t.Errorf("Hint should be equal to itself even with nil fields")
			}
		})
	}
}

func TestTopologyHintSetEnhancedFieldsNilSafety(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hopCount       *int
		bandwidth      *float64
		distance       *int
		score          *float64
		description    string
	}{
		{
			name:           "All nil inputs - feature enabled",
			featureEnabled: true,
			hopCount:       nil,
			bandwidth:      nil,
			distance:       nil,
			score:          nil,
			description:    "Should handle all nil inputs gracefully",
		},
		{
			name:           "All nil inputs - feature disabled",
			featureEnabled: false,
			hopCount:       nil,
			bandwidth:      nil,
			distance:       nil,
			score:          nil,
			description:    "Should handle all nil inputs when feature disabled",
		},
		{
			name:           "Mixed nil and valid inputs",
			featureEnabled: true,
			hopCount:       intPtr(2),
			bandwidth:      nil,
			distance:       intPtr(20),
			score:          nil,
			description:    "Should handle mixed nil and valid inputs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			hint := TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
			}

			// This should not panic even with nil inputs
			hint.SetEnhancedFields(tc.hopCount, tc.bandwidth, tc.distance, tc.score)

			if tc.featureEnabled {
				// When feature is enabled, fields should be set as provided (including nil)
				if hint.HopCount != tc.hopCount {
					t.Errorf("HopCount not set correctly: got %v, expected %v", hint.HopCount, tc.hopCount)
				}
				if hint.Bandwidth != tc.bandwidth {
					t.Errorf("Bandwidth not set correctly: got %v, expected %v", hint.Bandwidth, tc.bandwidth)
				}
				if hint.Distance != tc.distance {
					t.Errorf("Distance not set correctly: got %v, expected %v", hint.Distance, tc.distance)
				}
				if hint.Score != tc.score {
					t.Errorf("Score not set correctly: got %v, expected %v", hint.Score, tc.score)
				}
			} else {
				// When feature is disabled, all fields should remain nil
				if hint.HopCount != nil || hint.Bandwidth != nil || hint.Distance != nil || hint.Score != nil {
					t.Errorf("Enhanced fields should remain nil when feature disabled, got HopCount=%v, Bandwidth=%v, Distance=%v, Score=%v",
						hint.HopCount, hint.Bandwidth, hint.Distance, hint.Score)
				}
			}
		})
	}
}

func TestTopologyHintComparisonWithNilFields(t *testing.T) {
	testCases := []struct {
		name        string
		hint1       TopologyHint
		hint2       TopologyHint
		shouldEqual bool
		description string
	}{
		{
			name: "Both hints have all nil enhanced fields",
			hint1: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:        nil,
				Distance:         nil,
				Score:            nil,
			},
			hint2: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:         nil,
				Distance:         nil,
				Score:            nil,
			},
			shouldEqual: true,
			description: "Hints with matching nil fields should be equal",
		},
		{
			name: "One hint has nil, other has values",
			hint1: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:        nil,
				Distance:         nil,
				Score:            nil,
			},
			hint2: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			shouldEqual: false,
			description: "Hints with different nil/value patterns should not be equal",
		},
		{
			name: "Both hints have same enhanced values",
			hint1: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			hint2: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			shouldEqual: true,
			description: "Hints with matching enhanced values should be equal",
		},
		{
			name: "Partial nil fields - different patterns",
			hint1: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        nil,
				Distance:         intPtr(10),
				Score:            nil,
			},
			hint2: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         nil,
				Bandwidth:        float64Ptr(100.0),
				Distance:         nil,
				Score:            float64Ptr(50.0),
			},
			shouldEqual: false,
			description: "Hints with different partial nil patterns should not be equal",
		},
		{
			name: "Nil NUMANodeAffinity comparison",
			hint1: TopologyHint{
				NUMANodeAffinity: nil,
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			hint2: TopologyHint{
				NUMANodeAffinity: nil,
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(10),
				Score:            float64Ptr(50.0),
			},
			shouldEqual: true,
			description: "Hints with nil NUMANodeAffinity should be comparable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			result := tc.hint1.IsEqual(tc.hint2)
			if result != tc.shouldEqual {
				t.Errorf("IsEqual() returned %v, expected %v for %s", result, tc.shouldEqual, tc.description)
			}

			// Test symmetry
			reverseResult := tc.hint2.IsEqual(tc.hint1)
			if reverseResult != tc.shouldEqual {
				t.Errorf("IsEqual() symmetry failed: hint2.IsEqual(hint1) returned %v, expected %v", reverseResult, tc.shouldEqual)
			}
		})
	}
}

func TestEnhancedHintMergerNilHandling(t *testing.T) {
	testCases := []struct {
		name         string
		providersHints []map[string][]TopologyHint
		expectPanic  bool
		description  string
	}{
		{
			name: "Empty providers hints",
			providersHints: []map[string][]TopologyHint{},
			expectPanic: false,
			description: "Should handle empty providers gracefully",
		},
		{
			name: "Providers with nil hints",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {},
				},
			},
			expectPanic: false,
			description: "Should handle empty hint slices gracefully",
		},
		{
			name: "Providers with hints containing nil enhanced fields",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         nil,
							Bandwidth:        nil,
							Distance:         nil,
							Score:            nil,
						},
					},
				},
			},
			expectPanic: false,
			description: "Should handle hints with nil enhanced fields gracefully",
		},
		{
			name: "Mixed hints with some nil and some populated enhanced fields",
			providersHints: []map[string][]TopologyHint{
				{
					"resource1": {
						{
							NUMANodeAffinity: NewTestBitMask(0),
							Preferred:        true,
							HopCount:         intPtr(1),
							Bandwidth:        nil,
							Distance:         intPtr(10),
							Score:            nil,
						},
					},
				},
				{
					"resource2": {
						{
							NUMANodeAffinity: NewTestBitMask(1),
							Preferred:        true,
							HopCount:         nil,
							Bandwidth:        float64Ptr(100.0),
							Distance:         nil,
							Score:            float64Ptr(50.0),
						},
					},
				},
			},
			expectPanic: false,
			description: "Should handle mixed hint patterns gracefully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic but didn't get one")
					}
				}()
			}

			numaInfo := commonNUMAInfoTwoNodes()
			merger := NewEnhancedHintMerger(numaInfo, numaInfo.DefaultAffinityMask(), tc.providersHints)

			// This should not panic
			hint := merger.Merge()

			// Verify the result is reasonable
			if hint.NUMANodeAffinity == nil {
				t.Errorf("Merged hint should have valid NUMANodeAffinity")
			}

			// Test that getter methods work on merged hint
			_ = hint.GetHopCount()
			_ = hint.GetBandwidth()
			_ = hint.GetDistance()
			_ = hint.GetScore()
		})
	}
}

func TestCalculateTopologyScoreNilSafety(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hopCount       int
		bandwidth      float64
		distance       int
		dataSize       int64
		description    string
	}{
		{
			name:           "Zero values - feature enabled",
			featureEnabled: true,
			hopCount:       0,
			bandwidth:      0.0,
			distance:       0,
			dataSize:       0,
			description:    "Should handle zero values gracefully",
		},
		{
			name:           "Negative values - feature enabled",
			featureEnabled: true,
			hopCount:       -1,
			bandwidth:      -100.0,
			distance:       -10,
			dataSize:       -1024,
			description:    "Should handle negative values gracefully",
		},
		{
			name:           "Feature disabled",
			featureEnabled: false,
			hopCount:       1,
			bandwidth:      100.0,
			distance:       10,
			dataSize:       1024,
			description:    "Should return 0 when feature disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			// This should not panic with any input values
			score := CalculateTopologyScore(tc.hopCount, tc.bandwidth, tc.distance, tc.dataSize)

			if tc.featureEnabled {
				// When feature is enabled, score should be calculated (may be any value)
				_ = score // Just verify it doesn't panic
			} else {
				// When feature is disabled, score should be 0
				if score != 0.0 {
					t.Errorf("Expected score 0.0 when feature disabled, got %f", score)
				}
			}
		})
	}
}

// Note: intPtr and float64Ptr helper functions are defined in enhanced_topology_test.go