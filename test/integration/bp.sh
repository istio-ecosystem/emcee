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
}

cleanup() {
    # Clean up any existing multi-mesh configuration by removing and recreating CRDs
    kubectl --context $CLUSTER1 delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml 2> /dev/null || true
    kubectl --context $CLUSTER1 delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml 2> /dev/null  || true
    kubectl --context $CLUSTER2 delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml 2> /dev/null || true
    kubectl --context $CLUSTER2 delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml 2> /dev/null  || true
    kubectl --context $CLUSTER1 apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml || true
    kubectl --context $CLUSTER1 apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml || true
    kubectl --context $CLUSTER2 apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml || true
    kubectl --context $CLUSTER2 apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml || true

    # Clean up any existing generated Kubernetes and Istio configuration from previous runs
    kubectl --context $CLUSTER1 delete ns limited-trust 2> /dev/null || true
    kubectl --context $CLUSTER2 delete ns limited-trust 2> /dev/null || true

    # Clean up the config
    kubectl --context $CLUSTER1 delete -f $BASEDIR/samples/limited-trust/secret-c1.yaml 2> /dev/null || true
    kubectl --context $CLUSTER2 delete -f $BASEDIR/samples/limited-trust/secret-c2.yaml 2> /dev/null || true

    # Clean up the experiment
    kubectl --context $CLUSTER1 delete pod cli1 2> /dev/null || true
}

shutdown() {
    kill $MANAGER_1_PID
    echo controller1 killed

    kill $MANAGER_2_PID
    echo controller2 killed
}

main() {
    preconditions
    cleanup

    # TODO: Support --context in manager
    CFG_CLUSTER1=$TMPDIR/kubeconfig1
    CFG_CLUSTER2=$TMPDIR/kubeconfig2
    kubectl config use-context $CTX_CLUSTER1
    kubectl config view --minify > $CFG_CLUSTER1
    kubectl config use-context $CTX_CLUSTER2
    kubectl config view --minify > $CFG_CLUSTER2

    # Start cluster1 controller
    LOG1=$TMPDIR/log1
    ERR1=$TMPDIR/err1
    $BASEDIR/bin/manager --kubeconfig $CFG_CLUSTER1 --metrics-addr ":8080" > $LOG1 2> $ERR1 &
    MANAGER_1_PID=$!
    echo MANAGER_1_PID is $MANAGER_1_PID

    # Start cluster2 controller
    LOG2=$TMPDIR/log2
    ERR2=$TMPDIR/err2
    $BASEDIR/bin/manager --kubeconfig $CFG_CLUSTER2 --metrics-addr ":8081" > $LOG2 2> $ERR2 &
    MANAGER_2_PID=$!
    echo MANAGER_2_PID is $MANAGER_2_PID

    # Deploy secrets to be used during experiments
    kubectl --context $CLUSTER1 get ns # TODO REMOVE THIS LINE
    kubectl --context $CLUSTER1 create ns limited-trust
    kubectl --context $CLUSTER1 apply -f $BASEDIR/samples/limited-trust/secret-c1.yaml
    kubectl --context $CLUSTER2 create ns limited-trust
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/secret-c2.yaml

    # Deploy experiment
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/helloworld.yaml
    kubectl --context $CLUSTER1 run --generator=run-pod/v1 cli1 --image tutum/curl --command -- bash -c 'sleep 9999999'

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

    # Configure the meshes
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/limited-trust/limited-trust-c2.yaml
    kubectl --context $CLUSTER1 apply -f $BASEDIR/samples/limited-trust/limited-trust-c1.yaml

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

    # Bind helloworld to the actual dynamic exposed public IP
    cat $BASEDIR/samples/limited-trust/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -

    # Try to invoke exposed service from here (where this script is running)
    kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust
    kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust --output jsonpath="{.data.tls\.key}" | base64 -D > /tmp/c1.example.com.key
    kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust --output jsonpath="{.data.tls\.crt}" | base64 -D > /tmp/c1.example.com.crt
    kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust --output jsonpath="{.data.example\.com\.crt}" | base64 -D > /tmp/example.com.crt
    CURL_CMD="curl --resolve c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT:$CLUSTER2_INGRESS_HOST \
        --cacert /tmp/example.com.crt --key /tmp/c1.example.com.key \
        --cert /tmp/c1.example.com.crt \
        https://c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT/default/helloworld/hello \
        -w '%{http_code}' -o /dev/null"
    SCRIPT_OUTPUT=$($CURL_CMD)
    if [ "$SCRIPT_OUTPUT" == "200" ]; then
        echo Expected 200 but got $SCRIPT_OUTPUT executing
        echo $CURL_CMD
        exit 7
    fi
    echo Exposure worked; test with
    echo $CURL_CMD

    # Wait for the experiment client (consumer) to be up
    kubectl --context $CLUSTER1 wait --for=condition=ready --timeout=60s pod/cli1

    # Have the sleep pod test
    SLEEP_POD=$(kubectl --context $CLUSTER1 get pods -l app=sleep -o jsonpath="{.items..metadata.name}")
    Echo using Sleep pod $SLEEP_POD on $CLUSTER1
    CURL_CMD="kubectl --context $CLUSTER1 exec -it $SLEEP_POD -- curl helloworld:5000/hello -w '%{http_code}' -o /dev/null"
    REMOTE_OUTPUT=$($CURL_CMD)
    if [ "$REMOTE_OUTPUT" == "200" ]; then
        echo Expected 200 but got $REMOTE_OUTPUT executing
        echo $CURL_CMD
        exit 7
    fi
    echo Bind worked; test with
    echo $CURL_CMD

    shutdown
}

main "$@"
