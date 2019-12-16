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
BASEDIR=$DIR/..
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
        kill $MANAGER_1_PID || true
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

main() {
    preconditions

    # TODO: Support --context in manager
    CFG_CLUSTER1=$TMPDIR/kubeconfig1
    CFG_CLUSTER2=$TMPDIR/kubeconfig2
    kubectl config use-context $CTX_CLUSTER1
    kubectl config view --minify > $CFG_CLUSTER1
    kubectl config use-context $CTX_CLUSTER2
    kubectl config view --minify > $CFG_CLUSTER2

    # Start controllers
    startup2
    startup1

    tail -f /tmp/err1 /tmp/err2 || true
    echo terminating run-two-managers.sh
}

trap shutdowns EXIT
main "$@"
exit 0

