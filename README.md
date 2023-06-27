# sentry-kubernetes

Yet another Sentry Kubernetes agent (should it be called "YASKA"?)

## Configuration

`SENTRY_DSN` - Sentry DSN that will be used by the agent.

`SENTRY_K8S_WATCH_NAMESPACES` - a comma-separated list of namespaces that will be watched. Only the `default` namespace is watched by default.

`SENTRY_K8S_WATCH_HISTORICAL` - if set to `1`, all existing (old) events will also be reported. Default is `0` (old events will not be reported).

`SENTRY_K8S_CLUSTER_CONFIG_TYPE` - the type of the cluster initialization method. Allowed options: `auto`, `in-cluster`, `out-cluster`. Default is `auto`.

`SENTRY_K8S_KUBECONFIG_PATH` - filesystem path to the `kubeconfig` configuration that will be used to connect to the cluster. Not used if `SENTRY_K8S_CLUSTER_CONFIG_TYPE` is set to `in-cluster`.

### Adding custom tags

To add a custom tag to all events produced by the agent, set an environment variable, whose name is prefixed with `SENTRY_K8S_GLOBAL_TAG_`.

**Example:**

`SENTRY_K8S_GLOBAL_TAG_cluster_name=main-cluster` will add `cluster_name=main_cluster` tag to every outgoing Sentry event.
