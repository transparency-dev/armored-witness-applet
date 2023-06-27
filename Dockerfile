FROM amd64/ubuntu:latest

# Install dependencies.
RUN apt-get update && apt-get install -y make
RUN apt-get install -y wget

RUN wget https://github.com/usbarmory/tamago-go/releases/download/tamago-go1.20.5/tamago-go1.20.5.linux-amd64.tar.gz
RUN tar -xvf tamago-go1.20.5.linux-amd64.tar.gz -C /

WORKDIR /build

COPY . .

# Set Tamago path for Make rule.
ENV TAMAGO=/usr/local/tamago-go/bin/go

RUN make elf
