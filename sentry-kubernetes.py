# TODO grouping?

from kubernetes import client, config, watch
from kubernetes.client.rest import ApiException
from raven import breadcrumbs
from raven import Client as SentryClient

import argparse
import logging
import os
from pprint import pprint
import socket
import sys
import time


SDK_VALUE = {
    'name': 'sentry-kubernetes',
    'version': '1.0.0',
}

# mapping from k8s event types to event levels
LEVEL_MAPPING = {
    'normal': 'info',
}

SERVER_NAME = socket.gethostname() if hasattr(socket, 'gethostname') else None
try:
    SERVER_NAME = "-".join(SERVER_NAME.split("-")[:-2])
except:
    pass
DSN = os.environ.get('DSN')
ENV = os.environ.get('ENVIRONMENT')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--log-level", default="error")
    args = parser.parse_args()

    log_level = args.log_level.upper()
    logging.basicConfig(format='%(asctime)s %(message)s', level=log_level)
    logging.debug("log_level: %s" % log_level)

    try:
        config.load_incluster_config()
    except:
        config.load_kube_config()

    v1 = client.CoreV1Api()
    w = watch.Watch()

    sentry = SentryClient(
        dsn=DSN,
        install_sys_hook=False,
        install_logging_hook=False,
        include_versions=False,
        capture_locals=False,
        context={},
    )

    # try:
    #     resource_version = v1.list_event_for_all_namespaces().items[-1].metadata.resource_version
    # except:
    #     resource_version = 0

    while True:
        try:
            for event in w.stream(v1.list_event_for_all_namespaces):
                logging.debug("event: %s" % event)
                event_type = event['type'].lower()
                event = event['object']

                meta = {
                    k: v for k, v
                    in event.metadata.to_dict().items()
                    if v is not None
                }

                if event.involved_object:
                    meta['involved_object'] = {
                        k: v for k, v
                        in event.involved_object.to_dict().items()
                        if v is not None
                    }

                if event.source:
                    meta['source'] = event.source

                creation_timestamp = meta.pop('creation_timestamp', None)

                level = (event.type and event.type.lower())
                level = LEVEL_MAPPING.get(level, level)

                if level in ('warning', 'error') or event_type in ('error', ):
                    tags = {}
                    if event.reason:
                        tags['reason'] = event.reason
                    if 'namespace' in meta:
                        tags['namespace'] = meta['namespace']
                    if event.involved_object and event.involved_object.kind:
                        tags['kind'] = event.involved_object and event.involved_object.kind

                    data = {
                        'sdk': SDK_VALUE,
                        'server_name': SERVER_NAME,
                    }

                    sentry.captureMessage(
                        event.message,
                        date=creation_timestamp,
                        data=data,
                        extra=meta,
                        tags=tags,
                        level=level,
                        environment=ENV,
                    )

                data = {}
                if 'name' in meta:
                    data['name'] = meta['name']
                if 'namespace' in meta:
                    data['namespace'] = meta['namespace']

                breadcrumbs.record(
                    message=event.message,
                    level=level,
                    timestamp=creation_timestamp,
                    data=data,
                )
        except ApiException as e:
            logging.error("Exception when calling CoreV1Api->list_event_for_all_namespaces: %s\n" % e)
            time.sleep(5)

if __name__ == '__main__':
    main()
