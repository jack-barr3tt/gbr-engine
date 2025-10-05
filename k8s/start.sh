#!/bin/bash

if ! minikube status | grep -q "host: Running"; then
    minikube start
fi

kubectl kustomize k8s/bases --load-restrictor=LoadRestrictionsNone > k8s/rendered-manifests.yaml

kustomize build k8s/bases --load-restrictor=LoadRestrictionsNone | kubectl apply -f -

skaffold dev