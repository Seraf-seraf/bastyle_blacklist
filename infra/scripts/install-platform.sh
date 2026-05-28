#!/usr/bin/env bash
set -euo pipefail

kubectl create namespace vault --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace bastyle --dry-run=client -o yaml | kubectl apply -f -

helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update

helm upgrade --install vault hashicorp/vault \
  --namespace vault \
  -f infra/helm/platform/vault-values.yaml

helm upgrade --install vault-secrets-operator hashicorp/vault-secrets-operator \
  --namespace vault \
  -f infra/helm/platform/vault-secrets-operator-values.yaml
