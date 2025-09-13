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

package devicemanager

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
)

func TestDeviceManagerEnhancedTopologyHints(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	testCases := []struct {
		name                  string
		deviceRequest         int
		numaNodes            []int
		expectedHopCount     int
		expectedMinDistance  int
		expectedMinBandwidth float64
	}{
		{
			name:                  "Single NUMA node",
			deviceRequest:         1,
			numaNodes:            []int{0},
			expectedHopCount:     0,
			expectedMinDistance:  10,
			expectedMinBandwidth: 80.0, // 80 * (1 - 0 * 0.4)
		},
		{
			name:                  "Two NUMA nodes",
			deviceRequest:         2,
			numaNodes:            []int{0, 1},
			expectedHopCount:     1,
			expectedMinDistance:  30, // 10 + 1 * 20
			expectedMinBandwidth: 48.0, // 80 * (1 - 1 * 0.4)
		},
		{
			name:                  "Three NUMA nodes",
			deviceRequest:         3,
			numaNodes:            []int{0, 1, 2},
			expectedHopCount:     2,
			expectedMinDistance:  50, // 10 + 2 * 20
			expectedMinBandwidth: 16.0, // 80 * (1 - 2 * 0.4)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := ManagerImpl{
				allDevices:       NewResourceDeviceInstances(),
				healthyDevices:   make(map[string]sets.Set[string]),
				allocatedDevices: make(map[string]sets.Set[string]),
				podDevices:       newPodDevices(),
				sourcesReady:     &sourcesReadyStub{},
				activePods:       func() []*v1.Pod { return []*v1.Pod{} },
				numaNodes:        []int{0, 1, 2, 3},
			}

			// Create devices on specified NUMA nodes
			resourceName := "testdevice"
			m.allDevices[resourceName] = make(DeviceInstances)
			m.healthyDevices[resourceName] = sets.New[string]()

			for i, nodeID := range tc.numaNodes {
				deviceID := fmt.Sprintf("Dev%d", i)
				device := &pluginapi.Device{
					ID:       deviceID,
					Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: int64(nodeID)}}},
				}
				m.allDevices[resourceName][deviceID] = device
				m.healthyDevices[resourceName].Insert(deviceID)
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
								Limits: v1.ResourceList{
									v1.ResourceName(resourceName): resource.MustParse(fmt.Sprintf("%d", tc.deviceRequest)),
								},
							},
						},
					},
				},
			}

			hints := m.GetTopologyHints(pod, &pod.Spec.Containers[0])

			// Verify we got hints for the device resource
			deviceHints, ok := hints[resourceName]
			if !ok {
				t.Fatalf("No hints generated for device resource")
			}

			// Find the hint that matches our expected NUMA nodes
			var targetHint *topologymanager.TopologyHint
			for _, hint := range deviceHints {
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

			if targetHint.GetBandwidth() < tc.expectedMinBandwidth-0.1 { // Allow for small floating point differences
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

func TestDeviceManagerEnhancedTopologyHintsDisabled(t *testing.T) {
	// Ensure feature gate is disabled
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	m := ManagerImpl{
		allDevices:       NewResourceDeviceInstances(),
		healthyDevices:   make(map[string]sets.Set[string]),
		allocatedDevices: make(map[string]sets.Set[string]),
		podDevices:       newPodDevices(),
		sourcesReady:     &sourcesReadyStub{},
		activePods:       func() []*v1.Pod { return []*v1.Pod{} },
		numaNodes:        []int{0, 1},
	}

	// Create simple device setup
	resourceName := "testdevice"
	m.allDevices[resourceName] = make(DeviceInstances)
	m.healthyDevices[resourceName] = sets.New[string]()

	device := &pluginapi.Device{
		ID:       "Dev0",
		Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: 0}}},
	}
	m.allDevices[resourceName]["Dev0"] = device
	m.healthyDevices[resourceName].Insert("Dev0")

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
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	hints := m.GetTopologyHints(pod, &pod.Spec.Containers[0])

	// Verify we got hints for the device resource
	deviceHints, ok := hints[resourceName]
	if !ok {
		t.Fatalf("No hints generated for device resource")
	}

	// Verify enhanced fields are NOT set when feature is disabled
	for _, hint := range deviceHints {
		// When feature gate is disabled, SetEnhancedFields should not set any pointers
		// So GetHopCount should return 0 (default value) and we check via a different approach
		// We'll verify by checking that all getter methods return their default values
		hopCount := hint.GetHopCount()
		bandwidth := hint.GetBandwidth()
		distance := hint.GetDistance()
		score := hint.GetScore()
		
		// When feature is disabled, GetHopCount returns 0, GetBandwidth returns 0.0, 
		// GetDistance returns 10, GetScore returns 0.0
		if !(hopCount == 0 && bandwidth == 0.0 && distance == 10 && score == 0.0) {
			t.Errorf("Expected default values when feature gate disabled, got hopCount=%d, bandwidth=%f, distance=%d, score=%f", 
				hopCount, bandwidth, distance, score)
		}
	}
}

func TestDeviceManagerCalculateEnhancedTopologyFields(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	m := ManagerImpl{
		allDevices: NewResourceDeviceInstances(),
		numaNodes:  []int{0, 1},
	}

	// Test single NUMA node
	singleNUMAMask, _ := bitmask.NewBitMask(0)
	singleNUMAHint := &topologymanager.TopologyHint{
		NUMANodeAffinity: singleNUMAMask,
		Preferred:        true,
	}

	resourceName := "testdevice"
	m.allDevices[resourceName] = make(DeviceInstances)
	m.allDevices[resourceName]["Dev0"] = &pluginapi.Device{
		ID:       "Dev0",
		Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: 0}}},
	}

	m.calculateEnhancedTopologyFields(singleNUMAHint, resourceName, 1)

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

	m.allDevices[resourceName]["Dev1"] = &pluginapi.Device{
		ID:       "Dev1",
		Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: 1}}},
	}

	m.calculateEnhancedTopologyFields(multiNUMAHint, resourceName, 2)

	if multiNUMAHint.GetHopCount() != 1 {
		t.Errorf("Expected hop count 1 for multi-NUMA, got %d", multiNUMAHint.GetHopCount())
	}

	if multiNUMAHint.GetDistance() != 30 { // 10 + 1 * 20
		t.Errorf("Expected distance 30 for multi-NUMA, got %d", multiNUMAHint.GetDistance())
	}

	if multiNUMAHint.GetBandwidth() != 48.0 { // 80 * (1 - 1 * 0.4)
		t.Errorf("Expected bandwidth 48.0 for multi-NUMA, got %f", multiNUMAHint.GetBandwidth())
	}
}

func TestDeviceManagerRegenerateEnhancedTopologyHints(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, true)
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnhancedTopologyHints, false)

	m := ManagerImpl{
		allDevices: NewResourceDeviceInstances(),
		numaNodes:  []int{0, 1},
	}

	resourceName := "testdevice"
	m.allDevices[resourceName] = make(DeviceInstances)
	m.allDevices[resourceName]["Dev0"] = &pluginapi.Device{
		ID:       "Dev0",
		Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: 0}}},
	}
	m.allDevices[resourceName]["Dev1"] = &pluginapi.Device{
		ID:       "Dev1",
		Topology: &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: 1}}},
	}

	// Test enhanced topology when regenerating hints for allocated devices
	// This simulates the case where devices are already allocated and we need to regenerate hints
	allocated := sets.New[string]("Dev0", "Dev1")
	hints := m.generateDeviceTopologyHints(resourceName, allocated, sets.Set[string]{}, 2)

	if len(hints) != 1 {
		t.Fatalf("Expected 1 hint, got %d", len(hints))
	}

	hint := hints[0]
	if hint.GetHopCount() == -1 {
		t.Errorf("Expected enhanced fields to be set in regenerated hint")
	}

	if hint.GetHopCount() != 1 {
		t.Errorf("Expected hop count 1 for regenerated multi-NUMA hint, got %d", hint.GetHopCount())
	}

	if hint.GetDistance() != 30 { // 10 + 1 * 20
		t.Errorf("Expected distance 30 for regenerated multi-NUMA hint, got %d", hint.GetDistance())
	}
}