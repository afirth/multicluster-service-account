# Run e2e test
./hack/codegen.sh
./build/build.sh <version>
./test/e2e/e2e.sh <version>

## Teardown failed test
./test/e2e/tear_down_clusters.sh

## Notes
The `gke_` prefix on cluster names is not because the test is GKE specific, but rather to test an [issue](https://github.com/admiraltyio/multicluster-service-account/issues/10) when cluster names have invalid characters for k8s resource names
