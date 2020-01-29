#!/bin/bash

# Licensed Materials - Property of IBM
# (C) Copyright IBM Corp. 2019. All Rights Reserved.
# US Government Users Restricted Rights - Use, duplication or
# disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
# Copyright 2019 IBM Corporation


#./emcee --context $CLUSTER1 --metrics-addr ":8081" --grpc-server-addr ":50051" --grpc-discovery-addr "localhost:50052"

#./emcee --context $CLUSTER2 --metrics-addr ":8082" --grpc-server-addr ":50052" --grpc-discovery-addr "localhost:50051"


set -o errexit

if [ -n "$MM_DEBUG" ]; then
   set -o xtrace
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
BASEDIR=$DIR/../..
TMPDIR=/tmp
USEDISCOVERY=YES

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

if [ -z "$1" ]
  then
    echo "*** No argument supplied; using limited-trust configuration"
    MODE="limited-trust"
  else
    echo "*** Usings " $1 " configuration"
    MODE=$1
fi

if [ -z "$2" ]
  then
    echo "*** Not using automatic discovery"
    USEDISCOVERY=NO
  else
    echo "*** Using automatic discovery"
    USEDISCOVERY=YES
fi

if [[ $MODE != "limited-trust" && $MODE != "passthrough" ]];then
    echo "*** " $MODE " not an accepted mode"
    exit 1
fi


if [[ $MODE = "passthrough" ]];then
    if ! hash istioctl 2>/dev/null
    then
        echo "*** istioctl not found in PATH"
       exit 1
    fi
fi


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

remove_finalizers() {
    ctx=$1

    # Remove finalizers, so that deletes don't block forever if controller isn't running
    for type in meshfedconfig serviceexposition servicebinding; do
        set +o errexit
        kubectl --context $ctx get $type --all-namespaces -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace --no-headers > $TMPDIR/crds.txt
        set -o errexit
        if [ -s $TMPDIR/crds.txt ]; then
            cat $TMPDIR/crds.txt | \
                awk -v ctx=$ctx -v type=$type -v squote="'" '{ print "kubectl --context " ctx " -n " $2 " patch " type " " $1 " --type=merge --patch " squote "{\"metadata\": {\"finalizers\": []}}" squote }' | \
                xargs -0 bash -c
        fi
    done
}

cleanup() {
    CLUSTER_NAME=CLUSTER$1
    CLUSTER=${!CLUSTER_NAME}
    # NOTE: Don't call this method if the operator is down and the CRs have finalizers
    # (see remove_finalizers)
    remove_finalizers $CLUSTER

    # Clean up any existing multi-mesh configuration by removing and recreating CRDs
    kubectl --context $CLUSTER delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml 2> /dev/null || true
    kubectl --context $CLUSTER delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml 2> /dev/null  || true
    kubectl --context $CLUSTER delete -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml 2> /dev/null || true

    kubectl --context $CLUSTER apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml || true
    kubectl --context $CLUSTER apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_servicebindings.yaml || true
    kubectl --context $CLUSTER apply -f $BASEDIR/config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml || true

    # Clean up the experiment
    kubectl --context $CLUSTER delete pod cli1 2> /dev/null || true

    kubectl --context $CLUSTER delete ns $MODE 2> /dev/null || true
    until kubectl --context $CLUSTER create ns $MODE ; do
        echo Waiting creating $MODE namespace
        sleep 1
    done


}

startup1() {
    # Start cluster1 controller
    LOG1=$TMPDIR/log1
    ERR1=$TMPDIR/err1
    if [ $USEDISCOVERY == "YES" ]; then
       $BASEDIR/bin/emcee --context $CLUSTER1 --metrics-addr ":8081" --grpc-server-addr ":50051" --grpc-discovery-addr "localhost:50052" > $LOG1 2> $ERR1 &
    else
       $BASEDIR/bin/emcee --context $CLUSTER1 --metrics-addr ":8081" --grpc-server-addr ":50051" > $LOG1 2> $ERR1 &
    fi

    MANAGER_1_PID=$!
    echo MANAGER_1_PID is $MANAGER_1_PID
}

startup2() {
    # Start cluster2 controller
    LOG2=$TMPDIR/log2
    ERR2=$TMPDIR/err2
    if [ $USEDISCOVERY == "YES" ]; then
       $BASEDIR/bin/emcee --context $CLUSTER2 --metrics-addr ":8082" --grpc-server-addr ":50052" --grpc-discovery-addr "localhost:50051" > $LOG2 2> $ERR2 &
    else
       $BASEDIR/bin/emcee --context $CLUSTER2 --metrics-addr ":8082" --grpc-server-addr ":50052" > $LOG2 2> $ERR2 &
    fi
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

secrets_passthrough() {
    CLUSTER_NAME=CLUSTER$1
    CLUSTER=${!CLUSTER_NAME}

    kubectl --context $CLUSTER delete secret -n istio-system cacerts 2> /dev/null || true
    kubectl --context $CLUSTER create secret generic cacerts -n istio-system --from-file=samples/certs/ca-cert.pem     --from-file=samples/certs/ca-key.pem --from-file=samples/certs/root-cert.pem --from-file=samples/certs/cert-chain.pem
    istioctl --context $CLUSTER  manifest apply --set values.global.mtls.enabled=true,values.security.selfSigned=false
    kubectl --context $CLUSTER  delete secret istio.default

    kubectl --context $CLUSTER  create namespace passthrough  2> /dev/null || true
}

secrets_limited-trust() {
    CLUSTER_NAME=CLUSTER$1
    CLUSTER=${!CLUSTER_NAME}

    echo "creating secret" $BASEDIR/samples/$MODE/secret-c$1.yaml
    kubectl --context $CLUSTER apply -f $BASEDIR/samples/$MODE/secret-c$1.yaml
    sleep 5
    if [ $1 = 1 ]; then 
        # Try to invoke exposed service from here (where this script is running)
        kubectl --context $CLUSTER -n limited-trust get secret limited-trust
        kubectl --context $CLUSTER -n limited-trust get secret limited-trust --output jsonpath="{.data.tls\.key}" | base64 -D > $TMPDIR/c1.example.com.key
        kubectl --context $CLUSTER -n limited-trust get secret limited-trust --output jsonpath="{.data.tls\.crt}" | base64 -D > $TMPDIR/c1.example.com.crt
        kubectl --context $CLUSTER -n limited-trust get secret limited-trust --output jsonpath="{.data.example\.com\.crt}" | base64 -D > $TMPDIR/example.com.crt
    fi
}

end_to_end(){
    # Have the sleep pod test
    SVC_NAME=$1
    SLEEP_POD=cli1
    Echo using Sleep pod $SLEEP_POD on $CLUSTER1
    CURL_CMD="kubectl --context $CLUSTER1 exec -it $SLEEP_POD -- curl --silent ${SVC_NAME}:5000/hello -w '%{http_code}' -o /dev/null"
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


main() {
    preconditions
    for i in {2..1}
    do
        cleanup $i
        secrets_$MODE $i
    done
    # ------------------------ Exposing Cluster ----------------------
    startup2

    # Deploy experiment
    kubectl --context $CLUSTER2 apply -f $BASEDIR/test/integration/common/helloworld.yaml

    # Verify the controller is still running
    if ps -p $MANAGER_2_PID > /dev/null ; then
        echo MANAGER_2 running
    else
        echo MANAGER_2 is not running
        cat $ERR2
        exit 3
    fi

    # Wait for controller to be up
    # TODO is there a cleaner way?
    #while [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8082)" != "404" ]; do
    #    sleep 1
    #	echo "."
    #done

    # TODO remove
    sleep 5

    # Configure the mesh
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/$MODE/$MODE-c2.yaml

    # TODO remove
    sleep 5

    # Wait for the experiment helloworld (producer) to be up
    kubectl --context $CLUSTER2 wait --for=condition=available --timeout=60s deployment/helloworld-v1

    # Expose helloworld
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/$MODE/helloworld-expose.yaml

    if [ "$MODE" = "limited-trust" ]; then
        # Wait for the exposure to be affected
        until kubectl --context $CLUSTER2 -n $MODE get service istio-limited-trust-ingress-15443 ; do
            echo Waiting for controller to create exposure service istio-limited-trust-ingress-15443
            sleep 1
        done
        # Where is the gateway for traffic to the exposed service?
        CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n limited-trust --selector mesh=limited-trust,role=ingress-svc --output jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
    fi    
    if [ "$MODE" = "passthrough" ]; then
        CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n istio-system istio-ingressgateway --output jsonpath="{.status.loadBalancer.ingress[0].ip}")
    fi

    echo Using $CLUSTER2 ingress at $CLUSTER2_INGRESS:15443
    CLUSTER2_SECURE_INGRESS_PORT=15443
    CLUSTER2_INGRESS_HOST=$CLUSTER2_INGRESS
    if [ -z $CLUSTER2_INGRESS_HOST ]; then
        echo CLUSTER2_INGRESS_HOST is not set
        exit 6
    fi

    sleep 4

    if [ "$MODE" = "limited-trust" ]; then    
        CURL_CMD="curl --resolve c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT:$CLUSTER2_INGRESS_HOST \
            --cacert ${TMPDIR}/example.com.crt --key ${TMPDIR}/c1.example.com.key \
            --cert ${TMPDIR}/c1.example.com.crt \
            https://c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT/default/helloworld/hello \
            -w '%{http_code}' -o /dev/null"
        set +o errexit
        SCRIPT_OUTPUT=$($CURL_CMD)
        set -o errexit
        if [ "$SCRIPT_OUTPUT" != "'200'" ]; then
            echo Expected 200 but got $SCRIPT_OUTPUT executing
            echo $CURL_CMD
            exit 7
        fi
        echo
        echo =======================================================
        echo Exposure worked; test with
        echo $CURL_CMD
        echo =======================================================
        echo
    fi

    # ------------------------ Binding Cluster ----------------------
    startup1

    # Deploy experiment consumer
    kubectl --context $CLUSTER1 run --generator=run-pod/v1 cli1 --image tutum/curl --command -- bash -c 'sleep 9999999'

    # Verify the controllers are still running
    if ps -p $MANAGER_1_PID > /dev/null ; then
        echo MANAGER_1 running
    else
        echo MANAGER_1 is not running
        cat $ERR1
        exit 3
    fi

    # Wait for controllers to be up
    # TODO is there a cleaner way?
    #while [ "$(curl -s -o /dev/null -w "%{http_code}" localhost:8081)" != "404" ]; do
    #    sleep 1
    #done

    # Wait for the experiment client (consumer) to be up
    kubectl --context $CLUSTER1 wait --for=condition=ready --timeout=60s pod/cli1

    kubectl --context $CLUSTER1 apply -f $BASEDIR/samples/$MODE/$MODE-c1.yaml

    # Bind helloworld to the actual dynamic exposed public IP
    if [ $USEDISCOVERY == "NO" ]; then
       cat $BASEDIR/samples/$MODE/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
    fi

    # Wait for the exposure to be affected
    until kubectl --context $CLUSTER1 get service helloworld ; do
        echo Waiting for controller to create helloworld service
        sleep 1
    done

    # Simple end to end test
    sleep 5
    end_to_end  "helloworld"

    # exit 0

    # End to end test with alias in expose side
    kubectl --context $CLUSTER2 apply -f $BASEDIR/test/integration/common/holamundo.yaml
    kubectl --context $CLUSTER2 delete ServiceExposition --all
    sleep 10
    kubectl --context $CLUSTER2 apply -f $BASEDIR/samples/$MODE/helloworld-expose-with-alias.yaml
    sleep 5
    end_to_end  "helloworld"

    # exit 0

    # End to end test with alias in bind side
    kubectl --context $CLUSTER1 delete ServiceBinding --all
    sleep 10
    if [ $USEDISCOVERY == "NO" ]; then
        cat $BASEDIR/samples/$MODE/helloworld-binding-with-alias.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
    fi
    sleep 5
    end_to_end "helloworldyall"

}

trap shutdowns EXIT
main "$@"
exit 0
