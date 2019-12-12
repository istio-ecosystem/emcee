#!/bin/bash

# Licensed Materials - Property of IBM
# (C) Copyright IBM Corp. 2019. All Rights Reserved.
# US Government Users Restricted Rights - Use, duplication or
# disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
# Copyright 2019 IBM Corporation

set -o errexit

if [ -n "$MM_DEBUG" ]; then
   set -o xtrace
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
BASEDIR=$DIR/../..
TMPDIR=/tmp
CERTDIR=$BASEDIR/samples/passthrough/certs
# Set the istioctl if need to execute the commented out commands in main
# ISTIOCTL=$BASEDIR/samples/bin/istioctl

if [ -z "${CTX_CLUSTER1+xxx}" ]; then
   echo CTX_CLUSTER1 is not set
   exit 1
fi

if [ -z "${CTX_CLUSTER2+xxx}" ]; then
   echo CTX_CLUSTER2 is not set
   exit 1
fi

CLUSTER1=$CTX_CLUSTER1
CLUSTER2=$CTX_CLUSTER2

cleanup() {
    # Clean up any existing multi-mesh configuration by removing config,binding,exposure
    echo Cleanup starting
    echo Removing expose and bind
    kubectl --context $CLUSTER2 delete -f $BASEDIR/samples/passthrough/holamundo-expose.yaml 2> /dev/null || true
    kubectl --context $CLUSTER1 delete -f $BASEDIR/samples/passthrough/holamundo-binding.yaml 2> /dev/null || true  
    echo removing passthrough mesh fed config
    kubectl --context $CLUSTER1 delete -f $BASEDIR/samples/passthrough/passthrough-c1.yaml 2> /dev/null || true
    kubectl --context $CLUSTER2 delete -f $BASEDIR/samples/passthrough/passthrough-c2.yaml 2> /dev/null || true
    echo removing server holamundo and client cli2 pods
    kubectl --context $CLUSTER2 delete -f $BASEDIR/samples/passthrough/holamundo.yaml
    kubectl --context $CLUSTER1 delete pod cli2
    echo Cleanup done.
}



main() {
    cleanup
    sleep 5

    # TODO: Support --context in manager
    CFG_CLUSTER1=$TMPDIR/kubeconfig1
    CFG_CLUSTER2=$TMPDIR/kubeconfig2
    kubectl config use-context $CTX_CLUSTER1
    kubectl config view --minify > $CFG_CLUSTER1
    kubectl config use-context $CTX_CLUSTER2
    kubectl config view --minify > $CFG_CLUSTER2

    #Setup secrets
    kubectl --context $CLUSTER1 delete secret -n istio-system cacerts 2> /dev/null || true
    kubectl --context $CLUSTER1 create secret generic cacerts -n istio-system --from-file=$CERTDIR/ca-cert.pem    --from-file=$CERTDIR/ca-key.pem --from-file=$CERTDIR/root-cert.pem --from-file=$CERTDIR/cert-chain.pem
    # $ISTIOCTL --context $CLUSTER1  manifest apply --set values.global.mtls.enabled=true,values.security.selfSigned=false
    kubectl --context $CLUSTER1  delete secret istio.default

    kubectl --context $CLUSTER2 delete secret -n istio-system cacerts 2> /dev/null || true
    kubectl --context $CLUSTER2 create secret generic cacerts -n istio-system --from-file=$CERTDIR/ca-cert.pem     --from-file=$CERTDIR/ca-key.pem --from-file=$CERTDIR/root-cert.pem --from-file=$CERTDIR/cert-chain.pem
    # $ISTIOCTL --context $CLUSTER2  manifest apply --set values.global.mtls.enabled=true,values.security.selfSigned=false
    kubectl --context $CLUSTER2  delete secret istio.default

    # creating namespaces 
    kubectl --context $CLUSTER1  create namespace passthrough  2> /dev/null || true
    kubectl --context $CLUSTER2  create namespace passthrough  2> /dev/null || true
    
    # Deploy experiment
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/passthrough/holamundo.yaml
    # Configure the mesh
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/passthrough/passthrough-c2.yaml


    # Wait for the experiment holamundo (producer) to be up
    kubectl --context $CLUSTER2 wait --for=condition=available --timeout=60s deployment/holamundo-v1
    # Expose holamundo
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/passthrough/holamundo-expose.yaml


    # Where is the gateway for traffic to the exposed service? 
    CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n istio-system istio-ingressgateway --output jsonpath="{.status.loadBalancer.ingress[0].ip}")
    echo Using $CLUSTER2 ingress at $CLUSTER2_INGRESS:15443
    CLUSTER2_SECURE_INGRESS_PORT=15443
    CLUSTER2_INGRESS_HOST=$CLUSTER2_INGRESS
    if [ -z $CLUSTER2_INGRESS_HOST ]; then
        echo CLUSTER2_INGRESS_HOST is not set
        exit 6
    fi


    # Deploy experiment consumer
    kubectl --context $CLUSTER1 run --generator=run-pod/v1 cli2 --image tutum/curl --command -- bash -c 'sleep 9999999' 2> /dev/null || true
    # Wait for the experiment client (consumer) to be up
    kubectl --context $CLUSTER1 wait --for=condition=ready --timeout=60s pod/cli2

    # Configure the meshes
    kubectl --context $CLUSTER1 apply -f $BASEDIR/samples/passthrough/passthrough-c1.yaml

    # Bind helloworld to the actual dynamic exposed public IP
    cat $BASEDIR/samples/passthrough/holamundo-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -

    # Wait for the exposure to be affected
    # until kubectl --context $CLUSTER1 get service holamundo ; do
    #     echo Waiting for controller to create holamundo service
    #     sleep 1
    # done

    # TODO REMOVE
    sleep 5

    # Have the sleep pod test
    SLEEP_POD=cli2
    Echo using Sleep pod $SLEEP_POD on $CLUSTER1
    CURL_CMD="kubectl --context $CLUSTER1 exec -it $SLEEP_POD -- curl --silent holamundo:5000/hola -w '%{http_code}' -o /dev/null"
    set +o errexit
    REMOTE_OUTPUT=$($CURL_CMD)
    set -o errexit
    if [ "$REMOTE_OUTPUT" != "'200'" ]; then
        echo Expected 200 but got $REMOTE_OUTPUT executing
        echo $CURL_CMD
        exit 7
    fi
    echo
    echo =======================================================
    echo Bind worked; test with
    echo $CURL_CMD
    echo =======================================================
    echo
}


main "$@"
exit 0

