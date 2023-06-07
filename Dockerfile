FROM us-central1-docker.pkg.dev/serverless-log/jayhou-test/test-img:latest

RUN apt-get update && apt-get install -y make

WORKDIR /build

COPY Makefile .
COPY . .

ENV TAMAGO=/bin/tamago-go/go
RUN signify-openbsd -G -n -p armored-witness-applet.pub -s armored-witness-applet.sec
RUN echo $_APPLET_PRIVATE_KEY > armored-witness-applet.sec
ENV APPLET_PRIVATE_KEY=armored-witness-applet.sec

RUN make trusted_applet

