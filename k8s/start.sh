#!/bin/bash

if ! minikube status | grep -q "host: Running"; then
    minikube start
fi

kustomize build k8s/bases --load-restrictor=LoadRestrictionsNone | kubectl apply -f -

skaffold dev