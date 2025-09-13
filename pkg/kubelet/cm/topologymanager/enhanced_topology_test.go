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

func TestEnhancedTopologyHintsFeatureGate(t *testing.T) {
	// Test with the feature gate enabled (since it's Alpha, we can enable it)
	t.Run("Feature gate enabled", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)

		result := EnhancedTopologyHintsEnabled()
		if !result {
			t.Errorf("Expected EnhancedTopologyHintsEnabled() to return true when feature gate is enabled, got false")
		}
	})

	// Test the default state (feature gate disabled by default for Alpha)
	t.Run("Feature gate default state", func(t *testing.T) {
		// Don't set the feature gate - test the default behavior
		result := EnhancedTopologyHintsEnabled()
		// Alpha features are disabled by default
		if result {
			t.Errorf("Expected EnhancedTopologyHintsEnabled() to return false by default for Alpha feature, got true")
		}
	})
}

func TestTopologyHintSetEnhancedFields(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hopCount       *int
		bandwidth      *float64
		distance       *int
		score          *float64
		expectedNil    bool
	}{
		{
			name:           "Feature enabled - fields should be set",
			featureEnabled: true,
			hopCount:       intPtr(1),
			bandwidth:      float64Ptr(100.0),
			distance:       intPtr(20),
			score:          float64Ptr(50.0),
			expectedNil:    false,
		},
		{
			name:           "Feature disabled - fields should remain nil",
			featureEnabled: false,
			hopCount:       intPtr(1),
			bandwidth:      float64Ptr(100.0),
			distance:       intPtr(20),
			score:          float64Ptr(50.0),
			expectedNil:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			hint := TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
			}

			hint.SetEnhancedFields(tc.hopCount, tc.bandwidth, tc.distance, tc.score)

			if tc.expectedNil {
				if hint.HopCount != nil || hint.Bandwidth != nil || hint.Distance != nil || hint.Score != nil {
					t.Errorf("Expected enhanced fields to be nil when feature gate disabled, got HopCount=%v, Bandwidth=%v, Distance=%v, Score=%v",
						hint.HopCount, hint.Bandwidth, hint.Distance, hint.Score)
				}
			} else {
				if hint.HopCount == nil || hint.Bandwidth == nil || hint.Distance == nil || hint.Score == nil {
					t.Errorf("Expected enhanced fields to be set when feature gate enabled, got HopCount=%v, Bandwidth=%v, Distance=%v, Score=%v",
						hint.HopCount, hint.Bandwidth, hint.Distance, hint.Score)
				}
				if *hint.HopCount != *tc.hopCount {
					t.Errorf("Expected HopCount=%v, got %v", *tc.hopCount, *hint.HopCount)
				}
				if *hint.Bandwidth != *tc.bandwidth {
					t.Errorf("Expected Bandwidth=%v, got %v", *tc.bandwidth, *hint.Bandwidth)
				}
				if *hint.Distance != *tc.distance {
					t.Errorf("Expected Distance=%v, got %v", *tc.distance, *hint.Distance)
				}
				if *hint.Score != *tc.score {
					t.Errorf("Expected Score=%v, got %v", *tc.score, *hint.Score)
				}
			}
		})
	}
}

func TestTopologyHintGetMethods(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hopCount       *int
		bandwidth      *float64
		distance       *int
		score          *float64
		expectedHops   int
		expectedBW     float64
		expectedDist   int
		expectedScore  float64
	}{
		{
			name:           "Feature enabled with values",
			featureEnabled: true,
			hopCount:       intPtr(2),
			bandwidth:      float64Ptr(150.0),
			distance:       intPtr(30),
			score:          float64Ptr(75.0),
			expectedHops:   2,
			expectedBW:     150.0,
			expectedDist:   30,
			expectedScore:  75.0,
		},
		{
			name:           "Feature disabled - should return defaults",
			featureEnabled: false,
			hopCount:       intPtr(2),
			bandwidth:      float64Ptr(150.0),
			distance:       intPtr(30),
			score:          float64Ptr(75.0),
			expectedHops:   0,
			expectedBW:     0.0,
			expectedDist:   10,
			expectedScore:  0.0,
		},
		{
			name:           "Feature enabled with nil values",
			featureEnabled: true,
			hopCount:       nil,
			bandwidth:      nil,
			distance:       nil,
			score:          nil,
			expectedHops:   0,
			expectedBW:     0.0,
			expectedDist:   10,
			expectedScore:  0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			hint := TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         tc.hopCount,
				Bandwidth:        tc.bandwidth,
				Distance:         tc.distance,
				Score:            tc.score,
			}

			if hint.GetHopCount() != tc.expectedHops {
				t.Errorf("Expected GetHopCount()=%v, got %v", tc.expectedHops, hint.GetHopCount())
			}
			if hint.GetBandwidth() != tc.expectedBW {
				t.Errorf("Expected GetBandwidth()=%v, got %v", tc.expectedBW, hint.GetBandwidth())
			}
			if hint.GetDistance() != tc.expectedDist {
				t.Errorf("Expected GetDistance()=%v, got %v", tc.expectedDist, hint.GetDistance())
			}
			if hint.GetScore() != tc.expectedScore {
				t.Errorf("Expected GetScore()=%v, got %v", tc.expectedScore, hint.GetScore())
			}
		})
	}
}

func TestTopologyHintHasEnhancedFields(t *testing.T) {
	testCases := []struct {
		name     string
		hint     TopologyHint
		expected bool
	}{
		{
			name: "No enhanced fields",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
			},
			expected: false,
		},
		{
			name: "Has hop count only",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
			},
			expected: true,
		},
		{
			name: "Has bandwidth only",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				Bandwidth:        float64Ptr(100.0),
			},
			expected: true,
		},
		{
			name: "Has distance only",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				Distance:         intPtr(20),
			},
			expected: true,
		},
		{
			name: "Has score only",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				Score:            float64Ptr(50.0),
			},
			expected: true,
		},
		{
			name: "Has all enhanced fields",
			hint: TopologyHint{
				NUMANodeAffinity: NewTestBitMask(0),
				Preferred:        true,
				HopCount:         intPtr(1),
				Bandwidth:        float64Ptr(100.0),
				Distance:         intPtr(20),
				Score:            float64Ptr(50.0),
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.hint.hasEnhancedFields()
			if result != tc.expected {
				t.Errorf("Expected hasEnhancedFields()=%v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCalculateTopologyScore(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		hopCount       int
		bandwidth      float64
		distance       int
		dataSize       int64
		expectedScore  float64
	}{
		{
			name:           "Feature enabled - local access",
			featureEnabled: true,
			hopCount:       0,
			bandwidth:      100.0,
			distance:       10,
			dataSize:       1024 * 1024, // 1MB
			expectedScore:  10.0 + (0.0 * 10.0) + 0.0 + (float64(1024*1024) / (100.0 * 1024 * 1024 * 1024)),
		},
		{
			name:           "Feature enabled - 1-hop access",
			featureEnabled: true,
			hopCount:       1,
			bandwidth:      50.0,
			distance:       20,
			dataSize:       2 * 1024 * 1024, // 2MB
			expectedScore:  10.0 + (1.0 * 10.0) + 10.0 + (float64(2*1024*1024) / (50.0 * 1024 * 1024 * 1024)),
		},
		{
			name:           "Feature disabled - should return 0",
			featureEnabled: false,
			hopCount:       1,
			bandwidth:      50.0,
			distance:       20,
			dataSize:       1024 * 1024,
			expectedScore:  0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			score := CalculateTopologyScore(tc.hopCount, tc.bandwidth, tc.distance, tc.dataSize)
			if score != tc.expectedScore {
				t.Errorf("Expected CalculateTopologyScore()=%v, got %v", tc.expectedScore, score)
			}
		})
	}
}

func TestCreateEnhancedHint(t *testing.T) {
	testCases := []struct {
		name           string
		featureEnabled bool
		expectEnhanced bool
	}{
		{
			name:           "Feature enabled",
			featureEnabled: true,
			expectEnhanced: true,
		},
		{
			name:           "Feature disabled",
			featureEnabled: false,
			expectEnhanced: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, tc.featureEnabled)

			numaAffinity := NewTestBitMask(0)
			hint := CreateEnhancedHint(numaAffinity, true, 1, 100.0, 20, 1024*1024)

			if hint.NUMANodeAffinity.IsEqual(numaAffinity) == false {
				t.Errorf("Expected NUMANodeAffinity to be set correctly")
			}
			if hint.Preferred != true {
				t.Errorf("Expected Preferred to be true")
			}

			hasEnhanced := hint.hasEnhancedFields()
			if hasEnhanced != tc.expectEnhanced {
				t.Errorf("Expected enhanced fields presence to be %v, got %v", tc.expectEnhanced, hasEnhanced)
			}
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}