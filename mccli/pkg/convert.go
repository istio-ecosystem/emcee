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
	"context"
	"fmt"
	"log"

	multierror "github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.ibm.com/istio-research/mc2019/controllers"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// simple mock-up of OpenAPI for proof-of-concept
type info struct {
	Title       string
	Description string
	Version     string
}

type server struct {
	URL         string
	Description string
}

type response struct {
	Description string
}

type pathOp struct {
	Responses map[int]response
}

type path struct {
	Summary string
	Get     pathOp
}

// OpenAPI is simple mock-up of OpenAPI for proof-of-concept
type OpenAPI struct {
	OpenAPI string
	Info    info
	Servers []server
	Paths   map[string]path
}

// Convert converts mmv1.ServiceExpositions to Swagger
// TODO This version only does Boundary Protection expositions; add the rest
func Convert(cl client.Client, expositions []mmv1.ServiceExposition) (*OpenAPI, error) {
	retval := OpenAPI{
		OpenAPI: "3.0.0",
		Info: info{
			Title:       "Ingress API",
			Description: fmt.Sprintf("%d services advertised for this Private Ingress", len(expositions)),
			Version:     "0.0.1", // TODO
		},
		Paths: map[string]path{},
	}

	expToFed, err := mapExposureToMFC(cl, expositions)
	if err != nil {
		return nil, err
	}
	bpMFCs := getBPMFCs(expToFed)
	for _, bpMFC := range bpMFCs {
		retval.Servers = append(retval.Servers, server{
			// TODO Get real exposed IP from MFC's ingress Service external IP reverse lookup
			URL:         fmt.Sprintf("http://private-ingress-%s.mycluster.us-south.containers.appdomain.cloud:15443/", bpMFC.GetObjectMeta().GetName()),
			Description: fmt.Sprintf("Private ingress for %q mesh", bpMFC.GetObjectMeta().GetName()),
		})
	}

	for _, exposition := range expositions {
		mfc, ok := expToFed[kname(exposition.ObjectMeta)]
		if !ok {
			log.Printf("Cannot find MFC for %q", exposition.GetObjectMeta().GetName())
			continue
		}

		if isBoundaryProtection(mfc) {
			paths := getBPPaths(exposition)
			for _, p := range paths {
				retval.Paths[p] = path{
					Summary: "TODO",
					Get: pathOp{
						Responses: map[int]response{
							200: response{
								Description: "OK",
							},
						},
					},
				}
			}
		}
	}

	return &retval, nil
}

func getBPMFCs(expToFed map[string]*mmv1.MeshFedConfig) []*mmv1.MeshFedConfig {
	meshes := make(map[string]*mmv1.MeshFedConfig)
	for _, mfc := range expToFed {
		if isBoundaryProtection(mfc) {
			meshes[mfc.GetObjectMeta().GetName()] = mfc
		}
	}

	retval := []*mmv1.MeshFedConfig{}
	for _, mesh := range meshes {
		retval = append(retval, mesh)
	}
	return retval
}

func isBoundaryProtection(mfc *mmv1.MeshFedConfig) bool {
	return true
	// TODO Uncomment the next line when we are testing Boundary Protection again
	// return mfc.Spec.UseIngressGateway && mfc.Spec.DeepCopy().UseEgressGateway
}

// TODO depending on the exposure add extra canned paths or consult the microservice's internal OpenAPI
func getBPPaths(exp mmv1.ServiceExposition) []string {
	return []string{
		fmt.Sprintf("/%s/%s%s/*", exp.ObjectMeta.Namespace, getExposedName(exp), getExposedSubset(exp)),
	}
}

func getExposedSubset(exp mmv1.ServiceExposition) string {
	if exp.Spec.Subset != "" {
		return "/" + exp.Spec.Subset
	}
	return ""
}

func getExposedName(exp mmv1.ServiceExposition) string {
	if exp.Spec.Alias != "" {
		return exp.Spec.Alias
	}
	return exp.Spec.Name
}

func mapExposureToMFC(cl client.Client, expositions []mmv1.ServiceExposition) (map[string]*mmv1.MeshFedConfig, error) {
	expToConf := make(map[string]*mmv1.MeshFedConfig)
	ctx := context.Background()
	for _, exposure := range expositions {
		// TODO Cache these lookups for performance?
		mfc, err := controllers.GetMeshFedConfig(ctx, cl, exposure.Spec.MeshFedConfigSelector)
		if err != nil {
			return nil, multierror.Prefix(err, fmt.Sprintf("Failed to lookup mfc for exposure %q:", exposure.ObjectMeta.Name))
		}
		expToConf[kname(exposure.ObjectMeta)] = &mfc
	}

	return expToConf, nil
}

func kname(s metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s", s.GetName(), s.GetNamespace())
}
