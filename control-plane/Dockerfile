# Start from the official Go image
FROM golang:1.21-alpine

WORKDIR /app

WORKDIR /app

# Install git, which is needed
RUN apk add --no-cache git

# Copy the new, minimal go.mod
COPY go.mod ./

# Copy all the source code (go mod tidy needs it)
COPY . .

# Run go mod tidy to resolve all dependencies
# and build the go.sum file.
# We keep GOPROXY=direct as a safeguard.
RUN GOPROXY=direct go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /control-plane

# Expose the gRPC port
EXPOSE 18000

# Run the compiled binary
CMD [ "/control-plane" ]