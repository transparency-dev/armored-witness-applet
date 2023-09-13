FROM golang:1.21-alpine3.17@sha256:ecaab0e81d070800a399d2e805f0fcc22806e130166a2f880456abeb04e401b0

ARG TAMAGO_VERSION

# Install dependencies.
RUN apk update && apk add bash git make wget

RUN wget "https://github.com/usbarmory/tamago-go/releases/download/tamago-go${TAMAGO_VERSION}/tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz"
RUN tar -xvf "tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz" -C /

WORKDIR /build

COPY . .

# Set Tamago path for Make rule.
ENV TAMAGO=/usr/local/tamago-go/bin/go

RUN make trusted_applet_nosign
