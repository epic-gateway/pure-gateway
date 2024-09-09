package gateway

import (
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RefreshCondition(meta *meta_v1.ObjectMeta, condition *meta_v1.Condition) *meta_v1.Condition {
	condition.ObservedGeneration = meta.Generation
	condition.LastTransitionTime = meta_v1.NewTime(time.Now())
	return condition
}
