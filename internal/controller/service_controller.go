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
	goUnifi "github.com/paultyng/go-unifi/unifi"
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
	portForwardExists := false
	var portForwardID string
	for _, portForward := range portForwards {
		if portForward.Name != portForwardName {
			continue
		}
		portForwardExists = true
		portForwardID = portForward.ID

		if deletePortForward {
			log.Info(fmt.Sprintf("Deleting port forward: %s", portForwardName))
			if err := r.UnifiClient.Client.DeletePortForward(context.Background(), r.UnifiClient.SiteID, portForwardID); err != nil {
				log.Error(err, fmt.Sprintf("Failed to delete port forward: %s", portForwardName))
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
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
		portForward := &goUnifi.PortForward{
			Name:          portForwardName,
			ID:            portForwardID,
			SiteID:        r.UnifiClient.SiteID,
			DestinationIP: labelValues["dest-ip"],
			DstPort:       labelValues["dest-port"],
			FwdPort:       labelValues["fwd-port"],
			Fwd:           lbIP,
		}
		portForwardResult, err := r.UnifiClient.Client.UpdatePortForward(context.Background(), r.UnifiClient.SiteID, portForward)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to update port forward: %s", portForwardName))
			return ctrl.Result{}, err
		}
		log.Info(fmt.Sprintf("Updated port forward: %s", portForwardResult.Name))
	} else if !portForwardExists && len(labelValues) == len(operatorLabels) {
		log.Info(fmt.Sprintf("Creating port forward: %s", portForwardName))
		portForward := &goUnifi.PortForward{
			Name:          portForwardName,
			SiteID:        r.UnifiClient.SiteID,
			DestinationIP: labelValues["dest-ip"],
			DstPort:       labelValues["dest-port"],
			FwdPort:       labelValues["fwd-port"],
			Fwd:           lbIP,
		}

		portForwardResult, err := r.UnifiClient.Client.CreatePortForward(context.Background(), r.UnifiClient.SiteID, portForward)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to create port forward: %s", portForwardName))
			return ctrl.Result{}, err
		}

		log.Info(fmt.Sprintf("Created port forward: %s", portForwardResult.Name))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
