FROM golang:1.19.1-alpine AS development

ENV PROJECT_PATH=/chirpstack-network-server
ENV PATH=$PATH:$PROJECT_PATH/build
ENV CGO_ENABLED=0
ENV GO_EXTRA_BUILD_ARGS="-a -installsuffix cgo"
ENV GO111MODULE=on
ENV GOPROXY https://goproxy.cn,direct

RUN set -eux && sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
RUN apk add --no-cache ca-certificates tzdata make git bash protobuf
RUN git config --global --add safe.directory $PROJECT_PATH

RUN mkdir -p $PROJECT_PATH
COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH

RUN make dev-requirements
RUN make

FROM alpine:3.15.0 AS production

RUN set -eux && sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
RUN apk --no-cache add ca-certificates tzdata
COPY --from=development /chirpstack-network-server/build/chirpstack-network-server /usr/bin/chirpstack-network-server
USER nobody:nogroup
ENTRYPOINT ["/usr/bin/chirpstack-network-server"]
