package status

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MergeConditions adds or updates matching conditions, and updates the transition
// time if details of a condition have changed. Returns the updated condition array.
func MergeConditions(conditions []meta_v1.Condition, updates ...meta_v1.Condition) []meta_v1.Condition {
	return mergeConditions(conditions, updates...)
}
