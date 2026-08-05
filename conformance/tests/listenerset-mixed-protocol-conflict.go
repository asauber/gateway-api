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
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/conformance/utils/http"
	"sigs.k8s.io/gateway-api/conformance/utils/kubernetes"
	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/conformance/utils/tls"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func init() {
	ConformanceTests = append(ConformanceTests, ListenerSetMixedProtocolConflict)
}

var ListenerSetMixedProtocolConflict = confsuite.ConformanceTest{
	ShortName:   "ListenerSetMixedProtocolConflict",
	Description: "A conflicted ListenerSet TLS listener must not serve traffic",
	Features: []features.FeatureName{
		features.SupportGateway,
		features.SupportHTTPRoute,
		features.SupportTLSRoute,
		features.SupportListenerSet,
	},
	Manifests: []string{"tests/listenerset-mixed-protocol-conflict.yaml"},
	Test: func(t *testing.T, suite *confsuite.ConformanceTestSuite) {
		ns := confsuite.InfrastructureNamespace
		gwNN := types.NamespacedName{Name: "gateway-listenerset-conflicts", Namespace: ns}
		listenerSetNN := types.NamespacedName{Name: "listenerset-mixed-protocol-conflict", Namespace: ns}
		httpRouteNN := types.NamespacedName{Name: "listenerset-mixed-protocol-control", Namespace: ns}
		tlsCACertNN := types.NamespacedName{Name: "tls-checks-ca-certificate", Namespace: ns}

		kubernetes.NamespacesMustBeReady(t, suite.Client, suite.TimeoutConfig, []string{ns})

		gwAddr, err := kubernetes.WaitForGatewayAddress(t, suite.Client, suite.TimeoutConfig, kubernetes.NewGatewayRef(gwNN, "http"))
		require.NoErrorf(t, err, "timed out waiting for Gateway address to be assigned")

		t.Run("HTTP control listener serves traffic", func(t *testing.T) {
			kubernetes.HTTPRouteMustHaveRouteAcceptedConditionsTrue(t, suite.Client, suite.TimeoutConfig, httpRouteNN, gwNN)
			kubernetes.HTTPRouteMustHaveResolvedRefsConditionsTrue(t, suite.Client, suite.TimeoutConfig, httpRouteNN, gwNN)

			http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, gwAddr, http.ExpectedResponse{
				Request:   http.Request{Host: "control.example.com", Path: "/control"},
				Response:  http.Response{StatusCode: 200},
				Backend:   confsuite.InfraBackendServiceNameV1,
				Namespace: ns,
			})
		})

		t.Run("conflicted ListenerSet listener has ProtocolConflict status", func(t *testing.T) {
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, listenerSetNN, metav1.Condition{
				Type:   string(v1.ListenerSetConditionAccepted),
				Status: metav1.ConditionTrue,
			})
			kubernetes.ListenerSetMustHaveCondition(t, suite.Client, suite.TimeoutConfig, listenerSetNN, metav1.Condition{
				Type:   string(v1.ListenerSetConditionProgrammed),
				Status: metav1.ConditionTrue,
			})

			kubernetes.ListenerSetListenersMustHaveConditions(t, suite.Client, suite.TimeoutConfig, listenerSetNN, []metav1.Condition{
				{
					Type:   string(v1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(v1.ListenerReasonProtocolConflict),
				},
				{
					Type:   string(v1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(v1.ListenerReasonProtocolConflict),
				},
				{
					Type:   string(v1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(v1.ListenerReasonProtocolConflict),
				},
			}, "tls")
		})

		gwIP, _, err := net.SplitHostPort(gwAddr)
		require.NoErrorf(t, err, "failed to split Gateway address %q", gwAddr)
		tlsAddr := net.JoinHostPort(gwIP, "8443")
		tlsCAConfigMap, err := kubernetes.GetConfigMapData(suite.Client, suite.TimeoutConfig, tlsCACertNN)
		require.NoErrorf(t, err, "failed to get TLS backend CA certificate")
		tlsCACert, ok := tlsCAConfigMap["ca.crt"]
		require.True(t, ok, "ca.crt not found in TLS backend CA ConfigMap")

		t.Run("conflicted ListenerSet TLS listener rejects traffic", func(t *testing.T) {
			tls.MakeTLSConnectionAndExpectEventuallyConsistentFailure(t, suite.TimeoutConfig, tlsAddr, []byte(tlsCACert), "abc.example.com")
		})
	},
}
