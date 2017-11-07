sentry-kubernetes
=================

Watches Kubernetes events for warnings and errors and reports them to Sentry.

    kubectl run sentry-kubernetes \
      --image bretthoerner/sentry-kubernetes \
      --env="DSN=$YOUR_DSN"
