FROM golang:1.12.6

ENV GO111MODULE=on
WORKDIR /go/src/github.com/bemasher/rtlamr

COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

CMD ["rtlamr"]

# Run rtlamr container with non-dockerized rtl_tcp instance:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr

# For use with bemasher/rtl-sdr:
# Start rtl_tcp from rtl-sdr container:
# docker run -d --privileged -v /dev/bus/usb:/dev/bus/usb --name rtltcp bemasher/rtl-sdr rtl_tcp -a 0.0.0.0

# Run rtlamr container, link with rtl_tcp container:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr -server=rtltcp:1234
