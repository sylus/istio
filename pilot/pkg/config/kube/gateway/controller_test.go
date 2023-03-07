// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "sigs.k8s.io/gateway-api/apis/v1alpha2"
	k8sbeta "sigs.k8s.io/gateway-api/apis/v1beta1"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/memory"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/networking/core/v1alpha3"
	"istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/util/sets"
)

var (
	gatewayClassSpec = &k8s.GatewayClassSpec{
		ControllerName: ControllerName,
	}
	gatewaySpec = &k8s.GatewaySpec{
		GatewayClassName: "gwclass",
		Listeners: []k8s.Listener{
			{
				Name:          "default",
				Port:          9009,
				Protocol:      "HTTP",
				AllowedRoutes: &k8s.AllowedRoutes{Namespaces: &k8s.RouteNamespaces{From: func() *k8s.FromNamespaces { x := k8sbeta.NamespacesFromAll; return &x }()}},
			},
		},
	}
	httpRouteSpec = &k8s.HTTPRouteSpec{
		CommonRouteSpec: k8s.CommonRouteSpec{ParentRefs: []k8s.ParentReference{{
			Name: "gwspec",
		}}},
		Hostnames: []k8s.Hostname{"test.cluster.local"},
	}

	expectedgw = &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   9009,
					Name:     "default",
					Protocol: "HTTP",
				},
				Hosts: []string{"*/*"},
			},
		},
	}

	expectedvs = &networking.VirtualService{
		Hosts: []string{
			"test.cluster.local",
		},
		Gateways: []string{
			"ns1/gwspec-istio-autogenerated-k8s-gateway-default",
		},
		Http: []*networking.HTTPRoute{},
	}
)

var AlwaysReady = func(class config.GroupVersionKind, stop <-chan struct{}) bool {
	return true
}

func TestListInvalidGroupVersionKind(t *testing.T) {
	g := NewWithT(t)
	clientSet := kube.NewFakeClient()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, AlwaysReady, nil, controller.Options{})

	typ := config.GroupVersionKind{Kind: "wrong-kind"}
	c := controller.List(typ, "ns1")
	g.Expect(c).To(HaveLen(0))
}

func TestListGatewayResourceType(t *testing.T) {
	g := NewWithT(t)

	clientSet := kube.NewFakeClient()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, AlwaysReady, nil, controller.Options{})

	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.GatewayClass,
			Name:             "gwclass",
			Namespace:        "ns1",
		},
		Spec: gatewayClassSpec,
	})
	if _, err := store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.KubernetesGateway,
			Name:             "gwspec",
			Namespace:        "ns1",
		},
		Spec: gatewaySpec,
	}); err != nil {
		t.Fatal(err)
	}
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.HTTPRoute,
			Name:             "http-route",
			Namespace:        "ns1",
		},
		Spec: httpRouteSpec,
	})

	cg := v1alpha3.NewConfigGenTest(t, v1alpha3.TestOptions{})
	g.Expect(controller.Reconcile(cg.PushContext())).ToNot(HaveOccurred())
	cfg := controller.List(gvk.Gateway, "ns1")
	g.Expect(cfg).To(HaveLen(1))
	for _, c := range cfg {
		g.Expect(c.GroupVersionKind).To(Equal(gvk.Gateway))
		g.Expect(c.Name).To(Equal("gwspec" + "-" + constants.KubernetesGatewayName + "-default"))
		g.Expect(c.Namespace).To(Equal("ns1"))
		g.Expect(c.Spec).To(Equal(expectedgw))
	}
}

func TestListVirtualServiceResourceType(t *testing.T) {
	g := NewWithT(t)

	clientSet := kube.NewFakeClient()
	store := memory.NewController(memory.Make(collections.All))
	controller := NewController(clientSet, store, AlwaysReady, nil, controller.Options{})

	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.GatewayClass,
			Name:             "gwclass",
			Namespace:        "ns1",
		},
		Spec: gatewayClassSpec,
	})
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.KubernetesGateway,
			Name:             "gwspec",
			Namespace:        "ns1",
		},
		Spec: gatewaySpec,
	})
	store.Create(config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.HTTPRoute,
			Name:             "http-route",
			Namespace:        "ns1",
		},
		Spec: httpRouteSpec,
	})

	cg := v1alpha3.NewConfigGenTest(t, v1alpha3.TestOptions{})
	g.Expect(controller.Reconcile(cg.PushContext())).ToNot(HaveOccurred())
	cfg := controller.List(gvk.VirtualService, "ns1")
	g.Expect(cfg).To(HaveLen(1))
	for _, c := range cfg {
		g.Expect(c.GroupVersionKind).To(Equal(gvk.VirtualService))
		g.Expect(c.Name).To(Equal("http-route-0-" + constants.KubernetesGatewayName))
		g.Expect(c.Namespace).To(Equal("ns1"))
		g.Expect(c.Spec).To(Equal(expectedvs))
	}
}

func TestNamespaceEvent(t *testing.T) {
	clientSet := kube.NewFakeClient()
	store := memory.NewController(memory.Make(collections.All))
	c := NewController(clientSet, store, AlwaysReady, nil, controller.Options{})
	s := controller.NewFakeXDS()

	c.RegisterEventHandler(gvk.Namespace, func(_, cfg config.Config, _ model.Event) {
		s.ConfigUpdate(&model.PushRequest{
			Full:   true,
			Reason: []model.TriggerReason{model.NamespaceUpdate},
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c.Run(ctx.Done())
	c.state.ReferencedNamespaceKeys = sets.String{"allowed": struct{}{}}

	ns1 := v1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "ns1",
		Labels: map[string]string{
			"foo": "bar",
		},
	}}
	ns2 := v1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "ns2",
		Labels: map[string]string{
			"allowed": "true",
		},
	}}

	clientSet.Kube().CoreV1().Namespaces().Create(ctx, &ns1, metav1.CreateOptions{})
	s.AssertEmpty(t, time.Millisecond*10)

	clientSet.Kube().CoreV1().Namespaces().Create(ctx, &ns2, metav1.CreateOptions{})
	s.WaitOrFail(t, "xds")

	ns1.Annotations = map[string]string{"foo": "bar"}
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns1, metav1.UpdateOptions{})
	s.AssertEmpty(t, time.Millisecond*10)

	ns2.Annotations = map[string]string{"foo": "bar"}
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns2, metav1.UpdateOptions{})
	s.AssertEmpty(t, time.Millisecond*10)

	ns1.Labels["bar"] = "foo"
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns1, metav1.UpdateOptions{})
	s.AssertEmpty(t, time.Millisecond*10)

	ns2.Labels["foo"] = "bar"
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns2, metav1.UpdateOptions{})
	s.WaitOrFail(t, "xds")

	ns1.Labels["allowed"] = "true"
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns1, metav1.UpdateOptions{})
	s.WaitOrFail(t, "xds")

	ns2.Labels["allowed"] = "false"
	clientSet.Kube().CoreV1().Namespaces().Update(ctx, &ns2, metav1.UpdateOptions{})
	s.WaitOrFail(t, "xds")
}
