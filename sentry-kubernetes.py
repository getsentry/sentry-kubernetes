from kubernetes import client, config, watch
from kubernetes.client.rest import ApiException
from raven import Client as SentryClient

import os
from pprint import pprint
import time

dsn = os.environ.get('DSN')
sentry = SentryClient(dsn)

# Configs can be set in Configuration class directly or using helper utility
config.load_kube_config()

v1 = client.CoreV1Api()
w = watch.Watch()

while True:
    try:
        for event in w.stream(v1.list_event_for_all_namespaces):
            pprint(event)
            print()
    except ApiException as e:
        print("Exception when calling CoreV1Api->list_event_for_all_namespaces: %s\n" % e)
        time.sleep(5)
