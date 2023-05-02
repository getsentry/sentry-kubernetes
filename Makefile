
docker-build:
	docker build -t sentry-kubernetes .
.PHONY: docker-build

upload-image-kind: docker-build
	kind load docker-image sentry-kubernetes

reapply-deployment: upload-image-kind
	kubectl delete deployment sentry-kubernetes
	kubectl apply -f k8s/sa.yaml
	yq ".metadata.annotations.deploy = \"$(shell date +%s)\"" <k8s/deployment.yaml | kubectl apply -f -
