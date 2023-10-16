FROM golang:1.20-alpine as builder
RUN apk add git g++ fuse
ARG BRANCH=${BRANCH:-master}
ARG TAGS="elastic,ydb,gocdk,tikv"

WORKDIR /go/src/github.com/seaweedfs/seaweedfs
COPY . .
RUN cd /go/src/github.com/seaweedfs/seaweedfs/weed \
  && export LDFLAGS="-X github.com/seaweedfs/seaweedfs/weed/util.COMMIT=$(git rev-parse --short HEAD)" \
  && CGO_ENABLED=0 go install -tags "$TAGS" -ldflags "-extldflags -static ${LDFLAGS}"

FROM alpine AS final
LABEL author="Chris Lu"
COPY --from=builder /go/bin/weed /usr/bin/
RUN mkdir -p /etc/seaweedfs
COPY --from=builder /go/src/github.com/seaweedfs/seaweedfs/docker/filer.toml /etc/seaweedfs/filer.toml
COPY --from=builder /go/src/github.com/seaweedfs/seaweedfs/docker/entrypoint.sh /entrypoint.sh
RUN apk add fuse # for weed mount

# volume server gprc port
EXPOSE 18080
# volume server http port
EXPOSE 8080
# filer server gprc port
EXPOSE 18888
# filer server http port
EXPOSE 8888
# master server shared gprc port
EXPOSE 19333
# master server shared http port
EXPOSE 9333
# s3 server http port
EXPOSE 8333
# webdav server http port
EXPOSE 7333

RUN mkdir -p /data/filerldb2

VOLUME /data
WORKDIR /data

RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
