#!/usr/bin/env bash
set -euxo pipefail

kubectl delete deployment -l type=test-pod
kubectl delete cronjob -l type=test-pod
kubectl delete job -l type=test-pod
kubectl delete pod -l type=test-pod
