FROM golang:1.23.5-bullseye@sha256:462521f1b7cbf410002a8cc4d91bc897f35cd430854d7240596282f9441fe4a7 AS builder
LABEL stage=builder

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
RUN go build ./binary_transparency/firmware/cmd/ft_personality

# Build release image
FROM golang:1.23.5-bullseye@sha256:462521f1b7cbf410002a8cc4d91bc897f35cd430854d7240596282f9441fe4a7

COPY --from=builder /build/ft_personality /bin/ft_personality
ENTRYPOINT ["/bin/ft_personality"]
