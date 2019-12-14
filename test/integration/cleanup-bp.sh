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

for ctx in $CTX_CLUSTER1 $CTX_CLUSTER2; do
    CTX=$ctx ./tools/remove-finalizers.sh
    kubectl --context $ctx delete ns limited-trust 2> /dev/null || true
done

kubectl --context $CTX_CLUSTER1 delete servicebinding helloworld 2> /dev/null || true
kubectl --context $CTX_CLUSTER2 delete serviceexposition se1 2> /dev/null || true

echo Done

