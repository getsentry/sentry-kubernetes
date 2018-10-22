import os
import time
import logging
import argparse

from kubernetes import client, config, watch
from kubernetes.client.rest import ApiException
from raven import breadcrumbs
from raven import Client as SentryClient
from raven.transport.threaded_requests import ThreadedRequestsHTTPTransport
from urllib3.exceptions import ProtocolError


SDK_VALUE = {"name": "sentry-kubernetes", "version": "1.0.0"}

# mapping from k8s event types to event levels
LEVEL_MAPPING = {"normal": "info"}

DSN = os.environ.get("DSN")
ENV = os.environ.get("ENVIRONMENT")
RELEASE = os.environ.get("RELEASE")
EVENT_NAMESPACE = os.environ.get("EVENT_NAMESPACE")
MANGLE_NAMES = [
    name for name in os.environ.get("MANGLE_NAMES", default="").split(",") if name
]


def parse_cmd_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--log-level", default="error")
    parser.add_argument(
        "--ignore-ssl",
        action="store_true",
        help="Ignore SSL verify while requesting API",
    )
    return parser.parse_args()


def setup_logger(cmd_args):
    log_level = cmd_args.log_level.upper()
    logging.basicConfig(format="%(asctime)s %(message)s", level=log_level)
    logging.debug("log_level: %s" % log_level)


def make_config():
    try:
        config.load_incluster_config()
    except Exception:
        config.load_kube_config()


def main():
    cmd_args = parse_cmd_args()

    setup_logger(cmd_args)
    make_config()

    while True:
        try:
            watch_loop(cmd_args)
        except ApiException as e:
            logging.error(
                "Exception when calling CoreV1Api->list_event_for_all_namespaces: %s\n"
                % e
            )
            time.sleep(5)
        except ProtocolError:
            logging.warning("ProtocolError exception. Continuing...")
        except Exception as e:
            logging.exception("Unhandled exception occurred.")


def watch_loop(cmd_args):
    v1 = client.CoreV1Api()

    if cmd_args.ignore_ssl:
        v1.api_client.configuration.verify_ssl = False

    w = watch.Watch()

    sentry = SentryClient(
        dsn=DSN,
        install_sys_hook=False,
        install_logging_hook=False,
        include_versions=False,
        capture_locals=False,
        context={},
        environment=ENV,
        release=RELEASE,
        transport=ThreadedRequestsHTTPTransport,
    )

    # try:
    #     resource_version = v1.list_event_for_all_namespaces().items[-1].metadata.resource_version
    # except:
    #     resource_version = 0

    if EVENT_NAMESPACE:
        stream = w.stream(v1.list_namespaced_event, EVENT_NAMESPACE)
    else:
        stream = w.stream(v1.list_event_for_all_namespaces)

    for event in stream:
        logging.debug("event: %s" % event)

        event_type = event["type"].lower()
        event = event["object"]

        meta = {k: v for k, v in event.metadata.to_dict().items() if v is not None}

        creation_timestamp = meta.pop("creation_timestamp", None)

        level = event.type and event.type.lower()
        level = LEVEL_MAPPING.get(level, level)

        component = source_host = reason = namespace = name = short_name = kind = None
        if event.source:
            source = event.source.to_dict()

            if "component" in source:
                component = source["component"]
            if "host" in source:
                source_host = source["host"]

        if event.reason:
            reason = event.reason

        if event.involved_object and event.involved_object.namespace:
            namespace = event.involved_object.namespace
        elif "namespace" in meta:
            namespace = meta["namespace"]

        if event.involved_object and event.involved_object.kind:
            kind = event.involved_object.kind

        if event.involved_object and event.involved_object.name:
            name = event.involved_object.name
            if not MANGLE_NAMES or kind in MANGLE_NAMES:
                bits = name.split("-")
                if len(bits) in (1, 2):
                    short_name = bits[0]
                else:
                    short_name = "-".join(bits[:-2])
            else:
                short_name = name

        message = event.message

        if namespace and short_name:
            obj_name = "(%s/%s)" % (namespace, short_name)
        else:
            obj_name = "(%s)" % (namespace,)

        if level in ("warning", "error") or event_type in ("error",):
            if event.involved_object:
                meta["involved_object"] = {
                    k: v
                    for k, v in event.involved_object.to_dict().items()
                    if v is not None
                }

            fingerprint = []
            tags = {}

            if component:
                tags["component"] = component

            if reason:
                tags["reason"] = event.reason
                fingerprint.append(event.reason)

            if namespace:
                tags["namespace"] = namespace
                fingerprint.append(namespace)

            if short_name:
                tags["name"] = short_name
                fingerprint.append(short_name)

            if kind:
                tags["kind"] = kind
                fingerprint.append(kind)

            data = {
                "sdk": SDK_VALUE,
                "server_name": source_host or "n/a",
                "culprit": "%s %s" % (obj_name, reason),
            }

            sentry.captureMessage(
                message,
                # culprit=culprit,
                data=data,
                date=creation_timestamp,
                extra=meta,
                fingerprint=fingerprint,
                level=level,
                tags=tags,
            )

        data = {}
        if name:
            data["name"] = name
        if namespace:
            data["namespace"] = namespace

        breadcrumbs.record(
            data=data,
            level=level,
            message=message,
            timestamp=time.mktime(creation_timestamp.timetuple()),
        )


if __name__ == "__main__":
    main()
