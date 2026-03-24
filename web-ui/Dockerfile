FROM node:20-slim

# Install Go (needed for the validator binary)
RUN apt-get update && apt-get install -y wget && \
    wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz && \
    rm go1.22.0.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /app

# Copy Go validator source
COPY go-validator /app/go-validator
COPY profile.json /app/profile.json

# Build Go binary
RUN cd /app/go-validator && go build -o startup-factory .

# Copy web UI
COPY web-ui/package*.json /app/web-ui/
WORKDIR /app/web-ui
RUN npm install --production

COPY web-ui/ /app/web-ui/

# Create results directory
RUN mkdir -p /app/go-validator/results

EXPOSE 3737

CMD ["node", "server.js"]
