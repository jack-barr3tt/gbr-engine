#!/bin/bash

if ! minikube status | grep -q "host: Running"; then
    minikube start
fi

minikube addons enable ingress

kubectl delete jobs --all --ignore-not-found=true --wait=true --timeout=60s

kubectl kustomize k8s/bases --load-restrictor=LoadRestrictionsNone > k8s/rendered-manifests.yaml

pkill -f "minikube tunnel" || true
sleep 1
minikube tunnel &

skaffold dev