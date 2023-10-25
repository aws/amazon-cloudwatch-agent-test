# The Mock Server

## Overview

The Mock Server is a simple server designed for receiving metric and trace data, providing a simulated endpoint for testing purposes. It listens on two separate ports: 8080 and 443.
## Running the server
This server is runs as a docker container to run this server:
1. First build the docker container with
```sh
sudo docker build -t mockserver . 
```
2. Run the container by mapping the ports you would like to use, for example:
```sh
sudo docker run --name mockserver -d -p 8080:8080 -p 443:443  mockserver
```

## How it Works
### The Receiver

The receiver component of the Mock Server operates on port 443. It is responsible for receiving messages and incrementing the transaction count. To simulate real-world conditions, there is a built-in 15ms latency between each received message. The data received can be sent to three possible routes:

- **Check Receiver Status:** You can check if the receiver is alive by making a request to `/ping`.

- **Send Data:** Use the `/put-data` route to send data. This route supports two sub-routes:
    - `/put-data/trace/v1`: Use this sub-route for sending trace data.
    - `/put-data/metrics`: Use this sub-route for sending metrics data.

> [!Important]
> Currently, both traces and metrics are handled in the same way.

### The Verifier

The verifier component can be accessed via a listener on port 8080. It provides information about the transactions, including:

- **Transactions per Minute:** You can obtain the transactions per minute by making a request to `/tpm`.

- **Transaction Count:** To check the total transaction count, use the `/check-data` route.

- **Verifier Status:** Determine if the verification server is alive by sending a request to `/ping`.


