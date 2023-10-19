/*
Copyright 2020 The Kubernetes Authors.

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

package interferenceaware

import (
	"context"
	"fmt"
	"math"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// InterferenceBIO is a score plugin that favors nodes based on their allocatable
// resources.
type InterferenceBIO struct {
	handle  framework.FrameworkHandle
	metrics map[string]float64
}

var _ = framework.ScorePlugin(&InterferenceBIO{})

// Name is the name of the plugin used in the Registry and configurations.
const InterferenceBIOName = "InterferenceBIO"

// Name returns name of the plugin. It is used in logs, etc.
func (ib *InterferenceBIO) Name() string {
	return InterferenceBIOName
}

// New initializes a new plugin and returns it.
func NewInterferenceBIO(_ runtime.Object, h framework.FrameworkHandle) (framework.Plugin, error) {
	im, err := populateInterferenceMetrics("/etc/interferencemetrics", "bio")
	if err != nil {
		return nil, err
	}

	return &InterferenceBIO{
		handle:  h,
		metrics: im,
	}, nil
}

// Score invoked at the score extension point.
//
// Get pods on node
// for each pod:
//
//	get nextflowio/taskName label
//	aggregate all interference value for configured resource
//	turn aggregated score
func (ib *InterferenceBIO) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	iscore := 0.0

	/*
		pods, err := ic.handle.ClientSet().CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + nodeName,
		}	)
			if err != nil {
				return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting pods from node %s: %v", nodeName, err))
			}*/

	nodeInfo, err := ib.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	klog.V(1).Infof("[InterferenceBIO] collecting scores on node %s", nodeName)
	for _, pi := range nodeInfo.Pods {
		p := pi.Pod
		klog.V(1).Infof("[InterferenceBIO] %s", p.GetName())
		labels := p.GetObjectMeta().GetLabels()
		taskName, ok := labels["nextflow.io/taskName"]
		if !ok {
			klog.V(1).Info("[InterferenceBIO] skipping...")
			continue
		}
		key := sanitizeTaskName(taskName)
		klog.V(2).Infof("[InterferenceBIO] interference map key %s:", key)

		if metric, ok := ib.metrics[key]; ok {
			iscore += metric
		}
	}

	score := int64(math.Round(iscore))
	klog.V(1).Infof("[InterferenceBIO] score for node %s when scheduling %s: %d", nodeName, pod.GetName(), score)
	return score, nil
}

// ScoreExtensions of the Score plugin.
func (ib *InterferenceBIO) ScoreExtensions() framework.ScoreExtensions {
	return ib
}

// NormalizeScore invoked after scoring all nodes.
func (ib *InterferenceBIO) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	var higherScore int64
	for _, node := range scores {
		if higherScore < node.Score {
			higherScore = node.Score
		}
	}
	if higherScore > 0 {
		for i, node := range scores {
			scores[i].Score = framework.MaxNodeScore - (node.Score * framework.MaxNodeScore / higherScore)
		}
	}

	klog.V(1).Infof("[InterferenceBIO] Nodes final score: %v", scores)
	return nil
}
