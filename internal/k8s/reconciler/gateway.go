package reconciler

import (
	"reflect"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/internal/state"
	"github.com/hashicorp/go-hclog"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type K8sGateway struct {
	logger         hclog.Logger
	controllerName string

	gateway   *gw.Gateway
	listeners map[string]*K8sListener
}

var _ state.Gateway = &K8sGateway{}

type K8sGatewayConfig struct {
	Logger         hclog.Logger
	Client         gatewayclient.Client
	ControllerName string
}

func NewK8sGateway(gateway *gw.Gateway, config K8sGatewayConfig) *K8sGateway {
	gatewayLogger := config.Logger.Named("gateway").With("name", gateway.Name, "namespace", gateway.Namespace)
	listeners := make(map[string]*K8sListener)
	for _, listener := range gateway.Spec.Listeners {
		k8sListener := NewK8sListener(gateway, listener, K8sListenerConfig{
			Logger: gatewayLogger,
			Client: config.Client,
		})
		listeners[k8sListener.ID()] = k8sListener
	}

	return &K8sGateway{
		logger:         gatewayLogger,
		controllerName: config.ControllerName,
		gateway:        gateway,
		listeners:      listeners,
	}
}
func (g *K8sGateway) ID() string {
	return utils.NamespacedName(g.gateway).String()
}

func (g *K8sGateway) Logger() hclog.Logger {
	return g.logger
}

func (g *K8sGateway) Name() string {
	return g.gateway.Name
}

func (g *K8sGateway) Namespace() string {
	return g.gateway.Namespace
}

func (g *K8sGateway) Meta() map[string]string {
	return map[string]string{
		"managed_by":                               "consul-api-gateway",
		"consul-api-gateway/k8s/Gateway.Name":      g.gateway.Name,
		"consul-api-gateway/k8s/Gateway.Namespace": g.gateway.Namespace,
	}
}

func (g *K8sGateway) IsMoreRecent(other state.Gateway) bool {
	if otherGateway, ok := other.(*K8sGateway); ok {
		return g.gateway.Generation > otherGateway.gateway.Generation
	}
	return false
}

func (g *K8sGateway) Listeners() []state.Listener {
	listeners := []state.Listener{}

	for _, listener := range g.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

func (g *K8sGateway) Equals(other state.Gateway) bool {
	if otherGateway, ok := other.(*K8sGateway); ok {
		return reflect.DeepEqual(g.gateway.Spec, otherGateway.gateway.Spec)
	}
	return false
}

func (g *K8sGateway) Secrets() []string {
	secrets := []string{}
	for _, listener := range g.gateway.Spec.Listeners {
		if listener.TLS != nil {
			ref := listener.TLS.CertificateRef
			if ref != nil {
				n := ref.Namespace
				namespace := "default"
				if n != nil {
					namespace = string(*n)
				}
				secrets = append(secrets, utils.NewK8sSecret(namespace, ref.Name).String())
			}
		}
	}
	return secrets
}

func (g *K8sGateway) ShouldBind(route state.Route) bool {
	k8sRoute, ok := route.(*K8sRoute)
	if !ok {
		return false
	}
	for _, ref := range k8sRoute.CommonRouteSpec().ParentRefs {
		if namespacedName, isGateway := referencesGateway(k8sRoute.GetNamespace(), ref); isGateway {
			if utils.NamespacedName(g.gateway) == namespacedName {
				return true
			}
		}
	}

	return false
}
