sentry-kubernetes
=================

Errors and warnings in Kubernetes often go unnoticed by operators. Even when they are checked they are hard to read and understand in the context of what else is going on in the cluster. `sentry-kubernetes` is a small container you launch inside your Kubernetes cluster that will send errors and warnings to Sentry where they will be cleanly presented and intelligently grouped. Typical Sentry features such as notifications can then be used to help operation and developer visibility.

Create a new project on [Sentry](http://sentry.io/) and use your DSN when launching the `sentry-kubernetes` container:

    kubectl run sentry-kubernetes \
      --image getsentry/sentry-kubernetes \
      --env="DSN=$YOUR_DSN"

#### Filters and options

See the full list in sentry-kubernetes.py

| ENV var | Description |
---------|-------------
EVENT_NAMESPACES_EXCLUDED | A comma-separated list of namespaces. Ex.: 'qa,demo'. Events from these namespaces won't be sent to Sentry.

---

Events are grouped in Sentry:

![1](/1.png)

---

They come with useful tags for filtering, and breadcrumbs showing events that occurred prior to the warning/error:

![2](/2.png)

---

And include all of the extra data attached to the event:

![3](/3.png)

## Install using helm charts

```console
$ helm repo add incubator https://kubernetes-charts-incubator.storage.googleapis.com/
"incubator" has been added to your repositories

$ helm install incubator/sentry-kubernetes --name my-release --set sentry.dsn=<your-dsn>
```
