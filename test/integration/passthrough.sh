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







main() {
    preconditions

    # TODO: Support --context in manager
    CFG_CLUSTER1=$TMPDIR/kubeconfig1
    CFG_CLUSTER2=$TMPDIR/kubeconfig2
    kubectl config use-context $CTX_CLUSTER1
    kubectl config view --minify > $CFG_CLUSTER1
    kubectl config use-context $CTX_CLUSTER2
    kubectl config view --minify > $CFG_CLUSTER2

    

    #MBMB  Setuo secrets

    # Deploy experiment
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/helloworld.yaml


    # Configure the mesh
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/limited-trust-c2.yaml


    # Wait for the experiment helloworld (producer) to be up
    kubectl --context $CLUSTER2 wait --for=condition=available --timeout=60s deployment/helloworld-v1

    # Expose helloworld
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/helloworld-expose.yaml

    # Wait for the exposure to be affected
    until kubectl --context $CLUSTER2 -n limited-trust get service istio-limited-trust-ingress-15443 ; do
        echo Waiting for controller to create exposure service istio-limited-trust-ingress-15443
        sleep 1
    done

    # Where is the gateway for traffic to the exposed service?
    CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n limited-trust --selector mesh=limited-trust,role=ingress-svc --output jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
    echo Using $CLUSTER2 ingress at $CLUSTER2_INGRESS:15443
    CLUSTER2_SECURE_INGRESS_PORT=15443
    CLUSTER2_INGRESS_HOST=$CLUSTER2_INGRESS
    if [ -z $CLUSTER2_INGRESS_HOST ]; then
        echo CLUSTER2_INGRESS_HOST is not set
        exit 6
    fi


    # Deploy experiment consumer
    kubectl --context $CLUSTER1 run --generator=run-pod/v1 cli1 --image tutum/curl --command -- bash -c 'sleep 9999999'
    # Wait for the experiment client (consumer) to be up
    kubectl --context $CLUSTER1 wait --for=condition=ready --timeout=60s pod/cli1

    # Configure the meshes
    kubectl --context $CLUSTER1 apply -f $BASEDIR/samples/limited-trust/limited-trust-c1.yaml

    # Bind helloworld to the actual dynamic exposed public IP
    cat $BASEDIR/samples/limited-trust/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -

    # Wait for the exposure to be affected
    until kubectl --context $CLUSTER1 get service helloworld ; do
        echo Waiting for controller to create helloworld service
        sleep 1
    done

    # TODO REMOVE
    sleep 5

    # Have the sleep pod test
    SLEEP_POD=cli1
    Echo using Sleep pod $SLEEP_POD on $CLUSTER1
    CURL_CMD="kubectl --context $CLUSTER1 exec -it $SLEEP_POD -- curl --silent helloworld:5000/hello -w '%{http_code}' -o /dev/null"
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

trap shutdowns EXIT
main "$@"
exit 0

