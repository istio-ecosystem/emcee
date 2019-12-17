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
	"fmt"
	"log"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	mcCliPkg "github.ibm.com/istio-research/mc2019/mccli/pkg"
)

type openApi struct {
	Client    client.Client
	Namespace string
}

func (o *openApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	expositions, err := mcCliPkg.GetExposures(o.Client, o.Namespace)
	if err != nil {
		log.Fatalf("Failed to list exposures: %s", err)
	}

	openAPI, err := mcCliPkg.Convert(o.Client, *expositions)
	if err != nil {
		log.Fatalf("Failed to convert: %s", err)
	}

	_ = mcCliPkg.ToYAML(openAPI, w)
}

func main() {
	var namespace string
	flag.StringVar(&namespace, "namespace", "", "Kubernetes namespace")
	var kcontext string
	flag.StringVar(&kcontext, "context", "", "Kubernetes configuration context")
	var port string
	flag.StringVar(&port, "port", "8080", "Port to serve on")

	flag.Parse()

	cl, err := mcCliPkg.NewCliClient(namespace, kcontext)
	if err != nil {
		log.Fatalf("Failed to create client: %s", err)
	}

	var ns string
	if namespace != "" {
		ns = namespace
	} else {
		ns = "default"
	}

	mux := http.NewServeMux()
	swagger := &openApi{
		Client:    cl,
		Namespace: ns,
	}
	mux.Handle("/", swagger)

	fmt.Printf("Serving on %s\n", port)

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
