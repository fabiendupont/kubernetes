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
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateInterconnectInfo validates the InterconnectInfo fields
func ValidateInterconnectInfo(interconnectInfo *InterconnectInfo, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	
	if interconnectInfo == nil {
		return allErrs
	}

	// Validate HopCount
	if interconnectInfo.HopCount != nil {
		if *interconnectInfo.HopCount < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("hopCount"), *interconnectInfo.HopCount, "hop count must be non-negative"))
		}
		if *interconnectInfo.HopCount > 255 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("hopCount"), *interconnectInfo.HopCount, "hop count must be less than or equal to 255"))
		}
	}

	// Validate Bandwidth
	if interconnectInfo.Bandwidth != nil {
		if *interconnectInfo.Bandwidth < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("bandwidth"), *interconnectInfo.Bandwidth, "bandwidth must be non-negative"))
		}
		if *interconnectInfo.Bandwidth > 1000000 { // 1 TB/s seems like a reasonable upper limit
			allErrs = append(allErrs, field.Invalid(fldPath.Child("bandwidth"), *interconnectInfo.Bandwidth, "bandwidth must be less than or equal to 1000000 GB/s"))
		}
	}

	// Validate Distance
	if interconnectInfo.Distance != nil {
		if *interconnectInfo.Distance < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("distance"), *interconnectInfo.Distance, "distance must be non-negative"))
		}
		// NUMA distances typically follow Linux kernel conventions: 10, 20, 30, 40, etc.
		// Allow up to 255 as a reasonable upper bound
		if *interconnectInfo.Distance > 255 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("distance"), *interconnectInfo.Distance, "distance must be less than or equal to 255"))
		}
		// Check for common NUMA distance values (10, 20, 30, 40, etc.)
		if *interconnectInfo.Distance > 0 && *interconnectInfo.Distance < 10 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("distance"), *interconnectInfo.Distance, "distance should typically be at least 10 (Linux NUMA convention)"))
		}
	}

	// Validate Latency
	if interconnectInfo.Latency != nil {
		if *interconnectInfo.Latency < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("latency"), *interconnectInfo.Latency, "latency must be non-negative"))
		}
		if *interconnectInfo.Latency > 1000000 { // 1 second seems like a reasonable upper limit for system latency
			allErrs = append(allErrs, field.Invalid(fldPath.Child("latency"), *interconnectInfo.Latency, "latency must be less than or equal to 1000000 microseconds"))
		}
	}

	// Validate ConnectivityMatrix
	if interconnectInfo.ConnectivityMatrix != nil {
		allErrs = append(allErrs, validateConnectivityMatrix(interconnectInfo.ConnectivityMatrix, fldPath.Child("connectivityMatrix"))...)
	}

	return allErrs
}

// validateConnectivityMatrix validates the connectivity matrix entries
func validateConnectivityMatrix(matrix map[string]NodeConnectivity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for nodeID, connectivity := range matrix {
		nodePath := fldPath.Key(nodeID)
		
		// Validate that nodeID is not empty
		if nodeID == "" {
			allErrs = append(allErrs, field.Invalid(fldPath, nodeID, "node ID cannot be empty"))
			continue
		}

		// Validate Bandwidth in connectivity
		if connectivity.Bandwidth != nil {
			if *connectivity.Bandwidth < 0 {
				allErrs = append(allErrs, field.Invalid(nodePath.Child("bandwidth"), *connectivity.Bandwidth, "bandwidth must be non-negative"))
			}
			if *connectivity.Bandwidth > 1000000 {
				allErrs = append(allErrs, field.Invalid(nodePath.Child("bandwidth"), *connectivity.Bandwidth, "bandwidth must be less than or equal to 1000000 GB/s"))
			}
		}

		// Validate Latency in connectivity
		if connectivity.Latency != nil {
			if *connectivity.Latency < 0 {
				allErrs = append(allErrs, field.Invalid(nodePath.Child("latency"), *connectivity.Latency, "latency must be non-negative"))
			}
			if *connectivity.Latency > 1000000 {
				allErrs = append(allErrs, field.Invalid(nodePath.Child("latency"), *connectivity.Latency, "latency must be less than or equal to 1000000 nanoseconds"))
			}
		}
	}

	return allErrs
}

// ValidateNodeTopologyInfo validates the NodeTopologyInfo structure including InterconnectInfo
func ValidateNodeTopologyInfo(nodeTopology *NodeTopologyInfo, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	
	if nodeTopology == nil {
		return allErrs
	}

	// Validate NodeID
	if nodeTopology.NodeID < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("nodeID"), nodeTopology.NodeID, "node ID must be non-negative"))
	}

	// Validate Resources map
	if nodeTopology.Resources != nil {
		for resourceName, quantity := range nodeTopology.Resources {
			if resourceName == "" {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("resources"), resourceName, "resource name cannot be empty"))
			}
			if quantity < 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("resources").Key(resourceName), quantity, "resource quantity must be non-negative"))
			}
		}
	}

	// Validate InterconnectInfo
	if nodeTopology.InterconnectInfo != nil {
		allErrs = append(allErrs, ValidateInterconnectInfo(nodeTopology.InterconnectInfo, fldPath.Child("interconnectInfo"))...)
	}

	return allErrs
}

// ValidateResourceSlice validates the entire ResourceSlice including topology information
func ValidateResourceSlice(resourceSlice *ResourceSlice) field.ErrorList {
	allErrs := field.ErrorList{}
	
	if resourceSlice == nil {
		return allErrs
	}

	// Validate basic ResourceSlice fields
	if resourceSlice.Name == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("metadata", "name"), "name is required"))
	}

	specPath := field.NewPath("spec")
	
	// Validate driver name
	if resourceSlice.Spec.Driver == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("driver"), "driver name is required"))
	}

	// Validate NodeTopology if present
	if resourceSlice.Spec.NodeTopology != nil {
		allErrs = append(allErrs, ValidateNodeTopologyInfo(resourceSlice.Spec.NodeTopology, specPath.Child("nodeTopology"))...)
	}

	// Validate Pool (required field)
	if resourceSlice.Spec.Pool.Name == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("pool", "name"), "pool name is required"))
	}
	
	// Validate each device in the pool
	for i, device := range resourceSlice.Spec.Devices {
		devicePath := specPath.Child("devices").Index(i)
		if device.Name == "" {
			allErrs = append(allErrs, field.Required(devicePath.Child("name"), "device name is required"))
		}
	}

	return allErrs
}

// ValidateResourceSliceUpdate validates updates to ResourceSlice
func ValidateResourceSliceUpdate(newResourceSlice, oldResourceSlice *ResourceSlice) field.ErrorList {
	allErrs := ValidateResourceSlice(newResourceSlice)
	
	// Add update-specific validations
	if newResourceSlice.Spec.Driver != oldResourceSlice.Spec.Driver {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "driver"), "driver field is immutable"))
	}

	return allErrs
}