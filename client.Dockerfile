FROM golang:1.21-alpine3.19 AS build
WORKDIR /app
COPY go.* .
COPY client ./client
RUN env GOBIN=/build CGO_ENABLED=0 go install ./client

FROM scratch
COPY --from=build /build/client /Docker-registry-NDN-client
ENTRYPOINT ["/Docker-registry-NDN-client"]
