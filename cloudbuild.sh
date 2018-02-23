#!/bin/bash -ex

PKG=github.com/dparrish/build-web-application-demo

BUILD=/workspace/image
mkdir -p ${BUILD}

cp Dockerfile /etc/ssl/certs/ca-certificates.crt ${BUILD}/

export GOPATH=/workspace
mkdir -p ${GOPATH}/src/${PKG}
mv * ${GOPATH}/src/${PKG}/
go get -u ${GOPATH}/src/${PKG}/...

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ${BUILD}/frontend ${PKG}/frontend
strip ${BUILD}/frontend

if [ "$PROJECT" == "" ]; then
	echo "The PROJECT environment variable must be set" >&2
	exit 0
fi

TAG=$1
if [ "$TAG" == "" ]; then
	echo "Specify a tag" >&2
	exit 0
fi

#docker build -t gcr.io/${PROJECT}/frontend:${TAG} -f ${BUILD}/Dockerfile ${BUILD}
#gcloud docker -- push gcr.io/${PROJECT}/frontend:${TAG}
