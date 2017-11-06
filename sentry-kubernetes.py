from kubernetes import client, config, watch
from kubernetes.client.rest import ApiException
from raven import breadcrumbs
from raven import Client as SentryClient

import logging
import os
from pprint import pprint
import sys
import time


logging.basicConfig(format='%(asctime)s %(message)s')

SDK_VALUE = {
    'name': 'sentry-kubernetes',
    'version': '1.0.0',
}

# mapping from k8s event types to event levels
LEVEL_MAPPING = {
    'normal': 'info',
}

dsn = os.environ.get('DSN')
sentry = SentryClient(
    dsn=dsn,
    install_sys_hook=False,
    install_logging_hook=False,
    include_versions=False,
    capture_locals=False,
    context={},
)

# Configs can be set in Configuration class directly or using helper utility
try:
    config.load_incluster_config()
except:
    config.load_kube_config()

v1 = client.CoreV1Api()
w = watch.Watch()

while True:
    try:
        for event in w.stream(v1.list_event_for_all_namespaces):
            event = event['object']

            # TODO grouping?
            # TODO only log events after startup?
            # TODO server_name
            # TODO environment
            # TODO pull out tags

            meta = {k: v for k, v in event.metadata.to_dict().items() if v is not None}
            meta['source'] = event.source
            meta['involved_object'] = {k: v for k, v in event.involved_object.to_dict().items() if v is not None}

            creation_timestamp = meta.pop('creation_timestamp', None)

            level = event.type.lower()
            level = LEVEL_MAPPING.get(level, level)

            if level in ('warning', 'error'):
                sentry.captureMessage(
                    event.message,
                    date=creation_timestamp,
                    data={'sdk': SDK_VALUE},
                    extra=meta,
                    tags={
                        'reason': event.reason,
                    },
                    level=level,
                )

            breadcrumbs.record(
                message=event.message,
                level=level,
                timestamp=creation_timestamp,
                data={
                    'name': meta['name'],
                    'namespace': meta['namespace']
                }
            )


    except ApiException as e:
        logging.error("Exception when calling CoreV1Api->list_event_for_all_namespaces: %s\n" % e)
        time.sleep(5)
