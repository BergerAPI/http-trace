#!/bin/bash

# Create a local directory for JetStream data if it doesn't exist
JETSTREAM_DATA_DIR="./jetstream_data"
mkdir -p $JETSTREAM_DATA_DIR

# Start a persistent Docker container with NATS, JetStream, and the nats.conf file
docker run -d --name nats-server \
    -v ./conf/nats.conf:/etc/nats/nats.conf \
    -v ./$JETSTREAM_DATA_DIR:/data/jetstream \
    -p 4222:4222 -p 8222:8222 \
    nats:latest -c /etc/nats/nats.conf

echo "NATS server with JetStream is running in a Docker container."

# Check if nats is installed
if command -v nats &> /dev/null
then
    echo "nats is installed."
else
    echo "nats is not installed."
    exit 1
fi

# Run the command and store the output in a variable
output=$(nats s ls -n)

# Check if the output contains the word "jobs"
if echo "$output" | grep -q "jobs"; then
    echo "Worker queue is already initiated."
else
    echo "Creating worker queue 'jobs'"
  nats s add jobs --config ./conf/worker-queue.json
fi
