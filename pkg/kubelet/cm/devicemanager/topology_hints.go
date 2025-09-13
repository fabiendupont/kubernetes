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

package devicemanager

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/component-helpers/resource"
	"k8s.io/klog/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
)

// GetTopologyHints implements the TopologyManager HintProvider Interface which
// ensures the Device Manager is consulted when Topology Aware Hints for each
// container are created.
func (m *ManagerImpl) GetTopologyHints(pod *v1.Pod, container *v1.Container) map[string][]topologymanager.TopologyHint {
	// Garbage collect any stranded device resources before providing TopologyHints
	m.UpdateAllocatedDevices()

	// Loop through all device resources and generate TopologyHints for them.
	deviceHints := make(map[string][]topologymanager.TopologyHint)
	accumulatedResourceRequests := m.getContainerDeviceRequest(container)

	m.mutex.Lock()
	defer m.mutex.Unlock()
	for resource, requested := range accumulatedResourceRequests {
		// Only consider devices that actually contain topology information.
		if aligned := m.deviceHasTopologyAlignment(resource); !aligned {
			klog.InfoS("Resource does not have a topology preference", "resourceName", resource, "pod", klog.KObj(pod), "containerName", container.Name, "request", requested)
			deviceHints[resource] = nil
			continue
		}

		// Short circuit to regenerate the same hints if there are already
		// devices allocated to the Container. This might happen after a
		// kubelet restart, for example.
		allocated := m.podDevices.containerDevices(string(pod.UID), container.Name, resource)
		if allocated.Len() > 0 {
			if allocated.Len() != requested {
				klog.InfoS("Resource already allocated to pod with different number than request", "resourceName", resource, "pod", klog.KObj(pod), "containerName", container.Name, "request", requested, "allocated", allocated.Len())
				deviceHints[resource] = []topologymanager.TopologyHint{}
				continue
			}
			klog.InfoS("Regenerating TopologyHints for resource already allocated to pod", "resourceName", resource, "pod", klog.KObj(pod), "containerName", container.Name)
			deviceHints[resource] = m.generateDeviceTopologyHints(resource, allocated, sets.Set[string]{}, requested)
			continue
		}

		// Get the list of available devices, for which TopologyHints should be generated.
		available := m.getAvailableDevices(resource)
		reusable := m.devicesToReuse[string(pod.UID)][resource]
		if available.Union(reusable).Len() < requested {
			klog.InfoS("Unable to generate topology hints: requested number of devices unavailable", "resourceName", resource, "pod", klog.KObj(pod), "containerName", container.Name, "request", requested, "available", available.Union(reusable).Len())
			deviceHints[resource] = []topologymanager.TopologyHint{}
			continue
		}

		// Generate TopologyHints for this resource given the current
		// request size and the list of available devices.
		deviceHints[resource] = m.generateDeviceTopologyHints(resource, available, reusable, requested)
	}

	return deviceHints
}

// GetPodTopologyHints implements the topologymanager.HintProvider Interface which
// ensures the Device Manager is consulted when Topology Aware Hints for Pod are created.
func (m *ManagerImpl) GetPodTopologyHints(pod *v1.Pod) map[string][]topologymanager.TopologyHint {
	// Garbage collect any stranded device resources before providing TopologyHints
	m.UpdateAllocatedDevices()

	deviceHints := make(map[string][]topologymanager.TopologyHint)
	accumulatedResourceRequests := m.getPodDeviceRequest(pod)

	m.mutex.Lock()
	defer m.mutex.Unlock()
	for resource, requested := range accumulatedResourceRequests {
		// Only consider devices that actually contain topology information.
		if aligned := m.deviceHasTopologyAlignment(resource); !aligned {
			klog.InfoS("Resource does not have a topology preference", "resourceName", resource, "pod", klog.KObj(pod), "request", requested)
			deviceHints[resource] = nil
			continue
		}

		// Short circuit to regenerate the same hints if there are already
		// devices allocated to the Pod. This might happen after a
		// kubelet restart, for example.
		allocated := m.podDevices.podDevices(string(pod.UID), resource)
		if allocated.Len() > 0 {
			if allocated.Len() != requested {
				klog.InfoS("Resource already allocated to pod with different number than request", "resourceName", resource, "pod", klog.KObj(pod), "request", requested, "allocated", allocated.Len())
				deviceHints[resource] = []topologymanager.TopologyHint{}
				continue
			}
			klog.InfoS("Regenerating TopologyHints for resource already allocated to pod", "resourceName", resource, "pod", klog.KObj(pod), "allocated", allocated.Len())
			deviceHints[resource] = m.generateDeviceTopologyHints(resource, allocated, sets.Set[string]{}, requested)
			continue
		}

		// Get the list of available devices, for which TopologyHints should be generated.
		available := m.getAvailableDevices(resource)
		if available.Len() < requested {
			klog.InfoS("Unable to generate topology hints: requested number of devices unavailable", "resourceName", resource, "pod", klog.KObj(pod), "request", requested, "available", available.Len())
			deviceHints[resource] = []topologymanager.TopologyHint{}
			continue
		}

		// Generate TopologyHints for this resource given the current
		// request size and the list of available devices.
		deviceHints[resource] = m.generateDeviceTopologyHints(resource, available, sets.Set[string]{}, requested)
	}

	return deviceHints
}

func (m *ManagerImpl) deviceHasTopologyAlignment(resource string) bool {
	// If any device has Topology NUMANodes available, we assume they care about alignment.
	for _, device := range m.allDevices[resource] {
		if device.Topology != nil && len(device.Topology.Nodes) > 0 {
			return true
		}
	}
	return false
}

func (m *ManagerImpl) getAvailableDevices(resource string) sets.Set[string] {
	// Strip all devices in use from the list of healthy ones.
	return m.healthyDevices[resource].Difference(m.allocatedDevices[resource])
}

func (m *ManagerImpl) generateDeviceTopologyHints(resource string, available sets.Set[string], reusable sets.Set[string], request int) []topologymanager.TopologyHint {
	// Initialize minAffinitySize to include all NUMA Nodes
	minAffinitySize := len(m.numaNodes)

	// Iterate through all combinations of NUMA Nodes and build hints from them.
	hints := []topologymanager.TopologyHint{}
	bitmask.IterateBitMasks(m.numaNodes, func(mask bitmask.BitMask) {
		// First, update minAffinitySize for the current request size.
		devicesInMask := 0
		for _, device := range m.allDevices[resource] {
			if mask.AnySet(m.getNUMANodeIds(device.Topology)) {
				devicesInMask++
			}
		}
		if devicesInMask >= request && mask.Count() < minAffinitySize {
			minAffinitySize = mask.Count()
		}

		// Then check to see if all the reusable devices are part of the bitmask.
		numMatching := 0
		for d := range reusable {
			// Skip the device if it doesn't specify any topology info.
			if m.allDevices[resource][d].Topology == nil {
				continue
			}
			// Otherwise disregard this mask if its NUMANode isn't part of it.
			if !mask.AnySet(m.getNUMANodeIds(m.allDevices[resource][d].Topology)) {
				return
			}
			numMatching++
		}

		// Finally, check to see if enough available devices remain on the
		// current NUMA node combination to satisfy the device request.
		for d := range available {
			if mask.AnySet(m.getNUMANodeIds(m.allDevices[resource][d].Topology)) {
				numMatching++
			}
		}

		// If they don't, then move onto the next combination.
		if numMatching < request {
			return
		}

		// Otherwise, create a new hint from the NUMA mask and add it to the
		// list of hints.  We set all hint preferences to 'false' on the first
		// pass through.
		hint := topologymanager.TopologyHint{
			NUMANodeAffinity: mask,
			Preferred:        false,
		}
		
		// Add enhanced topology information if feature gate is enabled
		if topologymanager.EnhancedTopologyHintsEnabled() {
			m.calculateEnhancedTopologyFields(&hint, resource, request)
		}
		
		hints = append(hints, hint)
	})

	// Loop back through all hints and update the 'Preferred' field based on
	// counting the number of bits sets in the affinity mask and comparing it
	// to the minAffinity. Only those with an equal number of bits set will be
	// considered preferred.
	for i := range hints {
		if hints[i].NUMANodeAffinity.Count() == minAffinitySize {
			hints[i].Preferred = true
		}
	}

	return hints
}

func (m *ManagerImpl) getNUMANodeIds(topology *pluginapi.TopologyInfo) []int {
	if topology == nil {
		return nil
	}
	var ids []int
	for _, n := range topology.Nodes {
		ids = append(ids, int(n.ID))
	}
	return ids
}

func (m *ManagerImpl) getPodDeviceRequest(pod *v1.Pod) map[string]int {
	// for these device plugin resources, requests == limits
	limits := resource.PodLimits(pod, resource.PodResourcesOptions{
		ExcludeOverhead: true,
	})
	podRequests := make(map[string]int)
	for resourceName, quantity := range limits {
		if !m.isDevicePluginResource(string(resourceName)) {
			continue
		}
		podRequests[string(resourceName)] = int(quantity.Value())
	}
	return podRequests
}

func (m *ManagerImpl) getContainerDeviceRequest(container *v1.Container) map[string]int {
	containerRequests := make(map[string]int)
	for resourceObj, requestedObj := range container.Resources.Limits {
		resource := string(resourceObj)
		requested := int(requestedObj.Value())
		if !m.isDevicePluginResource(resource) {
			continue
		}
		containerRequests[resource] = requested
	}
	return containerRequests
}

// calculateEnhancedTopologyFields calculates enhanced topology metrics for KEP-10002
func (m *ManagerImpl) calculateEnhancedTopologyFields(hint *topologymanager.TopologyHint, resource string, requestedDevices int) {
	// Only calculate enhanced fields when the feature gate is enabled
	if !topologymanager.EnhancedTopologyHintsEnabled() {
		return
	}
	
	if hint.NUMANodeAffinity == nil {
		return
	}
	
	numaNodes := hint.NUMANodeAffinity.GetBits()
	if len(numaNodes) == 0 {
		return
	}
	
	// Calculate hop count: 0 for single NUMA node, 1+ for multi-NUMA
	hopCount := len(numaNodes) - 1
	if hopCount < 0 {
		hopCount = 0
	}
	
	// Calculate device-specific distance based on NUMA distance matrix
	var totalDistance int
	nodeCount := len(numaNodes)
	
	if nodeCount == 1 {
		// Local device access - use standard Linux NUMA distance
		totalDistance = 10
	} else {
		// Multi-NUMA: calculate average inter-node distance for device access
		// Device access is more sensitive to NUMA distance than CPU
		totalDistance = 10 + (hopCount * 20) // 10=local, 30=1-hop, 50=2-hop, etc.
	}
	
	// Calculate device bandwidth estimate (simplified model)
	// Device bandwidth varies significantly by device type and interconnect
	baseBandwidth := 80.0 // GB/s baseline for local device access (conservative estimate)
	bandwidthPenalty := float64(hopCount) * 0.4 // 40% penalty per hop for device interconnects
	bandwidth := baseBandwidth * (1.0 - bandwidthPenalty)
	if bandwidth < 10.0 {
		bandwidth = 10.0 // Minimum device bandwidth floor
	}
	
	// Calculate device-specific topology score: lower is better
	// Device workloads are sensitive to both latency and bandwidth
	
	// Count available devices in the selected NUMA nodes for utilization calculation
	availableDevices := 0
	for _, device := range m.allDevices[resource] {
		if device.Topology != nil {
			deviceNodes := m.getNUMANodeIds(device.Topology)
			for _, nodeID := range deviceNodes {
				for _, selectedNode := range numaNodes {
					if nodeID == selectedNode {
						availableDevices++
						break
					}
				}
			}
		}
	}
	
	// Device utilization ratio
	utilizationRatio := float64(requestedDevices) / float64(availableDevices)
	if utilizationRatio > 1.0 {
		utilizationRatio = 1.0
	}
	
	// Device-specific scoring factors
	distancePenalty := float64(totalDistance-10) * 2.0 // Moderate penalty for device distance
	hopPenalty := float64(hopCount) * 12.0 // Higher penalty for device cross-NUMA access
	utilizationBonus := (1.0 - utilizationRatio) * 10.0 // Bonus for better device utilization
	
	score := distancePenalty + hopPenalty - utilizationBonus
	if score < 0.0 {
		score = 0.0
	}
	
	// Set the enhanced fields
	hint.SetEnhancedFields(&hopCount, &bandwidth, &totalDistance, &score)
}

