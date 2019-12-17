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

package main

import (
	"flag"
	"log"
	"os"

	// This next line lets us use IBM Kubernetes Service
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	mcCliPkg "github.ibm.com/istio-research/mc2019/mccli/pkg"
)

func main() {
	var namespace string
	flag.StringVar(&namespace, "namespace", "", "Kubernetes namespace")
	var kcontext string
	flag.StringVar(&kcontext, "context", "", "Kubernetes configuration context")
	flag.Parse()

	var ns string
	if namespace != "" {
		ns = namespace
	} else {
		ns = "default"
	}

	cl, err := mcCliPkg.NewCliClient(namespace, kcontext)
	if err != nil {
		log.Fatalf("Failed to create cl: %s", err)
	}
	expositions, err := mcCliPkg.GetExposures(cl, ns)
	if err != nil {
		log.Fatalf("Failed to list exposures: %s", err)
	}
	// log.Printf("%d expositions in %q\n", len(*expositions), ns)
	// for _, exposition := range *expositions {
	// 	log.Printf("Exposition: %q\n", exposition.ObjectMeta.GetName())
	// }

	openAPI, err := mcCliPkg.Convert(cl, *expositions)
	if err != nil {
		log.Fatalf("Failed to convert: %s", err)
	}

	_ = mcCliPkg.ToYAML(openAPI, os.Stdout)
}
