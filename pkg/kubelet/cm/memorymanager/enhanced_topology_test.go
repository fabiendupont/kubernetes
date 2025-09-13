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

package memorymanager

import (
	"testing"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
)

func TestMemoryManagerEnhancedTopologyHints(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	testCases := []struct {
		name                  string
		memoryQuantity        string
		numaNodes            []int
		expectedHopCount     int
		expectedMinDistance  int
		expectedMinBandwidth float64
	}{
		{
			name:                  "Single NUMA node",
			memoryQuantity:        "1Gi",
			numaNodes:            []int{0},
			expectedHopCount:     0,
			expectedMinDistance:  10,
			expectedMinBandwidth: 90.0, // 100 * (1 - 0 * 0.3)
		},
		{
			name:                  "Two NUMA nodes",
			memoryQuantity:        "2Gi", 
			numaNodes:            []int{0, 1},
			expectedHopCount:     1,
			expectedMinDistance:  25, // 10 + 1 * 15
			expectedMinBandwidth: 70.0, // 100 * (1 - 1 * 0.3)
		},
		{
			name:                  "Three NUMA nodes",
			memoryQuantity:        "3Gi",
			numaNodes:            []int{0, 1, 2},
			expectedHopCount:     2,
			expectedMinDistance:  40, // 10 + 2 * 15
			expectedMinBandwidth: 40.0, // 100 * (1 - 2 * 0.3)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &staticPolicy{
				machineInfo: &cadvisorapi.MachineInfo{
					MachineID:     "test-machine",
					NumCores:      4,
					MemoryCapacity: 8 * 1024 * 1024 * 1024, // 8GB
				},
				initContainersReusableMemory: make(reusableMemory),
				systemReserved:               make(systemReservedMemory),
			}

			// Create machine state with test NUMA nodes
			machineState := state.NUMANodeMap{}
			for _, nodeID := range tc.numaNodes {
				machineState[nodeID] = &state.NUMANodeState{
					NumberOfAssignments: 0,
					Cells:               []int{nodeID},
					MemoryMap: map[v1.ResourceName]*state.MemoryTable{
						v1.ResourceMemory: {
							Allocatable: 2 * 1024 * 1024 * 1024, // 2GB per node
							Free:        2 * 1024 * 1024 * 1024, // 2GB free per node
						},
					},
				}
			}

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-uid",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "test-container",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse(tc.memoryQuantity),
								},
							},
						},
					},
				},
				Status: v1.PodStatus{
					QOSClass: v1.PodQOSGuaranteed,
				},
			}

			memoryQuantity := resource.MustParse(tc.memoryQuantity)
			requestedResources := map[v1.ResourceName]uint64{
				v1.ResourceMemory: uint64(memoryQuantity.Value()),
			}

			hints := policy.calculateHints(machineState, pod, requestedResources)

			// Verify we got hints for memory
			memoryHints, ok := hints[string(v1.ResourceMemory)]
			if !ok {
				t.Fatalf("No hints generated for memory resource")
			}

			// Find the hint that matches our expected NUMA nodes
			var targetHint *topologymanager.TopologyHint
			for _, hint := range memoryHints {
				if hint.NUMANodeAffinity != nil {
					hintNodes := hint.NUMANodeAffinity.GetBits()
					if len(hintNodes) == len(tc.numaNodes) {
						match := true
						for i, nodeID := range tc.numaNodes {
							if hintNodes[i] != nodeID {
								match = false
								break
							}
						}
						if match {
							targetHint = &hint
							break
						}
					}
				}
			}

			if targetHint == nil {
				t.Fatalf("Could not find hint matching expected NUMA nodes %v", tc.numaNodes)
			}

			// Verify enhanced topology fields are set correctly
			if !topologymanager.EnhancedTopologyHintsEnabled() {
				t.Fatalf("EnhancedTopologyHints feature gate should be enabled")
			}

			if targetHint.GetHopCount() == -1 {
				t.Errorf("Expected enhanced fields to be set")
			}

			if targetHint.GetHopCount() != tc.expectedHopCount {
				t.Errorf("Expected hop count %d, got %d", tc.expectedHopCount, targetHint.GetHopCount())
			}

			if targetHint.GetDistance() != tc.expectedMinDistance {
				t.Errorf("Expected distance %d, got %d", tc.expectedMinDistance, targetHint.GetDistance())
			}

			if targetHint.GetBandwidth() < tc.expectedMinBandwidth {
				t.Errorf("Expected bandwidth >= %f, got %f", tc.expectedMinBandwidth, targetHint.GetBandwidth())
			}

			// Verify score is calculated (should be > 0 for multi-NUMA)
			score := targetHint.GetScore()
			if tc.expectedHopCount > 0 && score <= 0 {
				t.Errorf("Expected score > 0 for multi-NUMA hint, got %f", score)
			}
		})
	}
}

func TestMemoryManagerEnhancedTopologyHintsDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	policy := &staticPolicy{
		machineInfo: &cadvisorapi.MachineInfo{
			MachineID:     "test-machine",
			NumCores:      4,
			MemoryCapacity: 8 * 1024 * 1024 * 1024,
		},
		initContainersReusableMemory: make(reusableMemory),
		systemReserved:               make(systemReservedMemory),
	}

	// Create simple machine state
	machineState := state.NUMANodeMap{
		0: &state.NUMANodeState{
			NumberOfAssignments: 0,
			Cells:               []int{0},
			MemoryMap: map[v1.ResourceName]*state.MemoryTable{
				v1.ResourceMemory: {
					Allocatable: 2 * 1024 * 1024 * 1024,
					Free:        2 * 1024 * 1024 * 1024,
				},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "test-container",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			QOSClass: v1.PodQOSGuaranteed,
		},
	}

	memoryQuantity := resource.MustParse("1Gi")
	requestedResources := map[v1.ResourceName]uint64{
		v1.ResourceMemory: uint64(memoryQuantity.Value()),
	}

	hints := policy.calculateHints(machineState, pod, requestedResources)

	// Verify we got hints for memory
	memoryHints, ok := hints[string(v1.ResourceMemory)]
	if !ok {
		t.Fatalf("No hints generated for memory resource")
	}

	// Verify enhanced fields are NOT set when feature is disabled
	for _, hint := range memoryHints {
		// When feature is disabled, GetHopCount returns 0, GetBandwidth returns 0.0, 
		// GetDistance returns 10, GetScore returns 0.0
		hopCount := hint.GetHopCount()
		bandwidth := hint.GetBandwidth()
		distance := hint.GetDistance()
		score := hint.GetScore()
		
		if !(hopCount == 0 && bandwidth == 0.0 && distance == 10 && score == 0.0) {
			t.Errorf("Expected default values when feature gate disabled, got hopCount=%d, bandwidth=%f, distance=%d, score=%f", 
				hopCount, bandwidth, distance, score)
		}
	}
}

func TestMemoryManagerCalculateEnhancedTopologyFields(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	policy := &staticPolicy{}

	// Test single NUMA node
	singleNUMAMask, _ := bitmask.NewBitMask(0)
	singleNUMAHint := &topologymanager.TopologyHint{
		NUMANodeAffinity: singleNUMAMask,
		Preferred:        true,
	}

	machineState := state.NUMANodeMap{
		0: &state.NUMANodeState{
			MemoryMap: map[v1.ResourceName]*state.MemoryTable{
				v1.ResourceMemory: {
					Allocatable: 4 * 1024 * 1024 * 1024, // 4GB
				},
			},
		},
	}

	policy.calculateEnhancedTopologyFields(singleNUMAHint, machineState, v1.ResourceMemory, 1024*1024*1024) // 1GB request

	if singleNUMAHint.GetHopCount() != 0 {
		t.Errorf("Expected hop count 0 for single NUMA, got %d", singleNUMAHint.GetHopCount())
	}

	if singleNUMAHint.GetDistance() != 10 {
		t.Errorf("Expected distance 10 for single NUMA, got %d", singleNUMAHint.GetDistance())
	}

	// Test multi-NUMA node
	multiNUMAMask, _ := bitmask.NewBitMask(0, 1)
	multiNUMAHint := &topologymanager.TopologyHint{
		NUMANodeAffinity: multiNUMAMask,
		Preferred:        false,
	}

	machineState[1] = &state.NUMANodeState{
		MemoryMap: map[v1.ResourceName]*state.MemoryTable{
			v1.ResourceMemory: {
				Allocatable: 4 * 1024 * 1024 * 1024, // 4GB
			},
		},
	}

	policy.calculateEnhancedTopologyFields(multiNUMAHint, machineState, v1.ResourceMemory, 2*1024*1024*1024) // 2GB request

	if multiNUMAHint.GetHopCount() != 1 {
		t.Errorf("Expected hop count 1 for multi-NUMA, got %d", multiNUMAHint.GetHopCount())
	}

	if multiNUMAHint.GetDistance() != 25 { // 10 + 1 * 15
		t.Errorf("Expected distance 25 for multi-NUMA, got %d", multiNUMAHint.GetDistance())
	}

	if multiNUMAHint.GetBandwidth() != 70.0 { // 100 * (1 - 1 * 0.3)
		t.Errorf("Expected bandwidth 70.0 for multi-NUMA, got %f", multiNUMAHint.GetBandwidth())
	}
}