package descheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/gengo/examples/set-gen/sets"
	"k8s.io/klog/v2"

	"github.com/karmada-io/karmada/pkg/descheduler/core"
)

var orderKey = "cluster-order"

func (d *Descheduler) updateScheduleResultV2(h *core.SchedulingResultHelper) error {
	message := descheduleSuccessMessage
	binding := h.ResourceBinding.DeepCopy()
	undesiredClusterInfos, undesiredClusterNames := h.GetUndesiredClustersV2()

	klog.InfoS("undesired cluster info", "clusters", undesiredClusterNames)
	//undesiredClusterSet := sets.NewString(undesiredClusters...)
	_, satisfyClusters := h.GetSatisfyClusters()
	satisfyClusterSet := sets.NewString(satisfyClusters...)
	var oldOrderClusters []string
	var newOrderClusters []string
	if binding.Annotations != nil {
		if val, ok := binding.Annotations[orderKey]; ok {
			oldOrderClusters = strings.Split(val, ",")
		}
	}

	if len(oldOrderClusters) == 0 {
		newOrderClusters = satisfyClusters
		newOrderClusters = append(newOrderClusters, undesiredClusterNames...)
	} else {
		newOrderClusters = make([]string, 0)
		for i, tmp := range oldOrderClusters {
			if i == 0 {
				continue
			}
			if satisfyClusterSet.Has(tmp) {
				newOrderClusters = append(newOrderClusters, tmp)
			}
		}
		// 处理新增的cluster
		newAdd := satisfyClusterSet.Difference(sets.NewString(oldOrderClusters...))
		if newAdd != nil && newAdd.Len() > 0 {
			newOrderClusters = append(newOrderClusters, newAdd.List()...)
		}
		// 第0个，放到最后
		if satisfyClusterSet.Has(oldOrderClusters[0]) {
			newOrderClusters = append(newOrderClusters, oldOrderClusters[0])
		}
	}

	deCount := int32(0)
	for i, tmp := range binding.Spec.Clusters {
		if clusterWrap, ok := undesiredClusterInfos[tmp.Name]; ok {
			binding.Spec.Clusters[i].Replicas = clusterWrap.Ready
			deCount += clusterWrap.Spec - clusterWrap.Ready
			message += fmt.Sprintf(", cluster %s from %d to %d", tmp.Name, clusterWrap.Spec, clusterWrap.Ready)
		}
	}

	for i, tmp := range binding.Spec.Clusters {
		if tmp.Name == newOrderClusters[0] {
			binding.Spec.Clusters[i].Replicas += deCount
		}
	}

	if binding.Annotations == nil {
		binding.Annotations = map[string]string{}
	}
	binding.Annotations[orderKey] = strings.Join(newOrderClusters, ",")

	if deCount == 0 {
		return nil
	}

	message += fmt.Sprintf(", %d total descheduled replica(s)", deCount)

	var err error
	defer func() {
		d.recordDescheduleResultEventForResourceBinding(binding, message, err)
	}()

	binding, err = d.KarmadaClient.WorkV1alpha2().ResourceBindings(binding.Namespace).Update(context.TODO(), binding, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (d *Descheduler) updateScheduleResultV3(h *core.SchedulingResultHelper) error {
	message := descheduleSuccessMessage
	binding := h.ResourceBinding.DeepCopy()
	undesiredClusterInfos, undesiredClusterNames := h.GetUndesiredClustersV2()

	klog.InfoS("undesired cluster info", "clusters", undesiredClusterNames)

	allClusters := make([]string, 0)
	for _, tmp := range h.TargetClusters {
		allClusters = append(allClusters, tmp.ClusterName)
	}

	scheduleAnno := ""
	if binding.Annotations != nil {
		if val, ok := binding.Annotations[orderKey]; ok {
			scheduleAnno = val
		}
	}

	newSchedulerInfo, newChooseCluster, found := procSchedule(allClusters, scheduleAnno, undesiredClusterNames)

	if !found {
		klog.V(4).InfoS("descheduler not update", "ns", h.Namespace, "name", h.Name)
		return nil
	}

	klog.InfoS("descheduler", "ns", h.Namespace, "name", h.Name, "chooseCluster", newChooseCluster, "oldAnno", scheduleAnno, "newAnno", newSchedulerInfo)

	deCount := int32(0)
	for i, tmp := range binding.Spec.Clusters {
		if clusterWrap, ok := undesiredClusterInfos[tmp.Name]; ok {
			binding.Spec.Clusters[i].Replicas = clusterWrap.Ready
			deCount += clusterWrap.Spec - clusterWrap.Ready
			message += fmt.Sprintf(", cluster %s from %d to %d", tmp.Name, clusterWrap.Spec, clusterWrap.Ready)
		}
	}

	for i, tmp := range binding.Spec.Clusters {
		if tmp.Name == newChooseCluster {
			binding.Spec.Clusters[i].Replicas += deCount
		}
	}

	if binding.Annotations == nil {
		binding.Annotations = map[string]string{}
	}
	binding.Annotations[orderKey] = newSchedulerInfo

	if deCount == 0 {
		return nil
	}

	message += fmt.Sprintf(", %d total descheduled replica(s)", deCount)

	var err error
	defer func() {
		d.recordDescheduleResultEventForResourceBinding(binding, message, err)
	}()

	klog.V(4).InfoS("descheduler prepare update", "ns", h.Namespace, "name", h.Name, "mesage", message)

	binding, err = d.KarmadaClient.WorkV1alpha2().ResourceBindings(binding.Namespace).Update(context.TODO(), binding, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func procSchedule(all []string, scheduleInfo string, undesireInfo []string) (string, string, bool) {
	undesireClusterSet := sets.NewString(undesireInfo...)
	allSet := sets.NewString(all...)

	clusterTimes := parseSchedule(scheduleInfo)
	// 处理新增集群
	for _, tmp := range all {
		if _, ok := clusterTimes[tmp]; !ok {
			clusterTimes[tmp] = 0
		}
	}

	for k := range clusterTimes {
		if !allSet.Has(k) {
			delete(clusterTimes, k)
		}
	}

	// 找最久未调度的
	newDeCluster := ""
	keys := make([]string, 0, len(clusterTimes))
	for k := range clusterTimes {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))
	for _, k := range keys {
		if undesireClusterSet.Has(k) {
			continue
		}

		if time.Now().Unix()-clusterTimes[k] < 5*60 {
			continue
		}

		newDeCluster = k
		break
	}

	// 没找到合适集群
	if len(newDeCluster) == 0 {
		return "", "", false
	}

	clusterTimes[newDeCluster] = time.Now().Unix()

	deInfo := encodeSchedule(clusterTimes)

	return deInfo, newDeCluster, true
}

type DescheduleInfo struct {
	Clusters []ClusterTimeItem `json:"clusters"`
}

type ClusterTimeItem struct {
	Cluster string `json:"cluster"`
	Time    int64  `json:"time"`
}

func parseSchedule(scheduleInfo string) map[string]int64 {
	var info DescheduleInfo
	clusterTime := map[string]int64{}

	if len(scheduleInfo) == 0 {
		return clusterTime
	}
	if err := json.Unmarshal([]byte(scheduleInfo), &info); err != nil {
		return clusterTime
	}

	for _, tmp := range info.Clusters {
		clusterTime[tmp.Cluster] = tmp.Time
	}
	return clusterTime
}

func encodeSchedule(info map[string]int64) string {
	var de DescheduleInfo
	clusters := make([]ClusterTimeItem, 0)
	for k, v := range info {
		clusters = append(clusters, ClusterTimeItem{
			Cluster: k,
			Time:    v,
		})
	}
	de.Clusters = clusters
	str, _ := json.Marshal(de)
	return string(str)
}
