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

    while True:
        try:
            watch_loop()
        except ApiException as e:
            logging.error("Exception when calling CoreV1Api->list_event_for_all_namespaces: %s\n" % e)
            time.sleep(5)

def watch_loop():
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

    for event in w.stream(v1.list_event_for_all_namespaces):
        logging.debug("event: %s" % event)

        event_type = event['type'].lower()
        event = event['object']

        meta = {
            k: v for k, v
            in event.metadata.to_dict().items()
            if v is not None
        }

        creation_timestamp = meta.pop('creation_timestamp', None)

        level = (event.type and event.type.lower())
        level = LEVEL_MAPPING.get(level, level)

        source_component = source_host = reason = namespace = name = short_name = kind = None
        if event.source:
            source = event.source.to_dict()

            if 'component' in source:
                source_component = source['component']
            if 'host' in source:
                source_host = source['host']

        if event.reason:
            reason = event.reason

        if event.involved_object and event.involved_object.namespace:
            namespace = event.involved_object.namespace
        elif 'namespace' in meta:
            namespace = meta['namespace']

        if event.involved_object and event.involved_object.name:
            name = event.involved_object.name
            short_name = "-".join(name.split('-')[:-2])

        if event.involved_object and event.involved_object.kind:
            kind = event.involved_object.kind

        message = event.message

        # if namespace and name:
        #     culprit = "%s.%s" % (namespace, name)
        #     # message = "%(msg)s (%(namespace)s/%(name)s)" % {
        #     #     'namespace': namespace,
        #     #     'name': name,
        #     #     'msg': event.message,
        #     # }
        # else:
        #     culprit = "%s" % (namespace, )
        #     # message = "%(msg)s (%(namespace)s)" % {
        #     #     'namespace': namespace,
        #     #     'msg': event.message,
        #     # }

        if level in ('warning', 'error') or event_type in ('error', ):
            if event.involved_object:
                meta['involved_object'] = {
                    k: v for k, v
                    in event.involved_object.to_dict().items()
                    if v is not None
                }

            fingerprint = []
            tags = {}

            if source_component:
                tags['source_component'] = source_component

            if source_host:
                tags['source_host'] = source_host

            if reason:
                tags['reason'] = event.reason
                fingerprint.append(event.reason)

            if namespace:
                tags['namespace'] = namespace
                fingerprint.append(namespace)

            if short_name:
                tags['name'] = short_name
                fingerprint.append(short_name)

            if kind:
                tags['kind'] = kind
                fingerprint.append(kind)

            data = {
                'sdk': SDK_VALUE,
                'server_name': SERVER_NAME,
                'culprit': reason,
            }

            sentry.captureMessage(
                message,
                # culprit=culprit,
                data=data,
                date=creation_timestamp,
                environment=ENV,
                extra=meta,
                fingerprint=fingerprint,
                level=level,
                tags=tags,
            )

        data = {}
        if name:
            data['name'] = name
        if namespace:
            data['namespace'] = namespace

        breadcrumbs.record(
            # data=data,
            level=level,
            message=message,
            timestamp=creation_timestamp,
        )


if __name__ == '__main__':
    main()
