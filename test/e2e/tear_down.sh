#!/usr/bin/env bash
set -euo pipefail

KUBECONFIG=kubeconfig-gke_cluster1 kubectl delete -f _out/install.yaml
KUBECONFIG=kubeconfig-gke_cluster2 kubectl delete namespace multicluster-service-account # TODO? run kubemcsa unstrap when it exists
