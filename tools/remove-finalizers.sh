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

if [ -z "${CTX+xxx}" ]; then
   CTX=$(kubectl config current-context)
   echo CTX not set, defaulting to $CTX
   exit 1
fi

ctx=$CTX

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

echo Done
