FROM ubuntu:14.04

RUN apt-get update && \
	apt-get install --no-install-recommends -y \
	git-core \
	bzr \
	mercurial \
	curl \
	ca-certificates \
	build-essential && \
	rm -rf /var/lib/apt/lists/*

# Download and install Go
RUN curl -s https://storage.googleapis.com/golang/go1.3.linux-amd64.tar.gz | tar -v -C /usr/local -xz

ENV GOROOT /usr/local/go
ENV PATH $PATH:$GOROOT/bin

ENV GOPATH /go
ENV PATH $PATH:$GOPATH/bin

# Build, test and install RTLAMR
WORKDIR /go/src/
RUN go get -v github.com/bemasher/rtlamr
RUN go test -v ./...

CMD ["rtlamr"]


# Run rtlamr container with non-dockerized rtl_tcp instance:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr

# For use with bemasher/rtl-sdr:
# Start rtl_tcp from rtl-sdr container:
# docker run -d --privileged -v /dev/bus/usb:/dev/bus/usb --name rtltcp bemasher/rtl-sdr rtl_tcp -a 0.0.0.0

# Run rtlamr container, link with rtl_tcp container:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr -server=rtltcp:1234

