#!/usr/bin/env bash
set -euxo pipefail

kubectl delete deployment -l type=test-pod -A
kubectl delete cronjob -l type=test-pod -A
kubectl delete job -l type=test-pod -A
kubectl delete pod -l type=test-pod -A
