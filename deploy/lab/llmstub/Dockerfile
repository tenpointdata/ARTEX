FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /llmstub .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /llmstub /llmstub
EXPOSE 8080
ENTRYPOINT ["/llmstub"]
