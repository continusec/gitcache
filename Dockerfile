# Pre-work, build a linux binary for gitcache:
# mkdir linux && docker run --rm -v $GOPATH:/go -w /go/src/github.com/continusec/gitcache/linux golang:alpine go build github.com/continusec/gitcache/cmd/gitcache && docker build -t gitcache

FROM alpine:latest

RUN apk --no-cache add \
    git

RUN ln -s /ssh /root/.ssh

COPY ./linux/gitcache /bin/

EXPOSE 9091

VOLUME /cache
VOLUME /ssh

ENTRYPOINT ["/bin/gitcache", "-cache", "/cache"]
