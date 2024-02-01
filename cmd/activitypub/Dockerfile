FROM golang:latest AS coreBuilder
WORKDIR /work

RUN go install github.com/google/wire/cmd/wire@latest
COPY ./go.mod ./go.sum ./
RUN go mod download && go mod verify
COPY ./ ./

RUN wire ./cmd/activitypub \
 && go build -ldflags "-s -w -X main.version=$(git describe) -X \"main.buildMachine=$(uname -srmo)\" -X \"main.buildTime=$(date)\" -X \"main.goVersion=$(go version)\"" -o ccactivitypub ./cmd/activitypub

FROM golang:latest

COPY --from=coreBuilder /work/ccactivitypub /usr/local/bin

CMD ["ccactivitypub"]
