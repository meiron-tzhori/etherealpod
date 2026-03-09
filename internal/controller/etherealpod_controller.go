/*
Copyright 2026.

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

package controller

import (
	appsv1alpha1 "ascendra.local/etherealpod-operator/api/v1alpha1"
	"context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
)

const (
	epPodLabelKey = "apps.ascendra.local/etherealpod"
)

// EtherealPodReconciler reconciles a EtherealPod object
type EtherealPodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.ascendra.local,resources=etherealpods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.ascendra.local,resources=etherealpods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.ascendra.local,resources=etherealpods/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EtherealPod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *EtherealPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	var ep appsv1alpha1.EtherealPod
	if err := r.Get(ctx, req.NamespacedName, &ep); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !ep.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	// after fetching ep successfully...

	podName := ep.Name + "-pod"
	selector := client.MatchingLabels{epPodLabelKey: ep.Name}

	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(ep.Namespace),
		selector,
	); err != nil {
		return ctrl.Result{}, err
	}

	logf.FromContext(ctx).Info("listed backing pods", "count", len(podList.Items), "ep", ep.Name)
	// ✅ Step 3.3 - enforce exactly one backing pod (create/delete extras)
	if len(podList.Items) == 0 {
		// Create desired Pod from spec.template
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: ep.Namespace,
				Labels:    map[string]string{},
			},
			Spec: ep.Spec.Template.Spec,
		}

		// Copy labels from template + enforce our selector label
		for k, v := range ep.Spec.Template.Labels {
			pod.Labels[k] = v
		}
		pod.Labels[epPodLabelKey] = ep.Name

		// OwnerRef (so pod is deleted when CR is deleted)
		if err := controllerutil.SetControllerReference(&ep, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, pod); err != nil {
			// If a race created it already, just requeue
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}

		logf.FromContext(ctx).Info("created backing pod", "pod", podName, "ep", ep.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	if len(podList.Items) > 1 {
		// Keep oldest, delete the rest
		pods := podList.Items
		sort.Slice(pods, func(i, j int) bool {
			return pods[i].CreationTimestamp.Time.Before(pods[j].CreationTimestamp.Time)
		})

		primary := pods[0]
		extras := pods[1:]

		for i := range extras {
			p := extras[i]
			_ = r.Delete(ctx, &p) // best-effort; we'll converge next reconcile
		}

		logf.FromContext(ctx).Info("deleted extra backing pods", "kept", primary.Name, "deleted", len(extras), "ep", ep.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// len == 1
	pod := podList.Items[0]

	if pod.DeletionTimestamp != nil ||
		pod.Status.Phase == corev1.PodFailed ||
		pod.Status.Phase == corev1.PodSucceeded {

		if err := r.Delete(ctx, &pod); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}

		logf.FromContext(ctx).Info("deleted terminal/terminating backing pod", "pod", pod.Name, "phase", pod.Status.Phase, "ep", ep.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// ✅ Step 3.5 - compute restarts + update EP status

	var restarts int32 = 0
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	// Always update derived status (no change detection logic)
	ep.Status.PodName = pod.Name
	ep.Status.Restarts = restarts

	if err := r.Status().Update(ctx, &ep); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	logf.FromContext(ctx).Info("updated EP status",
		"pod", pod.Name,
		"restarts", restarts,
		"ep", ep.Name,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtherealPodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.EtherealPod{}).
		Owns(&corev1.Pod{}).
		WatchesRawSource(
			source.Kind(
				mgr.GetCache(),
				&corev1.Pod{},
				handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
					epName := pod.Labels[epPodLabelKey]
					if epName == "" {
						return nil
					}
					return []reconcile.Request{{
						NamespacedName: types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      epName,
						},
					}}
				}),
			),
		).
		Complete(r)
}
