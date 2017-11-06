FROM python:3
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY sentry-kubernetes.py ./
CMD [ "python", "./sentry-kubernetes.py" ]
