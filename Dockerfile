# Stage 1 - compile the frontend.
FROM golang:1.9 as builder
WORKDIR /go/src/github.com/dparrish/build-web-application-demo
# Get initial dependencies, to save on repeated build time.
RUN go get -d -v \
  cloud.google.com/go/bigquery \
  cloud.google.com/go/civil \
  cloud.google.com/go/spanner \
  cloud.google.com/go/storage \
  github.com/auth0-community/go-auth0 \
  github.com/clbanning/mxj \
  github.com/docker/go-healthcheck \
  github.com/fsnotify/fsnotify \
  github.com/google/uuid \
  github.com/gorilla/context \
  github.com/gorilla/handlers \
  github.com/gorilla/mux \
  github.com/gtank/cryptopasta \
  github.com/hashicorp/golang-lru \
  go.opencensus.io/exporter/stackdriver \
  go.opencensus.io/stats \
  go.opencensus.io/stats/view \
  go.opencensus.io/tag \
  go.opencensus.io/trace \
  golang.org/x/oauth2/google \
  google.golang.org/api/cloudkms/v1 \
  google.golang.org/api/iterator \
  gopkg.in/go-on/wrap.v2 \
  gopkg.in/square/go-jose.v2 \
  gopkg.in/square/go-jose.v2/jwt \
  gopkg.in/urfave/cli.v1 \
  gopkg.in/yaml.v2
ADD . /go/src/github.com/dparrish/build-web-application-demo
# Get the rest of the dependencies.
RUN go get -d -v ...
# Compile the binary using static linking.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /tmp/frontend-bin github.com/dparrish/build-web-application-demo/frontend
RUN strip /tmp/frontend-bin

# Stage 2 - build a minimal frontend binary iamge.
FROM scratch
ENTRYPOINT ["/frontend-bin"]
COPY --from=builder /tmp/frontend-bin /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/
