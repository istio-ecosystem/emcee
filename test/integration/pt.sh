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

    echo Removing finalizers
    remove_finalizers $CLUSTER1
    remove_finalizers $CLUSTER2

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
    preconditions

    cleanup
    # sleep 5

    # TODO: Support --context in manager
    CFG_CLUSTER1=$TMPDIR/kubeconfig1
    CFG_CLUSTER2=$TMPDIR/kubeconfig2
    kubectl config use-context $CTX_CLUSTER1
    kubectl config view --minify > $CFG_CLUSTER1
    kubectl config use-context $CTX_CLUSTER2
    kubectl config view --minify > $CFG_CLUSTER2

    # Start controllers
    startup1
    startup2

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

    # Verify the controllers are still running
    if ps -p $MANAGER_1_PID > /dev/null ; then
        echo MANAGER_1 running
    else
        echo MANAGER_1 is not running
        cat $ERR1
        exit 3
    fi
    if ps -p $MANAGER_2_PID > /dev/null ; then
        echo MANAGER_2 running
    else
        echo MANAGER_2 is not running
        cat $ERR2
        exit 3
    fi

    # Wait for controllers to be up
    # TODO is there a cleaner way?
    while [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8080)" != "404" ]; do
        sleep 1
    done
    while [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8081)" != "404" ]; do
        sleep 1
    done

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

startup1() {
    # Start cluster1 controller
    LOG1=$TMPDIR/log1
    ERR1=$TMPDIR/err1
    $BASEDIR/bin/manager --kubeconfig $CFG_CLUSTER1 --metrics-addr ":8080" > $LOG1 2> $ERR1 &
    MANAGER_1_PID=$!
    echo MANAGER_1_PID is $MANAGER_1_PID
}

startup2() {
    # Start cluster2 controller
    LOG2=$TMPDIR/log2
    ERR2=$TMPDIR/err2
    $BASEDIR/bin/manager --kubeconfig $CFG_CLUSTER2 --metrics-addr ":8081" > $LOG2 2> $ERR2 &
    MANAGER_2_PID=$!
    echo MANAGER_2_PID is $MANAGER_2_PID

}

shutdowns() {
    if [ -n "$MANAGER_1_PID" ]; then
        kill $MANAGER_1_PID
        echo controller1 killed
        MANAGER_1_PID=""
    else
        echo controller1 not running
    fi

    if [ -n "$MANAGER_2_PID" ]; then
        kill $MANAGER_2_PID || true
        echo controller2 killed
        MANAGER_2_PID=""
    else
        echo controller2 not running
    fi
}

preconditions() {
    # Verify I don't already have controllers who have grabbed the port
    if [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8080)" == "404" ]; then
        echo There is already a listener on 8080
        ps -ef | grep 8080 | grep manager
        exit 4
    fi
    if [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8081)" == "404" ]; then
        echo There is already a listener on 8081
        ps -ef | grep 8081 | grep manager
        exit 4
    fi

    # Verify I am not using BoundaryProtection example at the same time
    set +o errexit
    for ctx in $CTX_CLUSTER1 $CTX_CLUSTER2; do
        kubectl --context $ctx get namespace limited-trust 2> /dev/null
        if [ $? -eq 0 ]; then
            echo There is a limited-trust namespace in context $ctx
            exit 5
        fi
    done
    kubectl --context $CTX_CLUSTER1 get servicebinding helloworld 2> /dev/null
    if [ $? -eq 0 ]; then
        echo There is a servicebinding helloworld in context $CTX_CLUSTER1
        exit 6
    fi
    kubectl --context $CTX_CLUSTER2 get serviceexposition se1 2> /dev/null
    if [ $? -eq 0 ]; then
        echo There is a serviceexposition se1 in context $CTX_CLUSTER2
        exit 7
    fi
    set -o errexit
}

remove_finalizers() {
    ctx=$1

    # Remove finalizers, so that deletes don't block forever if controller isn't running
    for type in meshfedconfig serviceexposition servicebinding; do
        set +o errexit
        kubectl --context $ctx get $type --all-namespaces -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace --no-headers > /tmp/crds.txt
        set -o errexit
        if [ -s /tmp/crds.txt ]; then
            cat /tmp/crds.txt | \
                awk -v ctx=$ctx -v type=$type -v squote="'" '{ print "kubectl --context " ctx " -n " $2 " patch " type " " $1 " --type=merge --patch " squote "{\"metadata\": {\"finalizers\": []}}" squote }' | \
                xargs -0 bash -c
        fi
    done
}

trap shutdowns EXIT
main "$@"
exit 0

