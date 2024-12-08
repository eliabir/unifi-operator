/*
Copyright 2024.

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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/eliabir/unifi-operator/internal/unifi"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	UnifiClient *unifi.UnifiClient
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update

const operatorLabelPrefix = "unifi-port-forward"

var operatorLabels []string = []string{"dest-ip", "dest-port", "fwd-port"}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)
	log := log.FromContext(ctx)

	// Get service object
	var service corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		log.Error(err, "unable to fetch Service")

		return ctrl.Result{}, err
	}

	// Check if service is type loadbalancer
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return ctrl.Result{}, nil
	}

	portForwardName := fmt.Sprintf("k8s-operator - %s - %s", service.Namespace, service.Name)

	log.Info("Fetching all port forwards")
	portForwards, err := r.UnifiClient.Client.ListPortForward(context.Background(), r.UnifiClient.SiteID)
	if err != nil {
		log.Error(err, "unable to list port forwards")

		return ctrl.Result{}, err
	}

	deletePortForward := false
	labelValues := make(map[string]string)
	for _, label := range operatorLabels {
		fullLabel := fmt.Sprintf("%s/%s", operatorLabelPrefix, label)
		if svcLabel, ok := service.Labels[fullLabel]; ok {
			labelValues[label] = svcLabel
		} else {
			deletePortForward = true
			break
		}
	}

	updatePortForward := false
	for _, portForward := range portForwards {
		if portForward.Name != portForwardName {
			continue
		}

		if deletePortForward {
			log.Info(fmt.Sprintf("Deleting port forward: %s", portForwardName))
			return ctrl.Result{}, err
		}

		if portForward.DestinationIP != labelValues["dest-ip"] || portForward.FwdPort != labelValues["fwd-port"] || portForward.DstPort != labelValues["dest-port"] {
			updatePortForward = true
			break
		} else {
			log.Info("Port forward is in sync")
			return ctrl.Result{}, nil
		}
	}

	// Fetch loadbalancerIP
	var lbIP string
	if len(service.Status.LoadBalancer.Ingress) > 0 {
		lbIP = service.Status.LoadBalancer.Ingress[0].IP
		log.Info(fmt.Sprintf("IP of fetched LoadBalancer service: %s", lbIP))
	}

	if updatePortForward {
		log.Info(fmt.Sprintf("Updating port forward: %s", portForwardName))
	} else {
		log.Info(fmt.Sprintf("Creating port forward: %s", portForwardName))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
