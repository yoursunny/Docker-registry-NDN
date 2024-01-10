# Docker Registry over NDN

This is a proof-of-concept implementation that runs a subset of [Docker Registry HTTP API](https://docs.docker.com/registry/spec/api/) over Named Data Networking (NDN).
It includes a server program co-located with the Docker registry, and a client program downloaded to the Docker client.

In Docker registry API, blob retrieval, i.e. pulling a layer in a Docker image, accounts for the majority of network traffic at a Docker registry.
Since each blob, identified by its digest, is immutable, this offers a prime opportunity to take advantage of in-network caching in the NDN network.
Therefore, this program translates blob retrieval requests to NDN segmented object retrieval, while all other requests are proxied over HTTPS.

## Server Installation (PM2)

The [server](server/) program should run on the same machine or very close to the Docker registry.
It requires a local NDN forwarder, which should have a globally reachable name prefix.

1. Install Node.js 20.x and PM2 process manager.
2. Clone this repository.
3. Copy `server/sample.env` to `server/.env` and make changes according to the instructions within.
4. Install dependencies: `corepack pnpm install --prod`
5. Start service: `pm2 start --name Docker-registry-NDN --restart-delay 10000 --cwd server main.js`

## Server Installation (Docker)

Pull or build the server container image:

```bash
docker pull docker.yoursunny.dev/registry-ndn-server

docker build -t docker.yoursunny.dev/registry-ndn-server -f server.Dockerfile .
```

Start the server container:

```bash
docker run -d --name registry-ndn-server \
  --mount type=bind,source=/run/nfd/nfd.sock,target=/run/nfd/nfd.sock,readonly=true \
  -e DOCKER_NDN_REGISTRY=http://172.17.0.2:5000 \
  -e DOCKER_NDN_NAME=/docker \
  docker.yoursunny.dev/registry-ndn-server
```

See [server/sample.env](server/sample.env) for explanation of the environment variables.

## Client Installation and Startup (Go)

The [client](client/) program should run on every client that intends to pull from the Docker registry.
It does not require a local NDN forwarder.

1. Install Go 1.21.
2. Build the client: `env GOBIN=$(pwd) CGO_ENABLED=0 go install github.com/yoursunny/Docker-registry-NDN/client@latest && mv client Docker-registry-NDN-client`

Run `./Docker-registry-NDN-client --help` to see available command line flags.
You need to specify at least `--upstream` and `--name` flags, so that the client retrieves from your server, instead of yoursunny private Docker registry.
For example:

```bash
./Docker-registry-NDN-client --upstream https://docker.example.com --name /example/docker
```

## Client Installation and Startup (Docker)

Pull or build the client container image:

```bash
docker pull docker.yoursunny.dev/registry-ndn-client

docker build -t docker.yoursunny.dev/registry-ndn-client -f client.Dockerfile .
```

Start the client container:

```bash
docker run -d --name registry-ndn-client \
  --mount type=bind,source=/run/nfd/nfd.sock,target=/run/nfd/nfd.sock,readonly=true \
  docker.yoursunny.dev/registry-ndn-client \
  --upstream https://docker.example.com --name /example/docker
```

See [server/sample.env](server/sample.env) for explanation of the environment variables.

## Usage

The client starts a local HTTP endpoint on `http://127.0.0.1:5000` (you can change this via `--listen` flag).
With the client running, you can pull Docker images from its HTTP endpoint:

```bash
docker pull localhost:5000/image
docker tag localhost:5000/image docker.example.com/image

# instead of:
docker pull docker.example.com/image
```
