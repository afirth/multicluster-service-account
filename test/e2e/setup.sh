#!/usr/bin/env bash
set -euo pipefail

VERSION="$1"

IMAGES=(
  "service-account-import-admission-controller"
  "service-account-import-controller"
  "multicluster-service-account-example-multicluster-client"
)

for IMAGE in "${IMAGES[@]}"; do
  kind load docker-image "quay.io/admiralty/$IMAGE:$VERSION" --name gke_cluster1
done
KUBECONFIG=kubeconfig-gke_cluster1 kubectl apply -f _out/install.yaml

OS=linux
kubemcsa="_out/kubemcsa-$OS-amd64"
$kubemcsa bootstrap \
  --target-kubeconfig kubeconfig-gke_cluster1 \
  --source-kubeconfig kubeconfig-gke_cluster2
