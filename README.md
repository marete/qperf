# qperf

Benchmark throughput achieved by the
[quic-go](https://github.com/lucas-clemente/quic-go) implementation of
the QUIC protocol in your environment.

## Protocol

On receiving a connection, the server opens a *uni*directional **stream** to the client and writes *n* bytes of data to it. The client must accept the unidirectional stream that the server opens.

### Application Level Next Protocol Negotiation (ALPN)

Both the client and server must set the TLS Next Protocol value to: `quic-perf-test`.

## Installation

`go install -v github.com/marete/qperf@latest`

## On the server

`qperf -s -key ~/example.com.key -cert ~/example.com.crt -alsologtostderr`

## On the client

`qperf -c example.com:32850`
