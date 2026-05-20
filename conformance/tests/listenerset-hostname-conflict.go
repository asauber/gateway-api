/*
Copyright 2025 The Kubernetes Authors.

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
	ConformanceTests = append(ConformanceTests, ListenerSetHostnameConflict)
}

var ListenerSetHostnameConflict = confsuite.ConformanceTest{
	ShortName:   "ListenerSetHostnameConflict",
	Description: "Validate Listener Precedence when a ListenerSet listener has a hostname conflict, including wildcard hostnames and cross-namespace ListenerSets",
	Features: []features.FeatureName{
		features.SupportGateway,
		features.SupportListenerSet,
		features.SupportHTTPRoute,
		features.SupportReferenceGrant,
	},
	Manifests: []string{
		"tests/listenerset-hostname-conflict.yaml",
	},
	Test: func(t *testing.T, suite *confsuite.ConformanceTestSuite) {
		ns := confsuite.InfrastructureNamespace
		kubernetes.NamespacesMustBeReady(t, suite.Client, suite.TimeoutConfig, []string{ns})

		hostnameConflictedListenerConditions := []metav1.Condition{
			{
				Type:   string(gatewayv1.ListenerConditionAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerReasonHostnameConflict),
			},
			{
				Type:   string(gatewayv1.ListenerConditionProgrammed),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerReasonHostnameConflict),
			},
			{
				Type:   string(gatewayv1.ListenerConditionConflicted),
				Status: metav1.ConditionTrue,
				Reason: string(gatewayv1.ListenerReasonHostnameConflict),
			},
		}

		assertListenerSetAcceptance := func(lsNN types.NamespacedName, accepted bool) {
			t.Helper()
			if accepted {
				kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsNN, metav1.Condition{
					Type:   string(gatewayv1.ListenerSetConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: "",
				})
				kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsNN, metav1.Condition{
					Type:   string(gatewayv1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionTrue,
					Reason: string(gatewayv1.ListenerSetReasonProgrammed),
				})
				return
			}

			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerSetReasonListenersNotValid),
			})
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionProgrammed),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerSetReasonListenersNotValid),
			})
		}

		t.Run("Same-namespace hostname conflicts", func(t *testing.T) {
			gwNN := types.NamespacedName{Name: "gateway-with-listenerset-hostname-conflict", Namespace: ns}
			kubernetes.GatewayMustHaveCondition(t, suite.Client, suite.TimeoutConfig, gwNN, metav1.Condition{
				Type:   string(gatewayv1.GatewayConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.GatewayListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, gwNN, generateAcceptedListenerConditions(), "gateway-listener")
			kubernetes.GatewayListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, gwNN, generateAcceptedListenerConditions(), "hostname-conflict-with-gateway-listener")
			kubernetes.GatewayMustHaveAttachedListeners(t, suite.Client, suite.TimeoutConfig, gwNN, 2)

			lsNN := types.NamespacedName{Name: "listenerset-with-hostname-conflict-with-gateway-1", Namespace: ns}
			assertListenerSetAcceptance(lsNN, true)
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, generateAcceptedListenerConditions(), "listener-set-1-listener")
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, hostnameConflictedListenerConditions, "hostname-conflict-with-gateway-listener")
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, generateAcceptedListenerConditions(), "hostname-conflict-with-listener-set-listener")

			lsNN = types.NamespacedName{Name: "listenerset-with-hostname-conflict-with-gateway-2", Namespace: ns}
			assertListenerSetAcceptance(lsNN, false)
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, hostnameConflictedListenerConditions, "hostname-conflict-with-gateway-listener")

			lsNN = types.NamespacedName{Name: "listenerset-with-hostname-conflict-with-listener-set-1", Namespace: ns}
			assertListenerSetAcceptance(lsNN, true)
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, generateAcceptedListenerConditions(), "listener-set-2-listener")
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, hostnameConflictedListenerConditions, "hostname-conflict-with-listener-set-listener")

			lsNN = types.NamespacedName{Name: "listenerset-with-hostname-conflict-with-listener-set-2", Namespace: ns}
			assertListenerSetAcceptance(lsNN, false)
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsNN, hostnameConflictedListenerConditions, "hostname-conflict-with-listener-set-listener")
		})

		t.Run("Wildcard hostname conflict", func(t *testing.T) {
			wildcardGwNN := types.NamespacedName{Name: "gateway-ls-wildcard-conflict", Namespace: ns}
			kubernetes.GatewayMustHaveCondition(t, suite.Client, suite.TimeoutConfig, wildcardGwNN, metav1.Condition{
				Type:   string(gatewayv1.GatewayConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.GatewayMustHaveAttachedListeners(t, suite.Client, suite.TimeoutConfig, wildcardGwNN, 1)

			winnerNN := types.NamespacedName{Name: "ls-wildcard-a", Namespace: ns}
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, winnerNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, winnerNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionProgrammed),
				Status: metav1.ConditionTrue,
				Reason: string(gatewayv1.ListenerSetReasonProgrammed),
			})
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, winnerNN,
				generateAcceptedListenerConditions(), "wildcard-listener")

			loserNN := types.NamespacedName{Name: "ls-wildcard-b", Namespace: ns}
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, loserNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerSetReasonListenersNotValid),
			})
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, loserNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionProgrammed),
				Status: metav1.ConditionFalse,
				Reason: string(gatewayv1.ListenerSetReasonListenersNotValid),
			})
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, loserNN,
				hostnameConflictedListenerConditions, "wildcard-listener")
		})

		t.Run("Cross-namespace hostname conflict precedence", func(t *testing.T) {
			nsA := "gateway-api-ls-conflict-ns-a"
			nsB := "gateway-api-ls-conflict-ns-b"
			kubernetes.NamespacesMustBeReady(t, suite.Client, suite.TimeoutConfig, []string{ns, nsA, nsB})

			gwNN := types.NamespacedName{Name: "gateway-ls-cross-ns-conflict", Namespace: ns}
			gwAddr, err := kubernetes.WaitForGatewayAddress(t, suite.Client, suite.TimeoutConfig, kubernetes.NewGatewayRef(gwNN, "gateway-listener"))
			require.NoErrorf(t, err, "timed out waiting for Gateway address to be assigned")
			kubernetes.GatewayMustHaveCondition(t, suite.Client, suite.TimeoutConfig, gwNN, metav1.Condition{
				Type:   string(gatewayv1.GatewayConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.GatewayMustHaveAttachedListeners(t, suite.Client, suite.TimeoutConfig, gwNN, 2)

			lsANN := types.NamespacedName{Name: "ls-cross-ns-conflict-a", Namespace: nsA}
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsANN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsANN,
				generateAcceptedListenerConditions(), "conflict-listener")
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsANN,
				generateAcceptedListenerConditions(), "unique-a-listener")

			lsBNN := types.NamespacedName{Name: "ls-cross-ns-conflict-b", Namespace: nsB}
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, lsBNN, metav1.Condition{
				Type:   string(gatewayv1.ListenerSetConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsBNN,
				hostnameConflictedListenerConditions, "conflict-listener")
			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, lsBNN,
				generateAcceptedListenerConditions(), "unique-b-listener")

			listenerSetGK := schema.GroupKind{
				Group: gatewayv1.GroupVersion.Group,
				Kind:  "ListenerSet",
			}
			lsARef := kubernetes.NewResourceRef(listenerSetGK, lsANN)
			kubernetes.RoutesAndParentMustBeAccepted(t, suite.Client, suite.TimeoutConfig, suite.ControllerName, lsARef, &gatewayv1.HTTPRoute{},
				types.NamespacedName{Name: "route-for-ls-a", Namespace: nsA})

			kubernetes.HTTPRouteMustHaveCondition(t, suite.Client, suite.TimeoutConfig,
				types.NamespacedName{Name: "route-for-ls-b-unique", Namespace: nsB}, lsBNN,
				metav1.Condition{
					Type:   string(gatewayv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				})

			testCases := []http.ExpectedResponse{
				{
					Request:   http.Request{Host: "cross-ns-conflict.com", Path: "/route-a"},
					Backend:   confsuite.InfraBackendServiceNameV1,
					Namespace: ns,
				},
				{
					Request:   http.Request{Host: "unique-b.com", Path: "/route-b"},
					Backend:   confsuite.InfraBackendServiceNameV2,
					Namespace: ns,
				},
			}
			for i := range testCases {
				tc := testCases[i]
				t.Run(tc.GetTestCaseName(i), func(t *testing.T) {
					t.Parallel()
					http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, gwAddr, tc)
				})
			}
		})
	},
}
