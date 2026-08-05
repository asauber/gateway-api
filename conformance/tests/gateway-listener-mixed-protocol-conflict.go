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
	ConformanceTests = append(ConformanceTests, GatewayListenerMixedProtocolConflict)
}

var GatewayListenerMixedProtocolConflict = confsuite.ConformanceTest{
	ShortName:   "GatewayListenerMixedProtocolConflict",
	Description: "Conflicting HTTPS and TLS listeners must not serve traffic",
	Features: []features.FeatureName{
		features.SupportGateway,
		features.SupportHTTPRoute,
		features.SupportTLSRoute,
	},
	Manifests: []string{"tests/gateway-listener-mixed-protocol-conflict.yaml"},
	Test: func(t *testing.T, suite *confsuite.ConformanceTestSuite) {
		ns := confsuite.InfrastructureNamespace
		gwNN := types.NamespacedName{Name: "gateway-listener-conflicts", Namespace: ns}
		controlRouteNN := types.NamespacedName{Name: "mixed-protocol-conflict-http", Namespace: ns}

		httpsCertNN := types.NamespacedName{Name: "tls-validity-checks-certificate", Namespace: ns}
		tlsCACertNN := types.NamespacedName{Name: "tls-checks-ca-certificate", Namespace: ns}

		kubernetes.NamespacesMustBeReady(t, suite.Client, suite.TimeoutConfig, []string{ns})
		gwAddr, err := kubernetes.WaitForGatewayAddress(t, suite.Client, suite.TimeoutConfig, kubernetes.NewGatewayRef(gwNN, "gateway-listener-conflicts-http"))
		require.NoErrorf(t, err, "timed out waiting for Gateway address to be assigned")

		t.Run("HTTP control listener serves traffic", func(t *testing.T) {
			kubernetes.HTTPRouteMustHaveRouteAcceptedConditionsTrue(t, suite.Client, suite.TimeoutConfig, controlRouteNN, gwNN)
			kubernetes.HTTPRouteMustHaveResolvedRefsConditionsTrue(t, suite.Client, suite.TimeoutConfig, controlRouteNN, gwNN)

			// The control route must be fully programmed
			http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, gwAddr, http.ExpectedResponse{
				Request:   http.Request{Host: "control.example.com", Path: "/control"},
				Response:  http.Response{StatusCode: 200},
				Backend:   confsuite.InfraBackendServiceNameV1,
				Namespace: ns,
			})
		})

		t.Run("conflicting listeners have ProtocolConflict status", func(t *testing.T) {
			conditions := []metav1.Condition{
				{
					Type:   string(v1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: "Invalid",
				},
				{
					Type:   string(v1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: "",
				},
				{
					Type:   string(v1.ListenerConditionConflicted),
					Status: metav1.ConditionTrue,
					Reason: string(v1.ListenerReasonProtocolConflict),
				},
			}

			kubernetes.GatewayListenerMustHaveConditions(t, suite.Client, suite.TimeoutConfig, gwNN, "gateway-listener-conflicts-https", conditions)
			kubernetes.GatewayListenerMustHaveConditions(t, suite.Client, suite.TimeoutConfig, gwNN, "gateway-listener-conflicts-tls", conditions)
		})

		gwIP, _, err := net.SplitHostPort(gwAddr)
		require.NoErrorf(t, err, "failed to split Gateway address %q", gwAddr)
		tlsAddr := net.JoinHostPort(gwIP, "8443")

		// Gather certificate material needed to attempt a raw TLS connection against the HTTPS listener
		httpsCert, _, err := kubernetes.GetTLSSecret(suite.Client, httpsCertNN)
		require.NoErrorf(t, err, "failed to get HTTPS listener certificate")

		// Gather certificate material needed to attempt a raw TLS connection against the TLS listener
		tlsCAConfigMap, err := kubernetes.GetConfigMapData(suite.Client, suite.TimeoutConfig, tlsCACertNN)
		require.NoErrorf(t, err, "failed to get TLS backend CA certificate")
		tlsCACert, ok := tlsCAConfigMap["ca.crt"]
		require.True(t, ok, "ca.crt not found in TLS backend CA ConfigMap")

		t.Run("HTTPS listener rejects traffic", func(t *testing.T) {
			// The rejected HTTPS listener must not complete a TLS handshake
			tls.MakeTLSConnectionAndExpectEventuallyConsistentFailure(t, suite.TimeoutConfig, tlsAddr, httpsCert, "https.example.org")
		})

		t.Run("TLS listener rejects traffic", func(t *testing.T) {
			// The rejected TLS listener must not complete a TLS handshake
			tls.MakeTLSConnectionAndExpectEventuallyConsistentFailure(t, suite.TimeoutConfig, tlsAddr, []byte(tlsCACert), "abc.example.com")
		})
	},
}
