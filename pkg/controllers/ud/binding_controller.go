package ud

import (
	"context"
	"github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/pkg/sharedcli/ratelimiterflag"
	"github.com/karmada-io/karmada/pkg/util/gclient"
	v1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ControllerName is the controller name that will be used when reporting events.
const ControllerName = "united-deployment-controller"

type UnitedDeploymentController struct {
	client.Client      // used to operate ClusterResourceBinding resources.
	EventRecorder      record.EventRecorder
	RateLimiterOptions ratelimiterflag.Options
}

// Reconcile performs a full reconciliation for the object referred to by the Request.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (c *UnitedDeploymentController) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	klog.V(4).Infof("Reconciling UnitedDeployment %s.", req.NamespacedName.String())

	deploy := &v1.Deployment{}
	if err := c.Client.Get(ctx, req.NamespacedName, deploy); err != nil {
		// The resource no longer exist, in which case we stop processing.
		if apierrors.IsNotFound(err) {
			klog.InfoS("deployment is null", "name", req.NamespacedName.String())
			return controllerruntime.Result{}, nil
		}
		klog.ErrorS(err, "fail to get deploy", "name", req.NamespacedName.String())
		return controllerruntime.Result{Requeue: true}, err
	}
	klog.V(4).InfoS("get deploy succ", "name", deploy.Name)
	clusters := &v1alpha1.ClusterList{}
	if err := c.Client.List(ctx, clusters); err != nil {
		klog.ErrorS(err, "failed to list clusters")
		return controllerruntime.Result{Requeue: true}, err
	}
	clusterNames := sets.NewString()
	for _, tmp := range clusters.Items {
		clusterNames.Insert(tmp.Name)
	}
	klog.V(4).InfoS("list clusters", "names", clusterNames.List())

	policy := &policyv1alpha1.PropagationPolicy{}
	found := true
	if err := c.Client.Get(ctx, req.NamespacedName, policy); err != nil {
		// The resource no longer exist, in which case we stop processing.
		if apierrors.IsNotFound(err) {
			found = false
		} else {
			return controllerruntime.Result{Requeue: true}, err
		}
	}

	klog.V(4).InfoS("get PropagationPolicy", "found", found)

	if found {
		newPolicy := genTemplate(deploy, clusterNames)
		if checkEqual(policy, newPolicy) {
			return controllerruntime.Result{}, nil
		}
		policy.Spec = newPolicy.Spec
		controllerutil.SetOwnerReference(deploy, policy, gclient.NewSchema())

		if err := c.Client.Update(ctx, policy); err != nil {
			return controllerruntime.Result{Requeue: true}, err
		}
		return controllerruntime.Result{}, nil

	} else {
		newPolicy := genTemplate(deploy, clusterNames)
		controllerutil.SetOwnerReference(deploy, newPolicy, gclient.NewSchema())
		if err := c.Client.Create(ctx, newPolicy); err != nil {
			klog.ErrorS(err, "create policy fail", "name", deploy.Name)
			return controllerruntime.Result{Requeue: true}, err
		}
		return controllerruntime.Result{}, nil
	}

}

// SetupWithManager creates a controller and register to controller manager.
func (c *UnitedDeploymentController) SetupWithManager(mgr controllerruntime.Manager) error {
	return controllerruntime.NewControllerManagedBy(mgr).For(&v1.Deployment{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				if e.Object != nil && e.Object.GetLabels() != nil && len(e.Object.GetLabels()["apps.kruise.io/subset-name"]) != 0 {
					return true
				}
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew != nil && e.ObjectNew.GetLabels() != nil && len(e.ObjectNew.GetLabels()["apps.kruise.io/subset-name"]) != 0 {
					return true
				}
				return false
			},

			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}).
		Watches(&policyv1alpha1.PropagationPolicy{}, handler.EnqueueRequestsFromMapFunc(c.newOverridePolicyFunc())).
		WithOptions(controller.Options{RateLimiter: ratelimiterflag.DefaultControllerRateLimiter(c.RateLimiterOptions)}).
		Complete(c)
}

func genTemplate(deploy *v1.Deployment, clusterNames sets.String) *policyv1alpha1.PropagationPolicy {

	policy := &policyv1alpha1.PropagationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
		},
		Spec: policyv1alpha1.PropagationSpec{
			PropagateDeps: true,
			ResourceSelectors: []policyv1alpha1.ResourceSelector{
				policyv1alpha1.ResourceSelector{
					APIVersion: deploy.APIVersion,
					Kind:       deploy.Kind,
					Namespace:  deploy.Namespace,
					Name:       deploy.Name,
				},
			},
			Placement: policyv1alpha1.Placement{
				ClusterAffinities: []policyv1alpha1.ClusterAffinityTerm{
					{
						AffinityName: "group1",
						ClusterAffinity: policyv1alpha1.ClusterAffinity{

							ClusterNames: clusterNames.List(),
						},
					},
				},
				ReplicaScheduling: &policyv1alpha1.ReplicaSchedulingStrategy{
					ReplicaSchedulingType:     policyv1alpha1.ReplicaSchedulingTypeDivided,
					ReplicaDivisionPreference: policyv1alpha1.ReplicaDivisionPreferenceWeighted,
				},
			},
		},
	}

	return policy

}

func checkEqual(old, new *policyv1alpha1.PropagationPolicy) bool {
	if !apiequality.Semantic.DeepEqual(old.Spec, new.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(old.OwnerReferences, new.OwnerReferences) {
		return false
	}
	return true
}

func (c *UnitedDeploymentController) newOverridePolicyFunc() handler.MapFunc {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		var requests []reconcile.Request
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: a.GetNamespace(), Name: a.GetName()}})
		return requests
	}
}
