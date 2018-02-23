#!/bin/bash -e

echo "Fetching Go dependencies"
go get ./... || true

echo "Copying code to the go source root"
PKG=github.com/dparrish/build-web-application-demo
mkdir -p ${GOPATH}/src/${PKG}
tar cf - * | (cd ${GOPATH}/src/${PKG} && tar xf -)

echo "Building the frontend binary"
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o frontend-bin ${PKG}/frontend
strip frontend-bin

echo "Copying files to the docker build root"
cp /etc/ssl/certs/ca-certificates.crt .

