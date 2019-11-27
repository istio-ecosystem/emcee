// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

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
