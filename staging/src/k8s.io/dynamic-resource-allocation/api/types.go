/*
Copyright 2024 The Kubernetes Authors.

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

package api

import (
	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceSlice struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec ResourceSliceSpec
}

type ResourceSliceSpec struct {
	Driver                 UniqueString
	Pool                   ResourcePool
	NodeName               *string
	NodeSelector           *v1.NodeSelector
	AllNodes               bool
	Devices                []Device
	PerDeviceNodeSelection *bool
	SharedCounters         []CounterSet
	NodeTopology           *NodeTopologyInfo
}

// NodeTopologyInfo describes NUMA topology for resources in a ResourceSlice.
// This enables topology-aware resource allocation and placement decisions.
type NodeTopologyInfo struct {
	// NodeID identifies the NUMA node that provides the resources.
	NodeID int32
	// Resources contains the available resource quantities per NUMA node.
	Resources map[string]int64
	// Properties contains NUMA-specific properties for the resources.
	Properties map[string]string
}

type CounterSet struct {
	Name     UniqueString
	Counters map[string]Counter
}

type ResourcePool struct {
	Name               UniqueString
	Generation         int64
	ResourceSliceCount int64
}
type Device struct {
	Name                     UniqueString
	Attributes               map[QualifiedName]DeviceAttribute
	Capacity                 map[QualifiedName]DeviceCapacity
	ConsumesCounters         []DeviceCounterConsumption
	NodeName                 *string
	NodeSelector             *v1.NodeSelector
	AllNodes                 *bool
	Taints                   []resourceapi.DeviceTaint
	BindsToNode              bool
	BindingConditions        []string
	BindingFailureConditions []string
	AllowMultipleAllocations *bool
}

type DeviceCounterConsumption struct {
	CounterSet UniqueString
	Counters   map[string]Counter
}

type QualifiedName string

type FullyQualifiedName string

type DeviceAttribute struct {
	IntValue     *int64
	BoolValue    *bool
	StringValue  *string
	VersionValue *string
}

type DeviceCapacity struct {
	Value         resource.Quantity
	RequestPolicy *CapacityRequestPolicy
}

type CapacityRequestPolicy struct {
	Default     *resource.Quantity
	ValidValues []resource.Quantity
	ValidRange  *CapacityRequestPolicyRange
}

type CapacityRequestPolicyRange struct {
	Min  *resource.Quantity
	Max  *resource.Quantity
	Step *resource.Quantity
}

type Counter struct {
	Value resource.Quantity
}

type DeviceTaint struct {
	Key       string
	Value     string
	Effect    DeviceTaintEffect
	TimeAdded *metav1.Time
}

type DeviceTaintEffect string

const (
	DeviceTaintEffectNoSchedule DeviceTaintEffect = "NoSchedule"

	DeviceTaintEffectNoExecute DeviceTaintEffect = "NoExecute"
)
