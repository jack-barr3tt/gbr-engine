#!/bin/bash

if ! minikube status | grep -q "host: Running"; then
    minikube start
fi

kubectl delete jobs --all --ignore-not-found=true

kubectl kustomize k8s/bases --load-restrictor=LoadRestrictionsNone > k8s/rendered-manifests.yaml

skaffold dev