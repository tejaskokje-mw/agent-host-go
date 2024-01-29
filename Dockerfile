FROM golang:1.21
WORKDIR /app
# COPY . .
RUN apt-get update && apt-get install -y ca-certificates openssl
RUN update-ca-certificates
COPY /build/mw-kube-agent /usr/bin/mw-agent
COPY configyamls-k8s/otel-config.yaml /app/otel-config.yaml
COPY configyamls-k8s/otel-config-nodocker.yaml /app/otel-config-nodocker.yaml

# A symlink to support existing k8s agent users
RUN ln -s /usr/bin/mw-agent /usr/bin/api-server

CMD ["mw-agent", "start"]
