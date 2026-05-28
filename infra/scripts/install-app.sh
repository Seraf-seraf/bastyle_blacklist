#!/usr/bin/env bash
set -euo pipefail

kubectl create namespace bastyle --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install bastyle infra/helm/bastyle \
  --namespace bastyle
