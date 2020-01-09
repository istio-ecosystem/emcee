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

package pkg

import (
	"encoding/json"
	"fmt"
	"log"

	multierror "github.com/hashicorp/go-multierror"
	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/pkg/validate"
)

// ValidateFile validates a .yaml file of Emcee CRs
func ValidateFile(filename string) error {
	resources, err := ReadKubernetesYaml(filename)
	if err != nil {
		return err
	}

	var retval error
	for _, obj := range *resources {
		spec, err := convertObjectSpec(obj)
		if err != nil {
			log.Fatalf("cannot convert: %v", err)
		}

		switch val := spec.(type) {
		case *mmv1.MeshFedConfigSpec:
			err = validate.MeshConfig(obj.ObjectMeta.GetName(),
				obj.ObjectMeta.GetNamespace(), *val)
		case *mmv1.ServiceExpositionSpec:
			err = validate.ServiceExposition(obj.ObjectMeta.GetName(),
				obj.ObjectMeta.GetNamespace(), *val)
		case *mmv1.ServiceBindingSpec:
			err = validate.ServiceBinding(obj.ObjectMeta.GetName(),
				obj.ObjectMeta.GetNamespace(), *val)
		default:
			err = fmt.Errorf("cannot validate: %v (a %T)", spec, spec)
		}

		if err != nil {
			retval = multierror.Append(retval, err)
		}
	}

	return retval
}

func convertObjectSpec(obj KubeKind) (interface{}, error) {
	var retval interface{}
	switch obj.TypeMeta.Kind {
	case "ServiceBinding":
		retval = &mmv1.ServiceBindingSpec{}
	case "ServiceExposition":
		retval = &mmv1.ServiceExpositionSpec{}
	case "MeshFedConfig":
		retval = &mmv1.MeshFedConfigSpec{}
	default:
		return nil, fmt.Errorf("Unknown Kind %q", obj.TypeMeta.Kind)
	}

	// Re-encode (!!!)  Istio does this via YAML and Protobufs, we use JSON.
	str, err := json.Marshal(obj.Spec)
	if err != nil {
		return nil, err
	}
	// Re-decode
	if err := json.Unmarshal(str, retval); err != nil {
		return nil, err
	}

	return retval, nil
}
