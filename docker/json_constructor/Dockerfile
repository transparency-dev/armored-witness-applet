FROM golang:1.20

ARG FIRMWARE_FINGERPRINT
ARG GIT_TAG
ARG GIT_COMMIT_FINGERPRINT
ARG TAMAGO_VERSION

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /json_constructor

RUN /json_constructor -git_tag=${GIT_TAG} \
  -git_commit_fingerprint=${GIT_COMMIT_FINGERPRINT} \
  -firmware_fingerprint=${FIRMWARE_FINGERPRINT} \
  -tamago_version=${TAMAGO_VERSION} >> output.json
