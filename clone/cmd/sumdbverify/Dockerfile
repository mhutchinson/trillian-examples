FROM golang:1.22.8-alpine3.20@sha256:f56a8a4a1aea41bc4694728b69c219af1523aea15690cbbed82dc9bac81e6603 AS builder

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

# Move to working directory /build
WORKDIR /build

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build ./clone/cmd/sumdbverify

# Build release image
FROM alpine:3.18.4@sha256:eece025e432126ce23f223450a0326fbebde39cdf496a85d8c016293fc851978

COPY --from=builder /build/sumdbverify /bin/verify
ENTRYPOINT ["/bin/verify"]
