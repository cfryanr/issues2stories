FROM golang:1.15.2 as build-env

WORKDIR /work
COPY . ./

# Build the executable binary (CGO_ENABLED=0 means static linking)
RUN mkdir out && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o out ./...

# Use a runtime image based on Debian slim
FROM debian:10.5-slim

# Get the certs
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the binaries from the build-env stage
COPY --from=build-env /work/out/issues2stories /usr/local/bin/issues2stories

# Document the port
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/issues2stories"]
