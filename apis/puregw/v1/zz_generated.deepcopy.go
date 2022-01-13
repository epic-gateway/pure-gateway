//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2022 Acnodal.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EPIC) DeepCopyInto(out *EPIC) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EPIC.
func (in *EPIC) DeepCopy() *EPIC {
	if in == nil {
		return nil
	}
	out := new(EPIC)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayClassConfig) DeepCopyInto(out *GatewayClassConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayClassConfig.
func (in *GatewayClassConfig) DeepCopy() *GatewayClassConfig {
	if in == nil {
		return nil
	}
	out := new(GatewayClassConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayClassConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayClassConfigList) DeepCopyInto(out *GatewayClassConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GatewayClassConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayClassConfigList.
func (in *GatewayClassConfigList) DeepCopy() *GatewayClassConfigList {
	if in == nil {
		return nil
	}
	out := new(GatewayClassConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayClassConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayClassConfigSpec) DeepCopyInto(out *GatewayClassConfigSpec) {
	*out = *in
	if in.EPIC != nil {
		in, out := &in.EPIC, &out.EPIC
		*out = new(EPIC)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayClassConfigSpec.
func (in *GatewayClassConfigSpec) DeepCopy() *GatewayClassConfigSpec {
	if in == nil {
		return nil
	}
	out := new(GatewayClassConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayClassConfigStatus) DeepCopyInto(out *GatewayClassConfigStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayClassConfigStatus.
func (in *GatewayClassConfigStatus) DeepCopy() *GatewayClassConfigStatus {
	if in == nil {
		return nil
	}
	out := new(GatewayClassConfigStatus)
	in.DeepCopyInto(out)
	return out
}
