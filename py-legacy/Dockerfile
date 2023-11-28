# Build environment has gcc and develop header files.
# The installation is copied to the smaller runtime container.
FROM python:3.7 AS build-image

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Start runtime container
FROM python:3.7-slim

RUN apt-get update && \
    apt-get install --no-install-recommends -y libyaml-0-2 && \
    rm -rf /var/lib/apt/lists/* /var/cache/debconf/*-old && \
    useradd --system --user-group app

# Install dependencies from build image
COPY --from=build-image /usr/local/lib/python3.7/site-packages/ /usr/local/lib/python3.7/site-packages/
COPY sentry-kubernetes.py ./
USER app
CMD [ "python", "./sentry-kubernetes.py" ]
