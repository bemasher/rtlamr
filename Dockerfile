########################################
FROM golang:1.12.6-alpine AS build

RUN apk --no-cache add git

WORKDIR /src/rtlamr

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -o /rtlamr

########################################
FROM scratch

COPY --from=build /rtlamr /rtlamr

ENTRYPOINT ["/rtlamr"]

# Run rtlamr container with non-dockerized rtl_tcp instance:
# docker run -d --name rtlamr --net=host bemasher/rtlamr

# For use with bemasher/rtl-sdr:
# Start rtl_tcp from rtl-sdr container:
# docker run -d --privileged -v /dev/bus/usb:/dev/bus/usb --name rtltcp bemasher/rtl-sdr rtl_tcp -a 0.0.0.0

# Run rtlamr container, link with rtl_tcp container:
# docker run -d --name rtlamr --link rtltcp:rtltcp bemasher/rtlamr -server=rtltcp:1234
