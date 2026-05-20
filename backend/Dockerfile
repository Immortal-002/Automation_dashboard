FROM golang:1.26-alpine

WORKDIR /app

COPY . .
RUN go build -mod=vendor -o main .
EXPOSE 9090
CMD ["./main"]
