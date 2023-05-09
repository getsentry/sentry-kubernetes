#!/usr/bin/env bash
set -euxo pipefail

kubectl delete pod -l type=test-pod
