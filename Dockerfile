# Use the official Golang image
FROM golang:1.23.3-alpine3.20

# Set the working directory inside the container
WORKDIR /app

# Copy all files into the container
COPY . .

# Install necessary dependencies
RUN go mod tidy

# Build the Go application
RUN go build -o api .

# Expose the port the app runs on
EXPOSE 8000

# Run the application when the container starts
CMD ["./api"]