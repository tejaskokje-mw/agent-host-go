RELEASE_VERSION=0.0.0
LD_FLAGS="-s -w -X main.agentVersion=${RELEASE_VERSION}"
build-windows:
	GOOS=windows CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-windows-agent.exe cmd/host-agent/main.go
build-linux:
	GOOS=linux CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-host-agent cmd/host-agent/main.go
build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags=${LD_FLAGS} -o build/mw-host-agent cmd/host-agent/main.go
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags=${LD_FLAGS} -o build/mw-host-agent cmd/host-agent/main.go
build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-host-agent-amd64 cmd/host-agent/main.go
build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-host-agent-arm64 cmd/host-agent/main.go
build-kube:
	GOOS=linux CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-kube-agent cmd/kube-agent/main.go
build-kube-config-updater:
	GOOS=linux CGO_ENABLED=0 go build -ldflags=${LD_FLAGS} -o build/mw-config-updater cmd/kube-config-updater/main.go
build: build-linux build-windows build-kube

package-windows: build-windows
	makensis -DVERSION=${RELEASE_VERSION} package-tooling/windows/package-windows.nsi 

package-linux-deb: build-linux-amd64 build-linux-arm64
	act -W .github/workflows/host-agent-deb-apt.yaml --input release_version=${RELEASE_VERSION} --container-options "-v ${PWD}/build:${PWD}/build"

package-linux-rpm: build-linux-amd64 build-linux-arm64
	act -W .github/workflows/host-agent-rpm.yaml --input release_version=${RELEASE_VERSION} --input release_number=1 --container-options "-v ${PWD}/build:${PWD}/build -v${PWD}/build:/root/build"

package-linux: package-linux-deb package-linux-rpm package-linux-docker

package-linux-docker:
	docker build .  --target build --build-arg GITHUB_TOKEN=$(GH_TOKEN) -t ghcr.io/middleware-labs/mw-agent:local -f Dockerfiles/DockerfileLinux

package-kube-config-updater:
	docker build .  --target build --build-arg GITHUB_TOKEN=$(GH_TOKEN)  -t ghcr.io/middleware-labs/mw-config-updater:local -f Dockerfiles/DockerfileKubeConfigUpdater

package-darwin: build-darwin-arm64
	package-tooling/darwin/create_installer.sh ${RELEASE_VERSION}

package: package-windows package-linux

clean:
	go clean
	rm mw-host-agent
	rm mw-windows-agent

test:
	go test ./...

test_coverage:
	go test ./... -coverprofile=coverage.out

vet:
	go vet

lint:
	golangci-lint run
