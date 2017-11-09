sentry-kubernetes
=================

Watches Kubernetes events for warnings and errors and reports them to Sentry.

    kubectl run sentry-kubernetes \
      --image bretthoerner/sentry-kubernetes \
      --env="DSN=$YOUR_DSN"

Events are grouped in Sentry:

![1](/1.png)

They come with useful tags for filtering, and breadcrumbs showing events that occurred prior to the warning/error:

![2](/2.png)

And include all of the extra data attached to the event:

![3](/3.png)
