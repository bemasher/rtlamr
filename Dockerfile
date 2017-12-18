FROM golang:1.9.2

WORKDIR /go/src/github.com/bemasher/rtlamr
COPY . .

RUN go-wrapper install

CMD ["go-wrapper", "run"]

# Run rtlamr container with non-dockerized rtl_tcp instance:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr

# For use with bemasher/rtl-sdr:
# Start rtl_tcp from rtl-sdr container:
# docker run -d --privileged -v /dev/bus/usb:/dev/bus/usb --name rtltcp bemasher/rtl-sdr rtl_tcp -a 0.0.0.0

# Run rtlamr container, link with rtl_tcp container:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr -server=rtltcp:1234
