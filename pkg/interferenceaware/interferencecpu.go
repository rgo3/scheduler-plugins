package interferenceaware

import (
	"context"
	"fmt"
	"math"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// InterferenceCPU is a score plugin that favors nodes based on their allocatable
// resources.
type InterferenceCPU struct {
	handle  framework.Handle
	metrics map[string]float64
}

var _ = framework.ScorePlugin(&InterferenceCPU{})

// Name is the name of the plugin used in the Registry and configurations.
const InterferenceCPUName = "InterferenceCPU"

// Name returns name of the plugin. It is used in logs, etc.
func (ic *InterferenceCPU) Name() string {
	return InterferenceCPUName
}

// New initializes a new plugin and returns it.
func NewInterferenceCPU(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	im, err := populateInterferenceMetrics("/etc/interferencemetrics", "cpu")
	if err != nil {
		return nil, err
	}

	return &InterferenceCPU{
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
func (ic *InterferenceCPU) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	iscore := 0.0
	/*
		pods, err := ic.handle.ClientSet().CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + nodeName,
		}	)
			if err != nil {
				return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting pods from node %s: %v", nodeName, err))
			}*/

	nodeInfo, err := ic.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}

	klog.V(1).Infof("[InterferenceCPU] collecting scores on node %s", nodeName)
	for _, pi := range nodeInfo.Pods {
		p := pi.Pod
		klog.V(1).Infof("[InterferenceCPU] %s", p.GetName())
		labels := p.GetObjectMeta().GetLabels()

		taskName, ok := labels["nextflow.io/taskName"]
		if !ok {
			klog.V(1).Info("[InterferenceCPU] skipping...")
			continue
		}
		key := sanitizeTaskName(taskName)
		klog.V(2).Infof("[InterferenceCPU] interference map key %s:", key)

		if metric, ok := ic.metrics[key]; ok {
			iscore += metric
		}
	}

	score := int64(math.Round(iscore))
	klog.V(1).Infof("[InterferenceCPU] score for node %s when scheduling %s: %d", nodeName, pod.GetName(), score)
	return score, nil
}

// ScoreExtensions of the Score plugin.
func (ic *InterferenceCPU) ScoreExtensions() framework.ScoreExtensions {
	return ic
}

// NormalizeScore invoked after scoring all nodes.
func (ic *InterferenceCPU) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
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

	klog.V(1).Infof("[InterferenceCPU] Nodes final score: %v", scores)
	return nil
}
