/*
Copyright The Kubernetes Authors.

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

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/conformance/utils/http"
	"sigs.k8s.io/gateway-api/conformance/utils/kubernetes"
	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func init() {
	ConformanceTests = append(ConformanceTests, ListenerSetRouteStatusScopedToParentRef)
}

// ListenerSetRouteStatusScopedToParentRef verifies that a Route's
// status.parents contains entries only for the parentRefs the Route explicitly
// declares — never for additional resources that happen to be related (such as
// the parent Gateway of a ListenerSet, or sibling ListenerSets).
//
// Concretely:
//   - route-gw-only: parentRef=Gateway → status.parents contains exactly the
//     Gateway; no ListenerSet entries.
//   - route-ls-only: parentRef=ListenerSet → status.parents contains exactly
//     the ListenerSet; no Gateway entry.
//
// This guards against implementations that walk the parent chain or enumerate
// attached ListenerSets when computing route status, which would produce
// spurious parentRef entries and incorrect AttachedRoutes counts.
var ListenerSetRouteStatusScopedToParentRef = confsuite.ConformanceTest{
	ShortName:   "ListenerSetRouteStatusScopedToParentRef",
	Description: "A Route's status.parents contains entries only for its explicitly declared parentRefs, not for related resources (parent Gateway of a ListenerSet, sibling ListenerSets, etc.)",
	Features: []features.FeatureName{
		features.SupportGateway,
		features.SupportListenerSet,
		features.SupportHTTPRoute,
	},
	Manifests: []string{
		"tests/listenerset-route-status-scoped-to-parentref.yaml",
	},
	Test: func(t *testing.T, suite *confsuite.ConformanceTestSuite) {
		ns := confsuite.InfrastructureNamespace
		kubernetes.NamespacesMustBeReady(t, suite.Client, suite.TimeoutConfig, []string{ns})

		gwNN := types.NamespacedName{Name: "gateway-ls-parentref-scope", Namespace: ns}
		gwAddr, err := kubernetes.WaitForGatewayAddress(t, suite.Client, suite.TimeoutConfig, kubernetes.NewGatewayRef(gwNN, "gw-scope-listener"))
		require.NoErrorf(t, err, "timed out waiting for Gateway address to be assigned")
		kubernetes.GatewayMustHaveCondition(t, suite.Client, suite.TimeoutConfig, gwNN, metav1.Condition{
			Type:   string(gatewayv1.GatewayConditionAccepted),
			Status: metav1.ConditionTrue,
		})
		kubernetes.GatewayMustHaveAttachedListeners(t, suite.Client, suite.TimeoutConfig, gwNN, 1)

		listenerSetGK := schema.GroupKind{
			Group: gatewayv1.GroupVersion.Group,
			Kind:  "ListenerSet",
		}
		lsNN := types.NamespacedName{Name: "ls-parentref-scope", Namespace: ns}
		lsRef := kubernetes.NewResourceRef(listenerSetGK, lsNN)

		// route-gw-only must be accepted by the Gateway.
		gwOnlyRouteNN := types.NamespacedName{Name: "route-gw-only", Namespace: ns}
		kubernetes.HTTPRouteMustHaveCondition(t, suite.Client, suite.TimeoutConfig, gwOnlyRouteNN, gwNN, metav1.Condition{
			Type:   string(gatewayv1.RouteConditionAccepted),
			Status: metav1.ConditionTrue,
		})
		// route-gw-only must have exactly the Gateway as its single parent in status.
		// A conformance-compliant implementation must not add a spurious ListenerSet entry.
		kubernetes.HTTPRouteMustHaveParents(t, suite.Client, suite.TimeoutConfig, gwOnlyRouteNN,
			[]gatewayv1.RouteParentStatus{
				{
					ParentRef: gatewayv1.ParentReference{
						Group:     (*gatewayv1.Group)(strPtr(string(gatewayv1.GroupVersion.Group))),
						Kind:      (*gatewayv1.Kind)(strPtr("Gateway")),
						Name:      "gateway-ls-parentref-scope",
						Namespace: (*gatewayv1.Namespace)(strPtr(ns)),
					},
					ControllerName: gatewayv1.GatewayController(suite.ControllerName),
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			true, // namespaceRequired
		)

		// route-ls-only must be accepted by the ListenerSet.
		lsOnlyRouteNN := types.NamespacedName{Name: "route-ls-only", Namespace: ns}
		kubernetes.RoutesAndParentMustBeAccepted(t, suite.Client, suite.TimeoutConfig, suite.ControllerName, lsRef, &gatewayv1.HTTPRoute{}, lsOnlyRouteNN)
		// route-ls-only must have exactly the ListenerSet as its single parent in status.
		// A conformance-compliant implementation must not add a spurious Gateway entry.
		kubernetes.HTTPRouteMustHaveParents(t, suite.Client, suite.TimeoutConfig, lsOnlyRouteNN,
			[]gatewayv1.RouteParentStatus{
				{
					ParentRef: gatewayv1.ParentReference{
						Group:     (*gatewayv1.Group)(strPtr(string(gatewayv1.GroupVersion.Group))),
						Kind:      (*gatewayv1.Kind)(strPtr("ListenerSet")),
						Name:      "ls-parentref-scope",
						Namespace: (*gatewayv1.Namespace)(strPtr(ns)),
					},
					ControllerName: gatewayv1.GatewayController(suite.ControllerName),
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			true, // namespaceRequired
		)

		// AttachedRoutes on the ListenerSet listener must count only route-ls-only
		// (not route-gw-only, which only declared a Gateway parentRef).
		kubernetes.ListenerSetStatusMustHaveListeners(t, suite.Client, suite.TimeoutConfig, lsNN, []gatewayv1.ListenerEntryStatus{
			{
				Name:           "ls-scope-listener",
				SupportedKinds: generateSupportedRouteKinds(),
				AttachedRoutes: 1, // only route-ls-only
				Conditions:     generateAcceptedListenerConditions(),
			},
		})

		// Data plane: route-gw-only reaches backend via the GW listener.
		// route-ls-only reaches backend via the LS listener.
		// Neither route crosses over to the other parent's listener.
		testCases := []http.ExpectedResponse{
			{
				Request:   http.Request{Host: "gw-scope.com", Path: "/gw-only"},
				Backend:   confsuite.InfraBackendServiceNameV1,
				Namespace: ns,
			},
			{
				// route-gw-only must NOT be served via the ListenerSet listener.
				Request:  http.Request{Host: "ls-scope.com", Path: "/gw-only"},
				Response: http.Response{StatusCode: 404},
			},
			{
				Request:   http.Request{Host: "ls-scope.com", Path: "/ls-only"},
				Backend:   confsuite.InfraBackendServiceNameV2,
				Namespace: ns,
			},
			{
				// route-ls-only must NOT be served via the Gateway listener.
				Request:  http.Request{Host: "gw-scope.com", Path: "/ls-only"},
				Response: http.Response{StatusCode: 404},
			},
		}
		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.GetTestCaseName(i), func(t *testing.T) {
				t.Parallel()
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, gwAddr, tc)
			})
		}
	},
}

// strPtr returns a pointer to the given string value.  Used when constructing
// ParentReference objects inline where pointer fields are required.
func strPtr(s string) *string { return &s }
