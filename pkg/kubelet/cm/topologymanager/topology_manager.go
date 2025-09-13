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

package topologymanager

import (
	"fmt"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/metrics"
)

const (
	// defaultMaxAllowableNUMANodes specifies the maximum number of NUMA Nodes that
	// the TopologyManager supports on the underlying machine.
	//
	// At present, having more than this number of NUMA Nodes will result in a
	// state explosion when trying to enumerate possible NUMAAffinity masks and
	// generate hints for them. As such, if more NUMA Nodes than this are
	// present on a machine and the TopologyManager is enabled, an error will
	// be returned and the TopologyManager will not be loaded.
	defaultMaxAllowableNUMANodes = 8
	// ErrorTopologyAffinity represents the type for a TopologyAffinityError
	ErrorTopologyAffinity = "TopologyAffinityError"
)

// TopologyAffinityError represents an resource alignment error
type TopologyAffinityError struct{}

func (e TopologyAffinityError) Error() string {
	return "Resources cannot be allocated with Topology locality"
}

func (e TopologyAffinityError) Type() string {
	return ErrorTopologyAffinity
}

// Manager interface provides methods for Kubelet to manage pod topology hints
type Manager interface {
	// PodAdmitHandler is implemented by Manager
	lifecycle.PodAdmitHandler
	// AddHintProvider adds a hint provider to manager to indicate the hint provider
	// wants to be consulted with when making topology hints
	AddHintProvider(HintProvider)
	// AddContainer adds pod to Manager for tracking
	AddContainer(pod *v1.Pod, container *v1.Container, containerID string)
	// RemoveContainer removes pod from Manager tracking
	RemoveContainer(containerID string) error
	// Store is the interface for storing pod topology hints
	Store
}

type manager struct {
	//Topology Manager Scope
	scope Scope
}

// HintProvider is an interface for components that want to collaborate to
// achieve globally optimal concrete resource alignment with respect to
// NUMA locality.
type HintProvider interface {
	// GetTopologyHints returns a map of resource names to a list of possible
	// concrete resource allocations in terms of NUMA locality hints. Each hint
	// is optionally marked "preferred" and indicates the set of NUMA nodes
	// involved in the hypothetical allocation. The topology manager calls
	// this function for each hint provider, and merges the hints to produce
	// a consensus "best" hint. The hint providers may subsequently query the
	// topology manager to influence actual resource assignment.
	GetTopologyHints(pod *v1.Pod, container *v1.Container) map[string][]TopologyHint
	// GetPodTopologyHints returns a map of resource names to a list of possible
	// concrete resource allocations per Pod in terms of NUMA locality hints.
	GetPodTopologyHints(pod *v1.Pod) map[string][]TopologyHint
	// Allocate triggers resource allocation to occur on the HintProvider after
	// all hints have been gathered and the aggregated Hint is available via a
	// call to Store.GetAffinity().
	Allocate(pod *v1.Pod, container *v1.Container) error
}

// Store interface is to allow Hint Providers to retrieve pod affinity
type Store interface {
	GetAffinity(podUID string, containerName string) TopologyHint
	GetPolicy() Policy
}

// TopologyHint is a struct containing the NUMANodeAffinity for a Container
type TopologyHint struct {
	NUMANodeAffinity bitmask.BitMask
	// Preferred is set to true when the NUMANodeAffinity encodes a preferred
	// allocation for the Container. It is set to false otherwise.
	Preferred bool
	
	// Enhanced topology fields for KEP-10002
	// These fields are optional and provide additional metrics for sophisticated
	// resource placement decisions based on interconnect characteristics.
	
	// HopCount indicates the number of hops required to reach the resource
	// from the requesting NUMA node. Lower values are preferred.
	// +optional
	// +featureGate=EnhancedTopologyHints
	HopCount *int `json:"hopCount,omitempty"`
	
	// Bandwidth indicates the interconnect bandwidth in GB/s between the
	// requesting NUMA node and the resource location. Higher values are preferred.
	// +optional
	// +featureGate=EnhancedTopologyHints
	Bandwidth *float64 `json:"bandwidth,omitempty"`
	
	// Distance indicates the NUMA distance matrix value following Linux kernel
	// conventions (10=local, 20=1-hop, 30=2-hop, etc.). Lower values are preferred.
	// +optional
	// +featureGate=EnhancedTopologyHints
	Distance *int `json:"distance,omitempty"`
	
	// Score provides a calculated placement score combining latency, bandwidth,
	// and other factors. Lower scores indicate better placements.
	// +optional
	// +featureGate=EnhancedTopologyHints
	Score *float64 `json:"score,omitempty"`
}

// IsEqual checks if TopologyHint are equal
func (th *TopologyHint) IsEqual(topologyHint TopologyHint) bool {
	if th.Preferred != topologyHint.Preferred {
		return false
	}
	
	// Check NUMANodeAffinity
	if th.NUMANodeAffinity == nil || topologyHint.NUMANodeAffinity == nil {
		if th.NUMANodeAffinity != topologyHint.NUMANodeAffinity {
			return false
		}
	} else if !th.NUMANodeAffinity.IsEqual(topologyHint.NUMANodeAffinity) {
		return false
	}
	
	// Check enhanced fields (nil-safe comparison following Kubernetes patterns)
	if !th.equalIntPointer(th.HopCount, topologyHint.HopCount) {
		return false
	}
	if !th.equalFloat64Pointer(th.Bandwidth, topologyHint.Bandwidth) {
		return false
	}
	if !th.equalIntPointer(th.Distance, topologyHint.Distance) {
		return false
	}
	if !th.equalFloat64Pointer(th.Score, topologyHint.Score) {
		return false
	}
	
	return true
}

// equalIntPointer compares two int pointers nil-safely
func (th *TopologyHint) equalIntPointer(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// equalFloat64Pointer compares two float64 pointers nil-safely
func (th *TopologyHint) equalFloat64Pointer(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// LessThan checks if TopologyHint `a` is less than TopologyHint `b`
// this means that either `a` is a preferred hint and `b` is not
// or `a` NUMANodeAffinity attribute is narrower than `b` NUMANodeAffinity attribute.
// When enhanced topology hints are available, additional metrics are considered.
func (th *TopologyHint) LessThan(other TopologyHint) bool {
	// Preferred hints always come first
	if th.Preferred != other.Preferred {
		return th.Preferred
	}
	
	// Traditional NUMA affinity comparison
	numaComparison := th.NUMANodeAffinity.IsNarrowerThan(other.NUMANodeAffinity)
	
	// If enhanced fields are available and feature is enabled, use them for finer-grained comparison
	if EnhancedTopologyHintsEnabled() && (th.hasEnhancedFields() || other.hasEnhancedFields()) {
		// Compare by score first (lower is better)
		if th.Score != nil && other.Score != nil {
			if *th.Score != *other.Score {
				return *th.Score < *other.Score
			}
		} else if th.Score != nil {
			return true  // Scored hints are better than unscored
		} else if other.Score != nil {
			return false
		}
		
		// Compare by hop count (lower is better)
		if th.HopCount != nil && other.HopCount != nil {
			if *th.HopCount != *other.HopCount {
				return *th.HopCount < *other.HopCount
			}
		}
		
		// Compare by distance (lower is better)
		if th.Distance != nil && other.Distance != nil {
			if *th.Distance != *other.Distance {
				return *th.Distance < *other.Distance
			}
		}
		
		// Compare by bandwidth (higher is better)
		if th.Bandwidth != nil && other.Bandwidth != nil {
			if *th.Bandwidth != *other.Bandwidth {
				return *th.Bandwidth > *other.Bandwidth
			}
		}
	}
	
	// Fall back to traditional NUMA comparison
	return numaComparison
}

// hasEnhancedFields returns true if any enhanced topology fields are set
func (th *TopologyHint) hasEnhancedFields() bool {
	return th.HopCount != nil || th.Bandwidth != nil || th.Distance != nil || th.Score != nil
}

// EnhancedTopologyHintsEnabled returns true if the enhanced topology hints feature is enabled
func EnhancedTopologyHintsEnabled() bool {
	return utilfeature.DefaultFeatureGate.Enabled(features.EnhancedTopologyHints)
}

// GetHopCount safely returns the hop count, considering feature gate
func (th *TopologyHint) GetHopCount() int {
	if !EnhancedTopologyHintsEnabled() || th.HopCount == nil {
		return 0  // Default to 0 hops when feature disabled or field nil
	}
	return *th.HopCount
}

// GetBandwidth safely returns the bandwidth, considering feature gate
func (th *TopologyHint) GetBandwidth() float64 {
	if !EnhancedTopologyHintsEnabled() || th.Bandwidth == nil {
		return 0.0  // Default to 0 bandwidth when feature disabled or field nil
	}
	return *th.Bandwidth
}

// GetDistance safely returns the distance, considering feature gate
func (th *TopologyHint) GetDistance() int {
	if !EnhancedTopologyHintsEnabled() || th.Distance == nil {
		return 10  // Default to local distance (10) when feature disabled or field nil
	}
	return *th.Distance
}

// GetScore safely returns the score, considering feature gate
func (th *TopologyHint) GetScore() float64 {
	if !EnhancedTopologyHintsEnabled() || th.Score == nil {
		return 0.0  // Default to 0 score when feature disabled or field nil
	}
	return *th.Score
}

// SetEnhancedFields safely sets enhanced fields only when feature gate is enabled
func (th *TopologyHint) SetEnhancedFields(hopCount *int, bandwidth *float64, distance *int, score *float64) {
	if !EnhancedTopologyHintsEnabled() {
		return  // Do nothing if feature is disabled
	}
	th.HopCount = hopCount
	th.Bandwidth = bandwidth
	th.Distance = distance
	th.Score = score
}

// CalculateTopologyScore computes a placement score for the topology hint
// based on hop count, bandwidth, and distance metrics. Lower scores indicate
// better placements. This follows the scoring formula from KEP-10002.
func CalculateTopologyScore(hopCount int, bandwidth float64, distance int, dataSize int64) float64 {
	if !EnhancedTopologyHintsEnabled() {
		return 0.0  // Return neutral score when feature disabled
	}
	
	// Base latency constants (following KEP-10002 formula)
	const (
		baseLatency = 10.0  // Local access baseline latency
		hopLatency  = 10.0  // Additional latency per hop
	)
	
	// Calculate latency component based on hop count and distance
	latency := baseLatency + (float64(hopCount) * hopLatency)
	
	// Add distance matrix penalty (distance 10=local, 20=1-hop, 30=2-hop)
	if distance > 10 {
		latency += float64(distance - 10)
	}
	
	// Calculate bandwidth penalty if bandwidth and data size are available
	bandwidthPenalty := 0.0
	if bandwidth > 0 && dataSize > 0 {
		// Convert bandwidth from GB/s to bytes/s and calculate transfer time
		bandwidthBytesPerSecond := bandwidth * 1024 * 1024 * 1024
		bandwidthPenalty = float64(dataSize) / bandwidthBytesPerSecond
	}
	
	// Final score is latency + bandwidth penalty
	return latency + bandwidthPenalty
}

// UpdateScore calculates and sets the score field based on current hint metrics
func (th *TopologyHint) UpdateScore(dataSize int64) {
	if !EnhancedTopologyHintsEnabled() {
		return  // Do nothing if feature disabled
	}
	
	hopCount := th.GetHopCount()
	bandwidth := th.GetBandwidth()
	distance := th.GetDistance()
	
	score := CalculateTopologyScore(hopCount, bandwidth, distance, dataSize)
	th.Score = &score
}

// CreateEnhancedHint creates a new TopologyHint with enhanced topology information
func CreateEnhancedHint(numaAffinity bitmask.BitMask, preferred bool, hopCount int, bandwidth float64, distance int, dataSize int64) TopologyHint {
	hint := TopologyHint{
		NUMANodeAffinity: numaAffinity,
		Preferred:        preferred,
	}
	
	if EnhancedTopologyHintsEnabled() {
		hint.HopCount = &hopCount
		hint.Bandwidth = &bandwidth
		hint.Distance = &distance
		
		// Calculate and set score
		score := CalculateTopologyScore(hopCount, bandwidth, distance, dataSize)
		hint.Score = &score
	}
	
	return hint
}

var _ Manager = &manager{}

// NewManager creates a new TopologyManager based on provided policy and scope
func NewManager(topology []cadvisorapi.Node, topologyPolicyName string, topologyScopeName string, topologyPolicyOptions map[string]string) (Manager, error) {
	// When policy is none, the scope is not relevant, so we can short circuit here.
	if topologyPolicyName == PolicyNone {
		klog.InfoS("Creating topology manager with none policy")
		return &manager{scope: NewNoneScope()}, nil
	}

	opts, err := NewPolicyOptions(topologyPolicyOptions)
	if err != nil {
		return nil, err
	}

	klog.InfoS("Creating topology manager with policy per scope", "topologyPolicyName", topologyPolicyName, "topologyScopeName", topologyScopeName, "topologyPolicyOptions", opts)

	numaInfo, err := NewNUMAInfo(topology, opts)
	if err != nil {
		return nil, fmt.Errorf("cannot discover NUMA topology: %w", err)
	}

	if topologyPolicyName != PolicyNone && len(numaInfo.Nodes) > opts.MaxAllowableNUMANodes {
		return nil, fmt.Errorf("unsupported on machines with more than %v NUMA Nodes", opts.MaxAllowableNUMANodes)
	}

	var policy Policy
	switch topologyPolicyName {

	case PolicyBestEffort:
		policy = NewBestEffortPolicy(numaInfo, opts)

	case PolicyRestricted:
		policy = NewRestrictedPolicy(numaInfo, opts)

	case PolicySingleNumaNode:
		policy = NewSingleNumaNodePolicy(numaInfo, opts)

	case PolicyDistributed:
		policy = NewDistributedPolicy(numaInfo, opts)

	default:
		return nil, fmt.Errorf("unknown policy: \"%s\"", topologyPolicyName)
	}

	var scope Scope
	switch topologyScopeName {

	case containerTopologyScope:
		scope = NewContainerScope(policy)

	case podTopologyScope:
		scope = NewPodScope(policy)

	default:
		return nil, fmt.Errorf("unknown scope: \"%s\"", topologyScopeName)
	}

	manager := &manager{
		scope: scope,
	}

	manager.initializeMetrics()

	return manager, nil
}

func (m *manager) initializeMetrics() {
	// ensure the values exist
	metrics.ContainerAlignedComputeResources.WithLabelValues(metrics.AlignScopeContainer, metrics.AlignedNUMANode).Add(0)
	metrics.ContainerAlignedComputeResources.WithLabelValues(metrics.AlignScopePod, metrics.AlignedNUMANode).Add(0)
	metrics.ContainerAlignedComputeResourcesFailure.WithLabelValues(metrics.AlignScopeContainer, metrics.AlignedNUMANode).Add(0)
	metrics.ContainerAlignedComputeResourcesFailure.WithLabelValues(metrics.AlignScopePod, metrics.AlignedNUMANode).Add(0)
}

func (m *manager) GetAffinity(podUID string, containerName string) TopologyHint {
	return m.scope.GetAffinity(podUID, containerName)
}

func (m *manager) GetPolicy() Policy {
	return m.scope.GetPolicy()
}

func (m *manager) AddHintProvider(h HintProvider) {
	m.scope.AddHintProvider(h)
}

func (m *manager) AddContainer(pod *v1.Pod, container *v1.Container, containerID string) {
	m.scope.AddContainer(pod, container, containerID)
}

func (m *manager) RemoveContainer(containerID string) error {
	return m.scope.RemoveContainer(containerID)
}

func (m *manager) Admit(attrs *lifecycle.PodAdmitAttributes) lifecycle.PodAdmitResult {
	klog.V(4).InfoS("Topology manager admission check", "pod", klog.KObj(attrs.Pod))
	metrics.TopologyManagerAdmissionRequestsTotal.Inc()

	startTime := time.Now()
	podAdmitResult := m.scope.Admit(attrs.Pod)
	metrics.TopologyManagerAdmissionDuration.Observe(float64(time.Since(startTime).Milliseconds()))

	klog.V(4).InfoS("Pod Admit Result", "Message", podAdmitResult.Message, "pod", klog.KObj(attrs.Pod))
	return podAdmitResult
}
