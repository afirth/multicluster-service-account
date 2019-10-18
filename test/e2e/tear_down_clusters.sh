#!/usr/bin/env bash
set -euo pipefail

for CLUSTER in gke_cluster1 gke_cluster2; do
  rm -f kubeconfig-$CLUSTER
  kind delete cluster --name $CLUSTER # if exists
done
