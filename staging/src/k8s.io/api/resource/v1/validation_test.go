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

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateInterconnectInfo(t *testing.T) {
	testCases := []struct {
		name            string
		interconnectInfo *InterconnectInfo
		expectedErrors  int
		description     string
	}{
		{
			name:            "nil InterconnectInfo",
			interconnectInfo: nil,
			expectedErrors:  0,
			description:     "nil InterconnectInfo should be valid",
		},
		{
			name: "valid InterconnectInfo with all fields",
			interconnectInfo: &InterconnectInfo{
				HopCount:  int32Ptr(1),
				Bandwidth: float64Ptr(100.0),
				Distance:  int32Ptr(20),
				Latency:   int32Ptr(100),
			},
			expectedErrors: 0,
			description:    "valid InterconnectInfo should pass validation",
		},
		{
			name: "valid InterconnectInfo with partial fields",
			interconnectInfo: &InterconnectInfo{
				HopCount: int32Ptr(0),
				Distance: int32Ptr(10),
			},
			expectedErrors: 0,
			description:    "partial InterconnectInfo should be valid",
		},
		{
			name: "negative hop count",
			interconnectInfo: &InterconnectInfo{
				HopCount: int32Ptr(-1),
			},
			expectedErrors: 1,
			description:    "negative hop count should be invalid",
		},
		{
			name: "hop count too large",
			interconnectInfo: &InterconnectInfo{
				HopCount: int32Ptr(256),
			},
			expectedErrors: 1,
			description:    "hop count > 255 should be invalid",
		},
		{
			name: "negative bandwidth",
			interconnectInfo: &InterconnectInfo{
				Bandwidth: float64Ptr(-1.0),
			},
			expectedErrors: 1,
			description:    "negative bandwidth should be invalid",
		},
		{
			name: "bandwidth too large",
			interconnectInfo: &InterconnectInfo{
				Bandwidth: float64Ptr(1000001.0),
			},
			expectedErrors: 1,
			description:    "bandwidth > 1000000 should be invalid",
		},
		{
			name: "negative distance",
			interconnectInfo: &InterconnectInfo{
				Distance: int32Ptr(-1),
			},
			expectedErrors: 1,
			description:    "negative distance should be invalid",
		},
		{
			name: "distance too large",
			interconnectInfo: &InterconnectInfo{
				Distance: int32Ptr(256),
			},
			expectedErrors: 1,
			description:    "distance > 255 should be invalid",
		},
		{
			name: "distance too small (non-zero)",
			interconnectInfo: &InterconnectInfo{
				Distance: int32Ptr(5),
			},
			expectedErrors: 1,
			description:    "distance < 10 (non-zero) should be invalid",
		},
		{
			name: "negative latency",
			interconnectInfo: &InterconnectInfo{
				Latency: int32Ptr(-1),
			},
			expectedErrors: 1,
			description:    "negative latency should be invalid",
		},
		{
			name: "latency too large",
			interconnectInfo: &InterconnectInfo{
				Latency: int32Ptr(1000001),
			},
			expectedErrors: 1,
			description:    "latency > 1000000 should be invalid",
		},
		{
			name: "multiple validation errors",
			interconnectInfo: &InterconnectInfo{
				HopCount:  int32Ptr(-1),
				Bandwidth: float64Ptr(-10.0),
				Distance:  int32Ptr(-5),
				Latency:   int32Ptr(-100),
			},
			expectedErrors: 4,
			description:    "multiple invalid fields should generate multiple errors",
		},
		{
			name: "valid connectivity matrix",
			interconnectInfo: &InterconnectInfo{
				ConnectivityMatrix: map[string]NodeConnectivity{
					"node1": {
						Bandwidth: float64Ptr(100.0),
						Latency:   int32Ptr(50),
					},
					"node2": {
						Bandwidth: float64Ptr(80.0),
						Latency:   int32Ptr(100),
					},
				},
			},
			expectedErrors: 0,
			description:    "valid connectivity matrix should pass validation",
		},
		{
			name: "invalid connectivity matrix - empty node ID",
			interconnectInfo: &InterconnectInfo{
				ConnectivityMatrix: map[string]NodeConnectivity{
					"": {
						Bandwidth: float64Ptr(100.0),
					},
				},
			},
			expectedErrors: 1,
			description:    "empty node ID in connectivity matrix should be invalid",
		},
		{
			name: "invalid connectivity matrix - negative values",
			interconnectInfo: &InterconnectInfo{
				ConnectivityMatrix: map[string]NodeConnectivity{
					"node1": {
						Bandwidth: float64Ptr(-100.0),
						Latency:   int32Ptr(-50),
					},
				},
			},
			expectedErrors: 2,
			description:    "negative values in connectivity matrix should be invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fldPath := field.NewPath("test")
			errs := ValidateInterconnectInfo(tc.interconnectInfo, fldPath)
			
			if len(errs) != tc.expectedErrors {
				t.Errorf("Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
			}
		})
	}
}

func TestValidateNodeTopologyInfo(t *testing.T) {
	testCases := []struct {
		name           string
		nodeTopology   *NodeTopologyInfo
		expectedErrors int
		description    string
	}{
		{
			name:           "nil NodeTopologyInfo",
			nodeTopology:   nil,
			expectedErrors: 0,
			description:    "nil NodeTopologyInfo should be valid",
		},
		{
			name: "valid NodeTopologyInfo",
			nodeTopology: &NodeTopologyInfo{
				NodeID: 0,
				Resources: map[string]int64{
					"gpu.vendor.com/gpu": 2,
					"memory":             1024,
				},
				InterconnectInfo: &InterconnectInfo{
					HopCount:  int32Ptr(0),
					Bandwidth: float64Ptr(100.0),
					Distance:  int32Ptr(10),
				},
			},
			expectedErrors: 0,
			description:    "valid NodeTopologyInfo should pass validation",
		},
		{
			name: "negative node ID",
			nodeTopology: &NodeTopologyInfo{
				NodeID: -1,
			},
			expectedErrors: 1,
			description:    "negative node ID should be invalid",
		},
		{
			name: "empty resource name",
			nodeTopology: &NodeTopologyInfo{
				NodeID: 0,
				Resources: map[string]int64{
					"": 1,
				},
			},
			expectedErrors: 1,
			description:    "empty resource name should be invalid",
		},
		{
			name: "negative resource quantity",
			nodeTopology: &NodeTopologyInfo{
				NodeID: 0,
				Resources: map[string]int64{
					"gpu": -1,
				},
			},
			expectedErrors: 1,
			description:    "negative resource quantity should be invalid",
		},
		{
			name: "invalid InterconnectInfo",
			nodeTopology: &NodeTopologyInfo{
				NodeID: 0,
				InterconnectInfo: &InterconnectInfo{
					HopCount: int32Ptr(-1),
				},
			},
			expectedErrors: 1,
			description:    "invalid InterconnectInfo should propagate errors",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fldPath := field.NewPath("test")
			errs := ValidateNodeTopologyInfo(tc.nodeTopology, fldPath)
			
			if len(errs) != tc.expectedErrors {
				t.Errorf("Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
			}
		})
	}
}

func TestValidateResourceSlice(t *testing.T) {
	testCases := []struct {
		name           string
		resourceSlice  *ResourceSlice
		expectedErrors int
		description    string
	}{
		{
			name:           "nil ResourceSlice",
			resourceSlice:  nil,
			expectedErrors: 0,
			description:    "nil ResourceSlice should be valid",
		},
		{
			name: "valid ResourceSlice with NodeTopology",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						Name: "test-pool",
					},
					NodeTopology: &NodeTopologyInfo{
						NodeID: 0,
						Resources: map[string]int64{
							"gpu": 2,
						},
						InterconnectInfo: &InterconnectInfo{
							HopCount:  int32Ptr(0),
							Bandwidth: float64Ptr(100.0),
							Distance:  int32Ptr(10),
						},
					},
				},
			},
			expectedErrors: 0,
			description:    "valid ResourceSlice should pass validation",
		},
		{
			name: "valid ResourceSlice with Devices",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						Name: "test-pool",
					},
					Devices: []Device{
						{Name: "gpu-0"},
						{Name: "gpu-1"},
					},
				},
			},
			expectedErrors: 0,
			description:    "valid ResourceSlice with Devices should pass validation",
		},
		{
			name: "missing name",
			resourceSlice: &ResourceSlice{
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						Name: "test-pool",
					},
				},
			},
			expectedErrors: 1,
			description:    "missing name should be invalid",
		},
		{
			name: "missing driver",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Pool: ResourcePool{
						Name: "test-pool",
					},
				},
			},
			expectedErrors: 1,
			description:    "missing driver should be invalid",
		},
		{
			name: "invalid NodeTopology",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						Name: "test-pool",
					},
					NodeTopology: &NodeTopologyInfo{
						NodeID: -1,
					},
				},
			},
			expectedErrors: 1,
			description:    "invalid NodeTopology should propagate errors",
		},
		{
			name: "invalid Pool - missing pool name",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						// Name missing
					},
					Devices: []Device{
						{Name: "gpu-0"},
					},
				},
			},
			expectedErrors: 1,
			description:    "missing pool name should be invalid",
		},
		{
			name: "invalid Devices - missing device name",
			resourceSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com",
					Pool: ResourcePool{
						Name: "test-pool",
					},
					Devices: []Device{
						{Name: "gpu-0"},
						{}, // Missing name
					},
				},
			},
			expectedErrors: 1,
			description:    "missing device name should be invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateResourceSlice(tc.resourceSlice)
			
			if len(errs) != tc.expectedErrors {
				t.Errorf("Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
			}
		})
	}
}

func TestValidateResourceSliceUpdate(t *testing.T) {
	baseResourceSlice := &ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-slice",
		},
		Spec: ResourceSliceSpec{
			Driver: "gpu.vendor.com",
			Pool: ResourcePool{
				Name: "test-pool",
			},
		},
	}

	testCases := []struct {
		name           string
		oldSlice       *ResourceSlice
		newSlice       *ResourceSlice
		expectedErrors int
		description    string
	}{
		{
			name:     "valid update - no driver change",
			oldSlice: baseResourceSlice,
			newSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "gpu.vendor.com", // Same driver
					Pool: ResourcePool{
						Name: "test-pool",
					},
					NodeTopology: &NodeTopologyInfo{
						NodeID: 0,
					},
				},
			},
			expectedErrors: 0,
			description:    "valid update should pass validation",
		},
		{
			name:     "invalid update - driver change",
			oldSlice: baseResourceSlice,
			newSlice: &ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				Spec: ResourceSliceSpec{
					Driver: "different.vendor.com", // Different driver
					Pool: ResourcePool{
						Name: "test-pool",
					},
				},
			},
			expectedErrors: 1,
			description:    "driver change should be invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateResourceSliceUpdate(tc.newSlice, tc.oldSlice)
			
			if len(errs) != tc.expectedErrors {
				t.Errorf("Expected %d errors, got %d: %v", tc.expectedErrors, len(errs), errs)
			}
		})
	}
}

// Helper functions for creating pointers
func int32Ptr(i int32) *int32 {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}