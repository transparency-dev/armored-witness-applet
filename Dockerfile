FROM golang:1.21-bookworm

ARG TAMAGO_VERSION
ARG FT_LOG_URL
ARG FT_BIN_URL
ARG LOG_ORIGIN
ARG LOG_PUBLIC_KEY
ARG APPLET_PUBLIC_KEY
ARG OS_PUBLIC_KEY1
ARG OS_PUBLIC_KEY2
ARG GIT_SEMVER_TAG

# Install dependencies.
RUN apt-get update && apt-get install -y git make wget

RUN wget --quiet "https://github.com/usbarmory/tamago-go/releases/download/tamago-go${TAMAGO_VERSION}/tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz"
RUN tar -xf "tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz" -C /

WORKDIR /build

COPY . .

# Set Tamago path for Make rule.
ENV TAMAGO=/usr/local/tamago-go/bin/go

# Firmware transparency parameters for output binary.
ENV FT_LOG_URL=${FT_LOG_URL} \
    FT_BIN_URL=${FT_BIN_URL} \
    LOG_ORIGIN=${LOG_ORIGIN} \
    LOG_PUBLIC_KEY=${LOG_PUBLIC_KEY} \
    APPLET_PUBLIC_KEY=${APPLET_PUBLIC_KEY} \
    OS_PUBLIC_KEY1=${OS_PUBLIC_KEY1} \
    OS_PUBLIC_KEY2=${OS_PUBLIC_KEY2} \
    GIT_SEMVER_TAG=${GIT_SEMVER_TAG}

RUN make trusted_applet_nosign
