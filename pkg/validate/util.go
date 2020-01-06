// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.

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

package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-multierror"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
)

const (
	dNS1123LabelMaxLength = 63
	dns1123LabelFmt       = "[a-zA-Z0-9](?:[-a-z-A-Z0-9]*[a-zA-Z0-9])?"
)

var (
	dns1123LabelRegexp = regexp.MustCompile("^" + dns1123LabelFmt + "$")
)

// MeshConfig validates a MeshFedConfigSpec
func MeshConfig(name, namespace string, mfc mmv1.MeshFedConfigSpec) error {
	var retval error
	if !strings.EqualFold(mfc.Mode, controllers.ModeBoundary) && !strings.EqualFold(mfc.Mode, controllers.ModePassthrough) {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: Unknown Mode %q", namespace, name, mfc.Mode))
	}

	if strings.EqualFold(mfc.Mode, controllers.ModeBoundary) {
		if len(mfc.TlsContextSelector) == 0 {
			retval = multierror.Append(retval, fmt.Errorf("%s/%s: %q requires tls_context_selector", namespace, name, strings.ToUpper(mfc.Mode)))
		}

		if !mfc.UseEgressGateway {
			if len(mfc.EgressGatewaySelector) != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify egress, but selects one", namespace, name))
			}

			if mfc.EgressGatewayPort != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify egress, but specifies port %v", namespace, name, mfc.EgressGatewayPort))
			}
		}

		if !mfc.UseIngressGateway {
			if len(mfc.IngressGatewaySelector) != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify ingress, but selects one", namespace, name))
			}

			if mfc.IngressGatewayPort != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify ingress, but specifies port %v", namespace, name, mfc.IngressGatewayPort))
			}
		}
	} else if strings.EqualFold(mfc.Mode, controllers.ModePassthrough) {
		if !mfc.UseEgressGateway {
			if len(mfc.EgressGatewaySelector) != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify egress, but selects one", namespace, name))
			}

			if mfc.EgressGatewayPort != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify egress, but specifies port %v", namespace, name, mfc.EgressGatewayPort))
			}
		}

		if !mfc.UseIngressGateway {
			if len(mfc.IngressGatewaySelector) != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify ingress, but selects one", namespace, name))
			}

			if mfc.IngressGatewayPort != 0 {
				retval = multierror.Append(retval, fmt.Errorf("%s/%s: does not specify ingress, but specifies port %v", namespace, name, mfc.IngressGatewayPort))
			}
		}
	}

	return retval
}

// ServiceExposition validates a ServiceExpositionSpec
func ServiceExposition(name, namespace string, se mmv1.ServiceExpositionSpec) error {
	var retval error
	if len(se.MeshFedConfigSelector) == 0 {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: requires mesh_fed_config_selector", namespace, name))
	}
	if !isDNSLabel(se.Name) {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: invalid name %q", namespace, name, se.Name))
	}

	return retval
}

// ServiceBinding validates a ServiceBindingSpec
func ServiceBinding(name, namespace string, sb mmv1.ServiceBindingSpec) error {
	var retval error
	if len(sb.MeshFedConfigSelector) == 0 {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: requires mesh_fed_config_selector", namespace, name))
	}
	if !isDNSLabel(sb.Name) {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: invalid name %q", namespace, name, sb.Name))
	}
	if sb.Alias != "" && !isDNSLabel(sb.Alias) {
		retval = multierror.Append(retval, fmt.Errorf("%s/%s: invalid alias %q", namespace, name, sb.Alias))
	}
	// Note that we allow no endpoints, because of the scenario where we create
	// with no endpoints and Service Discovery patches the binding to add them.

	return retval
}

func isDNSLabel(value string) bool {
	return len(value) <= dNS1123LabelMaxLength && dns1123LabelRegexp.MatchString(value)
}
