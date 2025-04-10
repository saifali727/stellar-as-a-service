# Use official Golang image as the base
FROM golang:1.24-alpine

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN go build -o /godocker
# Expose port
EXPOSE 8080

# Command to run the application
CMD ["/godocker"]