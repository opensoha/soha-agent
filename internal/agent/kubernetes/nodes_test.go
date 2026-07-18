package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildNodeDetailIncludesTaints(t *testing.T) {
	detail := buildNodeDetail(corev1.Node{
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
			Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule,
		}}},
	}, nil)

	if len(detail.Taints) != 1 || detail.Taints[0].Key != "dedicated" || detail.Taints[0].Effect != "NoSchedule" {
		t.Fatalf("unexpected taints: %#v", detail.Taints)
	}
}
